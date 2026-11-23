package catalog

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DetectTarget maps a user-typed argument to an internal scheme target:
// a directory on disk is a repo scan, a file on disk is an SBOM, a
// host-prefixed path is a forge repo (gitlab.com/group/sub/project,
// bitbucket.org/ws/repo, self-hosted alike), a two-segment path is a
// GitHub owner/repo, and a colon makes an image ref. Everything else is
// an error with a hint instead of a guess.
func DetectTarget(arg string) (string, error) {
	// explicit schemes pass through untouched
	for _, s := range []string{"repo:", "github:", "forge:", "image:", "sbom:"} {
		if strings.HasPrefix(arg, s) {
			return arg, nil
		}
	}
	if fi, err := os.Stat(arg); err == nil {
		if fi.IsDir() {
			return "repo:" + arg, nil
		}
		return "sbom:" + arg, nil
	}
	if strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "../") {
		return "", fmt.Errorf("%s does not exist on disk", arg)
	}
	if strings.Contains(arg, ":") {
		return "image:" + arg, nil
	}
	parts := strings.Split(arg, "/")
	if len(parts) >= 3 && strings.Contains(parts[0], ".") {
		return "forge:" + arg, nil // host/owner[/subgroup]/repo
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return "github:" + arg, nil
	}
	return "", fmt.Errorf("cannot tell what %q is: try a path (./name), a GitHub repo (owner/name), an org (--org name), or an image (name:tag)", arg)
}

// ParseOrgSpec reads a --org value: "acme" is GitHub, anything
// host-prefixed names its forge ("gitlab.com/mygroup",
// "bitbucket.org/ws", "gitlab.mycorp.dev/team" for self-hosted).
func ParseOrgSpec(spec string) (host, org string, err error) {
	if !strings.Contains(spec, "/") {
		if strings.Contains(spec, ".") || strings.Contains(spec, ":") {
			return "", "", fmt.Errorf("--org %s looks like a host without an org — use %s/<org>", spec, spec)
		}
		return "github.com", spec, nil
	}
	host, org, _ = strings.Cut(spec, "/")
	if host == "" || org == "" {
		return "", "", fmt.Errorf("--org %s: want host/org or just org", spec)
	}
	return host, org, nil
}

// ---- listing ----

type forgeRepo struct {
	Host string // github.com, gitlab.com, my-gitlab.dev, ...
	Slug string // owner/repo below the host
}

func (r forgeRepo) Target() string { return "forge:" + r.Host + "/" + r.Slug }

var httpClient = &http.Client{Timeout: 30 * time.Second}

// ListOrgRepos returns every active repo of an org on its forge:
// github.com (or a user account), gitlab.com and self-hosted GitLab,
// bitbucket.org.
func ListOrgRepos(host, org string) ([]forgeRepo, error) {
	return listForge(host, org)
}

func listForge(host, org string) ([]forgeRepo, error) {
	switch {
	case host == "github.com":
		return listGitHub(org)
	case host == "bitbucket.org":
		return listBitbucket(org)
	default:
		return listGitLab(host, org) // gitlab.com and self-hosted GitLab
	}
}

func httpJSON(req *http.Request, out any) error {
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func githubToken() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	if b, err := exec.Command("gh", "auth", "token").Output(); err == nil {
		return strings.TrimSpace(string(b))
	}
	return ""
}

func listGitHub(org string) ([]forgeRepo, error) {
	// an org first; a user account falls back when the org 404s
	kind := "orgs"
	token := githubToken()
	get := func(u string) ([][]struct {
		Name     string `json:"name"`
		Archived bool   `json:"archived"`
	}, error) {
		var pages [][]struct {
			Name     string `json:"name"`
			Archived bool   `json:"archived"`
		}
		for page := 1; page <= 20; page++ {
			req, _ := http.NewRequest("GET", fmt.Sprintf("%s?per_page=100&page=%d", u, page), nil)
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			var pageOut []struct {
				Name     string `json:"name"`
				Archived bool   `json:"archived"`
			}
			if err := httpJSON(req, &pageOut); err != nil {
				return pages, err
			}
			pages = append(pages, pageOut)
			if len(pageOut) < 100 {
				break
			}
		}
		return pages, nil
	}
	pages, err := get("https://api.github.com/" + kind + "/" + org + "/repos")
	if err != nil && strings.Contains(err.Error(), "HTTP 404") {
		kind = "users"
		pages, err = get("https://api.github.com/" + kind + "/" + org + "/repos")
	}
	if err != nil {
		if token == "" {
			return nil, fmt.Errorf("listing github.com/%s: %w (set GITHUB_TOKEN for private repos)", org, err)
		}
		return nil, fmt.Errorf("listing github.com/%s: %w", org, err)
	}
	var out []forgeRepo
	for _, page := range pages {
		for _, r := range page {
			if !r.Archived {
				out = append(out, forgeRepo{Host: "github.com", Slug: org + "/" + r.Name})
			}
		}
	}
	return out, nil
}

func listGitLab(host, group string) ([]forgeRepo, error) {
	scheme := "https"
	if h, ok := strings.CutPrefix(host, "http://"); ok {
		host, scheme = h, "http"
	}
	base := scheme + "://" + host + "/api/v4"
	token := os.Getenv("GITLAB_TOKEN")
	var out []forgeRepo
	for page := 1; page <= 50; page++ {
		u := fmt.Sprintf("%s/groups/%s/projects?per_page=100&page=%d&include_subgroups=true&archived=false&simple=true",
			base, url.PathEscape(group), page)
		req, _ := http.NewRequest("GET", u, nil)
		if token != "" {
			req.Header.Set("PRIVATE-TOKEN", token)
		}
		var pageOut []struct {
			PathWithNamespace string `json:"path_with_namespace"`
			HTTPURLToRepo     string `json:"http_url_to_repo"`
		}
		if err := httpJSON(req, &pageOut); err != nil {
			if token == "" && strings.Contains(err.Error(), "401") {
				return nil, fmt.Errorf("listing %s/%s: %w (set GITLAB_TOKEN for private groups)", host, group, err)
			}
			return nil, fmt.Errorf("listing %s/%s: %w", host, group, err)
		}
		if len(pageOut) == 0 {
			break
		}
		for _, r := range pageOut {
			out = append(out, forgeRepo{Host: host, Slug: r.PathWithNamespace})
		}
		if len(pageOut) < 100 {
			break
		}
	}
	return out, nil
}

func listBitbucket(ws string) ([]forgeRepo, error) {
	var out []forgeRepo
	next := fmt.Sprintf("https://api.bitbucket.org/2.0/workspaces/%s/repositories?pagelen=100", url.PathEscape(ws))
	for next != "" {
		req, _ := http.NewRequest("GET", next, nil)
		if t := os.Getenv("BITBUCKET_TOKEN"); t != "" { // "user:app_password"
			req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(t)))
		}
		var pageOut struct {
			Next   string `json:"next"`
			Values []struct {
				FullName string `json:"full_name"`
			} `json:"values"`
		}
		if err := httpJSON(req, &pageOut); err != nil {
			return nil, fmt.Errorf("listing bitbucket.org/%s: %w", ws, err)
		}
		for _, r := range pageOut.Values {
			out = append(out, forgeRepo{Host: "bitbucket.org", Slug: r.FullName})
		}
		next = pageOut.Next
	}
	return out, nil
}

// ---- scanning one forge repo ----

// ForgeSource scans a hosted repository (GitHub, GitLab, Bitbucket,
// self-hosted) identified by host and path: it shallow-clones the
// default branch into a temp directory, inventories it, and removes the
// clone before returning.
func ForgeSource(host, path string) (Source, error) {
	if host == "" || path == "" || strings.Contains(path, "..") {
		return nil, fmt.Errorf("forge target wants host/owner/repo, got %q", host+"/"+path)
	}
	return forgeSource{host: host, path: path}, nil
}

type forgeSource struct{ host, path string }

func (f forgeSource) Name() string { return f.host + "/" + f.path }
func (f forgeSource) Kind() string { return "repo" }

func (f forgeSource) Inventory() (Inventory, error) {
	inv, err := scanCloned(f.cloneURL(), f.Name())
	if err != nil {
		// private repos over https need credentials; try ssh as a fallback
		if inv2, sshErr := scanCloned(sshURL(f.host, f.path), f.Name()); sshErr == nil {
			return inv2, nil
		}
		return Inventory{}, err
	}
	return inv, nil
}

func (f forgeSource) cloneURL() string {
	return "https://" + f.host + "/" + f.path + ".git"
}

func sshURL(host, path string) string {
	return "git@" + host + ":" + path + ".git"
}

// scanCloned shallow-clones url into a temp dir and inventories it; the
// clone is removed before returning.
func scanCloned(cloneURL, name string) (Inventory, error) {
	tmp, err := os.MkdirTemp("", "zephy-clone-")
	if err != nil {
		return Inventory{}, err
	}
	defer os.RemoveAll(tmp)
	cmd := exec.Command("git", "clone", "--quiet", "--depth", "1", "--single-branch", "--no-tags", cloneURL, tmp)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_LFS_SKIP_SMUDGE=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		return Inventory{}, fmt.Errorf("clone %s: %s", name, strings.TrimSpace(string(out)))
	}
	return inventoryRepo(os.DirFS(tmp), trackedFiles(tmp)), nil
}
