package catalog

import (
	"regexp"
	"strconv"
	"strings"
)

// Version specifiers - npm/composer/poetry carets, ruby/terraform
// pessimistic constraints, PEP 440 compatible releases, wildcards and
// bounded ranges - reduce to the version a lifecycle check should judge.
// A range counts only when every version it allows sits inside one
// release line: the result is a "float" (a bare major "5" or a
// major.minor "5.2") that the judge resolves to the newest cycle of that
// line. If that newest cycle is past end-of-life, so is the whole range.
// Floors (>=3.6) and open ranges allow supported versions: no pin.

type specStyle int

const (
	styleNPM      specStyle = iota // ^ caret, ~ tilde = minor, x-ranges, || alternatives
	styleComposer                  // like npm, but ~1.2 = >=1.2 <2.0 and | alternatives
	stylePEP440                    // ==, ~=, ==3.6.*, comma-joined bounds
	styleGem                       // ~> pessimistic, comma-joined constraints
)

var (
	reStability      = regexp.MustCompile(`@(?:stable|dev|beta|alpha|rc|RC)\b`)
	reSpecConstraint = regexp.MustCompile(`^(\^|~>|~=|~|===|==|=|>=|<=|>|<|!=)?\s*v?(\d+(?:\.(?:\d+|x|X|\*))*)(?:[.\-+]?[A-Za-z][0-9A-Za-z.\-+]*|[.\-+][0-9A-Za-z.\-+]*)?$`)
	reSpecHyphen     = regexp.MustCompile(`^v?(\d+(?:\.\d+)*)\s+-\s+v?(\d+(?:\.\d+)*)$`)
)

// specVersion reduces a version specifier to the version to judge.
func specVersion(spec string, style specStyle) (string, bool) {
	spec = strings.TrimSpace(spec)
	if style == styleComposer {
		spec = reStability.ReplaceAllString(spec, "") // 2.5.*@stable
	}
	if spec == "" {
		return "", false
	}
	var alts []string
	switch style {
	case styleNPM:
		alts = strings.Split(spec, "||")
	case styleComposer:
		alts = strings.FieldsFunc(spec, func(r rune) bool { return r == '|' })
	default:
		alts = []string{spec}
	}
	best := ""
	for _, alt := range alts {
		v, ok := specAlternative(strings.TrimSpace(alt), style)
		if !ok {
			return "", false // one alternative is open-ended: supported versions allowed
		}
		if best == "" || compareVersionStrings(v, best) > 0 {
			best = v
		}
	}
	return best, best != ""
}

func specAlternative(alt string, style specStyle) (string, bool) {
	if alt == "" || alt == "*" || alt == "x" || strings.EqualFold(alt, "latest") {
		return "", false
	}
	if m := reSpecHyphen.FindStringSubmatch(alt); m != nil { // npm "1.2 - 1.4"
		return floatBelowOrAt(m[2]), true
	}
	// npm joins bounds with spaces, everyone else (poetry included) with
	// commas: splitting on both is safe for all of them
	parts := strings.FieldsFunc(alt, func(r rune) bool { return r == ',' || r == ' ' })
	// rejoin operators separated from their versions by a space (">= 1.2")
	var cons []string
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if strings.Trim(p, "^~<>=!") == "" && i+1 < len(parts) {
			p += parts[i+1]
			i++
		}
		cons = append(cons, p)
	}
	var lower, upper, exact string
	for _, c := range cons {
		m := reSpecConstraint.FindStringSubmatch(c)
		if m == nil {
			return "", false
		}
		op, v := m[1], m[2]
		switch op {
		case "!=":
			continue
		case ">=", ">":
			lower = v
		case "<":
			upper = floatBelow(v)
		case "<=":
			upper = floatBelowOrAt(v)
		case "^":
			exact = caretFloat(v)
		case "~":
			if style == styleComposer {
				exact = pessimisticFloat(v)
			} else {
				exact = tildeFloat(v)
			}
		case "~>", "~=":
			exact = pessimisticFloat(v)
		default: // exact, possibly wildcarded
			exact = wildcardFloat(v)
		}
	}
	switch {
	case exact != "":
		return exact, true
	case upper != "":
		if lower != "" {
			// the floor must not sit above the line the bound confines to:
			// >=2.2,<3 confines to 2.x; compare at the bound's precision
			lp := strings.Split(lower, ".")
			if n := len(strings.Split(upper, ".")); len(lp) > n {
				lp = lp[:n]
			}
			if compareVersionStrings(strings.Join(lp, "."), upper) > 0 {
				return "", false
			}
		}
		return upper, true
	default:
		return "", false // a floor alone: open-ended
	}
}

// caretFloat: ^1.2.3 allows 1.x; ^0.2.3 allows 0.2.x.
func caretFloat(v string) string {
	p := strings.Split(wildcardFloat(v), ".")
	if p[0] != "0" || len(p) == 1 {
		return p[0]
	}
	return p[0] + "." + p[1]
}

// tildeFloat (npm): ~1.2.3 and ~1.2 allow 1.2.x; ~1 allows 1.x.
func tildeFloat(v string) string {
	p := strings.Split(wildcardFloat(v), ".")
	if len(p) == 1 {
		return p[0]
	}
	return p[0] + "." + p[1]
}

// pessimisticFloat (~> 5.2, ~= 5.2, composer ~5.2): the last component
// floats. ~> 5.2 allows 5.x; ~> 5.2.1 allows 5.2.x.
func pessimisticFloat(v string) string {
	p := strings.Split(wildcardFloat(v), ".")
	if len(p) <= 1 {
		return p[0]
	}
	return strings.Join(p[:len(p)-1], ".")
}

// wildcardFloat: 1.2.x / 1.2.* -> 1.2; plain versions pass through.
func wildcardFloat(v string) string {
	p := strings.Split(v, ".")
	for i, x := range p {
		if x == "x" || x == "X" || x == "*" {
			return strings.Join(p[:i], ".")
		}
	}
	return v
}

// floatBelow: the release line just under an exclusive upper bound.
// <3, <3.0, <3.0.0 -> 3's predecessor major "2"; <2.3 -> "2.2".
func floatBelow(v string) string {
	p := strings.Split(wildcardFloat(v), ".")
	nums := make([]int, len(p))
	for i, x := range p {
		nums[i], _ = strconv.Atoi(x)
	}
	// trailing zeros don't narrow the bound
	for len(nums) > 1 && nums[len(nums)-1] == 0 {
		nums = nums[:len(nums)-1]
	}
	last := len(nums) - 1
	if nums[last] == 0 {
		return "" // <0: nothing allowed
	}
	if last == 0 {
		return strconv.Itoa(nums[0] - 1)
	}
	return joinInts(nums[:last]) + "." + strconv.Itoa(nums[last]-1)
}

// floatBelowOrAt: an inclusive bound pins exactly that version.
func floatBelowOrAt(v string) string { return wildcardFloat(v) }

func joinInts(n []int) string {
	s := make([]string, len(n))
	for i, x := range n {
		s[i] = strconv.Itoa(x)
	}
	return strings.Join(s, ".")
}

// compareVersionStrings compares dotted numeric versions.
func compareVersionStrings(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}
