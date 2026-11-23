package report

import (
	"encoding/json"
	"testing"

	"github.com/0xbadshah/zephy/internal/match"
)

// Code scanning needs a file path it can find in the repository and the
// line as a region, not "path:line" glued into the URI.
func TestSARIFLocations(t *testing.T) {
	r := Report{Findings: []match.Finding{
		{Product: "python", Version: "3.9", Release: "3.9", EOLOn: "2025-10-31", DaysPast: 10, Status: match.StatusEOL, Source: "repo", File: "Dockerfile:1"},
		{Product: "nodejs", Version: "22", Release: "22", EOLOn: "2027-04-30", Status: match.StatusExpiring, Source: "repo", File: ".nvmrc"},
		{Product: "django", Version: "4.2", Status: match.StatusEOL, Source: "repo", File: "my app/requirements.txt:12"},
		{Product: "request", Version: "2", Status: match.StatusDeprecated, Source: "curated", File: "package.json:5"},
		{Product: "python", Version: "3.9", Status: match.StatusEOL, Source: "image", File: "image:ghcr.io/org/app:1"},
		{Product: "openssl", Version: "1.1.1", Status: match.StatusEOL, Source: "sbom", File: "sbom:app.json"},
	}}
	out, err := r.SARIF()
	if err != nil {
		t.Fatal(err)
	}
	var log sarifLog
	if err := json.Unmarshal([]byte(out), &log); err != nil {
		t.Fatal(err)
	}
	type want struct {
		uri  string
		line int
		rule string
		msg  string
	}
	for i, w := range []want{
		{"Dockerfile", 1, "EOL001", "python 3.9 is eol (release 3.9, EOL 2025-10-31), 10 days ago"},
		{".nvmrc", 0, "EOL002", "nodejs 22 is expiring (release 22, EOL 2027-04-30)"},
		{"my%20app/requirements.txt", 12, "EOL001", "django 4.2 is eol"},
		{"package.json", 5, "EOL003", "request 2 is deprecated"},
		{"image:ghcr.io/org/app:1", 0, "EOL001", "python 3.9 is eol"},
		{"sbom:app.json", 0, "EOL001", "openssl 1.1.1 is eol"},
	} {
		res := log.Runs[0].Results[i]
		loc := res.Locations[0].PhysicalLocation
		line := 0
		if loc.Region != nil {
			line = loc.Region.StartLine
		}
		if loc.ArtifactLocation.URI != w.uri || line != w.line || res.RuleID != w.rule || res.Message.Text != w.msg {
			t.Errorf("result %d = %q line %d %s %q; want %q line %d %s %q",
				i, loc.ArtifactLocation.URI, line, res.RuleID, res.Message.Text, w.uri, w.line, w.rule, w.msg)
		}
	}
}
