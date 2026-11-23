package catalog

import "testing"

// SBOMs of every supported format feed the same mapping: the distro
// release, runtimes by name, language packages as identities.
func TestInventorySBOMFormats(t *testing.T) {
	cases := map[string]string{
		"syft": `{"distro": {"id": "debian", "versionID": "11", "version": "11 (bullseye)"},
		  "artifacts": [
		    {"name": "python", "version": "3.7.12", "type": "binary", "purl": "pkg:generic/python@3.7.12"},
		    {"name": "request", "version": "2.88.2", "type": "npm", "purl": "pkg:npm/request@2.88.2"},
		    {"name": "openssl", "version": "1.1.1n-0+deb11u3", "type": "deb", "purl": "pkg:deb/debian/openssl@1.1.1n-0+deb11u3"}]}`,
		"cyclonedx": `{"bomFormat": "CycloneDX", "components": [
		    {"type": "operating-system", "name": "debian", "version": "11"},
		    {"type": "application", "name": "python", "version": "3.7.12"},
		    {"type": "library", "name": "request", "version": "2.88.2", "purl": "pkg:npm/request@2.88.2"}]}`,
		"spdx": `{"spdxVersion": "SPDX-2.3", "packages": [
		    {"name": "python", "versionInfo": "3.7.12"},
		    {"name": "request", "versionInfo": "2.88.2", "externalRefs": [{"referenceType": "purl", "referenceLocator": "pkg:npm/request@2.88.2"}]}]}`,
	}
	for format, doc := range cases {
		inv, err := inventorySBOM([]byte(doc), "sbom:test")
		if err != nil {
			t.Fatalf("%s: %v", format, err)
		}
		got := pinSet(inv)
		if !got[pinKey{"python", "3.7.12"}] {
			t.Errorf("%s: python runtime pin missing: %v", format, got)
		}
		if format != "spdx" && !got[pinKey{"debian", "11"}] {
			t.Errorf("%s: distro pin missing: %v", format, got)
		}
		found := false
		for _, p := range inv.Packages {
			if p.Ecosystem == "npm" && p.Name == "request" {
				found = true
			}
			if p.Ecosystem == "deb" {
				t.Errorf("%s: OS packages are not package identities: %+v", format, p)
			}
		}
		if !found {
			t.Errorf("%s: npm package identity missing: %+v", format, inv.Packages)
		}
	}
}
