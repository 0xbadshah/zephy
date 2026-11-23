package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

// Target detection: what a developer types maps onto a scan source.
func TestDetectTarget(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "report.sbom.json")
	os.WriteFile(file, []byte("{}"), 0o644)
	cases := []struct{ arg, want string }{
		{dir, "repo:" + dir},
		{file, "sbom:" + file},
		{"./missing", "ERR"},
		{"/missing", "ERR"},
		{"acme/api", "github:acme/api"},
		{"gitlab.com/group/sub/project", "forge:gitlab.com/group/sub/project"},
		{"bitbucket.org/ws/repo", "forge:bitbucket.org/ws/repo"},
		{"gitlab.mycorp.dev/team/app", "forge:gitlab.mycorp.dev/team/app"},
		{"nginx:1.24-alpine", "image:nginx:1.24-alpine"},
		{"barename", "ERR"},
		{"repo:/some/dir", "repo:/some/dir"},
		{"github:acme/api", "github:acme/api"},
		{"image:nginx:1.24", "image:nginx:1.24"},
		{"sbom:./report.json", "sbom:./report.json"},
		{"forge:gitlab.com/g/p", "forge:gitlab.com/g/p"},
	}
	for _, c := range cases {
		got, err := DetectTarget(c.arg)
		if c.want == "ERR" {
			if err == nil {
				t.Errorf("%q: expected an error", c.arg)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", c.arg, err)
			continue
		}
		if got != c.want {
			t.Errorf("%q: got %s, want %s", c.arg, got, c.want)
		}
	}
}

// ParseOrgSpec: bare names are GitHub; hosts name their forge.
func TestParseOrgSpec(t *testing.T) {
	cases := []struct {
		spec, host, org string
		wantErr         bool
	}{
		{"acme", "github.com", "acme", false},
		{"gitlab.com/mygroup", "gitlab.com", "mygroup", false},
		{"bitbucket.org/ws", "bitbucket.org", "ws", false},
		{"gitlab.mycorp.dev/team", "gitlab.mycorp.dev", "team", false},
		{"mycorp.dev", "", "", true}, // a host without an org
		{"/only-org", "", "", true},
	}
	for _, c := range cases {
		host, org, err := ParseOrgSpec(c.spec)
		if c.wantErr {
			if err == nil {
				t.Errorf("%q: expected an error", c.spec)
			}
			continue
		}
		if host != c.host || org != c.org {
			t.Errorf("%q: got %s/%s, want %s/%s", c.spec, host, org, c.host, c.org)
		}
	}
}
