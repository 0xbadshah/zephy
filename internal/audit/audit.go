package audit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GitHub audit: opt-in (--check-upstream). A network feature, never default -
// the core scan stays offline. For each direct go module hosted on github it
// asks the API two questions: is the repository archived, and when was the
// last push. Answers are cached on disk so repeat scans stay cheap and
// rate-limit friendly.

type RepoState struct {
	Archived  bool      `json:"archived"`
	PushedAt  time.Time `json:"pushed_at"`
	CheckedAt time.Time `json:"checked_at"`
	NotFound  bool      `json:"not_found,omitempty"`
}

type cache map[string]RepoState

const cacheTTL = 24 * time.Hour

func cachePath() string {
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "zephy", "github.json")
	}
	return ""
}

func loadCache() cache {
	c := cache{}
	if p := cachePath(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			json.Unmarshal(b, &c)
		}
	}
	return c
}

func saveCache(c cache) {
	p := cachePath()
	if p == "" {
		return
	}
	os.MkdirAll(filepath.Dir(p), 0o755)
	b, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(p, b, 0o644)
}

// Result is one audited repository.
type Result struct {
	Repo     string
	Archived bool
	PushedAt time.Time
}

// Audit fetches repo states for github-hosted modules. Cached entries
// younger than cacheTTL are reused without a request.
func Audit(repos []string, staleAfter time.Duration) (map[string]Result, []string, error) {
	if len(repos) == 0 {
		return nil, nil, nil
	}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, nil, fmt.Errorf("--check-upstream needs GITHUB_TOKEN in the environment (60 req/h anonymous is not enough for a real tree)")
	}
	c := loadCache()
	client := &http.Client{Timeout: 15 * time.Second}
	results := map[string]Result{}
	var fetched int

	for _, repo := range repos {
		if st, ok := c[repo]; ok && time.Since(st.CheckedAt) < cacheTTL {
			results[repo] = Result{Repo: repo, Archived: st.Archived, PushedAt: st.PushedAt}
			continue
		}
		req, _ := http.NewRequest("GET", "https://api.github.com/repos/"+repo, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		resp, err := client.Do(req)
		if err != nil {
			return results, nil, fmt.Errorf("github api %s: %v", repo, err)
		}
		fetched++
		var body struct {
			Archived bool      `json:"archived"`
			PushedAt time.Time `json:"pushed_at"`
		}
		if resp.StatusCode == http.StatusOK {
			json.NewDecoder(resp.Body).Decode(&body)
			resp.Body.Close()
			c[repo] = RepoState{body.Archived, body.PushedAt, time.Now(), false}
			results[repo] = Result{repo, body.Archived, body.PushedAt}
		} else {
			resp.Body.Close()
			c[repo] = RepoState{CheckedAt: time.Now(), NotFound: true}
		}
		if fetched > 0 && fetched%10 == 0 {
			saveCache(c) // checkpoint long runs
		}
	}
	saveCache(c)
	_ = staleAfter
	return results, nil, nil
}

// GitHubModules filters direct go module paths down to github-hosted repos:
// github.com/owner/repo[/subpkg] -> owner/repo.
func GitHubModules(modules []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range modules {
		if !strings.HasPrefix(m, "github.com/") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(m, "github.com/"), "/")
		if len(parts) < 2 {
			continue
		}
		repo := parts[0] + "/" + parts[1]
		if !seen[repo] {
			seen[repo] = true
			out = append(out, repo)
		}
	}
	return out
}
