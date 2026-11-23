package catalog

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/0xbadshah/zephy/internal/ident"
)

// image records a container image reference: always as an image package
// identity (curated matching), and as lifecycle pins - the image's own
// product plus whatever its tag variant pins (node:12-stretch runs Debian
// 9, maven:3.6-jdk-8 runs Java 8). An unmapped image is untracked, never
// silently dropped.
func (b *builder) image(ref ident.ImageRef, file string, line int, section string) {
	if ref.CuratedPath() != "" && ref.Tag != "" {
		b.inv.Packages = append(b.inv.Packages, PackagePin{
			Ecosystem: "image", Name: ref.CuratedPath(), Version: ref.Tag, File: loc(file, line)})
	}
	if m := reNameDistro.FindStringSubmatch(ref.Base()); m != nil {
		// distroless/python3-debian10, static-debian12: the OS is in the
		// name, whatever the tag
		b.inv.Pins = append(b.inv.Pins, Pin{Product: m[1], Version: m[2], File: loc(file, line), Raw: ref.String(), section: section})
	} else if p, v, ok := osInImageName(ref.Base()); ok {
		// geerlingguy/docker-debian10-ansible, centos7-systemd: test images
		// named for the OS they run
		b.inv.Pins = append(b.inv.Pins, Pin{Product: p, Version: v, File: loc(file, line), Raw: ref.String(), section: section})
	}
	if m := reSCLName.FindStringSubmatch(ref.Base()[strings.LastIndex(ref.Base(), "/")+1:]); m != nil {
		// Software Collections / S2I builders name the runtime:
		// python-35 = Python 3.5, nodejs-10 = Node 10
		v := m[2] + "." + m[3]
		if m[1] == "nodejs" {
			v = m[2] + m[3]
		}
		b.inv.Pins = append(b.inv.Pins, Pin{Product: sclProducts[m[1]], Version: v, File: loc(file, line), Raw: ref.String(), section: section})
	}
	slug, mapped := imageProduct(ref)
	if m := reROSName.FindStringSubmatch(ref.Base()); m != nil && !mapped {
		// ros-kinetic-amr: a ROS distribution named in the image, whatever the tag
		b.inv.Pins = append(b.inv.Pins, Pin{Product: "ros", Version: m[1], File: loc(file, line), Raw: ref.String(), section: section})
	}
	if !mapped {
		if m := rePyName.FindStringSubmatch(ref.Base()); m != nil {
			// ci/py3.10-app: a Python named in the image path,
			// whatever the tag
			b.inv.Pins = append(b.inv.Pins, Pin{Product: "python", Version: m[1] + "." + m[2], File: loc(file, line), Raw: ref.String(), section: section})
		}
		// continuumio/miniconda2 is Python 2.7, whatever the tag
		if name := ref.Base()[strings.LastIndex(ref.Base(), "/")+1:]; strings.EqualFold(name, "miniconda2") {
			b.inv.Pins = append(b.inv.Pins, Pin{Product: "python", Version: "2.7", File: loc(file, line), Raw: ref.String(), section: section})
		}
	}
	// microsoft/nanoserver:10.0.14393.576: the build number names the
	// release (14393 = Nano 1607; the same build is Server 2016 for
	// server-core, whose dataset cycle is the supported "2016")
	if slug == "windows-nano-server" || slug == "windows-server-core" || slug == "windows-server" {
		if m := reWinBuildTag.FindStringSubmatch(ref.Tag); m != nil {
			rel, ok := winBuildReleases[slug][m[1]]
			if !ok {
				rel, ok = winBuildReleases["default"][m[1]]
			}
			if ok {
				b.inv.Pins = append(b.inv.Pins, Pin{Product: slug, Version: rel, File: loc(file, line), Raw: ref.String(), section: section})
				return
			}
		}
	}
	if ref.Tag == "" || ref.Tag == "latest" {
		return // floating: nothing to verify
	}
	tag := strings.ToLower(ref.Tag)
	if slug == "ros" && ros2Distros[strings.SplitN(tag, "-", 2)[0]] {
		slug = "ros-2" // ros:galactic, ros:humble-ros-base
	}
	if m := reCircleMachine.FindStringSubmatch(ref.Base()); m != nil {
		// CircleCI machine images: ubuntu-2004:202101-01 runs Ubuntu 20.04
		b.inv.Pins = append(b.inv.Pins, Pin{Product: "ubuntu", Version: m[1] + "." + m[2], File: loc(file, line), Raw: ref.String(), section: section})
		return
	}
	if strings.Contains(ref.Path, "devcontainers/") {
		// devcontainer tags carry the image definition's own major first:
		// python:0-3.7 is Python 3.7, typescript-node:1-16 is Node 16
		if m := reDevcontainerTag.FindStringSubmatch(tag); m != nil {
			tag = m[1]
		}
	}
	raw := ref.Base() + ":" + ref.Tag
	add := func(product, version string) {
		b.inv.Pins = append(b.inv.Pins, Pin{Product: product, Version: version, File: loc(file, line), Raw: raw, section: section})
	}

	if strings.HasSuffix(ref.CuratedPath(), "phusion/baseimage") {
		if v, ok := phusionUbuntu(tag); ok {
			b.inv.Pins = append(b.inv.Pins, Pin{Product: "ubuntu", Version: v, File: loc(file, line), Raw: raw, section: section})
			return
		}
	}
	version, variants := parseTag(slug, tag)
	if v, ok := nameVersion(ref.Base()); ok && mapped {
		// ubi8/openjdk-17-runtime:1.10: the name pins the runtime, the tag
		// is the image's own release
		version, variants = v, nil
		if slug == "java" {
			version = javaVersion(v)
		}
	}
	if !mapped && version == "" {
		// a bare distro codename names the OS whatever the image:
		// resin/rpi-raspbian:jessie runs Debian 8, buildpack-deps:stretch Debian 9
		first := strings.SplitN(tag, "-", 2)[0]
		if v, ok := debianCodenames[first]; ok {
			variants = append(variants, variantPin{"debian", v})
		} else if v, ok := ubuntuCodenames[first]; ok {
			variants = append(variants, variantPin{"ubuntu", v})
		}
	}

	switch {
	case mapped && version != "":
		add(slug, version)
	case mapped:
		// a named tag (lts, stable, edge) on a tracked image: the tag
		// floats, nothing to verify
	case version != "" && ref.Digest == "":
		b.untracked(ref.String(), ref.Base(), version, file, line, "image has no lifecycle mapping")
	}
	for _, v := range variants {
		if v.product != slug {
			add(v.product, v.version)
		}
	}
}

// imageProduct maps an image to its lifecycle product: by repository path
// first (dotnet/core/sdk, bitnami/postgresql), then by base name.
// notProductTags: images named after a product whose tags are the
// image's own release, not the product's version.
var notProductTags = map[string]bool{
	"dockercloud/haproxy": true, "jwilder/nginx-proxy": true, "nginxproxy/nginx-proxy": true,
	"tutum/haproxy": true, "nginxproxy/acme-companion": true,
}

func imageProduct(ref ident.ImageRef) (string, bool) {
	if notProductTags[ref.CuratedPath()] {
		return "", false
	}
	parts := strings.Split(ref.CuratedPath(), "/")
	for i := 0; i < len(parts); i++ {
		if s, ok := imageSlug(strings.Join(parts[i:], "/")); ok {
			return s, true
		}
	}
	if s, ok := imageSlug(ref.Base()); ok {
		return s, true
	}
	// third-party runtime images tag the runtime version:
	// bitwalker/alpine-elixir:1.6, ekidd/rust-musl-builder:1.34.2
	for _, seg := range strings.Split(ref.Base(), "-") {
		if p, ok := runtimeTokens[seg]; ok {
			return p, true
		}
	}
	return "", false
}

// runtimeTokens: name segments that make an image a language runtime image.
// "node" is left out on purpose (node-exporter, node-red).
var runtimeTokens = map[string]string{
	"elixir": "elixir", "rust": "rust", "ruby": "ruby", "php": "php", "python": "python", "erlang": "erlang",
	"golang": "go", "openjdk": "java", "nodejs": "nodejs", "julia": "julia", "dotnet": "dotnet",
}

type variantPin struct{ product, version string }

var (
	reCircleMachine   = regexp.MustCompile(`^ubuntu-(\d\d)(\d\d)$`)
	reDevcontainerTag = regexp.MustCompile(`^\d+-(\d.*)$`)
	reTagVersion      = regexp.MustCompile(`^v?(\d+(?:\.\d+)*)`)
	reOSToken         = regexp.MustCompile(`^(ubuntu|centos|rockylinux|rocky|ubi|almalinux|debian|amzn|oraclelinux|ol|rhel|fedora)(\d+(?:\.\d+)?)$`)
	reHadoopTag       = regexp.MustCompile(`^hadoop(\d+\.\d+)(?:\.\d+)*$`)
	rePgTag           = regexp.MustCompile(`^pg(\d+)$`)
	reAlpineSuffix    = regexp.MustCompile(`^alpine(\d+\.\d+)$`)
	reJavaToken       = regexp.MustCompile(`^(?:open)?(?:jdk|jre|java|temurin|corretto|zulu)-?(\d+)$`)
	rePrefixed        = regexp.MustCompile(`^(php|node|python|ruby|golang|go)-?(\d+(?:\.\d+)*)$`)
)

var osTokenProducts = map[string]string{
	"ubuntu": "ubuntu", "centos": "centos", "rockylinux": "rocky-linux", "rocky": "rocky-linux", "ubi": "rhel",
	"almalinux": "almalinux", "debian": "debian", "amzn": "amazon-linux", "oraclelinux": "oracle-linux",
	"ol": "oracle-linux", "rhel": "rhel", "fedora": "fedora",
}

var debianCodenames = map[string]string{
	"hamm": "2.0", "slink": "2.1", "potato": "2.2", "woody": "3.0", "sarge": "3.1", "etch": "4",
	"lenny": "5", "squeeze": "6", "wheezy": "7", "jessie": "8", "stretch": "9", "buster": "10",
	"bullseye": "11", "bookworm": "12", "trixie": "13",
}

var ubuntuCodenames = map[string]string{
	"warty": "4.10", "hoary": "5.04", "breezy": "5.10", "dapper": "6.06", "edgy": "6.10", "feisty": "7.04",
	"gutsy": "7.10", "hardy": "8.04", "intrepid": "8.10", "jaunty": "9.04", "karmic": "9.10", "lucid": "10.04",
	"maverick": "10.10", "natty": "11.04", "oneiric": "11.10", "precise": "12.04", "quantal": "12.10",
	"raring": "13.04", "saucy": "13.10", "trusty": "14.04", "utopic": "14.10", "vivid": "15.04", "wily": "15.10",
	"xenial": "16.04", "yakkety": "16.10", "zesty": "17.04", "artful": "17.10", "bionic": "18.04",
	"cosmic": "18.10", "disco": "19.04", "eoan": "19.10", "focal": "20.04", "groovy": "20.10",
	"hirsute": "21.04", "impish": "21.10", "jammy": "22.04", "kinetic": "22.10", "lunar": "23.04",
	"mantic": "23.10", "noble": "24.04", "oracular": "24.10", "plucky": "25.04", "questing": "25.10",
}

// nodeCodenames: node:carbon is Node 8.
var nodeCodenames = map[string]string{
	"argon": "4", "boron": "6", "carbon": "8", "dubnium": "10", "erbium": "12", "fermium": "14",
	"gallium": "16", "hydrogen": "18", "iron": "20", "jod": "22", "krypton": "24",
}

// osInImageName finds an OS release token in an image's own name
// (docker-debian10-ansible, ubuntu2004-systemd).
func osInImageName(base string) (string, string, bool) {
	name := base[strings.LastIndex(base, "/")+1:]
	for _, tok := range strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' }) {
		m := reOSToken.FindStringSubmatch(tok)
		if m == nil || osTokenProducts[m[1]] == "" || m[1] == "ol" || m[1] == "ubi" {
			continue
		}
		v := m[2]
		if m[1] == "ubuntu" && len(v) == 4 && !strings.Contains(v, ".") {
			v = v[:2] + "." + v[2:] // ubuntu2004
		}
		if m[1] == "ubuntu" && len(v) == 2 && !strings.Contains(v, ".") {
			v = v + ".04" // ubuntu24 is the 24.04 LTS
		}
		return osTokenProducts[m[1]], v, true
	}
	return "", "", false
}

var (
	reSCLName   = regexp.MustCompile(`^(python|ruby|nodejs|php|perl|dotnet)-(\d)(\d*)-(?:(?:jenkins-slave|s2i)-)?(?:centos|rhel|ubi)\d+$`)
	sclProducts = map[string]string{"python": "python", "ruby": "ruby", "nodejs": "nodejs", "php": "php", "perl": "perl", "dotnet": "dotnet"}
)

var reNameDistro = regexp.MustCompile(`-(debian|ubuntu)-?(\d+(?:\.\d+)?)$`)

// rePyName: a Python version in the image's own name (ci/py3.10-...).
var rePyName = regexp.MustCompile(`(?i)(?:^|[/\-.])(?:py|python)[-.]?(\d)\.(\d+)(?:[.\-/]|$)`)

// reWinBuildTag: 10.0.14393.576 names a Windows release by build number.
var reWinBuildTag = regexp.MustCompile(`^10\.0\.(\d{5})`)

// winBuildReleases: Windows build numbers, per image family — the same
// build 14393 is Nano 1607 (EOL) and Server 2016 (the supported cycle
// server-core's dataset names).
var winBuildReleases = map[string]map[string]string{
	"windows-nano-server": {"14393": "1607", "16299": "1709", "17134": "1803", "17763": "1809",
		"18362": "1903", "18363": "1909", "19041": "2004", "19042": "20h2"},
	"windows-server-core": {"14393": "2016", "16299": "1709", "17134": "1803", "17763": "1809",
		"18362": "1903", "18363": "1909", "19041": "2004", "19042": "20h2"},
	"default": {"14393": "1607", "16299": "1709", "17134": "1803", "17763": "1809",
		"18362": "1903", "18363": "1909", "19041": "2004", "19042": "20h2"},
}

// mssqlYears maps SQL Server release years to their version cycles.
var mssqlYears = map[string]string{"2017": "14.0", "2019": "15.0", "2022": "16.0", "2025": "17.0"}

// parseTag reads an image tag: the leading version of the image's own
// product, plus variant pins carried by later tokens.
func parseTag(product, tag string) (string, []variantPin) {
	tag = strings.SplitN(tag, "@", 2)[0]
	var version string
	var variants []variantPin
	tokens := strings.Split(tag, "-")
	switch product {
	case "ubuntu":
		if v, ok := ubuntuCodenames[tokens[0]]; ok {
			return v, nil
		}
	case "debian":
		if v, ok := debianCodenames[tokens[0]]; ok {
			return v, nil
		}
	case "mssqlserver":
		if m := reTagVersion.FindString(tokens[0]); m != "" {
			if v, ok := mssqlYears[m]; ok {
				return v, nil
			}
		}
	}
	if product == "nodejs" {
		if v, ok := nodeCodenames[tokens[0]]; ok {
			return v, nil
		}
	}
	if product == "ros" || product == "ros-2" {
		return tokens[0], nil // ROS releases are codenames: ros:kinetic-ros-base
	}
	if m := reTagVersion.FindStringSubmatch(tokens[0]); m != nil {
		version = m[1]
		if product == "java" {
			version = javaVersion(version)
		}
	} else if m := rePrefixed.FindStringSubmatch(tokens[0]); m != nil && product == "" {
		// tags like python3.8-nodejs14 on combined images
		tokens = append([]string{"", tokens[0]}, tokens[1:]...)
	}
	start := 1
	if version == "" {
		start = 0 // no leading version: every token may carry a variant
	}
	for i := start; i < len(tokens); i++ {
		t := tokens[i]
		if v, ok := debianCodenames[t]; ok && product != "debian" {
			variants = append(variants, variantPin{"debian", v})
			continue
		}
		if v, ok := ubuntuCodenames[t]; ok && product != "ubuntu" {
			variants = append(variants, variantPin{"ubuntu", v})
			continue
		}
		if p := osTokenProducts[t]; p != "" && p != product && i+1 < len(tokens) {
			// ocaml/opam:ubuntu-20.04-ocaml-4.14: the OS name and version as two tokens
			if m := reTagVersion.FindStringSubmatch(tokens[i+1]); m != nil {
				variants = append(variants, variantPin{p, m[1]})
				i++
				continue
			}
		}
		if m := reOSToken.FindStringSubmatch(t); m != nil {
			// nvidia/cuda:11.0-base-ubuntu20.04, ...-centos7, -ubi8, -rockylinux8
			if p := osTokenProducts[m[1]]; p == product && version == "" {
				version = m[2] // centos:centos7
			} else if p != "" && p != product {
				variants = append(variants, variantPin{p, m[2]})
			}
			continue
		}
		if m := reHadoopTag.FindStringSubmatch(t); m != nil && product != "apache-hadoop" {
			variants = append(variants, variantPin{"apache-hadoop", m[1]}) // bde2020 ...-hadoop2.8-java8
			continue
		}
		if m := rePgTag.FindStringSubmatch(t); m != nil && product != "postgresql" {
			// pgvector/pgvector:pg14, postgis tags: the Postgres major
			variants = append(variants, variantPin{"postgresql", m[1]})
			continue
		}
		if m := reAlpineSuffix.FindStringSubmatch(t); m != nil {
			variants = append(variants, variantPin{"alpine-linux", m[1]})
			continue
		}
		if m := reJavaToken.FindStringSubmatch(t); m != nil {
			variants = append(variants, variantPin{"java", javaVersion(m[1])})
			continue
		}
		if (t == "jdk" || t == "jre" || t == "openjdk" || t == "temurin" || t == "corretto" || t == "zulu" || t == "java") && i+1 < len(tokens) {
			if m := reTagVersion.FindStringSubmatch(tokens[i+1]); m != nil {
				variants = append(variants, variantPin{"java", javaVersion(m[1])})
				i++
				continue
			}
		}
		if m := rePrefixed.FindStringSubmatch(t); m != nil {
			p := map[string]string{"php": "php", "node": "nodejs", "python": "python", "ruby": "ruby", "golang": "go", "go": "go"}[m[1]]
			variants = append(variants, variantPin{p, m[2]})
		}
	}
	return version, variants
}

// javaVersion normalizes legacy "1.8" spellings to the feature release.
func javaVersion(v string) string {
	if strings.HasPrefix(v, "1.") {
		rest := strings.TrimPrefix(v, "1.")
		if major := strings.SplitN(rest, ".", 2)[0]; major >= "5" || len(major) > 1 {
			return rest
		}
	}
	return v
}

var (
	reFrom   = regexp.MustCompile(`(?im)^\s*FROM\s+(.+)$`)
	reArgEnv = regexp.MustCompile(`(?im)^\s*(?:ARG|ENV)\s+([A-Za-z_][A-Za-z0-9_]*)(?:\s*=\s*|\s+)("[^"]*"|'[^']*'|\S+)`)
	reVarRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::?[-=]([^}]*))?\}|\$([A-Za-z_][A-Za-z0-9_]*)`)
)

// expandVars substitutes Dockerfile build-arg/env defaults into a
// reference: FROM ruby:${RUBY_VERSION}-slim with ARG RUBY_VERSION=2.6.
// ok=false when any variable stays unresolved.
func expandVars(s string, vars map[string]string) (string, bool) {
	// defaults may name other args (RUNNER_IMAGE="debian:${DEBIAN_VERSION}"):
	// expand until nothing changes
	for i := 0; i < 4; i++ {
		next, ok := expandOnce(s, vars)
		if !ok || next == s || !strings.Contains(next, "$") {
			return next, ok
		}
		s = next
	}
	return expandOnce(s, vars)
}

func expandOnce(s string, vars map[string]string) (string, bool) {
	ok := true
	out := reVarRef.ReplaceAllStringFunc(s, func(m string) string {
		sub := reVarRef.FindStringSubmatch(m)
		name := sub[1]
		if name == "" {
			name = sub[3]
		}
		if v, found := vars[name]; found && v != "" {
			return v
		}
		if sub[2] != "" {
			return sub[2]
		}
		ok = false
		return m
	})
	return out, ok
}

func parseDockerfile(w *repoWalk, rel, _, txt string) {
	vars := map[string]string{}
	stages := map[string]bool{}
	// the final assignment of each name is the one that counts; a
	// redefinition overrides (ETCD_VERSION=0.5.0-alpha.4 then =0.4.6).
	// Usage counts only outside comments.
	code := stripComments(txt)
	type envAssign struct {
		val  string
		line int
	}
	finals := map[string]envAssign{}
	order := []string{}
	lines := strings.Split(txt, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if reArgEnv.FindStringSubmatch(line) == nil {
			continue
		}
		start := i
		// ENV A=1 \  (or ` on Windows images) B=2: join continuation lines
		for (strings.HasSuffix(strings.TrimSpace(line), "\\") || strings.HasSuffix(strings.TrimSpace(line), "`")) && i+1 < len(lines) {
			i++
			line = strings.TrimRight(strings.TrimSpace(line), "\\`") + " " + lines[i]
		}
		pairs := reEnvPair.FindAllStringSubmatch(line, -1)
		if len(pairs) == 0 {
			if m := reArgEnv.FindStringSubmatch(line); m != nil {
				pairs = [][]string{{"", m[1], m[2]}} // ENV NAME value
			}
		}
		for _, m := range pairs {
			v := strings.Trim(m[2], `"'`)
			if _, seen := vars[m[1]]; !seen || v != "" {
				vars[m[1]] = v
			}
			if _, ok := finals[m[1]]; !ok {
				order = append(order, m[1])
			}
			finals[m[1]] = envAssign{v, start + 1}
		}
	}
	for _, name := range order {
		fa := finals[name]
		// a Software Collections install rides the OS lifecycle: its
		// ENV PYTHON_VERSION describes the yum package, not a pin
		if scl := reSCLPackage.FindStringSubmatch(txt); scl != nil &&
			strings.HasPrefix(name, "PYTHON_") && fa.val == scl[1]+"."+scl[2] {
			continue
		}
		if strings.Count(code, "$"+name)+strings.Count(code, "${"+name)+strings.Count(strings.ToLower(code), "$env:"+strings.ToLower(name)) > 0 {
			w.versionVar(name, fa.val, rel, fa.line) // ENV NODE_VERSION 12.18.3 used by a RUN or FROM
		}
	}
	for _, m := range reFrom.FindAllStringSubmatchIndex(txt, -1) {
		fields := strings.Fields(txt[m[2]:m[3]])
		var ref string
		for i, f := range fields {
			if strings.HasPrefix(f, "--") { // --platform=...
				continue
			}
			ref = f
			if i+2 < len(fields) && strings.EqualFold(fields[i+1], "as") {
				stages[strings.ToLower(fields[i+2])] = true
			}
			break
		}
		if stages[strings.ToLower(ref)] {
			continue // a previous build stage, not an image
		}
		if expanded, ok := expandVars(ref, vars); ok {
			ref = expanded
		}
		if r, ok := ident.ParseImageRef(ref); ok {
			w.image(r, rel, lineOf(txt, m[2]), "")
		}
	}
	// a python the image installs as a distro package rides the OS
	// lifecycle (microdnf python38 on oraclelinux:8)
	scriptTxt := txt
	if m := reDistroPyInstall.FindStringSubmatch(txt); m != nil {
		tok := "python" + m[1] + "." + m[2]
		scriptTxt = strings.ReplaceAll(txt, tok, strings.Repeat(" ", len(tok)))
	}
	w.scriptRuntimes(rel, scriptTxt) // RUN nvm install / nodesource setup_18.x / pyenv install
	w.releaseDownloads(rel, txt)
	w.pipInstallRefs(rel, txt)
	// COPY --from=composer:1 /usr/bin/composer: tool images pulled mid-build
	for _, m := range reCopyFrom.FindAllStringSubmatchIndex(txt, -1) {
		ref := txt[m[2]:m[3]]
		if stages[strings.ToLower(ref)] || !strings.Contains(ref, ":") {
			continue
		}
		if r, ok := ident.ParseImageRef(ref); ok {
			w.image(r, rel, lineOf(txt, m[2]), "")
		}
	}
}

var reEnvPair = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)=("[^"]*"|'[^']*'|[^\s"']+)`)

// reSCLPackage: a yum-installed Software Collections runtime (rh-python35).
var reSCLPackage = regexp.MustCompile(`rh-python(\d)(\d+)`)

// reDistroPyInstall: a package-manager python3X names the version the
// distro supplies (dnf install python38).
var reDistroPyInstall = regexp.MustCompile(`(?:apt(?:-get)?|yum|dnf|microdnf|apk)[^\n]*?\bpython3?[.\s-]?(\d)(\d+)\b`)

var reCopyFrom = regexp.MustCompile(`(?im)^\s*COPY\s+--from=(\S+)`)

// phusionUbuntu: phusion/baseimage releases and the Ubuntu they build on.
func phusionUbuntu(tag string) (string, bool) {
	first := strings.SplitN(tag, "-", 2)[0]
	if v, ok := ubuntuCodenames[first]; ok {
		return v, true // focal-1.0.0
	}
	if m := regexp.MustCompile(`^(\d\d\.\d\d)$`).FindString(first); m != "" {
		return m, true // 18.04-1.0.0
	}
	p := strings.Split(first, ".")
	if len(p) < 2 || p[0] != "0" {
		return "", false
	}
	minor, patch := 0, 0
	fmt.Sscan(p[1], &minor)
	if len(p) > 2 {
		fmt.Sscan(p[2], &patch)
	}
	switch {
	case minor == 9 && patch <= 18:
		return "14.04", true
	case minor == 9 || minor == 10:
		return "16.04", true
	case minor == 11:
		return "18.04", true
	}
	return "", false
}

var reNameVersion = regexp.MustCompile(`^(?:openjdk|jdk|java|python|ruby|php|nodejs|node|golang|go|dotnet)-?(\d+(?:\.\d+)?)(?:-|$)`)

// nameVersion reads a runtime version from an image's name segment
// (openjdk-17-runtime, python-3.9-minimal), as Red Hat UBI images spell it.
func nameVersion(base string) (string, bool) {
	segs := strings.Split(base, "-")
	for i := range segs {
		if m := reNameVersion.FindStringSubmatch(strings.Join(segs[i:], "-")); m != nil {
			return m[1], true
		}
	}
	return "", false
}

var reROSName = regexp.MustCompile(`(?:^|-)ros-(indigo|jade|kinetic|lunar|melodic|noetic)(?:-|$)`)

var ros2Distros = map[string]bool{
	"ardent": true, "bouncy": true, "crystal": true, "dashing": true, "eloquent": true, "foxy": true,
	"galactic": true, "humble": true, "iron": true, "jazzy": true, "kilted": true, "lyrical": true, "rolling": true,
}
