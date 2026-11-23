package catalog

import (
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf16"
)

// skipDirs are dependency and tool directories: their contents are not
// the repository's own pins.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "bower_components": true,
	".venv": true, "venv": true, "tmp": true,
	"third_party": true, "thirdparty": true, "third-party": true, // vendored upstream projects
	"3rdparty": true, "3rd_party": true, "3rd-party": true, "3rdparties": true, "thirdparties": true,
	"dist":    true, // build output: generated manifests and effective configs
	".bundle": true,
}

// fixturePathParts are unambiguous test-data locations: skipped by
// default, deliberate findings only via the suppression file.
var fixturePathParts = []string{"/testdata/", "/fixtures/", "/test/fixtures/", "/e2e/", "/src/it/", "/test_files/"} // src/it: maven-invoker test projects

func isFixturePath(rel string) bool {
	r := "/" + rel + "/"
	for _, p := range fixturePathParts {
		if strings.Contains(r, p) {
			return true
		}
	}
	// __fixtures__, test-fixtures; __tests__ / __mocks__ data
	for _, seg := range strings.Split(strings.ToLower(rel), "/") {
		if strings.Contains(seg, "fixture") || seg == "__tests__" || seg == "__mocks__" || seg == "testfiles" {
			return true
		}
	}
	return false
}

// trackedFiles enumerates git-tracked files when root is a git repository:
// a dirty working tree holds files nobody ever committed. nil = not a
// repository, every file counts.
func trackedFiles(root string) map[string]bool {
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return nil
	}
	out, err := exec.Command("git", "-C", root, "ls-files").Output()
	if err != nil || len(out) == 0 {
		return nil
	}
	set := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if line != "" {
			set[line] = true
		}
	}
	return set
}

// repoFileSet lists the files that make up the repository: regular files
// only (a symlink duplicates its target), outside dependency directories,
// git-tracked when a tracked set is given, and outside fixture paths.
// This is the one place that decides what "the repo" is.
func repoFileSet(fsys fs.FS, tracked map[string]bool) []string {
	files := walkFiles(fsys, tracked)
	vendored := vendoredRoots(files)
	if tracked == nil {
		// no git index to tell a live submodule (a gitlink, contents untracked)
		// from files committed under a stale .gitmodules entry: a submodule
		// URL owned by someone else marks a copy, the project's own org does not
		vendored = append(vendored, foreignSubmodules(fsys)...)
	}
	out := files[:0]
	for _, f := range files {
		if !underAny(f, vendored) {
			out = append(out, f)
		}
	}
	return out
}

var (
	reSubmodule      = regexp.MustCompile(`(?m)^\s*path\s*=\s*(\S+)\s*\n\s*url\s*=\s*(\S+)`)
	reSubmoduleOwner = regexp.MustCompile(`(?:github\.com|gitlab\.com|bitbucket\.org)[:/]([\w.\-]+)/`)
)

// foreignSubmodules: .gitmodules paths that are someone else's code.
// Relative URLs and a single org's toolbox of its own repos are kept.
func foreignSubmodules(fsys fs.FS) []string {
	b, err := fs.ReadFile(fsys, ".gitmodules")
	if err != nil {
		return nil
	}
	type sub struct{ path, owner string }
	var subs []sub
	owners := map[string]int{}
	for _, m := range reSubmodule.FindAllStringSubmatch(string(b), -1) {
		o := ""
		if om := reSubmoduleOwner.FindStringSubmatch(m[2]); om != nil {
			o = strings.ToLower(om[1])
		} else if strings.HasPrefix(m[2], "../") || strings.HasPrefix(m[2], "./") {
			o = "." // relative URL: the same host and owner as the project
		}
		subs = append(subs, sub{strings.Trim(path.Clean(m[1]), "/"), o})
		owners[o]++
	}
	// an org that registers three or more of its repos, and most of the
	// entries, is the project's own (a toolbox of its guides); the rest are
	// dependencies
	own := ""
	for o, n := range owners {
		if o != "" && n >= 3 && 2*n > len(subs) {
			own = o
		}
	}
	var out []string
	for _, s := range subs {
		if s.path == "" || s.path == "." || s.owner == "." || (own != "" && s.owner == own) {
			continue
		}
		out = append(out, s.path+"/")
	}
	return out
}

var reCommittedGems = regexp.MustCompile(`/ruby/\d+\.\d+\.\d+/(?:gems|bundler/gems)/`)

// (lib-main/ is a branch archive download: the same thing)
var reReleaseDir = regexp.MustCompile(`[-_]v?\d+\.\d+(?:\.\d+)?(?:[-_.][\w.]+)?$|.-(?:main|master)$`)

// vendorSegments are directory names that hold copied-in upstream code.
var vendorSegments = map[string]bool{
	"include": true, "extern": true, "external": true, "externals": true, "3rdparty": true, "3rd_party": true,
	"3rd-party": true, "deps": true, "contrib": true, "submodules": true, "libs": true, "lib": true,
	"vendors": true,
}

// vendoredRoots finds copies of other projects inside the repository: a
// subdirectory carrying its own license, either below a vendor-ish
// directory (include/imgui, extern/fmt) or with its own CI config (a
// copied project with its own .github/). Their manifests describe somebody else's
// build. A monorepo package with a LICENSE and no such signal is kept.
func vendoredRoots(files []string) []string {
	licensed := map[string]bool{}
	ownCI := map[string]bool{}
	readme := map[string]bool{}
	// a .gitmodules or a pre-commit config belongs at one repo root each;
	// either found in a subdirectory marks a copied-in project
	repoRootFile := map[string]bool{}
	for _, f := range files {
		dir, base := path.Dir(f), strings.ToLower(path.Base(f))
		if dir == "." {
			continue
		}
		if strings.HasPrefix(base, "license") || strings.HasPrefix(base, "copying") {
			licensed[dir] = true
		}
		if strings.HasPrefix(base, "readme") {
			readme[dir] = true
		}
		if base == ".gitmodules" || base == ".pre-commit-config.yaml" {
			repoRootFile[dir] = true
		}
		// its own CI config (repo_meta.yaml or CODEOWNERS alone are not)
		for _, ci := range []string{"/.github/workflows/", "/.circleci/"} {
			if i := strings.Index(f, ci); i > 0 {
				ownCI[f[:i]] = true
			}
		}
		if base == ".travis.yml" || base == "appveyor.yml" || base == ".gitlab-ci.yml" || base == "azure-pipelines.yml" {
			ownCI[dir] = true
		}
	}
	// monorepo containers hold first-party packages, not copies; so do
	// the parent product's own demo/sample/web sub-apps (website-demo,
	// admin-sample)
	monorepoContainer := func(dir string) bool {
		for _, s := range strings.Split(strings.ToLower(dir), "/") {
			if s == "packages" || s == "apps" || s == "modules" || s == "crates" ||
				s == "projects" || s == "examples" || s == "services" || s == "website" ||
				s == "web" || s == "frontend" || s == "ui" ||
				strings.Contains(s, "demo") || strings.Contains(s, "sample") {
				return true
			}
		}
		return false
	}
	rootSet := map[string]bool{}
	// a copied repo root announces itself without any vendor-ish parent
	for dir := range repoRootFile {
		rootSet[dir+"/"] = true
	}
	var roots []string
	for dir := range licensed {
		// a LICENSE plus a README of its own: an upstream copy in a plain
		// folder, unless it sits in a monorepo container, where
		// first-party packages carry both
		if readme[dir] && !monorepoContainer(dir) {
			rootSet[dir+"/"] = true
			continue
		}
		// an extracted release (moby-17.05.0-ce/) or a copy with its own CI
		parent := path.Base(path.Dir(dir))
		// own CI alone is not enough: monorepo packages split into their
		// own repositories carry LICENSE and .travis.yml too
		vendorish := (ownCI[dir] && dirHasNoManifestSibling(dir, files)) || reReleaseDir.MatchString(path.Base(dir)) ||
			// WordPress themes and plugins shipped inside a site
			((parent == "themes" || parent == "plugins") && strings.HasSuffix(path.Dir(path.Dir(dir)), "wp-content")) ||
			parent == "themes" // hexo/hugo/jekyll themes copied into a site
		segs := strings.Split(strings.ToLower(dir), "/")
		for _, s := range segs[:len(segs)-1] {
			if vendorSegments[s] {
				vendorish = true
			}
		}
		if vendorish {
			rootSet[dir+"/"] = true
		}
	}
	for r := range rootSet {
		roots = append(roots, r)
	}
	return roots
}

func underAny(f string, roots []string) bool {
	lf := "/" + strings.ToLower(f)
	if strings.Contains(lf, "/wwwroot/lib/") || strings.Contains(lf, "/www/lib/") {
		return true // client libraries restored into ASP.NET / Ionic web roots
	}
	if strings.Contains(lf, "/.github/issue_template/") || strings.Contains(lf, "/.github/discussion_template/") {
		return true // forms people fill in: example versions, not pins
	}
	if reCommittedGems.MatchString(lf) {
		return true // a committed bundler install: ruby/2.7.0/gems/<gem>/
	}
	for _, r := range roots {
		if strings.HasPrefix(f, r) {
			return true
		}
	}
	return false
}

func walkFiles(fsys fs.FS, tracked map[string]bool) []string {
	var files []string
	fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries: move on
		}
		if d.IsDir() {
			if p != "." && skipDirs[strings.ToLower(d.Name())] && !strings.HasSuffix(p, "src/main/dist") {
				// src/main/dist is Gradle's distribution source, not output
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if tracked != nil && !tracked[p] {
			return nil
		}
		if isFixturePath(p) {
			return nil
		}
		files = append(files, p)
		return nil
	})
	return files
}

// maxManifestBytes bounds a single file read: manifests and lockfiles,
// not data dumps.
const maxManifestBytes = 32 << 20

// inventoryRepo walks a repository once: every file is selected by
// repoFileSet, read at most once, and parsed by every parser for its
// shape; each parse emits lifecycle pins and package identities together.
func inventoryRepo(fsys fs.FS, tracked map[string]bool) Inventory {
	files := repoFileSet(fsys, tracked)
	w := &repoWalk{fsys: fsys, helm: map[string]*helmState{}, selfGems: map[string]bool{}, files: map[string]bool{}}
	for _, f := range files {
		w.files[f] = true
	}
	for _, f := range files {
		if strings.HasSuffix(f, ".gemspec") {
			w.selfGems[strings.TrimSuffix(path.Base(f), ".gemspec")] = true
		}
	}
	for _, rel := range files {
		name := strings.ToLower(path.Base(rel))
		var txt string
		read := false
		for _, p := range parsers {
			if !p.match(rel, name) {
				continue
			}
			if !read {
				t, ok := w.read(rel)
				if !ok {
					break
				}
				txt, read = t, true
			}
			p.parse(w, rel, name, txt)
		}
	}
	// requirements files named only by reference (pip_pkgs.txt)
	inSet := map[string]bool{}
	for _, f := range files {
		inSet[f] = true
	}
	for ref := range w.reqRefs {
		if inSet[ref] && !isRequirements(ref, strings.ToLower(path.Base(ref))) {
			if txt, ok := w.read(ref); ok {
				parseRequirements(w, ref, "", txt)
			}
		}
	}
	w.resolveIDEJava()
	w.dropStaleFlatpaks()
	w.dropShadowedRust()
	w.inv.Pins = w.dropDisabledComponents(w.inv.Pins)
	return w.inv
}

// repoWalk is the walk's state: the builder plus repository-wide facts
// parsers need (helm component switches, the repo's own gem names).
type repoWalk struct {
	builder
	fsys     fs.FS
	helm     map[string]*helmState
	selfGems map[string]bool
	reqRefs  map[string]bool // requirements files referenced by -r / pip install -r
	files    map[string]bool // the repo file set, for sibling checks
	ideJava  []idePin        // Eclipse/Ant Java levels, kept when no build file declares one
	reqDone  map[string]bool
}

func (w *repoWalk) read(rel string) (string, bool) {
	info, err := fs.Stat(w.fsys, rel)
	if err != nil || info.Size() > maxManifestBytes {
		return "", false
	}
	b, err := fs.ReadFile(w.fsys, rel)
	if err != nil {
		return "", false
	}
	return decodeText(b), true
}

// decodeText handles the UTF-16 files Windows editors write (pip freeze >
// requirements.txt in PowerShell); everything else is read as UTF-8.
func decodeText(b []byte) string {
	if len(b) < 2 || !((b[0] == 0xFF && b[1] == 0xFE) || (b[0] == 0xFE && b[1] == 0xFF)) {
		return string(b)
	}
	little := b[0] == 0xFF
	u := make([]uint16, 0, len(b)/2)
	for i := 2; i+1 < len(b); i += 2 {
		if little {
			u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
		} else {
			u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
		}
	}
	return string(utf16.Decode(u))
}

// helmState holds, per chart directory, which top-level components the
// chart's own values and its environment overlays switch off.
type helmState struct {
	own, overlays map[string]bool
}

func (w *repoWalk) helmFor(dir string) *helmState {
	st := w.helm[dir]
	if st == nil {
		st = &helmState{own: map[string]bool{}, overlays: map[string]bool{}}
		w.helm[dir] = st
	}
	return st
}

// dropDisabledComponents removes pins whose helm component is disabled in
// the chart's own values or in any environment overlay: a values file
// describing a component nobody deploys is not a finding.
func (w *repoWalk) dropDisabledComponents(pins []Pin) []Pin {
	out := pins[:0]
	for _, p := range pins {
		if p.section != "" {
			file := strings.SplitN(p.File, ":", 2)[0]
			if st := w.helm[path.Dir(file)]; st != nil && (st.own[p.section] || st.overlays[p.section]) {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

// hasSibling reports a file in dir whose name has one of the suffixes.
func (w *repoWalk) hasSibling(dir string, suffixes ...string) bool {
	for f := range w.files {
		if path.Dir(f) != dir {
			continue
		}
		for _, suf := range suffixes {
			if strings.HasSuffix(strings.ToLower(f), suf) {
				return true
			}
		}
	}
	return false
}

// dirHasNoManifestSibling is false for monorepo packages: a package whose
// directory sits beside sibling packages under a packages/src/libs parent.
func dirHasNoManifestSibling(dir string, files []string) bool {
	parent := path.Base(path.Dir(dir))
	switch strings.ToLower(parent) {
	case "packages", "src", "libs", "modules", "components", "plugins", "apps", "services", "lit":
		return false
	}
	return true
}
