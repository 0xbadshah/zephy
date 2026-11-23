// Emitters: the scanner's private methods that turn pins, packages and
// audit states into findings. Same package as the pipeline, split out for
// locality: scan.go is the orchestration, this file is finding production.
package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0xbadshah/zephy/internal/audit"
	"github.com/0xbadshah/zephy/internal/catalog"
	"github.com/0xbadshah/zephy/internal/curated"
	"github.com/0xbadshah/zephy/internal/match"
)

// evaluate records the judge's verdict on one lifecycle pin.
func (s *scanner) evaluate(p catalog.Pin, repo, source string) {
	v := s.judge.Pin(p)
	switch v.Kind {
	case match.KindFinding:
		f := v.Finding
		f.Repo, f.Source = repo, source
		file := f.File
		if i := strings.LastIndex(file, ":"); i > 0 {
			file = file[:i]
		}
		key := f.Repo + "\x00" + f.Product + "\x00" + f.Version + "\x00" + file
		// one line read two ways (K8S_VERSION=v1.23.3+k3s1 as a variable and as a
		// k3s version) is still one declaration
		lineKey := f.Repo + "\x00" + f.Product + "\x00" + f.Release + "\x00" + f.File
		if s.seen[key] || s.seen[lineKey] {
			return // one finding per pin per file: ten spring-* artifacts at 3.2.4 are one upgrade
		}
		if s.seen == nil {
			s.seen = map[string]bool{}
		}
		s.seen[key], s.seen[lineKey] = true, true
		s.rep.Findings = append(s.rep.Findings, f)
	case match.KindOK:
		if s.opts.ShowOK {
			s.oks = append(s.oks, v.OK)
		}
	case match.KindSuppressed:
		s.suppressed++
	case match.KindUntracked:
		s.untracked = append(s.untracked, v.Untracked)
	}
}

// emitCurated appends a curated finding, honoring suppressions, canonical
// product names for image entries, per-repo dedupe, and the abandoned
// activity cross-check.
func (s *scanner) emitCurated(e *curated.DeadEntry, name, version, repo, file string) {
	status := match.Status(match.StatusDeprecated)
	if e.Status == "abandoned" {
		status = match.StatusAbandoned
	}
	if e.Status == "unsupported" {
		status = match.StatusUnsupported
	}
	product := name
	if e.Ecosystem == "image" {
		product = e.Name
	}
	// cross-check: an "abandoned" claim backed by a github repository must
	// not contradict upstream activity.
	if e.Status == "abandoned" {
		if ghRepo, ok := e.GitHubRepo(); ok && os.Getenv("GITHUB_TOKEN") != "" {
			states, _, err := audit.Audit([]string{ghRepo}, 0)
			if err == nil {
				if st, ok := states[ghRepo]; ok && !st.Archived && !st.PushedAt.IsZero() &&
					s.opts.Now.Sub(st.PushedAt) < 2*365*24*time.Hour {
					fmt.Fprintf(s.opts.stderr(),
						"curated cross-check: %s claimed abandoned but %s is active (last push %s) - finding dropped; update internal/curated/known-dead.json\n",
						name, ghRepo, st.PushedAt.Format("2006-01-02"))
					return
				}
			}
		}
	}
	for _, f := range s.rep.Findings {
		if f.Repo == repo && f.Product == product && f.Version == version && f.Status == status {
			return // same finding already emitted for this repo
		}
	}
	if ok, _ := s.pol.Suppressed(product, version, "", file); ok {
		s.suppressed++
		return
	}
	s.rep.Findings = append(s.rep.Findings, match.Finding{
		Repo: repo, Product: product, Version: version, Release: "",
		EOLOn: "", DaysPast: 0, Status: status,
		Source: "curated", File: file, Raw: e.Note,
	})
}

// emitAudit runs the github audit over direct go modules.
func (s *scanner) emitAudit(modules []string, repo string, staleAfter time.Duration) {
	results, _, err := audit.Audit(audit.GitHubModules(modules), staleAfter)
	if err != nil {
		fmt.Fprintf(s.opts.stderr(), "check-upstream: %v\n", err)
		return
	}
	for _, r := range results {
		if r.Archived {
			s.rep.Findings = append(s.rep.Findings, match.Finding{
				Repo: repo, Product: r.Repo, Version: "", Status: match.StatusAbandoned,
				Source: "audit", File: "go.mod", Raw: "repository archived upstream",
			})
			continue
		}
		if staleAfter > 0 && !r.PushedAt.IsZero() && s.opts.Now.Sub(r.PushedAt) > staleAfter {
			s.rep.Findings = append(s.rep.Findings, match.Finding{
				Repo: repo, Product: r.Repo, Version: "", Status: match.StatusAbandoned,
				Source: "audit", File: "go.mod",
				Raw: fmt.Sprintf("no upstream activity since %s (threshold %s)", r.PushedAt.Format("2006-01-02"), staleAfter),
			})
		}
	}
}

// emitEcosystemAudit resolves gem/pypi/npm package dependencies to their
// github repositories and audits them like go modules.
func (s *scanner) emitEcosystemAudit(pkgs []catalog.PackagePin, repo string, staleAfter time.Duration) {
	var refs []audit.PackageRef
	for _, pp := range pkgs {
		switch pp.Ecosystem {
		case "gem", "pypi", "npm":
			refs = append(refs, audit.PackageRef{Ecosystem: pp.Ecosystem, Name: pp.Name})
		}
	}
	if len(refs) == 0 {
		return
	}
	resolved, failures := audit.ResolveAll(refs)
	if failures > 0 {
		fmt.Fprintf(s.opts.stderr(), "audit: %d package(s) failed registry resolution\n", failures)
	}
	repoSet := map[string]bool{}
	var repos []string
	for _, r := range resolved {
		if !repoSet[r] {
			repoSet[r] = true
			repos = append(repos, r)
		}
	}
	if len(repos) == 0 {
		return
	}
	states, _, err := audit.Audit(repos, staleAfter)
	if err != nil {
		fmt.Fprintf(s.opts.stderr(), "audit: %v\n", err)
		return
	}
	seen := map[string]bool{}
	for _, pp := range pkgs {
		if pp.Ecosystem != "gem" && pp.Ecosystem != "pypi" && pp.Ecosystem != "npm" {
			continue
		}
		ghRepo := resolved[pp.Ecosystem+":"+strings.ToLower(pp.Name)]
		if ghRepo == "" {
			continue
		}
		st, ok := states[ghRepo]
		if !ok {
			continue
		}
		status := ""
		note := ""
		switch {
		case st.Archived:
			status, note = string(match.StatusAbandoned), "upstream github repository archived"
		case staleAfter > 0 && !st.PushedAt.IsZero() && s.opts.Now.Sub(st.PushedAt) > staleAfter:
			status, note = string(match.StatusAbandoned),
				fmt.Sprintf("no upstream activity since %s (threshold %s)", st.PushedAt.Format("2006-01-02"), staleAfter)
		}
		if status == "" {
			continue
		}
		dedupe := repo + "|" + pp.Name + "|" + status
		if seen[dedupe] {
			continue
		}
		seen[dedupe] = true
		s.rep.Findings = append(s.rep.Findings, match.Finding{
			Repo: repo, Product: pp.Name, Version: "", Status: match.Status(status),
			Source: "audit", File: pp.File, Raw: note,
		})
	}
}

// autodiscover resolves the policy file path: explicit wins, then the first
// repo target's directory, then the working directory. Empty = none found.
func autodiscover(explicit string, targets []string) string {
	if explicit != "" {
		return explicit
	}
	for _, t := range targets {
		if scheme, path, ok := strings.Cut(t, ":"); ok && scheme == "repo" {
			cand := filepath.Join(path, ".zephy.yml")
			if _, err := os.Stat(cand); err == nil {
				return cand
			}
		}
	}
	if _, err := os.Stat(".zephy.yml"); err == nil {
		return ".zephy.yml"
	}
	return ""
}
