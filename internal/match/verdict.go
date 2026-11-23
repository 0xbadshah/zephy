package match

import (
	"fmt"
	"time"

	"github.com/0xbadshah/zephy/internal/catalog"
	"github.com/0xbadshah/zephy/internal/data"
	"github.com/0xbadshah/zephy/internal/policy"
)

// Kind is the one outcome a lifecycle pin can have. There is no fifth:
// a pin that cannot be verified is Untracked with a reason, never nothing.
type Kind int

const (
	KindFinding    Kind = iota // past EOL, or EOL inside the lookahead window
	KindOK                     // verified supported
	KindSuppressed             // a finding the policy file suppresses
	KindUntracked              // no lifecycle data to verify against
)

// Verdict is the judgement on one pin. Exactly one of Finding, OK or
// Untracked is meaningful, per Kind; Reason explains Suppressed and
// Untracked verdicts.
type Verdict struct {
	Kind      Kind
	Finding   Finding // KindFinding, KindSuppressed (Repo/Source left to the caller)
	OK        OKPin
	Untracked catalog.Untracked
	Reason    string
}

// Judge owns every lifecycle rule: dataset lookup, version normalization,
// cycle matching, the floating-major rule, extended-support overrides,
// lookahead classification and suppression.
type Judge struct {
	Policy    *policy.File // nil = no policy
	Now       time.Time
	Lookahead time.Duration
}

// Pin judges one lifecycle pin.
func (j Judge) Pin(p catalog.Pin) Verdict {
	pol := j.Policy
	if pol == nil {
		pol = &policy.File{}
	}
	version := pol.NormalizeVersion(p.Version)
	untracked := func(reason string) Verdict {
		return Verdict{Kind: KindUntracked, Reason: reason, Untracked: catalog.Untracked{
			Raw: p.Raw, Product: p.Product, Version: version, File: p.File, Reason: reason}}
	}

	var cycles []data.Cycle
	var c *data.Cycle
	members := data.Members(p.Product)
	for _, m := range members {
		if cs := data.Load(m); cs != nil {
			if cycles == nil {
				cycles = cs
			}
			if c = CycleMatch(cs, version); c != nil {
				cycles = cs
				break
			}
		}
	}
	if cycles == nil {
		return untracked(fmt.Sprintf("no lifecycle data for %q", p.Product))
	}
	if c == nil {
		// release lines retire in order: a version older than a tracked
		// cycle that is already past EOL is past EOL too (spring 4.2 sits
		// below the dead 4.3; redis 4.0 predates the dataset's oldest 5.0)
		for _, m := range members {
			next := deadCycleAbove(data.Load(m), version, j.Now)
			if next == nil {
				continue
			}
			if st, eolOn, days := Classify(next, j.Now, 0); st == StatusEOL {
				f := Finding{Product: p.Product, Version: version, Release: "<" + next.Cycle,
					EOLOn: eolOn, DaysPast: days, Status: StatusEOL, File: p.File, Raw: p.Raw}
				if suppressed, reason := pol.Suppressed(p.Product, version, f.Release, p.File); suppressed {
					return Verdict{Kind: KindSuppressed, Finding: f, Reason: reason}
				}
				return Verdict{Kind: KindFinding, Finding: f}
			}
		}
		return untracked(fmt.Sprintf("version %q matches no %s release", version, p.Product))
	}

	var status Status
	var eolOn string
	var days int
	if until, ok := pol.Override(p.Product, c.Cycle); ok {
		// extended support: the policy owns the date
		status, eolOn, days = ClassifyDate(until, j.Now, j.Lookahead)
	} else {
		status, eolOn, days = Classify(c, j.Now, j.Lookahead)
		if status == StatusExpiring && isBareMajor(version) && c.Cycle != version && !newerMajorExists(cycles, version) {
			// a floating major tag inferred to a minor cycle (rabbitmq:4 ->
			// 4.4) follows the line: it is not "expiring on that minor's
			// date". Products that EOL majors as units (nodejs 22) declare
			// the major itself as a cycle and keep their date. Once a newer
			// major exists the line is closed (tailwind 3 after 4): its last
			// minor's date is the major's date. A whole major past EOL is
			// still eol.
			status = ""
		}
	}
	if status == "" {
		ok := OKPin{Product: p.Product, Version: version, Release: c.Cycle, File: p.File}
		if c.EOL.Set {
			ok.EOL = c.EOL.Date.Format("2006-01-02")
		}
		return Verdict{Kind: KindOK, OK: ok}
	}
	f := Finding{
		Product: p.Product, Version: version, Release: c.Cycle,
		EOLOn: eolOn, DaysPast: days, Status: status, File: p.File, Raw: p.Raw,
	}
	if suppressed, reason := pol.Suppressed(p.Product, version, c.Cycle, p.File); suppressed {
		return Verdict{Kind: KindSuppressed, Finding: f, Reason: reason}
	}
	return Verdict{Kind: KindFinding, Finding: f}
}

// deadCycleAbove: the lowest past-EOL cycle above an untracked version.
// The dataset tracks LTS lines and recent releases; a release line it
// skipped (julia 1.9 between the LTS 1.6/1.10 and the dead 1.11) is a dead
// intermediate once any newer line has died.
func deadCycleAbove(cycles []data.Cycle, version string, now time.Time) *data.Cycle {
	if len(numericParts(version)) == 0 || version[0] < '0' || version[0] > '9' {
		return nil
	}
	var best *data.Cycle
	for i := range cycles {
		c := &cycles[i]
		if len(numericParts(c.Cycle)) == 0 || compareVersions(c.Cycle, version) <= 0 {
			continue
		}
		if !c.EOL.Ended && !(c.EOL.Set && c.EOL.Date.Before(now)) {
			continue
		}
		if best == nil || compareVersions(c.Cycle, best.Cycle) < 0 {
			best = c
		}
	}
	return best
}

// compareVersions compares dotted numeric versions component by component
// ("4.0.14" < "5.0"); non-numeric suffixes are ignored.
func compareVersions(a, b string) int {
	pa, pb := numericParts(a), numericParts(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
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

func numericParts(v string) []int {
	var parts []int
	n, digits := 0, false
	for _, r := range v {
		switch {
		case r >= '0' && r <= '9':
			n, digits = n*10+int(r-'0'), true
		case r == '.':
			if !digits {
				return parts
			}
			parts = append(parts, n)
			n, digits = 0, false
		default:
			if digits {
				parts = append(parts, n)
			}
			return parts
		}
	}
	if digits {
		parts = append(parts, n)
	}
	return parts
}

// newerMajorExists reports a cycle on a higher major than a bare major.
func newerMajorExists(cycles []data.Cycle, major string) bool {
	m := numericParts(major)
	if len(m) == 0 {
		return false
	}
	for _, c := range cycles {
		if p := numericParts(c.Cycle); len(p) > 0 && p[0] > m[0] {
			return true
		}
	}
	return false
}
