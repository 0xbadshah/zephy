package curated

import (
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/Masterminds/semver/v3"
)

//go:embed known-dead.json
var knownDeadRaw []byte

// DeadEntry is one formally deprecated or abandoned package. Entries are
// deliberately conservative: each needs a public upstream statement
// (deprecation notice, rename, archive or EOL announcement) to belong.
type DeadEntry struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
	Until     string `json:"until,omitempty"` // range applies only below this version
	Status    string `json:"status"`          // deprecated | abandoned
	Note      string `json:"note"`
	// Evidence is mandatory: a public upstream statement. Folklore entries
	// ("everyone knows it is dead") flag live projects that merely have
	// old pins. Entries without evidence are dropped at load.
	Evidence string `json:"evidence"`
}

var entries []DeadEntry

func load() {
	if entries != nil {
		return
	}
	var raw []DeadEntry
	if err := json.Unmarshal(knownDeadRaw, &raw); err != nil {
		return // curated list corrupt: say nothing, never break scans
	}
	for _, e := range raw {
		if e.Evidence != "" && e.Note != "" {
			entries = append(entries, e)
		}
	}
}

// GitHubRepo extracts owner/repo from the entry's evidence URL when it
// points at github: the evidence IS the repo to cross-check activity
// against. Returns ok=false when the evidence is not a github repository
// (pypi pages, apache sites, blogs - those need no activity check).
func (e *DeadEntry) GitHubRepo() (string, bool) {
	i := strings.Index(e.Evidence, "github.com/")
	if i < 0 {
		return "", false
	}
	rest := e.Evidence[i+len("github.com/"):]
	rest = strings.Fields(rest)[0] // strip trailing prose like "(archived)"
	rest = strings.TrimSuffix(strings.Trim(rest, "/"), ".git")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	return strings.ToLower(parts[0]) + "/" + strings.ToLower(parts[1]), true
}

// Match reports the dead-entry for an ecosystem/name/version, or nil.
func Match(ecosystem, name, version string) *DeadEntry {
	load()
	name = strings.ToLower(strings.TrimSpace(name))
	v, err := semver.NewVersion(strings.TrimPrefix(version, "v"))

	for i := range entries {
		e := &entries[i]
		// image refs are matched by path suffix so registry mirrors
		// (registry.example.com/oss/ingress-nginx/controller) still hit
		if e.Ecosystem == "image" {
			if err != nil || !strings.HasSuffix(name, e.Name) {
				continue
			}
		} else if e.Ecosystem != ecosystem || e.Name != name {
			continue
		}
		if e.Until == "" {
			return e
		}
		// range-limited entry (e.g. log4j < 2.0.0, or a support-policy
		// window): semver when possible
		if v != nil {
			if lim, err2 := semver.NewVersion(e.Until); err2 == nil {
				if v.Compare(lim) < 0 {
					return e
				}
				continue
			}
		}
		return e // unparseable version: flag rather than guess-clear
	}
	return nil
}
