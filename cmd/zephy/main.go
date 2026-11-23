package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/0xbadshah/zephy/internal/catalog"
	"github.com/0xbadshah/zephy/internal/report"
	"github.com/0xbadshah/zephy/internal/scan"
	"github.com/0xbadshah/zephy/internal/ui"
)

// orgList collects repeated --org values.
type orgList []string

func (o *orgList) String() string     { return strings.Join(*o, ",") }
func (o *orgList) Set(v string) error { *o = append(*o, v); return nil }

// expandOrgs turns --org values into one forge target per active repo.
// emit reports per-org progress to the TUI; with no emit the plain
// stderr lines print instead.
func expandOrgs(specs []string, emit func(scan.ScanEvent)) ([]string, error) {
	var out []string
	for _, spec := range specs {
		host, org, err := catalog.ParseOrgSpec(spec)
		if err != nil {
			return nil, err
		}
		if emit != nil {
			emit(scan.ScanEvent{Kind: "listing", Target: host + "/" + org})
		}
		repos, err := catalog.ListOrgRepos(host, org)
		if err != nil {
			return nil, err
		}
		if emit != nil {
			emit(scan.ScanEvent{Kind: "listed", Target: host + "/" + org, Total: len(repos)})
		} else {
			fmt.Fprintf(os.Stderr, "%s/%s · %d active repos\n", host, org, len(repos))
		}
		for _, r := range repos {
			out = append(out, r.Target())
		}
	}
	return out, nil
}

func main() {
	lookahead := window(365 * day)
	flag.Var(&lookahead, "lookahead", "expiring window: days, weeks or years (90d, 6w, 1y) or a Go duration (720h)")
	staleAfter := window(5 * 365 * day)
	flag.Var(&staleAfter, "stale-after", "with --check-upstream: an upstream repo with no activity for this long is stale")
	var (
		format        = flag.String("format", "table", "report format: table | json | sarif")
		out           = flag.String("out", "-", "output file, - for stdout")
		failOn        = flag.String("fail-on", "eol", "exit 1 on: eol, expiring, deprecated, abandoned, unsupported (empty = never)")
		orgs          orgList // --org acme, --org gitlab.com/mygroup, ...
		policyPath    = flag.String("policy", "", "path to .zephy.yml; auto-discovered in the target dir when unset")
		showUntracked = flag.Bool("report-untracked", false, "list pins we cannot verify (no lifecycle data)")
		showOK        = flag.Bool("show-ok", false, "list pins verified as supported")
		noColor       = flag.Bool("no-color", false, "disable terminal colors (also honored: NO_COLOR env)")
		noTUI         = flag.Bool("no-tui", false, "plain progress output, even on a terminal")
		doRefresh     = flag.Bool("refresh", false, "update the cached lifecycle data now, then exit")
		offline       = flag.Bool("offline", false, "skip the lifecycle data update before the scan (also: ZEPHY_OFFLINE=1); data already downloaded is still used")
		checkUpstream = flag.Bool("check-upstream", false, "also flag dependencies whose upstream repo is archived or stale: Go modules and gem, PyPI and npm lockfile dependencies; calls the GitHub API and needs GITHUB_TOKEN")
		showVersion   = flag.Bool("version", false, "print the version and exit")
	)
	flag.Var(&orgs, "org", "GitHub/GitLab/Bitbucket org to scan: acme (= GitHub), gitlab.com/mygroup, bitbucket.org/ws, gitlab.mycorp.dev/team")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "zephy - find end-of-life software in repositories, images and SBOMs\n\n")
		fmt.Fprintf(os.Stderr, "Usage: zephy [flags] <path | owner/repo | image> ...\n\n")
		fmt.Fprintf(os.Stderr, "  zephy .                        scan the repo in the current directory\n")
		fmt.Fprintf(os.Stderr, "  zephy ./services ../web        several local checkouts\n")
		fmt.Fprintf(os.Stderr, "  zephy acme/api                 a GitHub repo (shallow-cloned; private repos too)\n")
		fmt.Fprintf(os.Stderr, "  zephy --org acme               every active repo of a GitHub org\n")
		fmt.Fprintf(os.Stderr, "  zephy --org gitlab.com/g/proj  GitLab, Bitbucket and self-hosted likewise\n")
		fmt.Fprintf(os.Stderr, "  zephy nginx:1.24-alpine        a container image (uses the syft CLI)\n")
		fmt.Fprintf(os.Stderr, "  zephy ./report.sbom.json       a Syft/SPDX/CycloneDX SBOM file\n\n")
		fmt.Fprintf(os.Stderr, "Detection: an existing directory is a repo, an existing file is an SBOM,\n")
		fmt.Fprintf(os.Stderr, "host/owner/repo names a forge repo, owner/repo is GitHub, ref:tag is an\n")
		fmt.Fprintf(os.Stderr, "image. Explicit schemes (repo:, github:, forge:, image:, sbom:) still work.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Println("zephy", versionString())
		os.Exit(0)
	}

	if *doRefresh {
		status, err := refresh()
		if err != nil {
			fmt.Fprintf(os.Stderr, "refresh: %v\n", err)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, status)
		os.Exit(0)
	}

	raw := flag.Args()
	if len(raw) == 0 && len(orgs) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	// lifecycle data first, before any TUI starts: the update's one line
	// on failure must not land in the middle of the progress display
	loadDataset(*offline || os.Getenv("ZEPHY_OFFLINE") != "")

	var targets []string
	for _, arg := range raw {
		t, err := catalog.DetectTarget(arg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		targets = append(targets, t)
	}

	// the progress UI starts BEFORE the org listing: the spinner is on
	// screen within milliseconds, and the API wait happens underneath it
	useTUI := ui.UseTUI(*noTUI)
	var events chan scan.ScanEvent
	var uiWait func()
	if useTUI {
		events = make(chan scan.ScanEvent, 64)
		p := ui.NewProgress(0, events)
		done := ui.RunProgress(p) // rendering now, during any blocking work
		uiWait = func() {
			close(events)
			<-done
		}
	}
	emit := func(ev scan.ScanEvent) {
		if events != nil {
			events <- ev
		}
	}

	orgTargets, err0 := expandOrgs(orgs, emit)
	if err0 != nil {
		if useTUI {
			emit(scan.ScanEvent{Kind: "list-fail"})
			uiWait()
		}
		fmt.Fprintln(os.Stderr, err0)
		os.Exit(2)
	}
	targets = append(targets, orgTargets...)
	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "no repos found to scan")
		os.Exit(2)
	}
	emit(scan.ScanEvent{Kind: "targets", Total: len(targets)})

	var progress func(scan.ScanEvent)
	if useTUI {
		progress = emit
	}

	rep, err := scan.Run(scan.Options{
		Lookahead:     time.Duration(lookahead),
		ShowOK:        *showOK,
		ShowUntracked: *showUntracked,
		CheckUpstream: *checkUpstream,
		StaleAfter:    time.Duration(staleAfter),
		PolicyPath:    *policyPath,
		Now:           time.Now(),
		Progress:      progress,
	}, targets)
	if uiWait != nil {
		uiWait()
	}
	if err2 := report.Write(rep, report.WriteOptions{
		Format:        *format,
		Out:           *out,
		ShowUntracked: *showUntracked,
		ShowOK:        *showOK,
		Colors:        report.ColorsOn(*noColor),
	}); err2 != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err2)
		os.Exit(2)
	}

	var fail []string
	if *failOn != "" {
		fail = strings.Split(*failOn, ",")
	}
	if err != nil {
		os.Exit(2)
	}
	os.Exit(rep.ExitCode(fail))
}
