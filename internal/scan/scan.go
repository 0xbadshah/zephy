// Package scan is the pipeline: targets in, one report out. Everything
// between - cataloging, lifecycle matching, policy application, curated
// matching, audits - lives behind Run. The CLI is a thin adapter over this
// seam, and the seam is the test surface.
package scan

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/0xbadshah/zephy/internal/catalog"
	"github.com/0xbadshah/zephy/internal/curated"
	"github.com/0xbadshah/zephy/internal/match"
	"github.com/0xbadshah/zephy/internal/policy"
	"github.com/0xbadshah/zephy/internal/report"
)

// Options is everything Run needs. Now is injectable so classification is
// deterministic in tests; Stderr is the diagnostics sink.
type Options struct {
	Lookahead     time.Duration
	ShowOK        bool
	ShowUntracked bool
	CheckUpstream bool          // flag archived or stale upstream repos
	StaleAfter    time.Duration // no upstream activity for this long is stale
	PolicyPath    string
	Now           time.Time
	Stderr        io.Writer
	// Progress receives per-target events for a UI. When set, the
	// plain stderr progress lines are suppressed.
	Progress func(ScanEvent)
}

// ScanEvent tells a progress UI where the scan is.
type ScanEvent struct {
	Kind     string // "start", "done", "final"
	Target   string
	Findings int
	Err      string
	Total    int // set on "final"
}

func (o Options) stderr() io.Writer {
	if o.Stderr == nil {
		return os.Stderr
	}
	return o.Stderr
}

// Run scans every target and returns the assembled report. Targets are
// scheme-prefixed: repo:<dir>, image:<ref>, sbom:<file>.
func Run(opts Options, targets []string) (report.Report, error) {
	s := &scanner{opts: opts, pol: &policy.File{}}

	if p := autodiscover(opts.PolicyPath, targets); p != "" {
		pol, err := policy.Load(p)
		if err != nil {
			return s.rep, fmt.Errorf("policy %s: %w", p, err)
		}
		s.pol = pol
	}
	s.judge = match.Judge{Policy: s.pol, Now: opts.Now, Lookahead: opts.Lookahead}

	var opErr bool
	emit := func(ev ScanEvent) {
		if s.opts.Progress != nil {
			s.opts.Progress(ev)
		}
	}
	for _, t := range targets {
		emit(ScanEvent{Kind: "start", Target: t})
		scheme, path, ok := strings.Cut(t, ":")
		if !ok {
			fmt.Fprintf(s.opts.stderr(), "unparseable target %q\n", t)
			opErr = true
			emit(ScanEvent{Kind: "done", Target: t, Err: "unparseable"})
			continue
		}
		var src catalog.Source
		var err error
		switch scheme {
		case "repo":
			src, err = catalog.RepoSource(path)
		case "github":
			src, err = catalog.GitHubSource(path)
			if err == nil && s.opts.Progress == nil {
				fmt.Fprintf(s.opts.stderr(), "scanning github:%s (shallow clone)\n", path)
			}
		case "forge":
			host, rest, _ := strings.Cut(path, "/")
			src, err = catalog.ForgeSource(host, rest)
			if err == nil && s.opts.Progress == nil {
				fmt.Fprintf(s.opts.stderr(), "scanning %s (shallow clone)\n", path)
			}
		case "image":
			src, err = catalog.ImageSource(path)
		case "sbom":
			src, err = catalog.SBOMSource(path)
		default:
			fmt.Fprintf(s.opts.stderr(), "unsupported target %q\n", t)
			opErr = true
			emit(ScanEvent{Kind: "done", Target: t, Err: "unsupported target"})
			continue
		}
		if err != nil {
			fmt.Fprintf(s.opts.stderr(), "%s %s: %v\n", scheme, path, err)
			opErr = true
			emit(ScanEvent{Kind: "done", Target: t, Err: err.Error()})
			continue
		}
		n0 := len(s.rep.Findings)
		if err := s.process(src); err != nil {
			fmt.Fprintf(s.opts.stderr(), "%s %s: %v\n", scheme, path, err)
			opErr = true
			emit(ScanEvent{Kind: "done", Target: t, Findings: len(s.rep.Findings) - n0, Err: err.Error()})
			continue
		}
		emit(ScanEvent{Kind: "done", Target: t, Findings: len(s.rep.Findings) - n0})
	}
	emit(ScanEvent{Kind: "final", Total: len(targets)})
	if s.suppressed > 0 {
		fmt.Fprintf(s.opts.stderr(), "note: %d finding(s) suppressed by policy\n", s.suppressed)
	}
	s.rep.OK = s.oks
	if s.opts.ShowUntracked {
		s.rep.Untracked = s.untracked
	}
	if opErr {
		return s.rep, fmt.Errorf("operational errors during scan")
	}
	return s.rep, nil
}

// scanner carries the accumulators as fields: no pointer side-channels in
// method signatures, and the emitters read shared context off the struct.
type scanner struct {
	opts       Options
	pol        *policy.File
	judge      match.Judge
	rep        report.Report
	oks        []match.OKPin
	untracked  []catalog.Untracked
	suppressed int
	seen       map[string]bool // finding locations already reported
}

// process runs one source through the whole pipeline: lifecycle pins,
// curated package matches, curated image policies, audits.
func (s *scanner) process(src catalog.Source) error {
	var goModules []string
	inv, err := src.Inventory()
	if err != nil {
		return err
	}
	for _, p := range inv.Pins {
		s.evaluate(p, src.Name(), src.Kind())
	}
	for _, pp := range inv.Packages {
		if e := curated.Match(pp.Ecosystem, pp.Name, pp.Version); e != nil {
			s.emitCurated(e, pp.Name, pp.Version, src.Name(), pp.File)
		}
		if pp.Ecosystem == "golang" {
			goModules = append(goModules, pp.Name)
		}
	}
	if s.opts.ShowUntracked {
		s.untracked = append(s.untracked, inv.Untracked...)
	}
	if s.opts.CheckUpstream {
		s.emitAudit(goModules, src.Name(), s.opts.StaleAfter)
		s.emitEcosystemAudit(inv.Packages, src.Name(), s.opts.StaleAfter)
	}
	return nil
}
