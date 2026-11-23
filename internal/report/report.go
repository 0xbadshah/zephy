package report

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/0xbadshah/zephy/internal/catalog"
	"github.com/0xbadshah/zephy/internal/match"
)

type Report struct {
	Findings  []match.Finding     `json:"findings"`
	OK        []match.OKPin       `json:"ok,omitempty"`
	Untracked []catalog.Untracked `json:"untracked,omitempty"`
}

// ansi wraps s in an escape sequence when colors are on.
type palette struct{ on bool }

func (p palette) wrap(code, s string) string {
	if !p.on {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (p palette) red(s string) string    { return p.wrap("31", s) }
func (p palette) yellow(s string) string { return p.wrap("33", s) }
func (p palette) dim(s string) string    { return p.wrap("2", s) }
func (p palette) bold(s string) string   { return p.wrap("1", s) }

// ColorsOn decides TTY coloring: colors only when stdout is a terminal,
// --no-color is off and NO_COLOR is unset (the standard convention).
func ColorsOn(noColorFlag bool) bool {
	if noColorFlag || os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ExitCode implements the CI contract: 0 clean, 1 findings matched by
// the --fail-on policy, 2 operational error.
func (r Report) ExitCode(failOn []string) int {
	want := map[string]bool{}
	for _, s := range failOn {
		want[strings.TrimSpace(s)] = true
	}
	for _, f := range r.Findings {
		if want[string(f.Status)] {
			return 1
		}
	}
	return 0
}

// statusRank orders statuses worst first: past-EOL, expiring, then the
// curated verdicts.
func statusRank(s match.Status) int {
	switch s {
	case match.StatusEOL:
		return 0
	case match.StatusExpiring:
		return 1
	case match.StatusDeprecated:
		return 2
	case match.StatusAbandoned:
		return 3
	case match.StatusUnsupported:
		return 4
	}
	return 5
}

// isReference reports whether a location names an image, SBOM or
// package rather than a file in a repo.
func isReference(file string) bool {
	return strings.HasPrefix(file, "image:") || strings.HasPrefix(file, "sbom:") || strings.HasPrefix(file, "pkg:")
}

// splitLine splits a repo location "path:line" into its parts. A
// reference, or a file with no line, comes back whole with line 0.
func splitLine(file string) (string, int) {
	if isReference(file) {
		return file, 0
	}
	if i := strings.LastIndexByte(file, ':'); i > 0 {
		if n, err := strconv.Atoi(file[i+1:]); err == nil && n > 0 {
			return file[:i], n
		}
	}
	return file, 0
}

// sortFindings returns the findings in the one order every format uses:
// worst status first, then repo, file, line, product and version. Scan
// order alone is not stable from run to run.
func sortFindings(fs []match.Finding) []match.Finding {
	out := slices.Clone(fs)
	slices.SortStableFunc(out, func(a, b match.Finding) int {
		pa, la := splitLine(a.File)
		pb, lb := splitLine(b.File)
		return cmp.Or(
			cmp.Compare(statusRank(a.Status), statusRank(b.Status)),
			cmp.Compare(a.Repo, b.Repo),
			cmp.Compare(pa, pb),
			cmp.Compare(la, lb),
			cmp.Compare(a.Product, b.Product),
			cmp.Compare(a.Version, b.Version),
		)
	})
	return out
}

func (r Report) JSON() (string, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	return string(b), err
}

func (r Report) Table(colors bool) string {
	p := palette{on: colors}
	var sb strings.Builder
	if len(r.Findings) == 0 && len(r.OK) == 0 {
		return p.green("no end-of-life components found") +
			" (every tracked pin verified supported; use --report-untracked for pins with no lifecycle data)"
	}
	if len(r.Findings) > 0 {
		w := func(s string, n int) string {
			if len(s) > n {
				return s[:n-1] + "…"
			}
			return s + strings.Repeat(" ", n-len(s))
		}
		fmt.Fprintln(&sb, p.bold(w("STATUS", 12)+w("REPO", 22)+w("PRODUCT", 15)+w("VERSION", 11)+w("RELEASE", 9)+w("EOL", 23)+"FILE"))
		fmt.Fprintln(&sb, strings.Repeat("-", 118))
		for _, f := range r.Findings {
			days := ""
			if f.Status == match.StatusEOL && f.DaysPast > 0 {
				days = fmt.Sprintf(" +%dd past", f.DaysPast)
			}
			row := w(strings.ToUpper(string(f.Status)), 12) + w(f.Repo, 22) + w(f.Product, 15) +
				w(f.Version, 11) + w(f.Release, 9) + w(f.EOLOn+days, 23) + f.File
			switch f.Status {
			case match.StatusEOL:
				fmt.Fprintln(&sb, p.red(row))
			case match.StatusExpiring:
				fmt.Fprintln(&sb, p.yellow(row))
			case match.StatusDeprecated, match.StatusAbandoned, match.StatusUnsupported:
				fmt.Fprintln(&sb, p.dim(row))
			default:
				fmt.Fprintln(&sb, row)
			}
		}
		fmt.Fprintf(&sb, "\n%d findings (%d past-EOL, %d expiring, %d deprecated/abandoned)\n",
			len(r.Findings), count(r, match.StatusEOL), count(r, match.StatusExpiring),
			count(r, match.StatusDeprecated)+count(r, match.StatusAbandoned)+count(r, match.StatusUnsupported))
	}
	if len(r.OK) > 0 {
		fmt.Fprintf(&sb, "\n%s\n", p.dim(fmt.Sprintf("OK (%d) - verified supported:", len(r.OK))))
		for _, o := range r.OK {
			fmt.Fprintln(&sb, p.dim(fmt.Sprintf("  %-24s %-12s release %-8s eol %-10s %s", o.Product, o.Version, o.Release, o.EOL, o.File)))
		}
	}
	return sb.String()
}

func (p palette) green(s string) string { return p.wrap("32", s) }

func count(r Report, s match.Status) int {
	n := 0
	for _, f := range r.Findings {
		if f.Status == s {
			n++
		}
	}
	return n
}

func untrackedSection(u []catalog.Untracked, colors bool) string {
	p := palette{on: colors}
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n%s\n", p.dim(fmt.Sprintf("UNTRACKED (%d) - pinned but no lifecycle data available; not verified, not cleared:", len(u))))
	for _, x := range u {
		line := fmt.Sprintf("  %-30s %-14s %-40s %s", x.Product, x.Version, x.File, x.Raw)
		if x.Reason != "" {
			line += "  (" + x.Reason + ")"
		}
		fmt.Fprintf(&sb, "%s\n", p.dim(line))
	}
	return sb.String()
}

// WriteOptions carries the presentation choices; Write itself stays the
// only entry point.
type WriteOptions struct {
	Format        string // table | json | sarif
	Out           string // "-" for stdout
	ShowUntracked bool
	ShowOK        bool
	Colors        bool
}

func Write(r Report, opts WriteOptions) error {
	r.Findings = sortFindings(r.Findings)
	var s string
	var err error
	switch opts.Format {
	case "json":
		s, err = r.JSON()
		if err != nil {
			return err
		}
	case "sarif":
		s, err = r.SARIF()
		if err != nil {
			return err
		}
	default:
		s = r.Table(opts.Colors)
		if opts.ShowUntracked && len(r.Untracked) > 0 {
			s += untrackedSection(r.Untracked, opts.Colors)
		}
	}
	if opts.Out == "" || opts.Out == "-" {
		fmt.Println(s)
		return nil
	}
	return os.WriteFile(opts.Out, []byte(s), 0o644)
}
