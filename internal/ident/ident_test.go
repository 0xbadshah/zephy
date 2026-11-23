package ident

import "testing"

func TestParseImageRef(t *testing.T) {
	cases := []struct {
		in                          string
		registry, path, tag, digest string
		base, curated               string
	}{
		{"postgres:14", "", "postgres", "14", "", "postgres", "postgres"},
		{"library/postgres:14-alpine", "", "library/postgres", "14-alpine", "", "postgres", "postgres"},
		{"myregistry:5000/team/app", "myregistry:5000", "team/app", "", "", "app", "team/app"},
		{"myregistry:5000/team/app:1.2", "myregistry:5000", "team/app", "1.2", "", "app", "team/app"},
		{"registry.example.com/oss/docker.io/library/ruby:3.4.10-slim", "registry.example.com", "oss/docker.io/library/ruby", "3.4.10-slim", "", "ruby", "ruby"},
		{"registry.example.com/oss/ingress-nginx/controller", "registry.example.com", "oss/ingress-nginx/controller", "", "", "controller", "ingress-nginx/controller"},
		{"nginx:1.25@sha256:abc", "", "nginx", "1.25", "sha256:abc", "nginx", "nginx"},
		{"localhost/app:dev", "localhost", "app", "dev", "", "app", "app"},
		{"ecr.aws/mirror/library-ruby:3.1", "ecr.aws", "mirror/library-ruby", "3.1", "", "ruby", "mirror/library-ruby"},
	}
	for _, c := range cases {
		r, ok := ParseImageRef(c.in)
		if !ok {
			t.Errorf("ParseImageRef(%q) failed", c.in)
			continue
		}
		if r.Registry != c.registry || r.Path != c.path || r.Tag != c.tag || r.Digest != c.digest {
			t.Errorf("ParseImageRef(%q) = %+v", c.in, r)
		}
		if r.Base() != c.base {
			t.Errorf("Base(%q) = %q, want %q", c.in, r.Base(), c.base)
		}
		if r.CuratedPath() != c.curated {
			t.Errorf("CuratedPath(%q) = %q, want %q", c.in, r.CuratedPath(), c.curated)
		}
	}
	for _, bad := range []string{"", "${BASE_IMAGE}", "ruby:${RUBY_VERSION}-slim", "$IMG", "a b"} {
		if _, ok := ParseImageRef(bad); ok {
			t.Errorf("ParseImageRef(%q) must fail", bad)
		}
	}
}

func TestParsePurl(t *testing.T) {
	cases := []struct {
		in, eco, name, version string
		os                     bool
	}{
		{"pkg:gem/rails@8.0.2", "gem", "rails", "8.0.2", false},
		{"pkg:pypi/Django@4.2", "pypi", "django", "4.2", false},
		{"pkg:pypi/zope.interface@5.0", "pypi", "zope-interface", "5.0", false},
		{"pkg:npm/request@2.88.2", "npm", "request", "2.88.2", false},
		{"pkg:npm/%40angular/core@15.2.0", "npm", "@angular/core", "15.2.0", false},
		{"pkg:maven/log4j/log4j@1.2.17", "maven", "log4j:log4j", "1.2.17", false},
		{"pkg:golang/github.com/jmoiron/sqlx@v1.3.5", "golang", "github.com/jmoiron/sqlx", "v1.3.5", false},
		{"pkg:apk/alpine/musl@1.2.5-r0?arch=x86_64&distro=alpine-3.20", "apk", "musl", "1.2.5-r0", true},
		{"pkg:deb/debian/openssl@3.0.11-1?arch=amd64", "deb", "openssl", "3.0.11-1", true},
	}
	for _, c := range cases {
		p, ok := ParsePurl(c.in)
		if !ok {
			t.Errorf("ParsePurl(%q) failed", c.in)
			continue
		}
		if p.Ecosystem() != c.eco || p.PackageName() != c.name || p.Version != c.version || p.IsOSPackage() != c.os {
			t.Errorf("ParsePurl(%q) = eco %q name %q version %q os %v", c.in, p.Ecosystem(), p.PackageName(), p.Version, p.IsOSPackage())
		}
	}
	for _, bad := range []string{"not-a-purl", "pkg:", "pkg:npm"} {
		if _, ok := ParsePurl(bad); ok {
			t.Errorf("ParsePurl(%q) must fail", bad)
		}
	}
}
