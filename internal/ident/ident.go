// Package ident turns the identifiers sources see - container image
// references and package URLs - into structured identities, once. Every
// source and every matcher reads these structures; nothing downstream
// re-parses a ref or a purl out of a string.
package ident

import (
	"net/url"
	"strings"
)

// ImageRef is a parsed container image reference:
// [registry/]path[:tag][@digest].
type ImageRef struct {
	Registry string // "" when implicit (docker hub)
	Path     string // repository path without the registry: "library/postgres", "bitnami/redis"
	Tag      string // "" when absent
	Digest   string // "sha256:..." when pinned by digest
}

// ParseImageRef parses a reference as written in a Dockerfile, compose
// file, helm values or on the command line. ok=false for anything that is
// not a literal reference: build-arg/env interpolation, whitespace, empty.
func ParseImageRef(s string) (ImageRef, bool) {
	s = strings.TrimSpace(strings.Trim(strings.TrimSpace(s), `"'`))
	if s == "" || strings.ContainsAny(s, "$ \t{}") || strings.Contains(s, "://") {
		return ImageRef{}, false
	}
	var r ImageRef
	if i := strings.Index(s, "@"); i >= 0 {
		r.Digest = s[i+1:]
		s = s[:i]
	}
	// a tag colon only ever follows the last slash; a colon before it
	// belongs to a registry port (host:5000/team/app)
	if i := strings.LastIndex(s, ":"); i >= 0 && !strings.Contains(s[i:], "/") {
		r.Tag = s[i+1:]
		s = s[:i]
	}
	parts := strings.Split(s, "/")
	if len(parts) > 1 && isRegistryHost(parts[0]) {
		r.Registry = parts[0]
		parts = parts[1:]
	}
	r.Path = strings.Join(parts, "/")
	if r.Path == "" || strings.Contains(r.Path, "//") {
		return ImageRef{}, false
	}
	return r, true
}

func isRegistryHost(s string) bool {
	return strings.ContainsAny(s, ".:") || s == "localhost"
}

// Base is the image's own name: the last path segment, with the
// "library-" prefix some mirrors flatten docker hub's library/ into.
// registry.example.com/oss/docker.io/library/ruby -> ruby
func (r ImageRef) Base() string {
	name := r.Path[strings.LastIndex(r.Path, "/")+1:]
	return strings.TrimPrefix(strings.ToLower(name), "library-")
}

// CuratedPath is the path used to match curated image entries: registry
// hosts (including ones nested by mirrors), "library" and mirror "oss"
// segments dropped. registry.example.com/oss/ingress-nginx/controller ->
// ingress-nginx/controller.
func (r ImageRef) CuratedPath() string {
	var out []string
	for _, p := range strings.Split(strings.ToLower(r.Path), "/") {
		if p == "oss" || p == "library" || isRegistryHost(p) {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, "/")
}

// String renders the reference back in canonical form.
func (r ImageRef) String() string {
	s := r.Path
	if r.Registry != "" {
		s = r.Registry + "/" + s
	}
	if r.Tag != "" {
		s += ":" + r.Tag
	}
	if r.Digest != "" {
		s += "@" + r.Digest
	}
	return s
}

// Purl is a parsed package URL: pkg:type/namespace/name@version?qualifiers#subpath.
type Purl struct {
	Type      string
	Namespace string
	Name      string
	Version   string
}

// ParsePurl parses a package URL. Components are percent-decoded
// (pkg:npm/%40babel/core -> namespace "@babel").
func ParsePurl(s string) (Purl, bool) {
	if !strings.HasPrefix(s, "pkg:") {
		return Purl{}, false
	}
	rest := strings.TrimLeft(strings.TrimPrefix(s, "pkg:"), "/")
	if i := strings.Index(rest, "#"); i >= 0 {
		rest = rest[:i]
	}
	if i := strings.Index(rest, "?"); i >= 0 {
		rest = rest[:i]
	}
	var p Purl
	if i := strings.LastIndex(rest, "@"); i >= 0 && !strings.Contains(rest[i:], "/") {
		p.Version = unescape(rest[i+1:])
		rest = rest[:i]
	}
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) < 2 {
		return Purl{}, false
	}
	p.Type = strings.ToLower(parts[0])
	p.Name = unescape(parts[len(parts)-1])
	if len(parts) > 2 {
		ns := make([]string, 0, len(parts)-2)
		for _, n := range parts[1 : len(parts)-1] {
			ns = append(ns, unescape(n))
		}
		p.Namespace = strings.Join(ns, "/")
	}
	if p.Name == "" {
		return Purl{}, false
	}
	return p, true
}

func unescape(s string) string {
	if u, err := url.PathUnescape(s); err == nil {
		return u
	}
	return s
}

var osPurlTypes = map[string]bool{"apk": true, "deb": true, "rpm": true, "alpm": true, "ebuild": true}

// IsOSPackage reports a distro package: its lifecycle is the OS release.
func (p Purl) IsOSPackage() bool { return osPurlTypes[p.Type] }

// Ecosystem is the purl type, the ecosystem name used across the scanner
// (gem, pypi, npm, golang, maven, cargo, composer, nuget, ...).
func (p Purl) Ecosystem() string { return p.Type }

// PackageName is the package's name as its own ecosystem spells it:
// npm "@scope/name", maven "group:artifact", golang and composer
// "namespace/name"; lowercased where the ecosystem is case-insensitive.
func (p Purl) PackageName() string {
	switch p.Type {
	case "npm":
		if p.Namespace != "" {
			return strings.ToLower(p.Namespace + "/" + p.Name)
		}
		return strings.ToLower(p.Name)
	case "maven":
		if p.Namespace != "" {
			return p.Namespace + ":" + p.Name
		}
		return p.Name
	case "golang", "composer", "github":
		if p.Namespace != "" {
			return strings.ToLower(p.Namespace + "/" + p.Name)
		}
		return strings.ToLower(p.Name)
	case "pypi":
		return NormalizePyPI(p.Name)
	default:
		return strings.ToLower(p.Name)
	}
}

// NormalizePyPI applies PEP 503 name normalization: case-insensitive,
// runs of "-", "_" and "." are equivalent.
func NormalizePyPI(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	prevSep := false
	for _, r := range name {
		if r == '-' || r == '_' || r == '.' {
			if !prevSep {
				b.WriteByte('-')
			}
			prevSep = true
			continue
		}
		prevSep = false
		b.WriteRune(r)
	}
	return b.String()
}
