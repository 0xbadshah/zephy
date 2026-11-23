package policy

import (
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

// File is the .zephy.yml contract. Every layer is optional.
type File struct {
	// Suppress drops findings before output or gating, e.g. when no newer
	// version exists to upgrade to.
	Suppress []struct {
		Product string `yaml:"product"` // product name, optional
		Version string `yaml:"version"` // exact pin, optional
		Release string `yaml:"release"` // release line (RELEASE column), optional
		Path    string `yaml:"path"`    // file path prefix/glob, optional
		Reason  string `yaml:"reason"`  // why the rule exists, for reviewers
	} `yaml:"suppress"`

	// Extends overrides a dataset EOL date for products with purchased
	// extended support (Red Hat ELTS-style agreements). The override
	// is treated as the authoritative date for gating and reports.
	Extends []struct {
		Product string `yaml:"product"` // slug, required
		Release string `yaml:"release"` // release line, required
		Until   string `yaml:"until"`   // YYYY-MM-DD, required
	} `yaml:"extends"`

	// Normalize applies custom version cleanup patterns before matching,
	// each with a single capture group: "3.4.3p32" -> "3.4.3" via
	// ^(\d+\.\d+\.\d+)p\d+. Runs before the built-in normalization.
	Normalize []string `yaml:"normalize"`
}

func Load(path string) (*File, error) {
	if path == "" {
		return &File{}, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f File
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// Override returns the extended-support date for a product release, if any.
func (f *File) Override(product, release string) (time.Time, bool) {
	for _, e := range f.Extends {
		if e.Product == product && e.Release == release {
			if t, err := time.Parse("2006-01-02", e.Until); err == nil {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

// Normalize applies the policy's custom version patterns, first match wins.
func (f *File) NormalizeVersion(version string) string {
	for _, pat := range f.Normalize {
		if re, err := regexp.Compile(pat); err == nil {
			if m := re.FindStringSubmatch(version); m != nil && len(m) > 1 {
				return m[1]
			}
		}
	}
	return version
}

// Suppressed reports whether a finding matches a suppression rule.
// Empty fields in a rule act as wildcards: a path-only rule suppresses
// anything under that path.
func (f *File) Suppressed(product, version, release, file string) (bool, string) {
	for _, s := range f.Suppress {
		if s.Product != "" && s.Product != product {
			continue
		}
		if s.Version != "" && s.Version != version {
			continue
		}
		if s.Release != "" && s.Release != release {
			continue
		}
		if s.Path != "" && !pathMatches(s.Path, file) {
			continue
		}
		return true, s.Reason
	}
	return false, ""
}

// pathMatches: plain prefix, or a trailing ** glob (a bare "**" matches
// everything). Deliberately simple.
func pathMatches(pattern, path string) bool {
	if len(pattern) >= 2 && pattern[len(pattern)-2:] == "**" {
		prefix := pattern[:len(pattern)-2]
		return len(path) >= len(prefix) && path[:len(prefix)] == prefix
	}
	return len(path) >= len(pattern) && path[:len(pattern)] == pattern
}
