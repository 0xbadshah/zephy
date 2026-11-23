package catalog

import (
	"os"
	"sort"
	"testing"
)

// The golden fixtures: every manifest shape the walker supports, frozen.
// This test is the CI tripwire - any parser change that alters what the
// scanner sees must update this expectation deliberately, never silently.
func TestGoldenWalk(t *testing.T) {
	inv := inventoryRepo(os.DirFS("testdata/mini-repo"), nil)

	type pinKey struct{ product, version, file string }
	got := map[pinKey]bool{}
	for _, p := range inv.Pins {
		got[pinKey{p.Product, p.Version, p.File}] = true
	}

	want := []pinKey{
		{"apache-airflow", "3.1.6", "requirements.txt:1"},
		{"django", "4.2.11", "requirements.txt:2"},
		{"go", "1.23", "go.mod:3"},
		{"gradle", "9.3.1", "gradle/wrapper/gradle-wrapper.properties:1"},
		{"kotlin", "2.3.21", "gradle/libs.versions.toml:2"},
		{"nodejs", "20", ".nvmrc"},
		{"nodejs", "20", "Dockerfile:5"},
		{"postgresql", "13", "helm/values.yaml:2"}, // split repository/tag pair
		{"python", "3.13", "pyproject.toml:2"},
		{"python", "3.9.0", ".python-version:1"},
		{"python", "3.9.0", "runtime.txt"},
		{"rabbitmq", "3", ".github/workflows/ci.yml:5"}, // workflow service container
		{"rails", "5.2.3", "Gemfile.lock:4"},
		{"redis", "4.0.14", "docker-compose.yml:3"},
		{"ruby", "2.4.4", ".ruby-version"},
		{"ruby", "2.4.4", "Gemfile.lock:7"},
		{"ruby", "2.7.4", "Gemfile:2"},
		{"ruby", "3.1.3", ".tool-versions:2"},
		{"ruby", "3.4.10", "Dockerfile:3"}, // registry-mirrored FROM
		{"debian", "12", "Dockerfile:3"},   // ...whose slim-bookworm variant runs Debian 12
		{"spring-boot", "3.2.2", "pom.xml:5"},
		{"terraform", "1.5.7", ".tool-versions:1"},
		{"terraform", "1.5.7", "main.tf:2"},
	}

	for _, w := range want {
		if !got[w] {
			t.Errorf("expected pin missing: %s %s (%s)", w.product, w.version, w.file)
		}
		delete(got, w)
	}
	for k := range got {
		t.Errorf("unexpected extra pin: %s %s (%s)", k.product, k.version, k.file)
	}

	// untracked: provider lock, unmapped image, and the chart triple
	// (ingress-nginx is not lifecycle-tracked - it surfaces via curated)
	var gotUn []string
	for _, u := range inv.Untracked {
		gotUn = append(gotUn, u.Product)
	}
	sort.Strings(gotUn)
	if len(gotUn) != 3 || gotUn[0] != "aws" || gotUn[1] != "controller" || gotUn[2] != "some-app" {
		t.Errorf("untracked set changed: %v", gotUn)
	}

	// package identities come from the same parse
	type pkgKey struct{ eco, name, version string }
	have := map[pkgKey]bool{}
	for _, p := range inv.Packages {
		have[pkgKey{p.Ecosystem, p.Name, p.Version}] = true
	}
	for _, w := range []pkgKey{
		{"gem", "rails", "5.2.3"},
		{"pypi", "django", "4.2.11"},
		{"maven", "org.springframework.boot:spring-boot-starter-parent", "3.2.2"},
		{"image", "ingress-nginx/controller", "v1.8.1"},
		{"image", "postgres", "13"},
	} {
		if !have[w] {
			t.Errorf("expected package identity missing: %+v", w)
		}
	}
}
