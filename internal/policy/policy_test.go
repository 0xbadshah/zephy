package policy

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func load(t *testing.T, yml string) *File {
	dir := t.TempDir()
	p := filepath.Join(dir, "p.yml")
	if err := os.WriteFile(p, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// the no-upgrade-available suppression case
func TestSuppression(t *testing.T) {
	f := load(t, `
suppress:
  - product: nodejs
    version: "20.18.0"
    path: "src/docket/**"
`)
	if ok, _ := f.Suppressed("nodejs", "20.18.0", "", "src/docket/e2e/recorded/node-app.dockerfile"); !ok {
		t.Fatal("matching suppression must apply")
	}
	if ok, _ := f.Suppressed("nodejs", "22.0.0", "", "src/docket/x"); ok {
		t.Fatal("different version must not be suppressed")
	}
	if ok, _ := f.Suppressed("nodejs", "20.18.0", "", "other/path/dockerfile"); ok {
		t.Fatal("path must be honored")
	}
	if ok, _ := f.Suppressed("rails", "8.0.2", "", "src/docket/x"); ok {
		t.Fatal("different product must not be suppressed")
	}
}

// purchased extended support must not be flagged.
func TestExtendsOverride(t *testing.T) {
	f := load(t, `
extends:
  - product: postgresql
    release: "13"
    until: "2028-11-13"
`)
	until, ok := f.Override("postgresql", "13")
	if !ok || !until.Equal(time.Date(2028, 11, 13, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("override not applied: %v %v", until, ok)
	}
	if _, ok := f.Override("postgresql", "14"); ok {
		t.Fatal("override must be release-scoped")
	}
}

// Path-only rules suppress anything under the path (wildcard semantics).
func TestPathOnlySuppression(t *testing.T) {
	f := load(t, `
suppress:
  - path: "testdata/**"
    reason: "fixtures"
`)
	if ok, _ := f.Suppressed("anything", "1.0", "", "testdata/mini-repo/Dockerfile:5"); !ok {
		t.Fatal("path-only rule must suppress any product under the path")
	}
	if ok, _ := f.Suppressed("anything", "1.0", "", "src/Dockerfile"); ok {
		t.Fatal("path-only rule must not match outside the path")
	}
}
