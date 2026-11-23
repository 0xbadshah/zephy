package match

import (
	"testing"
	"time"

	"github.com/0xbadshah/zephy/internal/data"
)

func d(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }

func TestCycleMatchPrefix(t *testing.T) {
	cycles := []data.Cycle{
		{Cycle: "8.1"}, {Cycle: "8.0"}, {Cycle: "7.2"},
	}
	if c := CycleMatch(cycles, "8.0.5.1"); c == nil || c.Cycle != "8.0" {
		t.Fatalf("8.0.5.1 must match 8.0, got %+v", c)
	}
	if c := CycleMatch(cycles, "8.1.3.1"); c == nil || c.Cycle != "8.1" {
		t.Fatalf("8.1.3.1 must match 8.1, got %+v", c)
	}
	if c := CycleMatch(cycles, "9.0.1"); c != nil {
		t.Fatalf("unmatched version must return nil, got %+v", c)
	}
}

// A bare-major float (rabbitmq:3-management) resolves to the newest
// cycle of that line (syft never catalogs the broker itself).
func TestBareMajorFallback(t *testing.T) {
	cycles := []data.Cycle{
		{Cycle: "4.4"},
		{Cycle: "4.1"},
		{Cycle: "3.13"},
		{Cycle: "3.12"},
	}
	c := CycleMatch(cycles, "3")
	if c == nil || c.Cycle != "3.13" {
		t.Fatalf("bare major 3 must resolve to newest 3.x cycle, got %+v", c)
	}
	// an exact cycle match must win over the fallback
	if c := CycleMatch(cycles, "4.4"); c == nil || c.Cycle != "4.4" {
		t.Fatalf("exact cycle must match, got %+v", c)
	}
	// a major with no cycles stays unknown (never guess)
	if c := CycleMatch(cycles, "2"); c != nil {
		t.Fatalf("unknown major must stay nil, got %+v", c)
	}
}

func TestClassifyPast(t *testing.T) {
	s, eol, days := ClassifyDate(d("2026-11-12"), d("2026-11-18"), 365*24*time.Hour)
	if s != StatusEOL || eol != "2026-11-12" || days != 6 {
		t.Fatalf("past EOL misclassified: %s %s %d", s, eol, days)
	}
}

func TestClassifyExpiring(t *testing.T) {
	s, _, _ := ClassifyDate(d("2027-04-30"), d("2026-11-18"), 365*24*time.Hour)
	if s != StatusExpiring {
		t.Fatalf("within lookahead must be expiring, got %s", s)
	}
	s, _, _ = ClassifyDate(d("2030-01-01"), d("2026-11-18"), 365*24*time.Hour)
	if s != "" {
		t.Fatalf("outside window must be unreportable, got %s", s)
	}
}
