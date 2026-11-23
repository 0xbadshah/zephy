package match

import (
	"strings"
	"time"

	"github.com/0xbadshah/zephy/internal/data"
)

type Status string

const (
	StatusEOL         Status = "eol"         // past end-of-life
	StatusExpiring    Status = "expiring"    // EOL within the lookahead window
	StatusDeprecated  Status = "deprecated"  // upstream formally deprecated/replaced (curated)
	StatusAbandoned   Status = "abandoned"   // upstream unmaintained (curated)
	StatusUnsupported Status = "unsupported" // outside the upstream support policy (curated)
)

// OKPin is a pin verified against the dataset and found supported
// (--show-ok): builds trust that the scan actually saw the tree.
type OKPin struct {
	Product string `json:"product"`
	Version string `json:"version"`
	Release string `json:"release"`
	EOL     string `json:"eol"`
	File    string `json:"file"`
}

// Finding is one verified lifecycle hit.
type Finding struct {
	Repo     string `json:"repo"`
	Product  string `json:"product"`   // endoflife.date slug
	Version  string `json:"version"`   // as pinned
	Release  string `json:"release"`   // release line the version belongs to
	EOLOn    string `json:"eol"`       // ISO date, "" if none declared
	DaysPast int    `json:"days_past"` // for status=eol, days since the date passed
	Status   Status `json:"status"`
	Source   string `json:"source"` // repo | image | sbom | curated | audit
	File     string `json:"file"`   // relative to repo root, with line where known
	Raw      string `json:"raw"`    // image ref / directive / pin as written
}

// CycleMatch returns the most specific cycle whose identifier is a
// version prefix of the pinned version ("8.0.5.1" -> "8.0", "17.0.2" -> "17").
// Bare-major floats ("3" in rabbitmq:3) match the newest cycle with that
// major prefix: brokers ship as name:3 tags while
// their real lines are 3.13, 3.12... - a float is the newest of its line.
func CycleMatch(cycles []data.Cycle, version string) *data.Cycle {
	var best *data.Cycle
	for i := range cycles {
		c := cycles[i]
		cyc := c.Cycle
		if version == cyc || strings.HasPrefix(version, cyc+".") || strings.HasPrefix(version, cyc+"-") {
			if best == nil || len(cyc) > len(best.Cycle) {
				best = &cycles[i]
			}
		}
	}
	if best != nil {
		return best
	}
	// bare-major fallback: "3" -> newest cycle starting "3."
	if isBareMajor(version) {
		for i := range cycles {
			if strings.HasPrefix(cycles[i].Cycle, version+".") {
				return &cycles[i] // dataset lists newest cycles first
			}
		}
	}
	return best
}

// isBareMajor reports a major-only version (a floating tag like
// rabbitmq:3): the tag follows the line, so it only expires when every
// cycle of the major is past end-of-life.
func isBareMajor(v string) bool {
	if v == "" || v[0] < '0' || v[0] > '9' {
		return false
	}
	for i := 1; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return false
		}
	}
	return true
}

// Classify applies the lookahead policy to a matched cycle.
// today/lookahead are passed in for testability.
func Classify(c *data.Cycle, today time.Time, lookahead time.Duration) (Status, string, int) {
	if !c.EOL.Set {
		if c.EOL.Ended {
			return StatusEOL, "", 0 // ended, date unpublished
		}
		return "", "", 0
	}
	return ClassifyDate(c.EOL.Date, today, lookahead)
}

// ClassifyDate is Classify against an arbitrary date, used by the
// extended-support policy override (policy owns the date, not the dataset).
func ClassifyDate(eol time.Time, today time.Time, lookahead time.Duration) (Status, string, int) {
	if today.After(eol) {
		days := int(today.Sub(eol).Hours() / 24)
		return StatusEOL, eol.Format("2006-01-02"), days
	}
	if eol.Sub(today) <= lookahead {
		return StatusExpiring, eol.Format("2006-01-02"), 0
	}
	return "", "", 0 // supported, not reportable
}
