package curated

import "testing"

func TestMatch(t *testing.T) {
	if e := Match("pypi", "oauth2client", "4.1.3"); e == nil {
		t.Fatal("oauth2client must match the curated list")
	}
	// range-limited entry: log4j 1.x only
	if e := Match("maven", "log4j:log4j", "1.2.17"); e == nil || e.Status != "abandoned" {
		t.Fatalf("log4j 1.2.17 must match as abandoned, got %+v", e)
	}
	if e := Match("maven", "log4j:log4j", "2.17.1"); e != nil {
		t.Fatalf("log4j 2.x must not match, got %+v", e)
	}
	if e := Match("npm", "request", "2.88.2"); e == nil {
		t.Fatal("request must match")
	}
	if e := Match("npm", "express", "4.18.0"); e != nil {
		t.Fatalf("maintained package must not match, got %+v", e)
	}
	if e := Match("gem", "rails", "8.0.2"); e != nil {
		t.Fatalf("lifecycle-tracked products stay out of the curated list, got %+v", e)
	}
}

// The evidence bar: every entry must carry a public upstream statement.
// byebug and httpclient are live projects that look dead from old pins:
// they must never be listed.
func TestEveryEntryHasEvidence(t *testing.T) {
	load()
	if len(entries) == 0 {
		t.Fatal("curated list is empty")
	}
	for _, e := range entries {
		if e.Evidence == "" {
			t.Errorf("entry %s has no evidence", e.Name)
		}
		if e.Note == "" {
			t.Errorf("entry %s has no note", e.Name)
		}
	}
	// live projects with old pins must stay out
	for _, name := range []string{"byebug", "httpclient"} {
		if e := Match("gem", name, "11.1.3"); e != nil {
			t.Errorf("%s must not match: it is an old pin of a live project", name)
		}
	}
}

func TestGitHubRepoFromEvidence(t *testing.T) {
	cases := []struct {
		evidence, want string
		ok             bool
	}{
		{"https://github.com/googleapis/oauth2client (repository archived by Google)", "googleapis/oauth2client", true},
		{"https://github.com/request/request (README: deprecated, issue 3142)", "request/request", true},
		{"https://pypi.org/project/slackclient/ (deprecated in favor of slack_sdk)", "", false},
		{"https://logging.apache.org/log4j/1.x/ (EOL banner: 1.x reached end of life in 2015)", "", false},
	}
	for _, c := range cases {
		e := DeadEntry{Evidence: c.evidence}
		got, ok := e.GitHubRepo()
		if ok != c.ok || got != c.want {
			t.Errorf("GitHubRepo(%q) = (%q, %v), want (%q, %v)", c.evidence, got, ok, c.want, c.ok)
		}
	}
}
