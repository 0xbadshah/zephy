package match

import (
	"testing"
	"time"

	"github.com/0xbadshah/zephy/internal/catalog"
	"github.com/0xbadshah/zephy/internal/policy"
	"gopkg.in/yaml.v3"
)

var now = time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)

func judge(t *testing.T, pol string) Judge {
	t.Helper()
	var f policy.File
	if err := yaml.Unmarshal([]byte(pol), &f); err != nil {
		t.Fatal(err)
	}
	return Judge{Policy: &f, Now: now, Lookahead: 365 * 24 * time.Hour}
}

// Every pin gets exactly one verdict; the reason travels with the
// unverifiable ones. These cases run against the embedded dataset with a
// fixed clock.
func TestJudgeVerdicts(t *testing.T) {
	j := judge(t, `
suppress:
  - product: redis
    path: "legacy/**"
extends:
  - product: postgresql
    release: "13"
    until: "2030-01-01"
normalize:
  - '^(\d+\.\d+\.\d+)p\d+'
`)
	cases := []struct {
		name    string
		pin     catalog.Pin
		kind    Kind
		status  Status
		release string
	}{
		{"past EOL", catalog.Pin{Product: "python", Version: "3.7.9"}, KindFinding, StatusEOL, "3.7"},
		{"expiring", catalog.Pin{Product: "python", Version: "3.11.4"}, KindFinding, StatusExpiring, "3.11"},
		{"supported", catalog.Pin{Product: "python", Version: "3.13.1"}, KindOK, "", "3.13"},
		{"policy normalize", catalog.Pin{Product: "ruby", Version: "2.4.4p296"}, KindFinding, StatusEOL, "2.4"},
		{"extended support", catalog.Pin{Product: "postgresql", Version: "13.4"}, KindOK, "", "13"},
		{"suppressed", catalog.Pin{Product: "redis", Version: "6.0.9", File: "legacy/docker-compose.yml:3"}, KindSuppressed, StatusEOL, "6.0"},
		{"not suppressed elsewhere", catalog.Pin{Product: "redis", Version: "6.0.9", File: "docker-compose.yml:3"}, KindFinding, StatusEOL, "6.0"},
		// a floating major inferred to an expiring minor follows the line
		{"floating major", catalog.Pin{Product: "rabbitmq", Version: "4"}, KindOK, "", "4.4"},
		// ...unless a newer major closed the line: tailwind 3's last minor
		{"closed major", catalog.Pin{Product: "tailwind-css", Version: "3"}, KindFinding, StatusExpiring, "3.4"},
		// a whole major past EOL is still eol
		{"dead major", catalog.Pin{Product: "rabbitmq", Version: "3"}, KindFinding, StatusEOL, "3.13"},
		{"unknown product", catalog.Pin{Product: "no-such-product", Version: "1.0"}, KindUntracked, "", ""},
		{"newer than every cycle", catalog.Pin{Product: "python", Version: "9.1"}, KindUntracked, "", ""},
		// an untracked intermediate line under a dead newer one
		{"dead intermediate", catalog.Pin{Product: "julia", Version: "1.9"}, KindFinding, StatusEOL, "<1.11"},
		// below a dead cycle: spring 4.2 is not tracked, 4.3 is long dead
		{"below a dead cycle", catalog.Pin{Product: "spring-framework", Version: "4.2.6.RELEASE"}, KindFinding, StatusEOL, "<4.3"},
		// java family: the first distribution shipping the line decides
		{"java lts", catalog.Pin{Product: "java", Version: "8"}, KindOK, "", "8"},
		{"java short-lived", catalog.Pin{Product: "java", Version: "13"}, KindFinding, StatusEOL, "13"},
		{"java legacy", catalog.Pin{Product: "java", Version: "7"}, KindFinding, StatusEOL, "7"},
		// older than every tracked cycle, oldest cycle long dead
		{"predates dataset", catalog.Pin{Product: "redis", Version: "4.0.14"}, KindFinding, StatusEOL, "<5.0"},
		{"predates a live oldest cycle", catalog.Pin{Product: "react", Version: "0.14.8"}, KindUntracked, "", ""},
		{"eol true, no date", catalog.Pin{Product: "nodejs", Version: "0.10.48"}, KindFinding, StatusEOL, "<1"},
	}
	for _, c := range cases {
		v := j.Pin(c.pin)
		if v.Kind != c.kind {
			t.Errorf("%s: kind = %v, want %v (%+v)", c.name, v.Kind, c.kind, v)
			continue
		}
		switch v.Kind {
		case KindFinding, KindSuppressed:
			if v.Finding.Status != c.status || v.Finding.Release != c.release {
				t.Errorf("%s: finding = %+v, want status %s release %s", c.name, v.Finding, c.status, c.release)
			}
		case KindOK:
			if v.OK.Release != c.release {
				t.Errorf("%s: ok release = %q, want %q", c.name, v.OK.Release, c.release)
			}
		case KindUntracked:
			if v.Reason == "" || v.Untracked.Reason == "" {
				t.Errorf("%s: untracked verdict without a reason", c.name)
			}
		}
	}
}

func TestOKVerdictCarriesEOLDate(t *testing.T) {
	v := judge(t, "").Pin(catalog.Pin{Product: "python", Version: "3.13.1"})
	if v.Kind != KindOK || v.OK.EOL == "" {
		t.Fatalf("OK verdict must carry the cycle's EOL date: %+v", v)
	}
}
