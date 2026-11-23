package catalog

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// walk inventories an in-memory repository: every test goes through the
// same interface the repo source uses.
func walk(files map[string]string) Inventory {
	fsys := fstest.MapFS{}
	for p, c := range files {
		fsys[p] = &fstest.MapFile{Data: []byte(c)}
	}
	return inventoryRepo(fsys, nil)
}

type pinKey struct{ product, version string }

func pinSet(inv Inventory) map[pinKey]bool {
	out := map[pinKey]bool{}
	for _, p := range inv.Pins {
		out[pinKey{p.Product, p.Version}] = true
	}
	return out
}

func TestImageTagsReduceToVersions(t *testing.T) {
	cases := map[string]pinKey{
		"FROM ruby:3.4.10-slim":                {"ruby", "3.4.10"},
		"FROM postgres:14-alpine":              {"postgresql", "14"},
		"FROM rabbitmq:3-management-alpine":    {"rabbitmq", "3"},
		"FROM redis:7.0.4-alpine":              {"redis", "7.0.4"},
		"FROM nginx:1.25@sha256:abc":           {"nginx", "1.25"},
		"FROM traefik:v1.7":                    {"traefik", "1.7"},
		"FROM eclipse-temurin:17.0.20.1_1-jre": {"eclipse-temurin", "17.0.20.1"},
		"FROM --platform=linux/amd64 node:14":  {"nodejs", "14"},
		"FROM ubuntu:bionic":                   {"ubuntu", "18.04"},
		"FROM alpine:3.12":                     {"alpine-linux", "3.12"},
		// mirrored registry: the "ruby:" hides after /docker.io/library/
		"FROM registry.example.com/oss/docker.io/library/ruby:3.4.10-slim": {"ruby", "3.4.10"},
	}
	for line, want := range cases {
		got := pinSet(walk(map[string]string{"Dockerfile": line + "\n"}))
		if len(got) != 1 || !got[want] {
			t.Errorf("%s: pins = %v, want %v", line, got, want)
		}
	}
}

func TestFloatingAndVariableRefsYieldNoPin(t *testing.T) {
	for _, line := range []string{
		"FROM postgres:latest",
		"FROM postgres",
		"FROM ruby:${RUBY_VERSION}-slim AS base",
		"FROM $BASE",
		"FROM base AS builder",
	} {
		inv := walk(map[string]string{"Dockerfile": line + "\n"})
		if len(inv.Pins) != 0 || len(inv.Untracked) != 0 {
			t.Errorf("%s: want nothing, got pins %v untracked %v", line, inv.Pins, inv.Untracked)
		}
	}
}

// Design rule 2: an unmapped image ref is untracked, never silently gone.
func TestUnmappedImageIsUntracked(t *testing.T) {
	inv := walk(map[string]string{"Dockerfile": "FROM some-vendor/some-app:1.2.3\n"})
	if len(inv.Pins) != 0 {
		t.Fatalf("unmapped image must not produce a pin: %+v", inv.Pins)
	}
	if len(inv.Untracked) != 1 || inv.Untracked[0].Version != "1.2.3" {
		t.Fatalf("unmapped image must be recorded untracked: %+v", inv.Untracked)
	}
}

// A registry port is not a tag: myregistry:5000/team/app has no version.
func TestRegistryPortIsNotATag(t *testing.T) {
	inv := walk(map[string]string{"Dockerfile": "FROM myregistry:5000/team/app\n"})
	if len(inv.Pins)+len(inv.Untracked) != 0 {
		t.Fatalf("registry port parsed as a tag: pins %v untracked %v", inv.Pins, inv.Untracked)
	}
}

// Helm values split repository/tag across lines with an unrelated key
// (pullPolicy) between them.
func TestHelmSplitPair(t *testing.T) {
	got := pinSet(walk(map[string]string{"values.yaml": "replicas: 2\nimage:\n" +
		"  repository: registry.example.com/oss/docker.io/library/postgres\n" +
		"  pullPolicy: IfNotPresent\n  tag: \"13\"\n"}))
	if len(got) != 1 || !got[pinKey{"postgresql", "13"}] {
		t.Fatalf("split repository/tag pair not parsed: %v", got)
	}
}

func TestComposeOneLine(t *testing.T) {
	got := pinSet(walk(map[string]string{"docker-compose.yml": "services:\n  cache:\n    image: redis:4.0.14\n"}))
	if len(got) != 1 || !got[pinKey{"redis", "4.0.14"}] {
		t.Fatalf("one-line image not parsed: %v", got)
	}
}

// A chart repository URL followed by a tag must not compose an image.
func TestChartRepoURLNotPaired(t *testing.T) {
	inv := walk(map[string]string{"values.yaml": "chartRepo:\n  repository: https://charts.example.com/stable\n  tag: \"13\"\n"})
	if len(inv.Pins)+len(inv.Untracked) != 0 {
		t.Fatalf("chart repo URL must not produce a pin: %v %v", inv.Pins, inv.Untracked)
	}
}

// A component an environment overlay disables is not a finding, even though the
// chart's own values still describe it.
func TestHelmOverlayDisablesComponent(t *testing.T) {
	values := "postgresql:\n  enabled: true\n  image:\n    repository: postgres\n    tag: \"13\"\n"
	on := walk(map[string]string{"chart/values.yaml": values})
	if !pinSet(on)[pinKey{"postgresql", "13"}] {
		t.Fatalf("enabled component must be pinned: %v", on.Pins)
	}
	off := walk(map[string]string{
		"chart/values.yaml":      values,
		"chart/values-prod.yaml": "postgresql:\n  enabled: false\n",
	})
	if pinSet(off)[pinKey{"postgresql", "13"}] {
		t.Fatalf("disabled postgresql pin survived the overlay pass: %v", off.Pins)
	}
}

// One parse feeds both outputs, so the lifecycle pin and the package
// identity can never disagree about which sections count.
func TestGemfileLockSections(t *testing.T) {
	lock := "GIT\n  remote: https://github.com/our-org/rails.git\n  specs:\n    rails (4.2.0)\n    our-gem (1.0.0)\n\n" +
		"PATH\n  remote: .\n  specs:\n    mygem (0.1.0)\n\n" +
		"GEM\n  remote: https://rubygems.org/\n  specs:\n    rack (2.2.3)\n    mygem (0.1.0)\n\n" +
		"RUBY VERSION\n   ruby 2.7.4p191\n"
	inv := walk(map[string]string{"Gemfile.lock": lock, "mygem.gemspec": "Gem::Specification.new"})
	for _, p := range inv.Pins {
		if p.Product == "rails" {
			t.Errorf("GIT-section rails must not become a lifecycle pin: %+v", p)
		}
	}
	if !pinSet(inv)[pinKey{"ruby", "2.7.4"}] {
		t.Errorf("RUBY VERSION pin missing: %v", inv.Pins)
	}
	if len(inv.Packages) != 1 || inv.Packages[0].Name != "rack" {
		t.Errorf("only the GEM-section, non-self gem must be a package: %+v", inv.Packages)
	}
}

// Requirement names are case-insensitive: Django==3.2.0 is django.
func TestRequirementsCaseInsensitive(t *testing.T) {
	inv := walk(map[string]string{"requirements.txt": "Django==3.2.0\nApache_Airflow==2.0.1\n"})
	got := pinSet(inv)
	if !got[pinKey{"django", "3.2.0"}] || !got[pinKey{"apache-airflow", "2.0.1"}] {
		t.Fatalf("mixed-case requirements must pin: %v", got)
	}
}

// Version files count wherever they sit: nested apps carry their own.
func TestNestedVersionFiles(t *testing.T) {
	got := pinSet(walk(map[string]string{
		"apps/web/.nvmrc":        "v14.17.0\n",
		"apps/api/.ruby-version": "ruby-2.6.3\n",
		"apps/api/.node-version": "16\n",
		"services/x/runtime.txt": "python-3.7.9\n",
	}))
	for _, w := range []pinKey{{"nodejs", "14.17.0"}, {"ruby", "2.6.3"}, {"nodejs", "16"}, {"python", "3.7.9"}} {
		if !got[w] {
			t.Errorf("nested version file pin missing: %v (have %v)", w, got)
		}
	}
}

// The file set: symlinks duplicate their targets, fixture paths are test
// data, and in a git repository only tracked files are the repo.
func TestRepoFileSet(t *testing.T) {
	fsys := fstest.MapFS{
		"Gemfile":                       {Data: []byte("ruby '2.4.4'\n")},
		"copy/Gemfile":                  {Data: []byte("ruby '2.4.4'\n"), Mode: fs.ModeSymlink},
		"spec/fixtures/app/Gemfile":     {Data: []byte("ruby '1.9.3'\n")},
		"node_modules/x/.nvmrc":         {Data: []byte("8\n")},
		"local-only/Gemfile":            {Data: []byte("ruby '2.0.0'\n")},
		"x/testdata/Dockerfile":         {Data: []byte("FROM node:8\n")},
		"sast/test/fixtures/Dockerfile": {Data: []byte("FROM node:8\n")},
	}
	tracked := map[string]bool{}
	for p := range fsys {
		if p != "local-only/Gemfile" {
			tracked[p] = true
		}
	}
	files := repoFileSet(fsys, tracked)
	if len(files) != 1 || files[0] != "Gemfile" {
		t.Fatalf("file set = %v, want [Gemfile]", files)
	}
}

// trackedFiles is the adapter over git: untracked working-tree files are
// not enumerated.
func TestTrackedFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		exec.Command("git", append([]string{"-C", dir}, args...)...).Run()
	}
	run("init", "-q")
	os.WriteFile(filepath.Join(dir, "Gemfile"), []byte("ruby '2.4.4'\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "local-only.txt"), []byte("x"), 0o644)
	run("add", "Gemfile")
	set := trackedFiles(dir)
	if set == nil || !set["Gemfile"] || set["local-only.txt"] {
		t.Fatalf("trackedFiles = %v", set)
	}
}

// A copied-in project (own license under include/, or own CI) is not the
// repository's build; a monorepo package with a license is.
func TestVendoredCopiesSkipped(t *testing.T) {
	inv := walk(map[string]string{
		"Engine/include/imgui/LICENSE.txt":                         "MIT",
		"Engine/include/imgui/examples/win32/example.vcxproj":      "<PlatformToolset>v100</PlatformToolset>",
		"thirdparty-ish/upstream-tool/LICENSE":                     "MIT",
		"thirdparty-ish/upstream-tool/.github/workflows/build.yml": "x: 1",
		"thirdparty-ish/upstream-tool/Dockerfile":                  "FROM python:3.6\n",
		"packages/app/LICENSE":                                     "MIT",
		"packages/app/Dockerfile":                                  "FROM node:10\n",
	})
	got := pinSet(inv)
	if got[pinKey{"visual-studio", "10.0"}] || got[pinKey{"python", "3.6"}] {
		t.Errorf("vendored copies must be skipped: %v", got)
	}
	if !got[pinKey{"nodejs", "10"}] {
		t.Errorf("a licensed monorepo package is the project's own: %v", got)
	}
}

// A Dockerfile ARG that nothing uses installs nothing.
func TestUnusedDockerfileVersionArg(t *testing.T) {
	got := pinSet(walk(map[string]string{"Dockerfile": "ARG DOTNET_VERSION=3.1.413\nFROM alpine:3.20\nARG NODE_VERSION=12.22.0\nRUN install-node $NODE_VERSION\n"}))
	if got[pinKey{"dotnet", "3.1"}] {
		t.Errorf("unused ARG must not pin: %v", got)
	}
	if !got[pinKey{"nodejs", "12.22.0"}] {
		t.Errorf("used ARG must pin: %v", got)
	}
}

// A copied-in upstream in a plain folder announces itself: its own README
// next to its own license, its own .gitmodules, or its own pre-commit
// config. First-party monorepo containers keep their packages.
func TestVendoredPlainFolderCopies(t *testing.T) {
	inv := walk(map[string]string{
		// license + README + own manifests, plain folder
		"upstream-lib/LICENSE":          "MIT",
		"upstream-lib/README.md":        "# upstream-lib",
		"upstream-lib/requirements.txt": "numpy==1.22.0\n",
		// upstream pre-commit config, licenses stripped
		"detector/.pre-commit-config.yaml":     "repos: []\n",
		"detector/requirements.txt":            "numpy==1.24.0\n",
		"detector/utils/docker/Dockerfile-cpu": "FROM ubuntu:18.04\n",
		// its own .gitmodules
		"hardware/board-core/.gitmodules":                                         "[submodule \"core\"]\n",
		"hardware/board-core/LICENSE":                                             "MIT",
		"hardware/board-core/README.md":                                           "# board-core",
		"hardware/board-core/tools/linux/src/loader/nbproject/project.properties": "javac.source=1.7\n",
		// first-party package in a monorepo container keeps both
		"services/rds/README.md":  "rds deployer",
		"services/rds/LICENSE":    "MIT",
		"services/rds/Dockerfile": "FROM python:3.9\n",
	})
	got := pinSet(inv)
	for _, bad := range []pinKey{{"numpy", "1.22"}, {"numpy", "1.24"}, {"ubuntu", "18.04"}, {"java", "7"}} {
		if got[bad] {
			t.Errorf("vendored pin leaked: %v in %v", bad, got)
		}
	}
	if !got[pinKey{"python", "3.9"}] {
		t.Errorf("first-party monorepo package pin lost: %v", got)
	}
}

// A Software Collections python installed by yum rides the OS lifecycle:
// its ENV PYTHON_VERSION labels the package, not a pin.
func TestSCLPythonRidesOS(t *testing.T) {
	inv := walk(map[string]string{
		"Dockerfile": "FROM registry.access.redhat.com/openshift3/jenkins-slave-base-rhel7\nENV PYTHON_VERSION=3.5\nRUN INSTALL_PKGS=\"rh-python35 rh-python35-python-pip\" && yum install -y $INSTALL_PKGS\n",
	})
	got := pinSet(inv)
	if got[pinKey{"python", "3.5"}] {
		t.Errorf("SCL distro package must ride the OS: %v", got)
	}
	if !got[pinKey{"rhel", "7"}] {
		t.Errorf("the base image OS must still pin: %v", got)
	}
}

// Prompt text, comments, and a Travis env var no step consumes name
// nothing; docker commands and heredoc YAML still carry their images.
func TestScriptTextAndDeadEnvYieldNoPins(t *testing.T) {
	inv := walk(map[string]string{
		"run.sh":      "read -p \"Ingrese el nombre de la imagen (nombre:tag, ej: ubuntu:20.04): \" x\n# docker pull openjdk:9-jdk\ndocker run -d golang:1.15\n",
		"net.sh":      "docker run --rm -it \\\n\t-v `pwd`:/tmp \\\n\t-v /var/run:/var/run \\\n\tsomeuser/ubuntu:16.04 \\\n\t/tmp/a.out\n",
		".travis.yml": "language: python\nenv:\n  matrix:\n    - DJANGO=1.8\n    - PYTHON=2.7\nscript:\n  - pip install $PYTHON\n",
	})
	got := pinSet(inv)
	if got[pinKey{"ubuntu", "20.04"}] || got[pinKey{"java", "9"}] || got[pinKey{"django", "1.8"}] {
		t.Errorf("prompt text, comments and dead env vars must not pin: %v", got)
	}
	if !got[pinKey{"go", "1.15"}] || !got[pinKey{"python", "2.7"}] || !got[pinKey{"ubuntu", "16.04"}] {
		t.Errorf("docker run, continuations and consumed env must pin: %v", got)
	}
}

// False-positive classes: usage inside comments, a redefinition that
// overrides, an unread matrix dimension, a distro python, a first-party
// demo sub-site, and the ubuntu24 alias.
func TestCommonFalsePositives(t *testing.T) {
	inv := walk(map[string]string{
		"etcd/Dockerfile":                "ENV ETCD_VERSION=0.5.0-alpha.4\nENV ETCD_VERSION=0.4.6\n# RUN etcd $ETCD_VERSION start\n",
		"dead.yml":                       "jobs:\n  b:\n    runs-on: ubuntu-24.04\n    strategy:\n      matrix:\n        python-version: ['3.10']\n    steps:\n      - run: echo hi\n",
		"oracle/Dockerfile":              "FROM ghcr.io/oracle/oraclelinux:8-slim\nRUN microdnf install -y python38 && microdnf install -y python38-pip\nRUN /usr/bin/python3.8 -m pip install -r requirements.txt\n",
		"demo/website-demo/package.json": `{"dependencies": {"eslint": "^8.29.0"}}`,
		"demo/website-demo/README.md":    "the demo site",
		"demo/website-demo/license.md":   "MIT",
		"ubuntu-alias.yml":               "jobs:\n  b:\n    container:\n      image: ubuntu24\n",
	})
	got := pinSet(inv)
	for _, bad := range []pinKey{
		{"etcd", "0.5.0-alpha.4"}, // ENV overridden to 0.4.6
		{"python", "3.10"},        // matrix dimension no step reads
		{"python", "3.8"},         // distro package rides the OS
		{"ubuntu", "24.10"},       // ubuntu24 is 24.04, supported
	} {
		if got[bad] {
			t.Errorf("FP leaked: %v in %v", bad, got)
		}
	}
	if !got[pinKey{"eslint", "8"}] {
		t.Errorf("first-party demo site must still pin: %v", got)
	}
}

// Scaffolding tools write "private": "true" as a string; the manifest
// still declares everything it declares.
func TestStringPrivatePackageJSON(t *testing.T) {
	inv := walk(map[string]string{
		"package.json": "{\"name\":\"x\",\"private\":\"true\",\"devDependencies\":{\"electron\":\"^30.0.2\"}}",
	})
	if !pinSet(inv)[pinKey{"electron", "30"}] {
		t.Fatalf("string private must not skip the manifest: %v", pinSet(inv))
	}
}

// resolutionStrategy pairs each group check with the useVersion in its
// own block: an empty block steals nothing from the next one.
func TestGradleForcedVersionsBlockScoped(t *testing.T) {
	code := "requested.group == \"io.netty\" && requested.name.startsWith(\"netty-\") -> {\n" +
		"\tbecause(\"pinned upstream\")\n" +
		"}\n" +
		"requested.group == \"org.springframework\" -> {\n" +
		"\tuseVersion(\"6.2.15\")\n" +
		"}\n"
	pairs := gradleForcedVersions(code)
	if len(pairs) != 1 || pairs[0][0] != "org.springframework" || pairs[0][1] != "6.2.15" {
		t.Fatalf("expected exactly the spring pair, got %v", pairs)
	}
}

// An IDE SDK pin under a manifest that pins the same product is not a
// declaration; without one it is the project's level.
func TestIdeaSuppressedByManifest(t *testing.T) {
	inv := walk(map[string]string{
		".idea/misc.xml":  "<component project-jdk-name=\"Python 3.8\" project-jdk-type=\"Python SDK\" />",
		"environment.yml": "dependencies:\n  - python=3.6.2\n",
	})
	got := pinSet(inv)
	if got[pinKey{"python", "3.8"}] || !got[pinKey{"python", "3.6.2"}] {
		t.Errorf("manifest wins over IDE SDK: %v", got)
	}
	inv2 := walk(map[string]string{
		".idea/misc.xml": "<component name=\"ProjectRootManager\" languageLevel=\"JDK_1_6\" />\n",
	})
	if !pinSet(inv2)[pinKey{"java", "6"}] {
		t.Errorf("IDE level without a build file is the project's: %v", pinSet(inv2))
	}
}

// A sibling module's same-named property is a different value: only the
// pom's own lineage (self, parents, inheriting modules) makes it used.
func TestPomPropertyScopedToLineage(t *testing.T) {
	inv := walk(map[string]string{
		"pom.xml":        "<project><artifactId>all</artifactId><modules><module>a</module><module>b</module></modules></project>",
		"a/pom.xml":      "<project><parent><artifactId>all</artifactId></parent><artifactId>a</artifactId><properties><spring.version>4.2.5.RELEASE</spring.version></properties></project>",
		"b/pom.xml":      "<project><parent><artifactId>all</artifactId></parent><artifactId>b</artifactId><properties><spring.version>5.3.30</spring.version></properties><dependencies><dependency><groupId>org.springframework</groupId><artifactId>spring-core</artifactId><version>${spring.version}</version></dependency></dependencies></project>",
		"parent/pom.xml": "<project><artifactId>base</artifactId><properties><kotlin.version>1.3.72</kotlin.version></properties></project>",
		"app/pom.xml":    "<project><parent><artifactId>base</artifactId><relativePath>../parent</relativePath></parent><artifactId>app</artifactId><dependencies><dependency><groupId>org.jetbrains.kotlin</groupId><artifactId>kotlin-stdlib</artifactId><version>${kotlin.version}</version></dependency></dependencies></project>",
	})
	got := pinSet(inv)
	if !got[pinKey{"spring-framework", "5.3.30"}] || got[pinKey{"spring-framework", "4.2.5"}] {
		t.Errorf("sibling's unused property pinned: %v", got)
	}
	if !got[pinKey{"kotlin", "1.3.72"}] {
		t.Errorf("parent property used by a module elsewhere was dropped: %v", got)
	}
}

// 3rdParties/ and branch archives (lib-main/ with its LICENSE) are
// somebody else's project.
func TestThirdPartyAndBranchArchiveSkipped(t *testing.T) {
	inv := walk(map[string]string{
		"3rdParties/Json60r3/Src/Newtonsoft.Json.csproj": `<Project><PropertyGroup><TargetFrameworkVersion>v4.0</TargetFrameworkVersion></PropertyGroup></Project>`,
		"media-lib-main/LICENSE":                         "MIT",
		"media-lib-main/package.json":                    `{"devDependencies": {"eslint": "^8.0.0"}}`,
		"package.json":                                   `{"dependencies": {"vue": "^2.6.0"}}`,
	})
	for _, p := range inv.Pins {
		if p.Product != "vue" {
			t.Errorf("vendored pin leaked: %+v", p)
		}
	}
}

// Without a git index, .gitmodules paths are someone else's code - unless
// one org registers a toolbox of its own repos.
func TestSubmodulePathsSkipped(t *testing.T) {
	inv := walk(map[string]string{
		".gitmodules":           "[submodule \"native\"]\n\tpath = native\n\turl = https://github.com/hrydgard/ppsspp-native.git\n[submodule \"lang\"]\n\tpath = lang\n\turl = https://github.com/other/lang.git\n",
		"native/native.vcxproj": "<Project><PlatformToolset>v100</PlatformToolset></Project>",
		"nativex/app.vcxproj":   "<Project><PlatformToolset>v100</PlatformToolset></Project>",
	})
	if len(inv.Pins) != 1 || !strings.HasPrefix(inv.Pins[0].File, "nativex/") {
		t.Errorf("want the nativex pin only: %+v", inv.Pins)
	}
	own := walk(map[string]string{
		".gitmodules":               "[submodule \"a\"]\n\tpath = guides/a\n\turl = https://github.com/acme/a\n[submodule \"b\"]\n\tpath = guides/b\n\turl = https://github.com/acme/b\n[submodule \"c\"]\n\tpath = guides/c\n\turl = https://github.com/acme/c\n",
		"guides/a/requirements.txt": "numpy==1.19.3\n",
	})
	if len(own.Pins) != 1 {
		t.Errorf("own-org toolbox dropped: %+v", own.Pins)
	}
}

// Issue forms carry example versions people overwrite, not pins.
func TestIssueTemplatesSkipped(t *testing.T) {
	inv := walk(map[string]string{
		".github/ISSUE_TEMPLATE/bug_report.yml": "body:\n  - type: input\n    attributes:\n      placeholder: v1.23.3+k3s1\n",
	})
	if len(inv.Pins) != 0 {
		t.Fatalf("issue template pinned: %+v", inv.Pins)
	}
}

// "zephy ." names the repo after its folder, never ".".
func TestRepoSourceNameIsFolder(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{".", "./", wd + "/"} {
		src, err := RepoSource(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := src.Name(); got != filepath.Base(wd) {
			t.Errorf("RepoSource(%q).Name() = %q, want %q", dir, got, filepath.Base(wd))
		}
	}
}
