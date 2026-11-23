package catalog

import (
	"strconv"
)

// Inventory is everything a source saw, before verification.
type Inventory struct {
	Pins      []Pin        // lifecycle pins: product slug + version
	Packages  []PackagePin // package identities, matched against the curated list
	Untracked []Untracked  // seen, but no lifecycle mapping exists
}

// Pin is a lifecycle pin discovered in a source, pre-verification.
type Pin struct {
	Product string // endoflife.date slug
	Version string // as pinned
	File    string // path relative to the source root (with :line where known)
	Raw     string // the directive / image ref as written

	section string // helm values: the top-level component this pin sits in
}

// PackagePin is a package identity: a language dependency, or a container
// image (ecosystem "image", name = ident.ImageRef.CuratedPath).
type PackagePin struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	File      string `json:"file"`
}

// Untracked is a pin we saw but cannot verify. First-class output: the
// silent-skip failure class, never dropped.
type Untracked struct {
	Raw     string `json:"raw"`     // purl / image ref / directive as written
	Product string `json:"product"` // best-effort name (not a verified slug)
	Version string `json:"version"`
	File    string `json:"file"`
	Reason  string `json:"reason,omitempty"`
}

// builder accumulates an Inventory. Every source emits through it, so the
// rules that turn an identity into a pin (image mapping, package mapping)
// live in one place.
type builder struct {
	inv Inventory
}

func loc(file string, line int) string {
	if line > 0 {
		return file + ":" + strconv.Itoa(line)
	}
	return file
}

// pin records a lifecycle pin whose product is already known.
func (b *builder) pin(product, version, file string, line int, raw string) {
	b.inv.Pins = append(b.inv.Pins, Pin{Product: product, Version: version, File: loc(file, line), Raw: raw})
}

// pkg records a language package. A package that is itself a tracked
// product (django, rails, spring-boot parent...) also becomes a lifecycle
// pin: one parse feeds both outputs.
func (b *builder) pkg(eco, name, version, file string, line int) {
	if name == "" || version == "" {
		return
	}
	b.inv.Packages = append(b.inv.Packages, PackagePin{Ecosystem: eco, Name: name, Version: version, File: loc(file, line)})
	if slug, ok := librarySlug(eco, name); ok {
		b.pin(slug, version, file, line, eco+":"+name+"@"+version)
	}
}

func (b *builder) untracked(raw, product, version, file string, line int, reason string) {
	if len(product) > 40 {
		product = product[:40]
	}
	b.inv.Untracked = append(b.inv.Untracked, Untracked{
		Raw: raw, Product: product, Version: version, File: loc(file, line), Reason: reason})
}
