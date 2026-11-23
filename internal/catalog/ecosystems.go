package catalog

import (
	"encoding/json"
	"path"
	"regexp"
	"strings"

	"github.com/0xbadshah/zephy/internal/ident"
)

// keyLine finds the line of the first `"key"` occurrence after offset 0,
// for JSON manifests whose decoder does not report positions.
func keyLine(txt, key string) int {
	loc := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `"\s*:`).FindStringIndex(txt)
	if loc == nil {
		return 0
	}
	return lineOf(txt, loc[0])
}

// keyLineIn finds "key": inside the "section": object.
func keyLineIn(txt, section, key string) int {
	sec := regexp.MustCompile(`"` + regexp.QuoteMeta(section) + `"\s*:\s*\{`).FindStringIndex(txt)
	if sec == nil {
		return keyLine(txt, key)
	}
	loc := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `"\s*:`).FindStringIndex(txt[sec[1]:])
	if loc == nil {
		return keyLine(txt, key)
	}
	return lineOf(txt, sec[1]+loc[0])
}

// jsonSection decodes one top-level string map of a JSON manifest.
func jsonSection(txt, section string) map[string]string {
	var doc map[string]json.RawMessage
	if json.Unmarshal([]byte(txt), &doc) != nil {
		return nil
	}
	var out map[string]string
	json.Unmarshal(doc[section], &out)
	return out
}

// ---- JavaScript ----

// vendoredJSPackage: a library copied into a static-asset directory
// (assets/reveal.js/package.json) is somebody else's manifest.
func vendoredJSPackage(rel, name string, private bool) bool {
	if private || name == "" {
		return false // "private": true is the project's own sub-app
	}
	assetSegs := 0
	for _, seg := range strings.Split(strings.ToLower(path.Dir(path.Dir(rel))), "/") {
		switch seg {
		case "assets", "static", "public", "lib", "libs", "js", "plugin", "plugins", "vendors", "external", "extern", "deps":
			assetSegs++
		}
	}
	// a library copied under a web asset tree: named for its directory
	// (assets/js/select2/), or nested deep in one (assets/js/reveal/plugin/x/)
	return assetSegs >= 2 || assetSegs == 1 && strings.EqualFold(path.Base(path.Dir(rel)), name)
}

func parsePackageJSON(w *repoWalk, rel, _, txt string) {
	var doc struct {
		Name             string            `json:"name"`
		Private          any               `json:"private"`
		Engines          json.RawMessage   `json:"engines"`
		Volta            map[string]string `json:"volta"`
		PackageManager   string            `json:"packageManager"`
		Dependencies     map[string]string `json:"dependencies"`
		DevDependencies  map[string]string `json:"devDependencies"`
		PeerDependencies map[string]string `json:"peerDependencies"`
		Pkg              struct {
			Targets []string `json:"targets"`
		} `json:"pkg"`
	}
	if json.Unmarshal([]byte(txt), &doc) != nil {
		// hand-edited manifests with trailing commas still describe the build
		if json.Unmarshal([]byte(reTrailingComma.ReplaceAllString(txt, "$1")), &doc) != nil {
			return
		}
	}
	// scaffolding tools write "private": "true" as a string; either form
	// means the same thing
	private := false
	switch v := doc.Private.(type) {
	case bool:
		private = v
	case string:
		private = strings.EqualFold(strings.TrimSpace(v), "true")
	}
	if vendoredJSPackage(rel, doc.Name, private) {
		return
	}
	var engines map[string]string
	if json.Unmarshal(doc.Engines, &engines) == nil {
		if spec, ok := engines["node"]; ok {
			w.runtime("nodejs", spec, styleNPM, rel, keyLineIn(txt, "engines", "node"))
		}
	}
	if v, ok := doc.Volta["node"]; ok {
		w.runtime("nodejs", v, styleNPM, rel, keyLineIn(txt, "volta", "node"))
	}
	// vercel/pkg bundles this Node into the shipped binary: node16-win-x64
	for _, t := range doc.Pkg.Targets {
		if m := rePkgTarget.FindStringSubmatch(t); m != nil {
			w.pin("nodejs", m[1], rel, keyLine(txt, "targets"), "pkg target "+t)
		}
	}
	// electron-packager --electron-version=2.0.0 in an npm script
	for _, m := range reElectronFlag.FindAllStringSubmatchIndex(txt, -1) {
		w.pin("electron", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "--electron-version "+txt[m[2]:m[3]])
	}
	if pm := doc.PackageManager; pm != "" {
		if name, ver, ok := strings.Cut(pm, "@"); ok && (name == "yarn" || name == "pnpm") {
			w.pin(name, strings.SplitN(ver, "+", 2)[0], rel, keyLine(txt, "packageManager"), pm)
		}
	}
	for sname, section := range map[string]map[string]string{"dependencies": doc.Dependencies, "devDependencies": doc.DevDependencies, "peerDependencies": doc.PeerDependencies} {
		for name, spec := range section {
			w.dep("npm", strings.ToLower(name), spec, styleNPM, rel, keyLineIn(txt, sname, name))
		}
	}
}

var reTrailingComma = regexp.MustCompile(`,(\s*[}\]])`)

var rePkgTarget = regexp.MustCompile(`^node(\d+)(?:-|$)`)

var reElectronFlag = regexp.MustCompile(`--electron-version[= ]v?(\d+(?:\.\d+){0,2})`)

// ---- PHP ----

func parseComposerJSON(w *repoWalk, rel, _, txt string) {
	var doc struct {
		Require    map[string]string `json:"require"`
		RequireDev map[string]string `json:"require-dev"`
		Config     struct {
			Platform map[string]string `json:"platform"`
		} `json:"config"`
	}
	if json.Unmarshal([]byte(txt), &doc) != nil {
		return
	}
	for sname, section := range map[string]map[string]string{"require": doc.Require, "require-dev": doc.RequireDev} {
		for name, spec := range section {
			switch {
			case name == "php":
				w.runtime("php", spec, styleComposer, rel, keyLineIn(txt, sname, "php"))
			case strings.HasPrefix(name, "ext-") || strings.HasPrefix(name, "lib-"):
			default:
				w.dep("composer", strings.ToLower(name), spec, styleComposer, rel, keyLineIn(txt, sname, name))
			}
		}
	}
	if v, ok := doc.Config.Platform["php"]; ok {
		w.runtime("php", v, styleComposer, rel, keyLine(txt, "platform"))
	}
}

// ---- Ruby ----

var (
	reGemSpec       = regexp.MustCompile(`^\s{4}([a-zA-Z0-9_.\-]+)\s+\(([^)]+)\)`)
	reGemSpecDep    = regexp.MustCompile(`^\s{6}([a-zA-Z0-9_.\-]+)(?:\s|$)`)
	reGemDependency = regexp.MustCompile(`^\s{2}([a-zA-Z0-9_.\-]+)!?(?:\s|$)`)
	reGemSection    = regexp.MustCompile(`^(GIT|PATH|GEM|PLUGIN|PLUGIN SOURCE|BUNDLED WITH|RUBY VERSION|DEPENDENCIES|PLATFORMS|CHECKSUMS)$`)
	reLockRuby      = regexp.MustCompile(`RUBY VERSION\s*\n\s*ruby\s+(\d+\.\d+(?:\.\d+)?)p?\d*`)
	reGemfileRuby   = regexp.MustCompile(`(?m)^\s*ruby\s+['"]([^'"]+)['"]`)
	reGemfileGem    = regexp.MustCompile(`(?m)^\s*gem\s+['"]([^'"]+)['"]((?:\s*,\s*['"][^'"]*['"])*)`)
	reQuoted        = regexp.MustCompile(`['"]([^'"]*)['"]`)
)

// parseGemfileLock reads a Gemfile.lock section by section. GEM specs are
// public gems; GIT and PATH sections are first-party pins (the org's own
// gems from git remotes or a repo folder) and resolving those by name
// against public RubyGems finds somebody else's project, so they are
// skipped, as are gems the repository itself defines.
func parseGemfileLock(w *repoWalk, rel, _, txt string) {
	if m := reLockRuby.FindStringSubmatchIndex(txt); m != nil {
		w.pin("ruby", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), rel)
	}
	lines := strings.Split(txt, "\n")
	// DEPENDENCIES lists what the Gemfile asks for: everything else in the
	// GEM section is transitive
	direct := map[string]bool{}
	section := ""
	for _, line := range lines {
		if reGemSection.MatchString(line) {
			section = strings.TrimSpace(line)
			continue
		}
		if section == "DEPENDENCIES" {
			if m := reGemDependency.FindStringSubmatch(line); m != nil {
				direct[m[1]] = true
			}
		}
		// the repository's own gem (PATH section) depends on these
		// directly through its gemspec
		if section == "PATH" {
			if m := reGemSpecDep.FindStringSubmatch(line); m != nil {
				direct[m[1]] = true
			}
		}
	}
	section = ""
	for i, line := range lines {
		if reGemSection.MatchString(line) {
			section = strings.TrimSpace(line)
			continue
		}
		if section != "GEM" {
			continue
		}
		m := reGemSpec.FindStringSubmatch(line)
		if m == nil || w.selfGems[m[1]] || (len(direct) > 0 && !direct[m[1]]) {
			continue
		}
		w.pkg("gem", m[1], m[2], rel, i+1)
	}
}

func parseGemfile(w *repoWalk, rel, _, txt string) {
	for _, m := range reGemfileRuby.FindAllStringSubmatchIndex(txt, -1) {
		w.runtime("ruby", txt[m[2]:m[3]], styleGem, rel, lineOf(txt, m[2]))
	}
	for _, m := range reGemfileGem.FindAllStringSubmatchIndex(txt, -1) {
		name := txt[m[2]:m[3]]
		if _, ok := librarySlug("gem", name); !ok {
			continue // version ranges of other gems are curated-list input via the lockfile
		}
		var specs []string
		for _, q := range reQuoted.FindAllStringSubmatch(txt[m[4]:m[5]], -1) {
			specs = append(specs, q[1])
		}
		if len(specs) > 0 {
			w.dep("gem", name, strings.Join(specs, ","), styleGem, rel, lineOf(txt, m[2]))
		}
	}
}

// ---- Python ----

var reReqInclude = regexp.MustCompile(`^-(?:r|c|-requirement|-constraint)\s*=?\s*(\S+\.(?:txt|in))`)

// referenceRequirements notes a requirements file another file installs
// (-r base.txt, pip install -r pip_pkgs.txt): parsed after the walk
// whatever it is called.
func (w *repoWalk) referenceRequirements(p string) {
	if w.reqRefs == nil {
		w.reqRefs = map[string]bool{}
	}
	w.reqRefs[path.Clean(p)] = true
}

var rePipInstallR = regexp.MustCompile(`pip3?\s+install\b[^\n]*?\s-r\s+["']?([\w./\-]+\.(?:txt|in))`)

// pipInstallRefs: requirements files installed by scripts and Dockerfiles,
// relative to the repository root.
func (w *repoWalk) pipInstallRefs(rel, txt string) {
	for _, m := range rePipInstallR.FindAllStringSubmatch(txt, -1) {
		w.referenceRequirements(strings.TrimPrefix(m[1], "./"))
	}
	// pip install numpy==1.19.3 django==2.2.*: exact pins installed inline.
	// Continuations become spaces of the same length, so offsets keep lines.
	stripped := stripComments(txt)
	joined := strings.ReplaceAll(stripped, "\\\n", "  ")
	for _, m := range rePipInstallArgs.FindAllStringSubmatchIndex(joined, -1) {
		args := joined[m[2]:m[3]]
		for _, a := range strings.Fields(args) {
			a = strings.Trim(a, `"'`)
			if rePipPinArg.MatchString(a) {
				w.requirement(a, rel, lineOf(stripped, m[2]))
			}
		}
	}
}

var (
	rePipInstallArgs = regexp.MustCompile(`(?:\bpip[\d.]*(?:\s+-\S+)*\s+install|\bpython[\d.]*\s+-m\s+pip(?:\s+-\S+)*\s+install|\$\{?PIP_INSTALL\}?)\s+([^\n&;|]*)`)
	rePipPinArg      = regexp.MustCompile(`^[A-Za-z0-9][\w.\-]*(?:\[[\w,\-]*\])?(?:==|~=)\d[\w.*]*$`)
)

var reReqLine = regexp.MustCompile(`^\s*([A-Za-z0-9][A-Za-z0-9._\-]*)\s*(?:\[[^\]]*\])?\s*([~=!<>][^;#]*)?`)

// requirement parses one PEP 508 requirement: name[extras] specifiers ; markers
func (w *repoWalk) requirement(req, rel string, line int) {
	req = strings.TrimSpace(req)
	if m := reReqInclude.FindStringSubmatch(req); m != nil {
		w.referenceRequirements(path.Join(path.Dir(rel), m[1]))
		return
	}
	if req == "" || strings.HasPrefix(req, "-") || strings.HasPrefix(req, "#") || strings.Contains(req, "://") {
		return
	}
	m := reReqLine.FindStringSubmatch(req)
	if m == nil || m[2] == "" {
		return
	}
	spec := strings.TrimSpace(m[2])
	if i := strings.Index(spec, " --"); i >= 0 {
		spec = spec[:i]
	}
	w.dep("pypi", ident.NormalizePyPI(m[1]), spec, stylePEP440, rel, line)
}

func parseRequirements(w *repoWalk, rel, _, txt string) {
	for i, line := range strings.Split(txt, "\n") {
		w.requirement(line, rel, i+1)
	}
}

// tomlSections splits a TOML document into [section] -> key/value lines,
// enough for the flat tables manifests use. Multi-line arrays are joined.
type tomlKV struct {
	section, key, value string
	line                int
}

var reTomlKV = regexp.MustCompile(`^\s*("?[A-Za-z0-9_.\-]+"?)\s*=\s*(.*)$`)

func tomlPairs(txt string) []tomlKV {
	var out []tomlKV
	section := ""
	lines := strings.Split(txt, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "[") && !strings.HasPrefix(line, "[[") {
			section = strings.Trim(line, "[] ")
			continue
		}
		if strings.HasPrefix(line, "[[") {
			section = strings.Trim(line, "[] ")
			continue
		}
		m := reTomlKV.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		val, start := m[2], i
		if strings.HasPrefix(strings.TrimSpace(val), "[") {
			for !strings.Contains(stripTomlComment(val), "]") && i+1 < len(lines) {
				i++
				val += "\n" + lines[i]
			}
		}
		out = append(out, tomlKV{section, strings.Trim(m[1], `"`), val, start + 1})
	}
	return out
}

// stripTomlComment drops # comments line by line, outside strings; the
// line structure (and so every offset's line) is kept.
func stripTomlComment(s string) string {
	lines := strings.Split(s, "\n")
	for li, line := range lines {
		inStr := byte(0)
		for i := 0; i < len(line); i++ {
			c := line[i]
			switch {
			case inStr != 0 && c == inStr:
				inStr = 0
			case inStr == 0 && (c == '"' || c == '\''):
				inStr = c
			case inStr == 0 && c == '#':
				line = line[:i]
			}
		}
		lines[li] = line
	}
	return strings.Join(lines, "\n")
}

var reTomlString = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"|'([^']*)'`)

// tomlStringLine: lines between a multi-line value's start and its n-th string.
func tomlStringLine(v string, n int) int {
	clean := stripTomlComment(v)
	locs := reTomlString.FindAllStringIndex(clean, -1)
	if n >= len(locs) {
		return 0
	}
	return strings.Count(clean[:locs[n][0]], "\n")
}

func tomlStrings(v string) []string {
	var out []string
	for _, m := range reTomlString.FindAllStringSubmatch(stripTomlComment(v), -1) {
		out = append(out, m[1]+m[2])
	}
	return out
}

// tomlVersion reads `"spec"` or `{ version = "spec", ... }`.
func tomlVersion(v string) (string, bool) {
	v = strings.TrimSpace(stripTomlComment(v))
	if strings.HasPrefix(v, "{") {
		if m := regexp.MustCompile(`version\s*=\s*["']([^"']+)["']`).FindStringSubmatch(v); m != nil {
			return m[1], true
		}
		return "", false
	}
	if s := tomlStrings(v); len(s) == 1 {
		return s[0], true
	}
	return "", false
}

func parsePipfile(w *repoWalk, rel, _, txt string) {
	for _, kv := range tomlPairs(txt) {
		switch kv.section {
		case "requires":
			if kv.key == "python_version" || kv.key == "python_full_version" {
				if v, ok := tomlVersion(kv.value); ok {
					w.runtime("python", v, stylePEP440, rel, kv.line)
				}
			}
		case "packages", "dev-packages":
			if v, ok := tomlVersion(kv.value); ok {
				w.dep("pypi", ident.NormalizePyPI(kv.key), v, stylePEP440, rel, kv.line)
			}
		}
	}
}

func parsePyproject(w *repoWalk, rel, _, txt string) {
	for _, kv := range tomlPairs(txt) {
		switch {
		case kv.section == "project" && kv.key == "requires-python":
			if v, ok := tomlVersion(kv.value); ok {
				w.runtime("python", v, stylePEP440, rel, kv.line)
			}
		case kv.section == "project" && kv.key == "dependencies",
			strings.HasPrefix(kv.section, "project.optional-dependencies"),
			kv.section == "dependency-groups",
			kv.section == "build-system" && kv.key == "requires":
			for i, req := range tomlStrings(kv.value) {
				w.requirement(req, rel, kv.line+tomlStringLine(kv.value, i))
			}
		case kv.section == "tool.cibuildwheel" && kv.key == "build":
			w.cibwBuild(kv.value, rel, kv.line)
		case kv.section == "tool.poetry.dependencies" || kv.section == "tool.poetry.dev-dependencies" ||
			(strings.HasPrefix(kv.section, "tool.poetry.group.") && strings.HasSuffix(kv.section, ".dependencies")):
			v, ok := tomlVersion(kv.value)
			if !ok {
				continue
			}
			if kv.key == "python" {
				w.runtime("python", v, styleNPM, rel, kv.line)
			} else {
				w.dep("pypi", ident.NormalizePyPI(kv.key), v, styleNPM, rel, kv.line)
			}
		}
	}
}

var (
	reQuotedRequirement   = regexp.MustCompile(`['"]([A-Za-z][A-Za-z0-9._\-]*(?:\[[^\]'"]*\])?\s*(?:==|~=|<=|<|>=|>|!=)[^'"]*)['"]`)
	reSetupPythonRequires = regexp.MustCompile(`python_requires\s*=\s*['"]([^'"]+)['"]`)
)

// parseSetupPy: requirement strings anywhere in setup.py (install_requires
// is often a module-level list), and python_requires.
func parseSetupPy(w *repoWalk, rel, _, txt string) {
	for _, q := range reQuotedRequirement.FindAllStringSubmatchIndex(txt, -1) {
		w.requirement(txt[q[2]:q[3]], rel, lineOf(txt, q[2]))
	}
	if m := reSetupPythonRequires.FindStringSubmatchIndex(txt); m != nil {
		w.runtime("python", txt[m[2]:m[3]], stylePEP440, rel, lineOf(txt, m[2]))
	}
}

var reCondaDep = regexp.MustCompile(`^\s*-\s*["']?(?:[\w.\-]+::)?([A-Za-z0-9][A-Za-z0-9._\-]*)\s*(==?|>=|<=|~=)?\s*([0-9][^\s=#'"]*)?`)

var (
	reCondaChannels = regexp.MustCompile(`(?m)^channels\s*:`)
	reCondaDeps     = regexp.MustCompile(`(?m)^dependencies\s*:`)
)

// parseCondaEnvByContent: a conda environment is any YAML with top-level
// dependencies (and channels or a name), whatever the file is called.
func parseCondaEnvByContent(w *repoWalk, rel, name, txt string) {
	if !reCondaDeps.MatchString(txt) {
		return
	}
	if !reCondaChannels.MatchString(txt) && !isCondaEnv(rel, name) && !strings.HasPrefix(txt, "name:") {
		return
	}
	parseCondaEnv(w, rel, name, txt)
}

// parseCondaEnv reads conda environment files: "python=3.6" pins the 3.6
// line (conda's single = is a prefix match); pip: sub-lists are PEP 508.
func parseCondaEnv(w *repoWalk, rel, _, txt string) {
	inPip := false
	pipIndent := 0
	for i, line := range strings.Split(txt, "\n") {
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if strings.TrimSpace(line) == "- pip:" {
			inPip, pipIndent = true, indent
			continue
		}
		if inPip && indent <= pipIndent && strings.TrimSpace(line) != "" {
			inPip = false
		}
		if inPip {
			w.requirement(strings.TrimPrefix(strings.TrimSpace(line), "- "), rel, i+1)
			continue
		}
		m := reCondaDep.FindStringSubmatch(line)
		if m == nil || m[3] == "" {
			continue
		}
		spec := "==" + m[3]
		if m[2] == "=" || m[2] == "" {
			spec = "==" + strings.TrimSuffix(m[3], ".*") + ".*"
			if strings.Count(m[3], ".") >= 2 {
				spec = "==" + m[3]
			}
		} else if m[2] != "==" {
			spec = m[2] + m[3]
		}
		if strings.EqualFold(m[1], "python") {
			w.runtime("python", spec, stylePEP440, rel, i+1)
		} else {
			w.dep("conda", ident.NormalizePyPI(m[1]), spec, stylePEP440, rel, i+1)
		}
		// ros-conda-mutex=1.0=melodic: the ROS distro the env pins
		if dm := reCondaROSDistro.FindStringSubmatch(line); dm != nil {
			w.pin("ros", dm[1], rel, i+1, line)
		}
	}
}

// reCondaROSDistro: a ROS distro named in a conda package build string.
var reCondaROSDistro = regexp.MustCompile(`(?i)ros[-_](?:conda[-_]mutex|roscpp|\w+)=\d[\w.]*=(indigo|jade|kinetic|lunar|melodic|noetic)`)

// ---- Go ----

var (
	reGoDirective = regexp.MustCompile(`(?m)^go\s+(\d+\.\d+)`)
	reGoToolchain = regexp.MustCompile(`(?m)^toolchain\s+go(\d+\.\d+(?:\.\d+)?)`)
	reGoRequire   = regexp.MustCompile(`(?m)^(?:\t|require\s+)(\S+)\s+(v\S+)(.*)$`)
)

func parseGoMod(w *repoWalk, rel, _, txt string) {
	if m := reGoDirective.FindStringSubmatchIndex(txt); m != nil {
		w.pin("go", txt[m[2]:m[3]], rel, lineOf(txt, m[2]),
			"go.mod directive: minimum language version, not a runtime pin")
	}
	if m := reGoToolchain.FindStringSubmatchIndex(txt); m != nil {
		w.pin("go", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "go.mod toolchain")
	}
	for _, m := range reGoRequire.FindAllStringSubmatchIndex(txt, -1) {
		if strings.Contains(txt[m[6]:m[7]], "indirect") || txt[m[2]:m[3]] == "(" {
			continue
		}
		w.pkg("golang", txt[m[2]:m[3]], strings.TrimPrefix(txt[m[4]:m[5]], "v"), rel, lineOf(txt, m[2]))
	}
	// k8s.io/api and controller-runtime are client libraries, not a
	// cluster version: the server pin comes from CI (minikube jobs,
	// kubectl installs) instead.
}

// ---- version-manager files ----

var reToolVers = regexp.MustCompile(`(?m)^([a-z][a-z0-9-]*)\s+(\S+)`)

var toolVersionProducts = map[string]string{
	"ruby": "ruby", "nodejs": "nodejs", "node": "nodejs", "python": "python", "golang": "go", "go": "go",
	"terraform": "terraform", "java": "java", "elixir": "elixir", "erlang": "erlang", "php": "php",
	"deno": "deno", "bun": "bun", "kotlin": "kotlin", "gradle": "gradle", "maven": "apache-maven",
	"rust": "rust", "scala": "scala", "yarn": "yarn", "pnpm": "pnpm", "postgres": "postgresql",
	"redis": "redis", "dotnet": "dotnet", "dotnet-core": "dotnet", "helm": "", "kubectl": "kubernetes",
}

var reLeadingVersion = regexp.MustCompile(`(\d+(?:\.\d+)*)`)

func parseToolVersions(w *repoWalk, rel, _, raw string) {
	txt := stripComments(raw) // comments are cut per line: offsets keep their lines
	for _, m := range reToolVers.FindAllStringSubmatchIndex(txt, -1) {
		tool, ver := txt[m[2]:m[3]], txt[m[4]:m[5]]
		product := toolVersionProducts[tool]
		if product == "" {
			continue
		}
		v := reLeadingVersion.FindString(ver)
		if v == "" || ver == "system" || strings.HasPrefix(ver, "ref:") {
			continue
		}
		if product == "java" {
			v = javaVersion(v)
		}
		w.pin(product, v, rel, lineOf(txt, m[2]), tool+" "+ver)
	}
}

var reSdkman = regexp.MustCompile(`(?m)^(java|gradle|maven|kotlin|scala)\s*=\s*(\S+)`)

func parseSdkmanrc(w *repoWalk, rel, _, txt string) {
	products := map[string]string{"java": "java", "gradle": "gradle", "maven": "apache-maven", "kotlin": "kotlin", "scala": "scala"}
	for _, m := range reSdkman.FindAllStringSubmatchIndex(txt, -1) {
		v := reLeadingVersion.FindString(txt[m[4]:m[5]])
		if v == "" {
			continue
		}
		p := products[txt[m[2]:m[3]]]
		if p == "java" {
			v = javaVersion(v)
		}
		w.pin(p, v, rel, lineOf(txt, m[2]), txt[m[2]:m[5]])
	}
}

var reRustChannel = regexp.MustCompile(`(?m)^\s*(?:channel\s*=\s*["'])?(\d+\.\d+(?:\.\d+)?)`)

func parseRustToolchain(w *repoWalk, rel, _, txt string) {
	if m := reRustChannel.FindStringSubmatchIndex(txt); m != nil {
		w.pin("rust", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "rust-toolchain")
	}
}

// ---- Terraform ----

var (
	reTfLock       = regexp.MustCompile(`(?ms)provider\s+"([^"]+)"\s*\{[^}]*?version\s*=\s*"([^"]+)"`)
	reTfRequired   = regexp.MustCompile(`(?m)required_version\s*=\s*"([^"]+)"`)
	reTfRuntime    = regexp.MustCompile(`(?m)^\s*runtime\s*=\s*"([^"]+)"`)
	reTfEKSVersion = regexp.MustCompile(`(?m)^\s*(?:cluster_version|kubernetes_version)\s*=\s*"(\d+\.\d+)"`)
)

// parseTerraformLock: provider lifecycle dates are not in the dataset, so
// every pinned provider becomes a visible untracked candidate.
func parseTerraformLock(w *repoWalk, rel, _, txt string) {
	for _, m := range reTfLock.FindAllStringSubmatchIndex(txt, -1) {
		prov := txt[m[2]:m[3]]
		w.untracked(prov+"@"+txt[m[4]:m[5]], prov[strings.LastIndex(prov, "/")+1:], txt[m[4]:m[5]],
			rel, lineOf(txt, m[2]), "terraform provider: no lifecycle data")
	}
}

func parseTerraform(w *repoWalk, rel, _, txt string) {
	for _, m := range reTfRequired.FindAllStringSubmatchIndex(txt, -1) {
		w.runtime("terraform", txt[m[2]:m[3]], styleGem, rel, lineOf(txt, m[2]))
	}
	for _, m := range reTfRuntime.FindAllStringSubmatchIndex(txt, -1) {
		if v := txt[m[2]:m[3]]; reLambdaRuntime.MatchString(v) {
			w.pin("aws-lambda", v, rel, lineOf(txt, m[2]), "runtime = "+v)
		}
	}
	w.tfManagedEngines(rel, txt)
	w.cloudOSRefs(rel, txt)
	if strings.Contains(txt, "eks") {
		for _, m := range reTfEKSVersion.FindAllStringSubmatchIndex(txt, -1) {
			w.pin("amazon-eks", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "EKS cluster version")
		}
	}
}

// ---- engines and build systems ----

var (
	reGodotFeatures = regexp.MustCompile(`config/features\s*=\s*PackedStringArray\(\s*"(\d+\.\d+)"`)
	reGodotConfig   = regexp.MustCompile(`(?m)^config_version\s*=\s*(\d+)`)
	reCMakeQt       = regexp.MustCompile(`(?i)find_package\s*\(\s*Qt(\d)(?:\D|$)`)
)

// parseGodotProject: Godot 4 records its engine line in config/features;
// a Godot 3 project (config_version=4) only says "3".
func parseGodotProject(w *repoWalk, rel, _, txt string) {
	if m := reGodotFeatures.FindStringSubmatchIndex(txt); m != nil {
		w.pin("godot", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "config/features "+txt[m[2]:m[3]])
		return
	}
	if m := reGodotConfig.FindStringSubmatchIndex(txt); m != nil && txt[m[2]:m[3]] == "4" {
		w.pin("godot", "3", rel, lineOf(txt, m[2]), "config_version=4 (Godot 3)")
	}
}

// parseCMake: find_package(Qt5) builds against Qt 5, a release line that
// is past EOL as a whole.
func parseCMake(w *repoWalk, rel, _, raw string) {
	// Qt4/Qt5 fallback branches are common: the highest major is the build
	txt := stripComments(raw)
	best, line := "", 0
	for _, m := range reCMakeQt.FindAllStringSubmatchIndex(txt, -1) {
		if v := txt[m[2]:m[3]]; v > best {
			best, line = v, lineOf(txt, m[2])
		}
	}
	if best != "" {
		w.pin("qt", best, rel, line, "find_package(Qt"+best+")")
	}
	// find_package(Lua51), find_package(Lua 5.3 EXACT): the Lua API built against
	w.luaPkgConfig(rel, txt)
	if m := reCMakeLua.FindStringSubmatchIndex(txt); m != nil {
		v := ""
		if m[2] >= 0 {
			v = txt[m[2]:m[3]] + "." + txt[m[4]:m[5]]
		} else if m[6] >= 0 && strings.Contains(txt[m[0]:m[1]], "EXACT") {
			v = txt[m[6]:m[7]]
		}
		if v != "" {
			w.pin("lua", v, rel, lineOf(txt, m[0]), txt[m[0]:m[1]])
		}
	}
	// find_package(PythonLibs 2.7), find_package(Python 3.8 EXACT): the
	// interpreter the native module builds against
	for _, m := range reCMakePython.FindAllStringSubmatchIndex(txt, -1) {
		w.pin("python", txt[m[2]:m[3]], rel, lineOf(txt, m[0]), txt[m[0]:m[1]])
	}
	// set(CMAKE_PREFIX_PATH "/opt/Qt/Qt.6.11.1-static"): a Qt install
	// path names the release built against
	for _, m := range reCMakeQtPath.FindAllStringSubmatchIndex(txt, -1) {
		w.pin("qt", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "CMAKE_PREFIX_PATH Qt."+txt[m[2]:m[3]])
	}
}

// reCMakePython: find_package(PythonLibs 2.7), find_package(Python 3.8 EXACT).
var reCMakePython = regexp.MustCompile(`(?i)find_package\s*\(\s*Python(?:Libs|Interp)?3?\s+(\d+(?:\.\d+)?)`)

// reCMakeQtPath: Qt/Qt.6.11.1-static in a prefix path.
var reCMakeQtPath = regexp.MustCompile(`(?i)Qt\.(\d+\.\d+(?:\.\d+)?)`)

// reLuaPkg: pkg-config names the Lua API a native module builds against.
var reLuaPkg = regexp.MustCompile(`(?:pkg-config|pkg_check_modules|pkg_search_module)[^\n]*?\blua-?(5\.\d)\b`)

func (w *repoWalk) luaPkgConfig(rel, txt string) {
	if m := reLuaPkg.FindStringSubmatchIndex(txt); m != nil {
		w.pin("lua", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "pkg-config lua"+txt[m[2]:m[3]])
	}
}

var reCMakeLua = regexp.MustCompile(`(?i)find_package\s*\(\s*Lua(?:(5)(\d)\b|\s+(5\.\d)\b[^)]*)`)

var rePreCommitPython = regexp.MustCompile(`(?m)^\s*(?:language_version|python)\s*:\s*["']?python(\d\.\d+)`)

// parsePreCommit: hooks pinned to a Python (language_version: python3.8).
func parsePreCommit(w *repoWalk, rel, _, txt string) {
	for _, m := range rePreCommitPython.FindAllStringSubmatchIndex(txt, -1) {
		w.pin("python", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "pre-commit python"+txt[m[2]:m[3]])
	}
	// mirrors-eslint rev: v8.44.0 runs that ESLint
	for _, m := range rePreCommitESLint.FindAllStringSubmatchIndex(txt, -1) {
		w.pin("eslint", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "mirrors-eslint "+txt[m[2]:m[3]])
	}
	// additional_dependencies: the tool the hook runs, pip or npm style
	// (- "eslint@9.35.0", - mypy==1.4.1)
	for _, m := range rePreCommitDep.FindAllStringSubmatchIndex(txt, -1) {
		name, ver := txt[m[2]:m[3]], txt[m[6]:m[7]]
		if m[4] < 0 {
			continue
		}
		if txt[m[4]:m[5]] == "==" {
			w.dep("pypi", strings.ToLower(name), ver, stylePEP440, rel, lineOf(txt, m[2]))
		} else {
			w.dep("npm", strings.ToLower(name), ver, styleNPM, rel, lineOf(txt, m[2]))
		}
	}
}

// rePreCommitDep: an additional_dependencies entry, pip (==) or npm (@).
var rePreCommitDep = regexp.MustCompile(`(?m)^\s*-\s*["']?(@?[\w./-]+?)(==|@)(\d[\w.+-]*)["']?\s*$`)

var rePreCommitESLint = regexp.MustCompile(`mirrors-eslint\s*\n\s*rev\s*:\s*["']?v?(\d+\.\d+(?:\.\d+)?)`)

// stackageGHC maps Stackage LTS majors to the GHC line they ship.
var stackageGHC = map[string]string{
	"2": "7.8", "3": "7.10", "4": "7.10", "5": "7.10", "6": "7.10", "7": "8.0", "8": "8.0", "9": "8.0",
	"10": "8.2", "11": "8.2", "12": "8.4", "13": "8.6", "14": "8.6", "15": "8.8", "16": "8.8", "17": "8.10",
	"18": "8.10", "19": "9.0", "20": "9.2", "21": "9.4", "22": "9.6", "23": "9.8", "24": "9.10",
}

var (
	reStackResolver = regexp.MustCompile(`(?m)^\s*(?:resolver|snapshot)\s*:\s*["']?lts-(\d+)\.\d+|stackage-snapshots/[\w/]*?/lts/(\d+)/\d+\.ya?ml`)
	reStackCompiler = regexp.MustCompile(`(?m)^\s*(?:compiler|resolver)\s*:\s*["']?ghc-(\d+\.\d+(?:\.\d+)?)`)
	reBuildozerPy   = regexp.MustCompile(`(?m)^\s*requirements\s*=.*?\bpython3==(\d+\.\d+(?:\.\d+)?)`)
)

var reStackNightly = regexp.MustCompile(`(?m)^\s*(?:resolver|snapshot)\s*:\s*["']?nightly-(20\d\d)-`)

// stackageNightlyGHC: the GHC line Stackage nightlies shipped by year.
var stackageNightlyGHC = map[string]string{
	"2015": "7.10", "2016": "8.0", "2017": "8.2", "2018": "8.6", "2019": "8.8", "2020": "8.10",
	"2021": "9.0", "2022": "9.2", "2023": "9.6", "2024": "9.8", "2025": "9.10",
}

var reStackLocalSnapshot = regexp.MustCompile(`(?m)^\s*(?:resolver|snapshot)\s*:\s*["']?([\w./\-]+\.ya?ml)`)

func parseStackYaml(w *repoWalk, rel, name, txt string) {
	// a custom snapshot file names the real resolver: follow it once
	if m := reStackLocalSnapshot.FindStringSubmatch(txt); m != nil && name == "stack.yaml" {
		if snap, ok := w.read(path.Join(path.Dir(rel), m[1])); ok {
			parseStackYaml(w, path.Join(path.Dir(rel), m[1]), "snapshot", snap)
		}
	}
	if m := reStackNightly.FindStringSubmatchIndex(txt); m != nil {
		if v, ok := stackageNightlyGHC[txt[m[2]:m[3]]]; ok {
			w.pin("ghc", v, rel, lineOf(txt, m[2]), "nightly-"+txt[m[2]:m[3]])
		}
	}
	if m := reStackCompiler.FindStringSubmatchIndex(txt); m != nil {
		w.pin("ghc", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "ghc-"+txt[m[2]:m[3]])
		return
	}
	if m := reStackResolver.FindStringSubmatchIndex(txt); m != nil {
		g := 2 // lts-20.11, or the resolver url .../lts/20/11.yaml
		if m[g] < 0 {
			g = 4
		}
		if v, ok := stackageGHC[txt[m[g]:m[g+1]]]; ok {
			w.pin("ghc", v, rel, lineOf(txt, m[g]), "lts-"+txt[m[g]:m[g+1]]+" (GHC "+v+")")
		}
	}
}

func parseBuildozer(w *repoWalk, rel, _, txt string) {
	if m := reBuildozerPy.FindStringSubmatchIndex(txt); m != nil {
		w.pin("python", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "python3=="+txt[m[2]:m[3]])
	}
}

// anacondaPython: the Python an Anaconda distribution release ships, for
// pyenv names like anaconda3-2020.02.
var anacondaPython = map[string]string{
	"5.0": "3.6", "5.1": "3.6", "5.2": "3.6", "5.3": "3.7", "2018.12": "3.7", "2019.03": "3.7",
	"2019.07": "3.7", "2019.10": "3.7", "2020.02": "3.7", "2020.07": "3.8", "2020.11": "3.8",
	"2021.04": "3.8", "2021.05": "3.8", "2021.11": "3.9", "2022.05": "3.9", "2022.10": "3.9",
	"2023.03": "3.10", "2023.07": "3.11", "2023.09": "3.11", "2024.02": "3.11", "2024.06": "3.12",
	"2024.10": "3.12",
}

var reAnaconda = regexp.MustCompile(`^\s*anaconda3-(\d+\.\d+)`)

func parsePythonVersionFile(w *repoWalk, rel, _, txt string) {
	for i, line := range strings.Split(txt, "\n") {
		line = stripComments(line)
		if m := reLineVersion.FindStringSubmatch(line); m != nil {
			w.pin("python", m[1], rel, i+1, rel)
		} else if m := reAnaconda.FindStringSubmatch(line); m != nil {
			if v, ok := anacondaPython[m[1]]; ok {
				w.pin("python", v, rel, i+1, strings.TrimSpace(line))
			}
		}
	}
}

var reActionNode = regexp.MustCompile(`(?m)^\s*using\s*:\s*["']?node(\d+)["']?\s*$`)

// parseActionMetadata: a JavaScript action runs on the Node its
// action.yml names (runs.using: node16).
func parseActionMetadata(w *repoWalk, rel, _, txt string) {
	for _, m := range reActionNode.FindAllStringSubmatchIndex(txt, -1) {
		w.pin("nodejs", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "runs.using: node"+txt[m[2]:m[3]])
	}
	// a composite action's steps use setup actions like a workflow job
	if steps := yamlGet(yamlGet(yamlRoot(txt), "runs"), "steps"); steps != nil {
		w.setupSteps(steps, nil, rel)
	}
	w.scriptRuntimes(rel, txt)
}

var (
	reMixDep    = regexp.MustCompile(`\{\s*:([a-z_][a-z0-9_]*)\s*,\s*"([^"]+)"`)
	reMixElixir = regexp.MustCompile(`\belixir:\s*"([^"]+)"`)
)

// parseMixExs: Elixir project deps ({:phoenix, "~> 1.3.2"}) and the
// required Elixir range.
func parseMixExs(w *repoWalk, rel, _, txt string) {
	for _, m := range reMixDep.FindAllStringSubmatchIndex(txt, -1) {
		w.dep("hex", txt[m[2]:m[3]], txt[m[4]:m[5]], styleGem, rel, lineOf(txt, m[2]))
	}
	if m := reMixElixir.FindStringSubmatchIndex(txt); m != nil {
		w.runtime("elixir", txt[m[2]:m[3]], styleGem, rel, lineOf(txt, m[2]))
	}
}

// parseMise: mise.toml [tools] pins runtimes like .tool-versions does.
func parseMise(w *repoWalk, rel, _, txt string) {
	for _, kv := range tomlPairs(txt) {
		if kv.section != "tools" {
			continue
		}
		product := toolVersionProducts[kv.key]
		if product == "" {
			continue
		}
		v, ok := tomlVersion(kv.value)
		if !ok {
			if s := tomlStrings(kv.value); len(s) > 0 {
				v, ok = s[0], true
			}
		}
		if !ok {
			continue
		}
		if lv := reLeadingVersion.FindString(v); lv != "" {
			if product == "java" {
				lv = javaVersion(lv)
			}
			w.pin(product, lv, rel, kv.line, kv.key+" = "+v)
		}
	}
}

var reAmperJdk = regexp.MustCompile(`(?m)^\s*jdk:\s*\n\s+version:\s*["']?(\d+)`)

// parseAmper: Amper module files pin the JDK (jvm: jdk: version: 21).
func parseAmper(w *repoWalk, rel, _, txt string) {
	if m := reAmperJdk.FindStringSubmatchIndex(txt); m != nil {
		w.pin("java", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "amper jdk "+txt[m[2]:m[3]])
	}
}
