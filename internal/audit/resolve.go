package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Multi-ecosystem repository resolution: gem, pypi and npm packages map to
// their upstream github repository through the ecosystem's public registry
// API. Resolutions cache for a week (repo urls rarely change); the github
// activity checks reuse the existing repo-state cache.

type resolveEntry struct {
	Repo      string    `json:"repo"`
	CheckedAt time.Time `json:"checked_at"`
}

const resolveTTL = 7 * 24 * time.Hour

// resolveMu serializes cache load+save in Resolve: parallel workers would
// otherwise clobber each other's entries (last writer wins).

var resolveMu sync.Mutex

var resolveCachePath = func() string {
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "zephy", "resolve.json")
	}
	return ""
}

func loadResolveCache() map[string]resolveEntry {
	c := map[string]resolveEntry{}
	if p := resolveCachePath(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			json.Unmarshal(b, &c)
		}
	}
	return c
}

func saveResolveCache(c map[string]resolveEntry) {
	p := resolveCachePath()
	if p == "" {
		return
	}
	os.MkdirAll(filepath.Dir(p), 0o755)
	b, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(p, b, 0o644)
}

// ParseGitHubURL extracts owner/repo from any URL form registries use:
// git+https, ssh, git@, tree paths, www, trailing .git.
func ParseGitHubURL(raw string) (string, bool) {
	u := raw
	u = strings.TrimPrefix(u, "git+")
	u = strings.TrimPrefix(u, "ssh://")
	u = strings.TrimPrefix(u, "git://")
	u = strings.Replace(u, "git@github.com:", "github.com/", 1)
	i := strings.Index(u, "github.com/")
	if i < 0 {
		return "", false
	}
	rest := u[i+len("github.com/"):]
	rest = strings.SplitN(rest, "#", 2)[0]
	rest = strings.SplitN(rest, "?", 2)[0]
	rest = strings.Trim(rest, "/ ")
	rest = strings.TrimSuffix(rest, ".git")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	if strings.EqualFold(parts[1], "tree") || strings.EqualFold(parts[1], "blob") {
		return "", false // org pages and odd paths are not repos
	}
	return strings.ToLower(parts[0] + "/" + parts[1]), true
}

func httpGetBytes(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 20<<20))
}

// Resolve maps (ecosystem, name) to its github owner/repo via the
// ecosystem registry API. Returns ("", false) when the package declares
// no github repository.
func Resolve(ecosystem, name string) (string, bool, error) {
	key := ecosystem + ":" + strings.ToLower(name)
	resolveMu.Lock()
	defer resolveMu.Unlock()
	c := loadResolveCache()
	if e, ok := c[key]; ok && time.Since(e.CheckedAt) < resolveTTL {
		return e.Repo, e.Repo != "", nil
	}
	client := &http.Client{Timeout: 15 * time.Second}
	var repoURL string
	var err error
	switch ecosystem {
	case "gem":
		var b []byte
		if b, err = httpGetBytes(client, "https://rubygems.org/api/v1/gems/"+name+".json"); err == nil {
			var g struct {
				SourceCodeURI string `json:"source_code_uri"`
				HomepageURI   string `json:"homepage_uri"`
			}
			json.Unmarshal(b, &g)
			repoURL = g.SourceCodeURI
			if repoURL == "" {
				repoURL = g.HomepageURI
			}
		}
	case "pypi":
		var b []byte
		if b, err = httpGetBytes(client, "https://pypi.org/pypi/"+name+"/json"); err == nil {
			var p struct {
				Info struct {
					ProjectURLs map[string]string `json:"project_urls"`
					ProjectURL  string            `json:"project_url"`
					HomePge     string            `json:"home_page"`
				} `json:"info"`
			}
			json.Unmarshal(b, &p)
			for _, u := range p.Info.ProjectURLs {
				if strings.Contains(u, "github.com") {
					repoURL = u
					break
				}
			}
			if repoURL == "" && strings.Contains(p.Info.ProjectURL, "github.com") {
				repoURL = p.Info.ProjectURL
			}
			if repoURL == "" {
				repoURL = p.Info.HomePge
			}
		}
	case "npm":
		enc := name
		if strings.HasPrefix(name, "@") {
			enc = strings.Replace(name, "/", "%2F", 1)
		}
		var b []byte
		if b, err = httpGetBytes(client, "https://registry.npmjs.org/"+enc+"/latest"); err == nil {
			var raw map[string]json.RawMessage
			if json.Unmarshal(b, &raw) == nil {
				if rb, ok := raw["repository"]; ok {
					var obj struct {
						URL string `json:"url"`
					}
					if json.Unmarshal(rb, &obj) == nil && obj.URL != "" {
						repoURL = obj.URL
					} else {
						var s string
						if json.Unmarshal(rb, &s) == nil {
							repoURL = s
						}
					}
				}
			}
		}
	default:
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("%s/%s: %v", ecosystem, name, err)
	}
	repo, ok := ParseGitHubURL(repoURL)
	c[key] = resolveEntry{Repo: repo, CheckedAt: time.Now()}
	saveResolveCache(c)
	return repo, ok, nil
}

// ResolveAll resolves many packages with bounded concurrency, using the
// cache for repeats. Returns the resolution map and a failure count.
func ResolveAll(pins []PackageRef) (map[string]string, int) {
	type target struct {
		key       string
		eco, name string
	}
	seen := map[string]bool{}
	var targets []target
	for _, p := range pins {
		key := p.Ecosystem + ":" + strings.ToLower(p.Name)
		if !seen[key] {
			seen[key] = true
			targets = append(targets, target{key, p.Ecosystem, p.Name})
		}
	}
	results := map[string]string{}
	failures := 0
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, t := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(t target) {
			defer wg.Done()
			defer func() { <-sem }()
			repo, ok, err := Resolve(t.eco, t.name)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures++
				return
			}
			if ok {
				results[t.key] = repo
			}
		}(t)
	}
	wg.Wait()
	return results, failures
}

// PackageRef is one auditable package dependency.
type PackageRef struct {
	Ecosystem string
	Name      string
}
