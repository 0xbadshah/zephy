package audit

import "testing"

func TestGitHubModules(t *testing.T) {
	mods := GitHubModules([]string{
		"github.com/owner/repo",
		"github.com/owner/repo/subpkg",
		"gopkg.in/other.v1",
		"github.com/onlyowner",
	})
	if len(mods) != 1 || mods[0] != "owner/repo" {
		t.Fatalf("github module extraction wrong: %v", mods)
	}
}

func TestAuditTokenRequired(t *testing.T) {
	if _, _, err := Audit([]string{"x/y"}, 0); err == nil {
		t.Fatal("audit must refuse to run without GITHUB_TOKEN")
	}
}
