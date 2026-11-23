package catalog

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed maps.json
var mapsRaw []byte

// Identifier translation tables, shipped as data: image base names, distro
// ids, SBOM package names, and ecosystem-qualified library names
// ("pypi:django") to endoflife.date slugs. Every slug must exist in the
// dataset; maps_test.go enforces it.
type mapsDoc struct {
	Images    map[string]string `json:"images"`
	Distros   map[string]string `json:"distros"`
	Packages  map[string]string `json:"packages"`
	Libraries map[string]string `json:"libraries"`
}

var (
	mapsOnce sync.Once
	maps     mapsDoc
)

func loadMaps() {
	mapsOnce.Do(func() {
		if err := json.Unmarshal(mapsRaw, &maps); err != nil {
			// corrupt maps file: scan proceeds with empty tables rather
			// than failing; every lookup misses and pins surface untracked
			maps = mapsDoc{}
		}
	})
}

// imageSlug resolves an image base name to a lifecycle slug.
func imageSlug(name string) (string, bool) {
	loadMaps()
	s, ok := maps.Images[name]
	return s, ok
}

// distroSlug resolves a distro identifier (or pretty name) to a slug.
func distroSlug(id string) (string, bool) {
	loadMaps()
	s, ok := maps.Distros[strings.ToLower(strings.TrimSpace(id))]
	return s, ok
}

// packageSlug resolves an SBOM package name to a slug.
func packageSlug(name string) (string, bool) {
	loadMaps()
	s, ok := maps.Packages[strings.ToLower(strings.TrimSpace(name))]
	return s, ok
}

// librarySlug resolves an ecosystem package ("pypi", "django") that is
// itself a tracked product.
func librarySlug(eco, name string) (string, bool) {
	loadMaps()
	if s, ok := maps.Libraries[eco+":"+name]; ok {
		return s, true
	}
	if eco == "conda" { // conda names follow PyPI's for Python packages
		return librarySlug("pypi", name)
	}
	if eco == "maven" { // a whole group: org.springframework.boot:*
		if group, artifact, ok := strings.Cut(name, ":"); ok {
			for _, prefix := range ownVersioning[group] {
				if strings.HasPrefix(artifact, prefix) {
					return "", false
				}
			}
			s, ok := maps.Libraries["maven:"+group+":*"]
			return s, ok
		}
	}
	return "", false
}

// ownVersioning: artifacts inside a mapped group that version
// independently of the product (spring-security-jwt 1.x, ActiveMQ Artemis).
var ownVersioning = map[string][]string{
	"org.springframework.security": {"spring-security-jwt", "spring-security-oauth", "spring-security-rsa"},
	"org.springframework":          {"springloaded"},
	"org.apache.activemq":          {"artemis", "activemq-protobuf"},
	"org.apache.hbase":             {"hbase-thirdparty"},
	"org.apache.tomcat":            {"jakartaee-migration"},
	"org.jetbrains.kotlin":         {"kotlin-native-prebuilt"},
	"org.hibernate":                {"hibernate-validator", "hibernate-search", "hibernate-ogm", "hibernate-jpa", "hibernate-commons-annotations", "hibernate-tools"},
}
