package catalog

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Source is the seam every scan target satisfies: one interface, three real
// adapters (repository checkouts, container images, SBOM files). The scan
// pipeline is written once against this interface.
type Source interface {
	// Name labels findings from this source (repo basename, image ref).
	Name() string
	// Kind reports the source type for finding provenance.
	Kind() string
	// Inventory produces everything the pipeline consumes: lifecycle pins,
	// package identities for curated matching, and untracked candidates.
	Inventory() (Inventory, error)
}

// RepoSource scans a repository checkout. Monorepo-aware.
func RepoSource(path string) (Source, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("not a directory")
	}
	return repoSource{dir: path}, nil
}

type repoSource struct{ dir string }

// Name is the repo's folder name: "zephy ." reports the folder, not ".".
func (r repoSource) Name() string {
	dir := r.dir
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return filepath.Base(dir)
}
func (r repoSource) Kind() string { return "repo" }
func (r repoSource) Inventory() (Inventory, error) {
	return inventoryRepo(os.DirFS(r.dir), trackedFiles(r.dir)), nil
}

// ImageSource scans a container image via the syft CLI. Fails fast when
// the companion tool is missing.
func ImageSource(ref string) (Source, error) {
	if _, err := exec.LookPath("syft"); err != nil {
		return nil, fmt.Errorf("image mode requires the syft CLI on PATH (https://github.com/anchore/syft)")
	}
	return imageSource{ref: ref}, nil
}

type imageSource struct{ ref string }

func (i imageSource) Name() string                  { return i.ref }
func (i imageSource) Kind() string                  { return "image" }
func (i imageSource) Inventory() (Inventory, error) { return inventoryImage(i.ref) }

// SBOMSource scans a syft-json, SPDX or CycloneDX SBOM file.
func SBOMSource(path string) (Source, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("is a directory, not an SBOM file")
	}
	return sbomSource{path: path}, nil
}

type sbomSource struct{ path string }

func (s sbomSource) Name() string { return s.path }
func (s sbomSource) Kind() string { return "sbom" }
func (s sbomSource) Inventory() (Inventory, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return Inventory{}, err
	}
	return inventorySBOM(raw, "sbom:"+filepath.Base(s.path))
}

// GitHubSource scans a GitHub repository without a prior checkout: it
// shallow-clones the default branch into a temp directory, inventories it,
// and removes the clone before returning. End users point CI at
// `zephy github:owner/repo` instead of writing their own clone step.
// Kept alongside the original schemes as an explicit escape hatch; the
// auto-detected path is `zephy owner/repo`.
func GitHubSource(slug string) (Source, error) {
	owner, repo, ok := strings.Cut(slug, "/")
	if !ok || owner == "" || repo == "" {
		return nil, fmt.Errorf("github target wants owner/repo, got %q", slug)
	}
	return githubSource{owner: owner, repo: repo}, nil
}

type githubSource struct{ owner, repo string }

func (g githubSource) Name() string { return g.owner + "/" + g.repo }
func (g githubSource) Kind() string { return "repo" }

func (g githubSource) Inventory() (Inventory, error) {
	inv, err := scanCloned("https://github.com/"+g.Name()+".git", g.Name())
	if err != nil {
		if inv2, sshErr := scanCloned("git@github.com:"+g.Name()+".git", g.Name()); sshErr == nil {
			return inv2, nil
		}
		return Inventory{}, err
	}
	return inv, nil
}
