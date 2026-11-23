package scan

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/0xbadshah/zephy/internal/catalog"
	"github.com/0xbadshah/zephy/internal/match"
)

// fixedNow keeps classification deterministic for the fixture pins.
var fixedNow = time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)

func fixtureOpts(t *testing.T) Options {
	t.Helper()
	return Options{
		Lookahead: 365 * 24 * time.Hour,
		Now:       fixedNow,
		Stderr:    &bytes.Buffer{},
	}
}

// The golden fixtures carry deliberately end-of-life pins: the seam test
// asserts the pipeline produces exactly the expected findings for a fixed
// clock, without running the binary.
func TestRunGoldenFixtures(t *testing.T) {
	rep, err := Run(fixtureOpts(t), []string{"repo:../catalog/testdata/mini-repo"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(rep.Findings) == 0 {
		t.Fatal("fixture scan produced no findings")
	}
	byKey := map[match.Status]int{}
	for _, f := range rep.Findings {
		byKey[f.Status]++
	}
	// every lifecycle pin in the fixture is past EOL at fixedNow
	// (ruby 3.4.10 is expiring, python 3.13 is supported: excluded).
	// redis 4.0.14 predates the dataset's oldest redis cycle and still
	// counts as a past-EOL finding.
	if byKey[match.StatusEOL] != 19 {
		t.Errorf("past-EOL count = %d, want 19 (golden fixture drift)", byKey[match.StatusEOL])
	}
	// the chart triple must surface the curated retirement entry
	foundRetired := false
	for _, f := range rep.Findings {
		if f.Status == match.StatusAbandoned && f.Product == "ingress-nginx/controller" {
			foundRetired = true
		}
	}
	if !foundRetired {
		t.Error("ingress-nginx/controller retirement finding missing")
	}
}

// No silent skips: every lifecycle pin the source saw comes out as exactly
// one of finding, OK, suppressed or untracked.
func TestRunAccountsForEveryPin(t *testing.T) {
	opts := fixtureOpts(t)
	opts.ShowOK, opts.ShowUntracked = true, true
	rep, err := Run(opts, []string{"repo:../catalog/testdata/mini-repo"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	src, _ := catalog.RepoSource("../catalog/testdata/mini-repo")
	inv, _ := src.Inventory()
	lifecycle := 0
	for _, f := range rep.Findings {
		if f.Source == "repo" && f.Release != "" {
			lifecycle++
		}
	}
	fromVerdicts := 0
	for _, u := range rep.Untracked {
		if strings.Contains(u.Reason, "lifecycle data for") || strings.Contains(u.Reason, "matches no") {
			fromVerdicts++
		}
	}
	if got := lifecycle + len(rep.OK) + fromVerdicts; got != len(inv.Pins) {
		t.Errorf("pins in = %d, verdicts out = %d (findings %d, ok %d, untracked %d)",
			len(inv.Pins), got, lifecycle, len(rep.OK), fromVerdicts)
	}
}

// The dogfood behavior, unit-tested at the seam: a policy suppressing the
// fixtures must zero the findings - this is the exact contract our own CI
// relies on.
func TestRunPolicySuppression(t *testing.T) {
	opts := fixtureOpts(t)
	opts.PolicyPath = "../catalog/testdata/suppress-fixtures.yml"
	rep, err := Run(opts, []string{"repo:../catalog/testdata/mini-repo"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(rep.Findings) != 0 {
		t.Errorf("suppressed scan must be empty, got %d findings", len(rep.Findings))
	}
}

// Unparseable targets are operational errors, not silent skips.
func TestRunBadTarget(t *testing.T) {
	_, err := Run(fixtureOpts(t), []string{"not-a-scheme:whatever"})
	if err == nil {
		t.Fatal("bad target must return an operational error")
	}
}
