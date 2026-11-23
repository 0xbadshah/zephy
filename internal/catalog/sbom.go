package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/0xbadshah/zephy/internal/ident"
)

// osPackageTypes are never reported individually: their lifecycle is the
// OS release, which the distro pin covers.
var osPackageTypes = map[string]bool{
	"apk": true, "deb": true, "rpm": true, "apk-archive": true,
	"alpm": true, "portage": true, "binary+apk": true,
}

type syftDoc struct {
	Artifacts []struct {
		Name      string `json:"name"`
		Version   string `json:"version"`
		Type      string `json:"type"`
		Purl      string `json:"purl"`
		Locations []struct {
			Path string `json:"path"`
		} `json:"locations"`
	} `json:"artifacts"`
	Distro struct {
		Name      string `json:"name"`
		Version   string `json:"version"`
		VersionID string `json:"versionID"`
		ID        string `json:"id"`
	} `json:"distro"`
}

// SPDX 2.x JSON: packages[].name / versionInfo / externalRefs[].purl
type spdxDoc struct {
	SPDXVersion string `json:"spdxVersion"`
	Packages    []struct {
		Name         string `json:"name"`
		VersionInfo  string `json:"versionInfo"`
		ExternalRefs []struct {
			ReferenceType    string `json:"referenceType"`
			ReferenceLocator string `json:"referenceLocator"`
		} `json:"externalRefs"`
	} `json:"packages"`
}

// CycloneDX JSON: components[].name / version / purl
type cdxDoc struct {
	BOMFormat  string `json:"bomFormat"`
	Components []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Type    string `json:"type"`
		Purl    string `json:"purl"`
	} `json:"components"`
}

// sbomPackage is one cataloged package, whatever the SBOM format.
type sbomPackage struct {
	name, version, purl string
	os, located         bool
}

// addPackage is the shared package mapping for every SBOM format:
// runtimes/frameworks by name (or purl name) become lifecycle pins;
// language packages become package identities for curated matching, and
// untracked candidates when nothing maps them.
func (b *builder) addPackage(p sbomPackage, label string) {
	if p.version == "" {
		return
	}
	purl, hasPurl := ident.ParsePurl(p.purl)
	isOS := p.os || (hasPurl && purl.IsOSPackage())
	if hasPurl && !isOS {
		b.inv.Packages = append(b.inv.Packages, PackagePin{
			Ecosystem: purl.Ecosystem(), Name: purl.PackageName(), Version: p.version, File: label})
	}
	if isOS {
		// distro packages (nginx, python3.11, openjdk-11-jre from apt) are
		// serviced by the distro for the release's lifetime: the OS pin
		// carries their lifecycle
		return
	}
	candidates := []string{p.name}
	if hasPurl && purl.Name != p.name {
		candidates = append(candidates, purl.Name)
	}
	if hasPurl && libraryEcosystems[purl.Type] {
		// a library's bare name means nothing on its own: the PostgreSQL
		// JDBC driver (maven postgresql) is not the database
		candidates = nil
	}
	for _, c := range candidates {
		if slug, ok := packageSlug(c); ok {
			b.inv.Pins = append(b.inv.Pins, Pin{Product: slug, Version: p.version, File: label, Raw: p.purl})
			return
		}
	}
	if hasPurl && !isOS {
		if slug, ok := librarySlug(purl.Ecosystem(), purl.PackageName()); ok {
			b.inv.Pins = append(b.inv.Pins, Pin{Product: slug, Version: p.version, File: label, Raw: p.purl})
			return
		}
	}
	// untracked, but only for things whose lifecycle could matter:
	// language packages and binaries, not every OS package
	if !isOS && (p.located || hasPurl) {
		b.untracked(p.purl, p.name, p.version, label, 0, "package has no lifecycle mapping")
	}
}

var libraryEcosystems = map[string]bool{
	"maven": true, "npm": true, "pypi": true, "gem": true, "golang": true, "nuget": true, "cargo": true,
	"composer": true, "hex": true, "pub": true, "cocoapods": true, "swift": true, "conda": true,
}

func (b *builder) addDistro(id, name, version, label string) {
	slug, ok := distroSlug(id)
	if !ok {
		slug, ok = distroSlug(name)
	}
	if ok && version != "" {
		b.inv.Pins = append(b.inv.Pins, Pin{Product: slug, Version: version, File: label, Raw: id + " " + version})
	}
}

func sniffSBOMFormat(b []byte) string {
	var probe struct {
		SPDXVersion string `json:"spdxVersion"`
		BOMFormat   string `json:"bomFormat"`
	}
	if json.Unmarshal(b, &probe) == nil {
		if probe.SPDXVersion != "" {
			return "spdx"
		}
		if probe.BOMFormat != "" {
			return "cyclonedx"
		}
	}
	return "syft" // syft-json: no discriminator key, fields differ
}

// inventorySBOM reads any supported SBOM: syft-json, SPDX 2.x JSON,
// CycloneDX JSON.
func inventorySBOM(raw []byte, label string) (Inventory, error) {
	var b builder
	switch sniffSBOMFormat(raw) {
	case "spdx":
		var doc spdxDoc
		if err := json.Unmarshal(raw, &doc); err != nil {
			return Inventory{}, fmt.Errorf("not valid spdx-json: %w", err)
		}
		for _, p := range doc.Packages {
			purl := ""
			for _, r := range p.ExternalRefs {
				if r.ReferenceType == "purl" {
					purl = r.ReferenceLocator
				}
			}
			b.addPackage(sbomPackage{name: p.Name, version: p.VersionInfo, purl: purl, located: purl != ""}, label)
		}
	case "cyclonedx":
		var doc cdxDoc
		if err := json.Unmarshal(raw, &doc); err != nil {
			return Inventory{}, fmt.Errorf("not valid cyclonedx-json: %w", err)
		}
		for _, c := range doc.Components {
			if c.Type == "operating-system" {
				b.addDistro(c.Name, c.Name, c.Version, label)
				continue
			}
			b.addPackage(sbomPackage{name: c.Name, version: c.Version, purl: c.Purl, located: c.Purl != ""}, label)
		}
	default:
		var doc syftDoc
		if err := json.Unmarshal(raw, &doc); err != nil {
			return Inventory{}, fmt.Errorf("not a recognized sbom (syft-json, spdx-json, cyclonedx-json): %w", err)
		}
		version := doc.Distro.VersionID // "12"; Version is "12 (bookworm)"
		if version == "" {
			version = strings.SplitN(doc.Distro.Version, " ", 2)[0]
		}
		b.addDistro(doc.Distro.ID, doc.Distro.Name, version, label)
		for _, a := range doc.Artifacts {
			b.addPackage(sbomPackage{name: a.Name, version: a.Version, purl: a.Purl,
				os: osPackageTypes[strings.ToLower(a.Type)], located: len(a.Locations) > 0}, label)
		}
	}
	return b.inv, nil
}

// inventoryImage catalogs a container image via the syft CLI (kept
// decoupled on purpose: no version coupling with the cataloger's library
// releases). The reference itself is evaluated too: the tag the user asked
// for is the most honest pin of all (syft may not catalog the runtime -
// rabbitmq ships inside an erlang release and never appears in the SBOM).
func inventoryImage(ref string) (Inventory, error) {
	syft, err := exec.LookPath("syft")
	if err != nil {
		return Inventory{}, fmt.Errorf("image mode requires the syft CLI on PATH (https://github.com/anchore/syft)")
	}
	tmp, err := os.MkdirTemp("", "zephy-")
	if err != nil {
		return Inventory{}, err
	}
	defer os.RemoveAll(tmp)
	out := filepath.Join(tmp, "sbom.json")
	cmd := exec.Command(syft, ref, "-o", "syft-json="+out)
	if combined, err := cmd.CombinedOutput(); err != nil {
		return Inventory{}, fmt.Errorf("syft %s: %s", ref, lastLines(string(combined), 5))
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		return Inventory{}, err
	}
	label := "image:" + ref
	inv, err := inventorySBOM(raw, label)
	if err != nil {
		return inv, err
	}
	var b builder
	if r, ok := ident.ParseImageRef(ref); ok {
		b.image(r, label, 0, "")
	}
	inv.Pins = append(b.inv.Pins, inv.Pins...)
	inv.Packages = append(b.inv.Packages, inv.Packages...)
	inv.Untracked = append(b.inv.Untracked, inv.Untracked...)
	return inv, nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
