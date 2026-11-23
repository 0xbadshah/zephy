package catalog

import (
	"path"
	"regexp"
	"strings"

	"github.com/0xbadshah/zephy/internal/ident"
	"gopkg.in/yaml.v3"
)

// ---- version variables ----

// A variable named for a product's version (PYTHON_VERSION, NODE_VERSION,
// ENVTEST_K8S_VERSION) is how Dockerfiles, CI configs and Makefiles pin
// the runtimes their scripts install. The name decides the product.
var reVersionVar = regexp.MustCompile(`^(?:[A-Z0-9]+_)*?(PYTHON|NODE|NODEJS|RUBY|GO|GOLANG|PHP|JAVA|JDK|OPENJDK|TERRAFORM|KUBECTL|KUBERNETES|K8S|MAVEN|GRADLE|KOTLIN|DOTNET|DOTNET_SDK|POSTGRES|POSTGRESQL|PG|MYSQL|MARIADB|REDIS|MONGO|MONGODB|DJANGO|RAILS|ELIXIR|ERLANG|OTP|RUST|DENO|BUN|YARN|PNPM|ALPINE|DEBIAN|UBUNTU|NGINX|TOMCAT|ELASTICSEARCH|NUMPY|LARAVEL|SYMFONY|ANGULAR|ELECTRON|JULIA|REDMINE|TYPO3|DRUPAL|WORDPRESS|MAGENTO|NVM|GIT|SOLR|ARANGO|K3S|KUBE|PROMETHEUS|GRAFANA|GRAYLOG|KIBANA|LOGSTASH|RABBITMQ|NGINX|HAPROXY|TRAEFIK|CONSUL|VAULT|KEYCLOAK|TOMCAT|ZOOKEEPER|KAFKA|SPARK|HADOOP|CASSANDRA|INFLUXDB|TELEGRAF|BIND|BIND9|QT|OPENSSL|ETCD|DOCKER|VSCMD)_?(?:VERSION|VER)$`)

var versionVarProducts = map[string]string{
	"PYTHON": "python", "NODE": "nodejs", "NODEJS": "nodejs", "RUBY": "ruby", "GO": "go", "GOLANG": "go",
	"PHP": "php", "JAVA": "java", "JDK": "java", "OPENJDK": "java", "TERRAFORM": "terraform",
	"KUBECTL": "kubernetes", "KUBERNETES": "kubernetes", "K8S": "kubernetes", "MAVEN": "apache-maven",
	"GRADLE": "gradle", "KOTLIN": "kotlin", "DOTNET": "dotnet", "DOTNET_SDK": "dotnet",
	"POSTGRES": "postgresql", "POSTGRESQL": "postgresql", "PG": "postgresql", "MYSQL": "mysql",
	"MARIADB": "mariadb", "REDIS": "redis", "MONGO": "mongodb", "MONGODB": "mongodb", "DJANGO": "django",
	"RAILS": "rails", "ELIXIR": "elixir", "ERLANG": "erlang", "OTP": "erlang", "RUST": "rust", "DENO": "deno",
	"BUN": "bun", "YARN": "yarn", "PNPM": "pnpm", "ALPINE": "alpine-linux", "DEBIAN": "debian",
	"UBUNTU": "ubuntu", "NGINX": "nginx", "TOMCAT": "tomcat", "ELASTICSEARCH": "elasticsearch",
	"NUMPY": "numpy", "LARAVEL": "laravel", "SYMFONY": "symfony", "ANGULAR": "angular", "ELECTRON": "electron",
	"JULIA": "julia", "REDMINE": "redmine", "TYPO3": "typo3", "DRUPAL": "drupal", "WORDPRESS": "wordpress",
	"MAGENTO": "magento", "NVM": "nvm", "GIT": "git", "SOLR": "solr", "ARANGO": "arangodb", "K3S": "kubernetes", "KUBE": "kubernetes",
	"PROMETHEUS": "prometheus", "GRAFANA": "grafana", "GRAYLOG": "graylog", "KIBANA": "kibana", "LOGSTASH": "logstash",
	"RABBITMQ": "rabbitmq", "HAPROXY": "haproxy", "TRAEFIK": "traefik", "CONSUL": "consul", "VAULT": "hashicorp-vault",
	"KEYCLOAK": "keycloak", "ZOOKEEPER": "zookeeper", "KAFKA": "apache-kafka", "SPARK": "apache-spark",
	"HADOOP": "apache-hadoop", "CASSANDRA": "apache-cassandra", "INFLUXDB": "influxdb", "TELEGRAF": "telegraf", "BIND": "bind-9", "BIND9": "bind-9", "ETCD": "etcd", "DOCKER": "docker-engine", "QT": "qt", "OPENSSL": "openssl", "VSCMD": "visual-studio",
}

var floorTokens = map[string]bool{"MIN": true, "MINIMUM": true, "REQUIRED": true, "LEAST": true, "LOWEST": true, "OLDEST": true}

var reVarValue = regexp.MustCompile(`^v?(\d+(?:\.\d+){0,2})(?:$|[-+._ a-zA-Z])`)

// versionVar records NAME=value when the name pins a product's version.
func (w *repoWalk) versionVar(name, value, rel string, line int) {
	m := reVersionVar.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(name)))
	if m == nil {
		return
	}
	for _, tok := range strings.Split(m[0], "_") {
		if floorTokens[tok] {
			return // REQUIRED_NODE_VERSION, MIN_GO_VERSION: a floor check, not an install
		}
	}
	value = strings.Trim(strings.TrimSpace(value), `"'`)
	if strings.Contains(value, "$") {
		return
	}
	v := reVarValue.FindStringSubmatch(value)
	if v == nil {
		return
	}
	product, version := versionVarProducts[m[1]], v[1]
	switch product {
	case "java":
		version = javaVersion(version)
	case "dotnet":
		if p := strings.Split(version, "."); len(p) >= 2 {
			version = p[0] + "." + p[1]
		}
	}
	w.pin(product, version, rel, line, name+"="+value)
}

// yamlVersionVars: CI env blocks, compose build args, any YAML mapping
// key named for a product version; Travis-style "A=1 B=2" env strings.
func (w *repoWalk) yamlVersionVars(root *yaml.Node, rel string) {
	if strings.HasPrefix(strings.ToLower(path.Base(rel)), "taskfile") {
		return // task runners pass these to flags (go mod tidy -compat), not installers
	}
	yamlWalk(root, func(key string, val *yaml.Node, _ *yaml.Node) {
		if val.Kind == yaml.ScalarNode && reVersionVar.MatchString(strings.ToUpper(key)) {
			w.versionVar(key, val.Value, rel, val.Line)
		}
		if key == "env" || key == "global" || key == "matrix" {
			for _, s := range yamlScalars(val) {
				for _, kv := range strings.Fields(s.Value) {
					if name, value, ok := strings.Cut(kv, "="); ok {
						w.versionVar(name, value, rel, s.Line)
						if name == "TOXENV" { // TOXENV=py27,py36 picks the CI Python
							for _, env := range strings.Split(value, ",") {
								if f := reToxPy.FindStringSubmatch(strings.SplitN(env, "-", 2)[0]); f != nil {
									w.pin("python", f[1]+"."+f[2], rel, s.Line, "TOXENV="+value)
								}
							}
						}
					}
				}
			}
		}
	})
}

var reMakeListVar = regexp.MustCompile(`(?m)^[ \t]*(?:export[ \t]+)?([A-Z][A-Z0-9_]*_VERSIONS)[ \t]*(?:\?=|:=|::=|=)[ \t]*([^\n#]+)`)

var reMakeVar = regexp.MustCompile(`(?m)^\s*(?:export\s+)?([A-Z][A-Z0-9_]*)\s*(?:\?=|:=|::=|=)\s*(\S+)`)

// parseMakefile: version variables, and image references to tracked
// images (golang:1.15 in a docker run recipe).
func parseMakefile(w *repoWalk, rel, _, txt string) {
	for _, m := range reMakeVar.FindAllStringSubmatchIndex(txt, -1) {
		w.versionVar(txt[m[2]:m[3]], txt[m[4]:m[5]], rel, lineOf(txt, m[2]))
	}
	// ETCD_VERSIONS := 3.2.32 3.3.27: a test matrix, one pin per entry
	for _, m := range reMakeListVar.FindAllStringSubmatchIndex(txt, -1) {
		for _, v := range strings.Fields(txt[m[4]:m[5]]) {
			w.versionVar(strings.TrimSuffix(txt[m[2]:m[3]], "S"), v, rel, lineOf(txt, m[2]))
		}
	}
	w.trackedImageRefs(rel, txt)
	w.releaseDownloads(rel, txt)
	w.pipInstallRefs(rel, txt)
	w.luaPkgConfig(rel, stripComments(txt))
	w.scriptRuntimes(rel, txt) // python3.9 -m pip build recipes
	w.npmInstallRefs(rel, txt)
}

var reLooseImageRef = regexp.MustCompile(`(?:^|[\s"'=(/])((?:[a-z0-9.\-]+(?::\d+)?/)?[a-z0-9][a-z0-9._\-/]*:v?\d[\w.\-]*)`)

// trackedImageRefs finds image references in free text (Makefiles,
// scripts) and keeps only those naming a tracked image: free text holds
// too many host:port strings to trust anything else.
func (w *repoWalk) trackedImageRefs(rel, txt string) {
	for _, m := range reLooseImageRef.FindAllStringSubmatchIndex(txt, -1) {
		r, ok := ident.ParseImageRef(txt[m[2]:m[3]])
		if !ok || r.Tag == "" {
			continue
		}
		if _, mapped := imageProduct(r); !mapped {
			continue
		}
		w.image(r, rel, lineOf(txt, m[2]), "")
	}
}

// ---- CI shell steps ----

var reScriptRuntime = []struct {
	re      *regexp.Regexp
	product string
}{
	{regexp.MustCompile(`conda (?:create|install)[^\n]*?\bpython\s*=\s*=?\s*(\d\.\d+)`), "python"},
	{regexp.MustCompile(`\bnvm (?:install|use) v?(\d+(?:\.\d+){0,2})\b`), "nodejs"},
	{regexp.MustCompile(`\.nvm/versions/node/v(\d+\.\d+\.\d+)/`), "nodejs"},
	{regexp.MustCompile(`\bpyenv (?:install|global|local) (\d\.\d+(?:\.\d+)?)\b`), "python"},
	{regexp.MustCompile(`\brvm (?:install|use) (?:ruby-)?(\d\.\d+(?:\.\d+)?)\b`), "ruby"},
	{regexp.MustCompile(`\brbenv (?:install|global|local)(?: -[sfv])* (\d\.\d+(?:\.\d+)?)\b`), "ruby"},
	{regexp.MustCompile(`set\s+:(?:rbenv_ruby|rvm_ruby_version|rvm_ruby_string),\s*["'](?:ruby-)?(\d\.\d+(?:\.\d+)?)`), "ruby"},
	{regexp.MustCompile(`(?:deb|rpm)\.nodesource\.com/setup_(\d+)\.x`), "nodejs"},
	{regexp.MustCompile(`\brustup (?:default|toolchain install|install|override set)\s+(\d+\.\d+(?:\.\d+)?)\b`), "rust"},
	{regexp.MustCompile(`(?s)npm_config_runtime=["']?electron\b.{0,200}?npm_config_target=["']?(\d+\.\d+(?:\.\d+)?)|npm_config_target=["']?(\d+\.\d+(?:\.\d+)?).{0,200}?npm_config_runtime=["']?electron\b`), "electron"},
	{regexp.MustCompile(`\bnpm (?:i|install|add)\b[^\n]*?\beslint@(\d+(?:\.\d+){0,2})`), "eslint"},
	{regexp.MustCompile(`\bmodule load python/v?(\d+(?:\.\d+){0,2})`), "python"},
	{regexp.MustCompile(`\bmodule load (?:node|nodejs)/v?(\d+(?:\.\d+){0,2})`), "nodejs"},
	{regexp.MustCompile(`\bmodule load (?:ruby|r)/v?(\d+(?:\.\d+){0,2})`), "ruby"},
	{regexp.MustCompile(`\bmodule load go/v?(\d+(?:\.\d+){0,2})`), "go"},
	{regexp.MustCompile(`\bmodule load ghc/v?(\d+(?:\.\d+){0,2})`), "ghc"},
	{regexp.MustCompile(`\bpython(\d\.\d+)(?:\.\d+)?\s+(?:-m\s+)?pip install\b`), "python"},
	{regexp.MustCompile(`\buv (?:run|sync|pip|python)\b[^\n]*?--python[= ](\d\.\d+)`), "python"},
	{regexp.MustCompile(`\buv python install (\d\.\d+)`), "python"},
}

// thirdPartyPPAs: packages from these PPAs are upstream builds the PPA
// maintains, not the distribution's own, so they carry their own lifecycle.
var thirdPartyPPAs = []struct {
	ppa     string
	re      *regexp.Regexp
	product string
}{
	{"ppa:ondrej/php", regexp.MustCompile(`\bapt(?:-get)?\s+(?:-\S+\s+)*install\b[^\n]*?\bphp(\d\.\d)\b`), "php"},
	{"ppa:webupd8team/java", regexp.MustCompile(`\boracle-java(\d+)-installer\b`), "java"},
	{"ppa:deadsnakes/ppa", regexp.MustCompile(`\bapt(?:-get)?\s+(?:-\S+\s+)*install\b[^\n]*?\bpython(3\.\d+)\b`), "python"},
}

func (w *repoWalk) scriptRuntimes(rel, txt string) {
	joined := strings.ReplaceAll(txt, "\\\n", "  ") // same length: offsets keep their lines
	for _, p := range thirdPartyPPAs {
		if !strings.Contains(txt, p.ppa) {
			continue
		}
		for _, m := range p.re.FindAllStringSubmatchIndex(joined, -1) {
			w.pin(p.product, joined[m[2]:m[3]], rel, lineOf(txt, m[2]), p.ppa+" "+p.product+joined[m[2]:m[3]])
		}
	}
	for _, s := range reScriptRuntime {
		for _, m := range s.re.FindAllStringSubmatchIndex(txt, -1) {
			for g := 2; g+1 < len(m); g += 2 { // the alternative that matched
				if m[g] >= 0 {
					w.pin(s.product, txt[m[g]:m[g+1]], rel, lineOf(txt, m[g]), strings.TrimSpace(txt[m[0]:m[1]]))
					break
				}
			}
		}
	}
	// for v in 18 20 22; do nvm install $v; done: each listed version installs
	for _, m := range reForLoop.FindAllStringSubmatchIndex(txt, -1) {
		v, list, body := txt[m[2]:m[3]], txt[m[4]:m[5]], txt[m[6]:m[7]]
		for _, inst := range loopInstallers {
			if !regexp.MustCompile(inst.cmd + `\s+["']?\$\{?` + v + `\b`).MatchString(body) {
				continue
			}
			for _, x := range strings.Fields(list) {
				w.pin(inst.product, strings.TrimPrefix(x, "v"), rel, lineOf(txt, m[4]), inst.cmd+" "+x)
			}
		}
	}
}

var (
	reForLoop      = regexp.MustCompile(`(?s)\bfor\s+(\w+)\s+in\s+((?:v?\d+(?:\.\d+)*\s+)*v?\d+(?:\.\d+)*)\s*;?\s*(?:\\\s*)?do\b(.{0,400}?)\bdone\b`)
	loopInstallers = []struct{ cmd, product string }{{`nvm install`, "nodejs"}, {`pyenv install`, "python"}, {`rvm install`, "ruby"}}
)

// isCIFile: files CI systems execute.
func isCIFile(rel, n string) bool {
	return isWorkflow(rel, n) || rel == ".travis.yml" || isGitLabCI(rel, n) || strings.HasPrefix(rel, ".circleci/") ||
		strings.HasPrefix(n, "azure-pipelines") || n == "bitbucket-pipelines.yml" || n == ".drone.yml" ||
		strings.HasPrefix(n, "buildspec") || n == "appveyor.yml" || n == ".appveyor.yml" || n == "cloudbuild.yaml" ||
		n == "cloudbuild.yml" || n == "jenkinsfile" || n == ".gitpod.yml" || n == "codemagic.yaml"
}

func parseCIScripts(w *repoWalk, rel, _, txt string) {
	w.scriptRuntimes(rel, txt)
	w.pipInstallRefs(rel, txt)
	w.releaseDownloads(rel, txt)
	if m := reCIBW.FindAllStringSubmatchIndex(txt, -1); m != nil {
		for _, x := range m {
			w.cibwBuild(txt[x[2]:x[3]], rel, lineOf(txt, x[2]))
		}
	}
	// a CircleCI orb job pins the runtime it tests: - node/test: with
	// version: 14.18.3 (the orbs: block's own version is the orb's, not
	// the runtime's)
	for _, m := range reCircleOrbJob.FindAllStringSubmatchIndex(txt, -1) {
		if product, ok := matrixProducts[strings.ToLower(txt[m[2]:m[3]])]; ok {
			w.runtime(product, txt[m[4]:m[5]], styleNPM, rel, lineOf(txt, m[4]))
		}
	}
}

// reCircleOrbJob: "- node/test:" followed by "version: 14.18.3".
var reCircleOrbJob = regexp.MustCompile(`(?m)^\s*-\s*([\w-]+)/[\w-]+:\s*\n\s+version:\s*["']?v?(\d[\d.]*)["']?\s*$`)

var (
	reCIBW     = regexp.MustCompile(`(?m)CIBW_BUILD\s*[:=]\s*["']?([^"'\n#]+)`)
	reCIBWTag  = regexp.MustCompile(`\bcp(\d)(\d+)-`)
	reCIBWToml = regexp.MustCompile(`(?m)^\s*build\s*=\s*(.+)$`)
)

// cibwBuild: cibuildwheel selectors (cp38-*) build wheels for CPython 3.8.
func (w *repoWalk) cibwBuild(sel, rel string, line int) {
	sel = strings.Join(expandBraces(sel), " ") // cp3{10,11}-*
	seen := map[string]bool{}
	for _, m := range reCIBWTag.FindAllStringSubmatch(sel+" ", -1) {
		v := m[1] + "." + m[2]
		if !seen[v] {
			seen[v] = true
			w.pin("python", v, rel, line, "cibuildwheel cp"+m[1]+m[2])
		}
	}
}

// ---- assorted declaration formats ----

// Travis osx_image xcodeN runs on the macOS that Xcode image ships.
var travisXcodeMacOS = []struct{ prefix, macos string }{
	{"xcode6", "10.10"}, {"xcode7", "10.11"}, {"xcode8.3", "10.12"}, {"xcode8", "10.11"}, {"xcode9.4", "10.13"},
	{"xcode9.3", "10.13"}, {"xcode9", "10.12"}, {"xcode10.2", "10.14"}, {"xcode10.3", "10.14"}, {"xcode10", "10.13"},
	{"xcode11", "10.14"}, {"xcode12.5", "11"}, {"xcode12", "10.15"}, {"xcode13", "11"}, {"xcode14", "12"},
}

func travisOSXImage(v string) (string, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	for _, x := range travisXcodeMacOS {
		if strings.HasPrefix(v, x.prefix) {
			return x.macos, true
		}
	}
	return "", false
}

var reDabSuite = regexp.MustCompile(`(?m)^Suite:\s*(\w+)`)

// parseDabConf: Proxmox DAB appliance definitions build on a suite.
func parseDabConf(w *repoWalk, rel, _, txt string) {
	for _, m := range reDabSuite.FindAllStringSubmatchIndex(txt, -1) {
		w.codename(txt[m[2]:m[3]], rel, lineOf(txt, m[2]))
	}
}

// codename pins the OS a Debian or Ubuntu release codename names.
func (w *repoWalk) codename(name, rel string, line int) {
	name = strings.ToLower(strings.Trim(strings.TrimSpace(name), `"'`))
	if v, ok := debianCodenames[name]; ok {
		w.pin("debian", v, rel, line, name)
	} else if v, ok := ubuntuCodenames[name]; ok {
		w.pin("ubuntu", v, rel, line, name)
	}
}

var reToolset = regexp.MustCompile(`<PlatformToolset>(?:v(\d+)(?:_xp)?|(Windows(?:KernelMode|UserMode)Driver\d+\.\d+))</PlatformToolset>`)

var toolsetVS = map[string]string{"90": "9.0", "100": "10.0", "110": "11.0", "120": "12.0", "140": "14.0", "141": "15", "142": "16", "143": "17",
	"WindowsKernelModeDriver8.0": "11.0", "WindowsKernelModeDriver8.1": "12.0", "WindowsUserModeDriver8.1": "12.0"}

var (
	reAppveyorVS     = regexp.MustCompile(`^(?:Previous\s+)?Visual Studio (20\d\d)$`)
	reAppveyorPython = regexp.MustCompile(`(?i)^C:\\+Python(\d)(\d+)(?:-x64|-32)?\\*$`)
	reAppveyorUbuntu = regexp.MustCompile(`^(?:Previous\s+)?Ubuntu(\d\d)(\d\d)$`)
	appveyorVS       = map[string]string{"2010": "10.0", "2012": "11.0", "2013": "12.0", "2015": "14.0", "2017": "15", "2019": "16", "2022": "17"}
)

// parseAppveyor: build worker images name the Visual Studio or Ubuntu
// release a job runs on (image: Visual Studio 2015, Ubuntu1804), also per
// matrix entry (APPVEYOR_BUILD_WORKER_IMAGE).
var reAppveyorRuntimeDir = regexp.MustCompile(`(?i)C:\\+Ruby(\d)(\d)(?:-x64)?\\`)

// parseEmberTry: config/ember-try.js scenarios pin the ember-source each
// scenario builds against ("minimum-supported-version" etc.).
var reEmberTryDep = regexp.MustCompile(`(?m)['"]([\w@/.-]+)['"]\s*:\s*['"]([\^~>=]?[\d.][^'"]*)['"]`)

func parseEmberTry(w *repoWalk, rel, _, txt string) {
	for _, m := range reEmberTryDep.FindAllStringSubmatchIndex(txt, -1) {
		name, spec := txt[m[2]:m[3]], txt[m[4]:m[5]]
		if spec == "" || !strings.ContainsAny(spec[:1], "^~>=0123456789") {
			continue
		}
		w.dep("npm", strings.ToLower(name), spec, styleNPM, rel, lineOf(txt, m[2]))
	}
}

// stack: go 1.13 — the preinstalled toolchain a job runs on
var reAppveyorStack = regexp.MustCompile(`(?im)^\s*(?:-\s*)?stack\s*:\s*(go|node|nodejs|python|ruby|java|jdk)\s+(\d[\d.]*)`)

func parseAppveyor(w *repoWalk, rel, _, txt string) {
	for _, m := range reAppveyorRuntimeDir.FindAllStringSubmatchIndex(txt, -1) {
		w.pin("ruby", txt[m[2]:m[3]]+"."+txt[m[4]:m[5]], rel, lineOf(txt, m[2]), txt[m[0]:m[1]]) // PATH=C:\Ruby22\bin
	}
	// stack: go 1.13 — the preinstalled toolchain a job runs on
	for _, m := range reAppveyorStack.FindAllStringSubmatchIndex(txt, -1) {
		if product, ok := matrixProducts[strings.ToLower(txt[m[2]:m[3]])]; ok {
			w.pin(product, txt[m[4]:m[5]], rel, lineOf(txt, m[4]), "stack "+txt[m[2]:m[3]]+" "+txt[m[4]:m[5]])
		}
	}
	yamlWalk(yamlRoot(txt), func(key string, val, _ *yaml.Node) {
		if val.Kind == yaml.ScalarNode {
			// PYTHON: "C:\\Python35-x64": the preinstalled interpreter a job uses
			if m := reAppveyorPython.FindStringSubmatch(strings.TrimSpace(val.Value)); m != nil {
				w.pin("python", m[1]+"."+m[2], rel, val.Line, val.Value)
			}
		}
		if key != "image" && key != "APPVEYOR_BUILD_WORKER_IMAGE" {
			return
		}
		for _, s := range yamlScalars(val) {
			v := strings.TrimSpace(s.Value)
			if m := reAppveyorVS.FindStringSubmatch(v); m != nil && appveyorVS[m[1]] != "" {
				w.pin("visual-studio", appveyorVS[m[1]], rel, s.Line, v)
			} else if m := reAppveyorUbuntu.FindStringSubmatch(v); m != nil {
				w.pin("ubuntu", m[1]+"."+m[2], rel, s.Line, v)
			}
		}
	})
}

// AI model IDs pinned in code and config retire on a published schedule
// (endoflife.date tracks the Claude and OpenAI API models). Only quoted,
// dated or versioned IDs count: "claude-3-5-sonnet-20240620", "gpt-4-0613".
var (
	reClaudeModel   = regexp.MustCompile(`["'\x60](claude-(?:instant-)?\d[\w.\-]*|claude-(?:opus|sonnet|haiku)-\d[\w.\-]*)["'\x60]`)
	reOpenAIModel   = regexp.MustCompile(`["'\x60]((?:gpt-[34][\w.\-]*|gpt-5[\w.\-]*|o[134](?:-mini|-pro)?(?:-\d{4}-\d{2}-\d{2})?|text-(?:davinci|curie|babbage|ada)-\d{3}|code-davinci-\d{3}|chatgpt-4o-latest|codex-mini-latest))["'\x60]`)
	modelSourceExts = map[string]bool{".py": true, ".js": true, ".mjs": true, ".cjs": true, ".ts": true, ".tsx": true, ".go": true, ".rb": true,
		".java": true, ".kt": true, ".cs": true, ".php": true, ".rs": true, ".yaml": true, ".yml": true, ".toml": true}
)

func isModelSource(rel, n string) bool {
	return modelSourceExts[path.Ext(n)] && !isTestPath(rel)
}

func parseModelIDs(w *repoWalk, rel, _, txt string) {
	if len(txt) > 512<<10 || !(strings.Contains(txt, "claude-") || strings.Contains(txt, "gpt-") || strings.Contains(txt, "davinci") ||
		strings.Contains(txt, `"o1`) || strings.Contains(txt, `"o3`) || strings.Contains(txt, `'o1`) || strings.Contains(txt, `'o3`)) {
		return
	}
	type hit struct {
		product, id string
		line        int
	}
	var hits []hit
	seen := map[string]bool{}
	for _, r := range []struct {
		re      *regexp.Regexp
		product string
	}{{reClaudeModel, "claude"}, {reOpenAIModel, "openai-api-models"}} {
		for _, m := range r.re.FindAllStringSubmatchIndex(txt, -1) {
			id := txt[m[2]:m[3]]
			ls := strings.LastIndex(txt[:m[0]], "\n") + 1
			if t := strings.TrimSpace(txt[ls:m[0]]); strings.HasPrefix(t, "#") || strings.HasPrefix(t, "//") {
				continue // commented out
			}
			if !seen[id] {
				seen[id] = true
				hits = append(hits, hit{r.product, id, lineOf(txt, m[2])})
			}
		}
	}
	if len(hits) > 4 {
		return // a model catalog or price table lists them all; it pins none
	}
	for _, h := range hits {
		w.pin(h.product, h.id, rel, h.line, h.id)
	}
}

var (
	reSqlprojDSP = regexp.MustCompile(`<DSP>[\w.]*\.Sql(\d+)0DatabaseSchemaProvider</DSP>`)
	// the release's final service pack: what a maintained server runs
	sqlDSP = map[string]string{"9": "9.0-sp4", "10": "10.0-sp4", "11": "11.0-sp4", "12": "12.0-sp3", "13": "13.0-sp3", "14": "14.0", "15": "15.0", "16": "16.0"}
)

// parseSqlproj: a database project targets one SQL Server release
// (Sql130DatabaseSchemaProvider = SQL Server 2016), and an MSBuild
// TargetFrameworkVersion names the .NET it builds on.
func parseSqlproj(w *repoWalk, rel, _, txt string) {
	if m := reSqlprojDSP.FindStringSubmatchIndex(txt); m != nil {
		if v, ok := sqlDSP[txt[m[2]:m[3]]]; ok {
			w.pin("mssqlserver", v, rel, lineOf(txt, m[2]), txt[m[0]:m[1]])
		}
	}
	for _, m := range reTargetFxVersion.FindAllStringSubmatchIndex(txt, -1) {
		if strings.Contains(txt, "<TargetFrameworkProfile>Profile") {
			break // a Portable Class Library profile, not a release
		}
		w.pin("dotnetfx", fxVersion(txt[m[2]:m[3]]), rel, lineOf(txt, m[2]), "TargetFrameworkVersion v"+txt[m[2]:m[3]])
	}
}

var (
	reWerckerBox     = regexp.MustCompile(`(?m)^\s*box\s*:\s*["']?([^\s"'#]+)`)
	reWerckerBoxID   = regexp.MustCompile(`(?m)^\s*box\s*:\s*\n\s+id\s*:\s*["']?([^\s"'#]+)`)
	reGemspecRubyReq = regexp.MustCompile(`required_ruby_version\s*=\s*\[?\s*["']([^"']+)["']`)
)

// parseWercker: Wercker CI builds in its box image.
func parseWercker(w *repoWalk, rel, _, txt string) {
	for _, re := range []*regexp.Regexp{reWerckerBox, reWerckerBoxID} {
		for _, m := range re.FindAllStringSubmatchIndex(txt, -1) {
			if r, ok := ident.ParseImageRef(txt[m[2]:m[3]]); ok {
				w.image(r, rel, lineOf(txt, m[2]), "")
			}
		}
	}
}

// parseGemspec: required_ruby_version '~> 2.3' allows only Ruby 2.x.
func parseGemspec(w *repoWalk, rel, _, txt string) {
	if m := reGemspecRubyReq.FindStringSubmatchIndex(txt); m != nil {
		spec := txt[m[2]:m[3]]
		if v, ok := specVersion(spec, styleGem); ok {
			w.pin("ruby", v, rel, lineOf(txt, m[2]), "required_ruby_version "+spec)
		}
	}
}

var (
	reCDKRuntime = regexp.MustCompile(`\bRuntime\.(NODEJS|PYTHON|JAVA|RUBY|DOTNET_CORE|DOTNET|GO)_(\d+)(?:_(\d+|X))?\b`)
	reSQLPython  = regexp.MustCompile(`(?is)\bLANGUAGE\s+PYTHON\b.{0,400}?\bRUNTIME_VERSION\s*=\s*'?(\d\.\d+)`)
	reBatSet     = regexp.MustCompile(`(?im)^\s*(?:@?set\s+"?|\$env:|\$)([A-Za-z_][A-Za-z0-9_]*)\s*=\s*["']?([^\s"']+)`)
)

// parseCDKRuntimes: AWS CDK picks Lambda runtimes in code
// (lambda.Runtime.NODEJS_10_X = nodejs10.x).
func parseCDKRuntimes(w *repoWalk, rel, _, txt string) {
	if !strings.Contains(txt, "Runtime.") {
		return
	}
	for _, m := range reCDKRuntime.FindAllStringSubmatchIndex(txt, -1) {
		lang, major := strings.ToLower(txt[m[2]:m[3]]), txt[m[4]:m[5]]
		minor := ""
		if m[6] >= 0 {
			minor = txt[m[6]:m[7]]
		}
		var cycle string
		switch lang {
		case "nodejs":
			cycle = "nodejs" + major + ".x"
		case "python", "ruby":
			if minor == "" || minor == "X" {
				continue
			}
			cycle = lang + major + "." + minor
		case "java":
			cycle = "java" + major
		case "dotnet_core":
			cycle = "dotnetcore" + major + "." + minor
		case "dotnet":
			cycle = "dotnet" + major
		case "go":
			cycle = "go1.x"
		}
		w.pin("aws-lambda", cycle, rel, lineOf(txt, m[2]), txt[m[0]:m[1]])
	}
}

// parseSQLRuntimes: Snowflake UDFs and procedures name their Python
// runtime (LANGUAGE PYTHON RUNTIME_VERSION = '3.8').
func parseSQLRuntimes(w *repoWalk, rel, _, txt string) {
	for _, m := range reSQLPython.FindAllStringSubmatchIndex(txt, -1) {
		w.pin("python", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "RUNTIME_VERSION "+txt[m[2]:m[3]])
	}
}

// parseWindowsScript: batch and PowerShell build scripts set version
// variables (set QT_VERSION=5.12.3) and download releases.
func parseWindowsScript(w *repoWalk, rel, _, txt string) {
	for _, m := range reBatSet.FindAllStringSubmatchIndex(txt, -1) {
		name := txt[m[2]:m[3]]
		if strings.Contains(txt, "%"+name+"%") || strings.Contains(txt, "$env:"+name) || strings.Contains(txt, "$"+name) {
			w.versionVar(name, txt[m[4]:m[5]], rel, lineOf(txt, m[2]))
		}
	}
	w.releaseDownloads(rel, txt)
}

// parsePackerTemplate: Packer builds machine images from an install ISO
// (openSUSE-Leap-15.0-NET-x86_64.iso).
func parsePackerTemplate(w *repoWalk, rel, _, txt string) {
	if strings.Contains(txt, "iso_url") || strings.Contains(txt, "source_ami") {
		w.releaseDownloads(rel, txt)
	}
}

// parseMoonToolchain: moon installs the toolchain versions it lists
// (node: version: 18.12.0, rust: version: 1.99.0).
func parseMoonToolchain(w *repoWalk, rel, _, txt string) {
	root := yamlRoot(txt)
	for key, product := range map[string]string{"node": "nodejs", "rust": "rust", "python": "python", "deno": "deno", "bun": "bun", "go": "go"} {
		if v := yamlGet(yamlGet(root, key), "version"); v != nil && v.Kind == yaml.ScalarNode {
			w.runtime(product, v.Value, styleNPM, rel, v.Line)
		}
	}
}

// parseVcxproj: the platform toolset names the Visual Studio release a C++
// project builds with (v100 = Visual Studio 2010).
func parseVcxproj(w *repoWalk, rel, _, txt string) {
	seen := map[string]bool{}
	for _, m := range reToolset.FindAllStringSubmatch(txt, -1) {
		key := m[1] + m[2]
		v, ok := toolsetVS[key]
		if !ok || seen[v] {
			continue
		}
		seen[v] = true
		w.pin("visual-studio", v, rel, lineOf(txt, strings.Index(txt, m[0])), "PlatformToolset "+key)
	}
}

var (
	reWDLDocker   = regexp.MustCompile(`\bdocker\s*:\s*["']([^"'\s$~]+)["']`)
	reNFContainer = regexp.MustCompile(`\bcontainer\s*=?\s*["']([^"'\s$]+)["']`)
	reCWLDocker   = regexp.MustCompile(`\bdockerPull\s*:\s*["']?([^"'\s]+)`)
)

// parseWorkflowLang: bioinformatics workflow languages pin task images
// (WDL runtime docker, Nextflow container, CWL dockerPull).
func parseWorkflowLang(w *repoWalk, rel, name, txt string) {
	re := reWDLDocker
	switch {
	case strings.HasSuffix(name, ".nf") || name == "nextflow.config":
		re = reNFContainer
	case strings.HasSuffix(name, ".cwl"):
		re = reCWLDocker
	}
	for _, m := range re.FindAllStringSubmatchIndex(txt, -1) {
		if r, ok := ident.ParseImageRef(txt[m[2]:m[3]]); ok {
			w.image(r, rel, lineOf(txt, m[2]), "")
		}
	}
}

func parseBower(w *repoWalk, rel, _, txt string) {
	deps := jsonSection(txt, "dependencies")
	for name, spec := range deps {
		w.dep("bower", strings.ToLower(name), spec, styleNPM, rel, keyLineIn(txt, "dependencies", name))
	}
	for name, spec := range jsonSection(txt, "devDependencies") {
		w.dep("bower", strings.ToLower(name), spec, styleNPM, rel, keyLineIn(txt, "devDependencies", name))
	}
}

// parseReadTheDocs: the Python (and OS) the docs build runs on.
func parseReadTheDocs(w *repoWalk, rel, _, txt string) {
	root := yamlRoot(txt)
	for _, n := range []*yaml.Node{yamlGet(yamlGet(root, "python"), "version"), yamlGet(yamlGet(yamlGet(root, "build"), "tools"), "python")} {
		if n != nil && n.Kind == yaml.ScalarNode {
			w.runtime("python", n.Value, styleNPM, rel, n.Line)
		}
	}
	if n := yamlGet(yamlGet(yamlGet(root, "build"), "tools"), "nodejs"); n != nil && n.Kind == yaml.ScalarNode {
		w.runtime("nodejs", n.Value, styleNPM, rel, n.Line)
	}
	if n := yamlGet(yamlGet(root, "build"), "os"); n != nil && n.Kind == yaml.ScalarNode {
		w.runner(n.Value, rel, n.Line)
	}
}

var (
	reToxSection = regexp.MustCompile(`(?m)^\[testenv(?::[^\]]*)?\]`)
	reToxDeps    = regexp.MustCompile(`(?m)^deps\s*=\s*(.*(?:\n[ \t]+.*)*)`)
	reToxFactor  = regexp.MustCompile(`^[\w,{}.\-]+:\s*`)
)

// toxDeps: [testenv] deps lines are requirements the test matrix installs.
func (w *repoWalk) toxDeps(rel, txt string) {
	for _, m := range reToxDeps.FindAllStringSubmatchIndex(txt, -1) {
		if !reToxSection.MatchString(txt[:m[0]]) {
			continue
		}
		block := txt[m[2]:m[3]]
		for i, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			line = reToxFactor.ReplaceAllString(line, "")
			if line == "" || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "{") {
				continue
			}
			w.requirement(line, rel, lineOf(txt, m[2])+i)
		}
	}
}

// ---- Terraform managed databases ----

var (
	reTfEngine        = regexp.MustCompile(`^\s*engine\s*=\s*"([\w-]+)"`)
	reTfEngineVersion = regexp.MustCompile(`^\s*engine_version\s*=\s*"(\d+(?:\.\d+)*)"`)
)

var tfEngines = map[string]string{
	"mysql": "amazon-rds-mysql", "postgres": "amazon-rds-postgresql", "mariadb": "amazon-rds-mariadb",
	"aurora-mysql": "amazon-aurora-mysql", "aurora-postgresql": "amazon-aurora-postgresql", "redis": "amazon-elasticache-redis",
}

// tfManagedEngines pairs engine and engine_version inside one block.
func (w *repoWalk) tfManagedEngines(rel, txt string) {
	engine, engineLine := "", -100
	for i, line := range strings.Split(txt, "\n") {
		if m := reTfEngine.FindStringSubmatch(line); m != nil {
			engine, engineLine = m[1], i
			continue
		}
		if m := reTfEngineVersion.FindStringSubmatch(line); m != nil && engine != "" && i-engineLine <= 40 {
			if product, ok := tfEngines[engine]; ok {
				w.pin(product, m[1], rel, i+1, engine+" "+m[1])
			}
		}
	}
}

var (
	reCfnEngine        = regexp.MustCompile(`^\s*"?Engine"?\s*:\s*["']?([\w-]+)`)
	reCfnEngineVersion = regexp.MustCompile(`^\s*"?EngineVersion"?\s*:\s*["']?(\d+(?:\.\d+)*)`)
	reCfnParam         = regexp.MustCompile(`^\s*"?(\w*Engine(?:Version)?)"?\s*:\s*\{?\s*$`)
	reCfnDefault       = regexp.MustCompile(`^\s*"?Default"?\s*:\s*["']?([\w.\-]+)`)
	reNixosChannel     = regexp.MustCompile(`nixpkgs[/:-](?:nixos|release)-(\d\d\.\d\d)`)
)

// cfnManagedEngines: CloudFormation RDS/ElastiCache Engine + EngineVersion.
func (w *repoWalk) cfnManagedEngines(rel, txt string) {
	engine, engineLine := "", -100
	param, paramLine := "", -100
	for i, line := range strings.Split(txt, "\n") {
		// template parameters: DBEngine / DBEngineVersion with a Default
		if m := reCfnParam.FindStringSubmatch(line); m != nil {
			param, paramLine = m[1], i
			continue
		}
		if m := reCfnDefault.FindStringSubmatch(line); m != nil && param != "" && i-paramLine <= 6 {
			if strings.HasSuffix(strings.ToLower(param), "engineversion") {
				if product, ok := tfEngines[engine]; ok && i-engineLine <= 60 {
					w.pin(product, m[1], rel, i+1, engine+" "+m[1])
				}
			} else if strings.HasSuffix(strings.ToLower(param), "engine") {
				engine, engineLine = strings.ToLower(m[1]), i
			}
			param = ""
			continue
		}
		if m := reCfnEngine.FindStringSubmatch(line); m != nil {
			engine, engineLine = strings.ToLower(m[1]), i
			continue
		}
		if m := reCfnEngineVersion.FindStringSubmatch(line); m != nil && engine != "" && i-engineLine <= 30 {
			if product, ok := tfEngines[engine]; ok {
				w.pin(product, m[1], rel, i+1, engine+" "+m[1])
			}
		}
	}
}

func parseCloudFormation(w *repoWalk, rel, _, txt string) {
	if isTestPath(rel) {
		return // sample templates exercising a schema or a linter
	}
	if strings.Contains(txt, "amzn") || strings.Contains(txt, "al2023-ami") || strings.Contains(txt, "ubuntu/images") ||
		strings.Contains(txt, "debian-cloud") || strings.Contains(txt, "cos-cloud") {
		w.cloudOSRefs(rel, txt) // AMI name filters
	}
	if strings.Contains(txt, "EngineVersion") {
		w.cfnManagedEngines(rel, txt)
	}
}

// parseNix: a flake or shell pinned to a NixOS release channel.
func parseNix(w *repoWalk, rel, _, txt string) {
	seen := map[string]bool{}
	for _, m := range reNixosChannel.FindAllStringSubmatchIndex(txt, -1) {
		v := txt[m[2]:m[3]]
		if !seen[v] {
			seen[v] = true
			w.pin("nixos", v, rel, lineOf(txt, m[2]), "nixos-"+v)
		}
	}
}

var (
	reMakeDependsVersion = regexp.MustCompile(`(?m)^\$\(package\)_version\s*=\s*(\d+(?:\.\d+)+)`)
	reCircle1            = regexp.MustCompile(`(?m)^\s{2}(python|node|ruby|php|java)\s*:\s*\n\s+version\s*:\s*["']?([\w.]+)`)
	reScrutinizerImage   = regexp.MustCompile(`(?m)^\s*image\s*:\s*default-(\w+)`)
	reCirrusFreeBSD      = regexp.MustCompile(`freebsd-(\d+)-(\d+)`)
	reOSGiBREE           = regexp.MustCompile(`(?m)^Bundle-RequiredExecutionEnvironment:\s*J(?:ava)?SE-(\d+(?:\.\d+)?)`)
	reJDTCompliance      = regexp.MustCompile(`(?m)^org\.eclipse\.jdt\.core\.compiler\.(?:compliance|source)\s*=\s*(\d+(?:\.\d+)?)`)
	reSingularityFrom    = regexp.MustCompile(`(?mi)^From:\s*(\S+)`)
	reGCEImage           = regexp.MustCompile(`\b(?:debian-cloud/)?debian-(\d+)(?:-[a-z]+)?(?:-v\d+)?\b|\bcos-(?:cloud/cos-)?(\d+)-lts\b`)
	reAMIUbuntu          = regexp.MustCompile(`ubuntu/images/[\w-]+/ubuntu-(\w+)-(\d\d\.\d\d)`)
	reAmazonLinuxStack   = regexp.MustCompile(`Amazon Linux (2023|2)\b|\b(?:amzn(2)|al(2023))-ami|codebuild/amazonlinux(2)-`)
	reKitchenPlatform    = regexp.MustCompile(`(?m)^\s*-?\s*name\s*:\s*["']?(ubuntu|debian|centos)-(\d+(?:\.\d+)?)`)
)

// parseDependsMk: bitcoin-style depends/packages/<name>.mk pin a
// toolchain by file name.
func parseDependsMk(w *repoWalk, rel, name, txt string) {
	product := map[string]string{"rust.mk": "rust", "qt.mk": "qt", "openssl.mk": "openssl", "native_rust.mk": "rust"}[name]
	if product == "" {
		return
	}
	for _, m := range reMakeDependsVersion.FindAllStringSubmatchIndex(txt, -1) {
		w.pin(product, txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "$(package)_version="+txt[m[2]:m[3]])
	}
}

// parseCircle1: CircleCI 1.0 circle.yml machine runtimes.
func parseCircle1(w *repoWalk, rel, _, txt string) {
	products := map[string]string{"python": "python", "node": "nodejs", "ruby": "ruby", "php": "php", "java": "java"}
	for _, m := range reCircle1.FindAllStringSubmatchIndex(txt, -1) {
		w.runtime(products[txt[m[2]:m[3]]], txt[m[4]:m[5]], styleNPM, rel, lineOf(txt, m[4]))
	}
}

var reScrutinizerRuntime = regexp.MustCompile(`(?m)^\s*(php|python|node|ruby)\s*:\s*(?:\n\s*version\s*:\s*)?["']?v?(\d+\.\d+(?:\.\d+)?)`)

func parseScrutinizer(w *repoWalk, rel, _, txt string) {
	for _, m := range reScrutinizerRuntime.FindAllStringSubmatchIndex(txt, -1) {
		product := map[string]string{"php": "php", "python": "python", "node": "nodejs", "ruby": "ruby"}[txt[m[2]:m[3]]]
		w.pin(product, txt[m[4]:m[5]], rel, lineOf(txt, m[4]), txt[m[2]:m[3]]+" "+txt[m[4]:m[5]])
	}
	for _, m := range reScrutinizerImage.FindAllStringSubmatchIndex(txt, -1) {
		w.codename(txt[m[2]:m[3]], rel, lineOf(txt, m[2]))
	}
}

func parseCirrus(w *repoWalk, rel, _, txt string) {
	for _, m := range reCirrusFreeBSD.FindAllStringSubmatchIndex(txt, -1) {
		w.pin("freebsd", txt[m[2]:m[3]]+"."+txt[m[4]:m[5]], rel, lineOf(txt, m[2]), txt[m[0]:m[1]])
	}
}

// parseManifestMF: an OSGi bundle's required execution environment.
func parseManifestMF(w *repoWalk, rel, _, txt string) {
	if m := reOSGiBREE.FindStringSubmatchIndex(txt); m != nil {
		w.pin("java", javaVersion(txt[m[2]:m[3]]), rel, lineOf(txt, m[2]), "BREE JavaSE-"+txt[m[2]:m[3]])
	}
}

// parseJDTPrefs: Eclipse projects pin their Java compliance level.
func parseJDTPrefs(w *repoWalk, rel, _, txt string) {
	// IDE metadata: kept only when the project's Maven/Gradle build does
	// not declare a level itself (decided after the walk)
	if m := reJDTCompliance.FindStringSubmatchIndex(txt); m != nil {
		w.ideJava = append(w.ideJava, idePin{path.Dir(path.Dir(rel)), Pin{Product: "java", Version: javaVersion(txt[m[2]:m[3]]),
			File: loc(rel, lineOf(txt, m[2])), Raw: "jdt compliance " + txt[m[2]:m[3]]}})
	}
}

// idePin is a secondary Java-level declaration (Eclipse settings, an Ant
// build next to Maven/Gradle) for the project in dir.
type idePin struct {
	dir string
	pin Pin
}

// resolveIDEJava adds Eclipse-declared Java levels for projects whose
// build file declares none. A Python SDK recorded the same way is kept
// when no manifest pins Python for that project.
func (w *repoWalk) resolveIDEJava() {
	declared := map[string]map[string]bool{"java": {}, "python": {}}
	for _, p := range w.inv.Pins {
		file := strings.SplitN(p.File, ":", 2)[0]
		base := path.Base(file)
		if p.Product == "java" && (base == "pom.xml" || strings.HasSuffix(base, ".gradle") || strings.HasSuffix(base, ".gradle.kts")) {
			declared["java"][path.Dir(file)] = true // the Maven/Gradle build decides
		}
		if p.Product == "python" && (base == "pyproject.toml" || base == "setup.py" || base == "Pipfile" ||
			strings.HasPrefix(base, "requirements") || base == "environment.yml" || base == "runtime.txt") {
			declared["python"][path.Dir(file)] = true // a manifest pins Python
		}
	}
	for _, p := range w.ideJava {
		covered := false
		for d := p.dir; ; d = path.Dir(d) {
			if declared[p.pin.Product][d] {
				covered = true // this project's (or an enclosing) build decides
				break
			}
			if d == "." || d == "/" {
				break
			}
		}
		if !covered {
			w.inv.Pins = append(w.inv.Pins, p.pin)
		}
	}
}

var (
	reIdeaLangLevel = regexp.MustCompile(`languageLevel="JDK_(\d)(?:_(\d+))?"`)
	reIdeaJdkName   = regexp.MustCompile(`project-jdk-name="([^"]*)"`)
	reIdeaPyVersion = regexp.MustCompile(`(?i)^python\s+(\d+\.\d+)`)
)

// ideaLangLevel: JDK_1_6 is Java 6, JDK_15 is Java 15.
func ideaLangLevel(major, minor string) string {
	if minor != "" {
		return minor // JDK_1_6
	}
	return major // JDK_15
}

// parseIdeaMisc: an IntelliJ project file names the JDK / Python SDK the
// IDE builds with. IDE metadata: kept only when the project has no build
// file declaring the level (decided after the walk).
func parseIdeaMisc(w *repoWalk, rel, _, txt string) {
	dir := path.Dir(path.Dir(rel)) // .idea/ sits in the project root
	if m := reIdeaLangLevel.FindStringSubmatch(txt); m != nil {
		w.ideJava = append(w.ideJava, idePin{dir, Pin{Product: "java", Version: ideaLangLevel(m[1], m[2]),
			File: loc(rel, lineOf(txt, 0)), Raw: "languageLevel JDK_" + m[1] + "_" + m[2]}})
	}
	for _, m := range reIdeaJdkName.FindAllStringSubmatch(txt, -1) {
		name := m[1]
		if pm := reIdeaPyVersion.FindStringSubmatch(name); pm != nil {
			w.ideJava = append(w.ideJava, idePin{dir, Pin{Product: "python", Version: pm[1],
				File: loc(rel, lineOf(txt, 0)), Raw: "project-jdk " + name}})
			continue
		}
		if nm := reBareNumber.FindStringSubmatch(name); nm != nil {
			w.ideJava = append(w.ideJava, idePin{dir, Pin{Product: "java", Version: javaVersion(nm[1]),
				File: loc(rel, lineOf(txt, 0)), Raw: "project-jdk " + name}})
			continue
		}
		if jm := reJavaToken.FindStringSubmatch(strings.ToLower(name)); jm != nil {
			w.ideJava = append(w.ideJava, idePin{dir, Pin{Product: "java", Version: jm[1],
				File: loc(rel, lineOf(txt, 0)), Raw: "project-jdk " + name}})
		}
	}
}

// reBareNumber: project-jdk-name="9" or "1.8".
var reBareNumber = regexp.MustCompile(`^(\d+(?:\.\d+)?)$`)

// parseRequirementsLike: a pip-spec list that misspells its filename
// (requirments.txt) still declares what the project installs. Strict
// shape gate: at least three specifier lines and most non-comment lines
// are specifiers.
var rePipSpecLine = regexp.MustCompile(`(?m)^\s*([A-Za-z0-9_][A-Za-z0-9_.\-]*)\s*((?:==|~=|>=|<=|!=)[A-Za-z0-9.+!]+)\s*$`)

func parseRequirementsLike(w *repoWalk, rel, name, txt string) {
	if isRequirements(rel, name) {
		return // the named parser owns it
	}
	nonEmpty := 0
	for _, l := range strings.Split(txt, "\n") {
		t := strings.TrimSpace(l)
		if t != "" && !strings.HasPrefix(t, "#") {
			nonEmpty++
		}
	}
	specs := rePipSpecLine.FindAllStringSubmatchIndex(txt, -1)
	if len(specs) < 3 || nonEmpty == 0 || len(specs)*10 < nonEmpty*6 {
		return
	}
	for _, m := range specs {
		w.dep("pypi", strings.ToLower(txt[m[2]:m[3]]), txt[m[4]:m[5]], stylePEP440, rel, lineOf(txt, m[2]))
	}
}

// parseSingularity: Apptainer/Singularity definitions bootstrap from an image.
func parseSingularity(w *repoWalk, rel, _, txt string) {
	if !strings.Contains(strings.ToLower(txt), "bootstrap:") {
		return
	}
	for _, m := range reSingularityFrom.FindAllStringSubmatchIndex(txt, -1) {
		if r, ok := ident.ParseImageRef(txt[m[2]:m[3]]); ok {
			w.image(r, rel, lineOf(txt, m[2]), "")
		}
	}
}

// cloudOSRefs: AMI name filters and Beanstalk solution stacks name an OS.
func (w *repoWalk) cloudOSRefs(rel, raw string) {
	txt := stripComments(raw) // comments name AMIs too; lines are kept
	// GCE boot images: debian-cloud/debian-9, cos-cloud/cos-73-lts
	if strings.Contains(txt, "debian-cloud") || strings.Contains(txt, "cos-cloud") || strings.Contains(txt, "-lts") {
		for _, m := range reGCEImage.FindAllStringSubmatchIndex(txt, -1) {
			if m[2] >= 0 && strings.Contains(txt[max(0, m[0]-30):m[0]], "image") {
				w.pin("debian", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), txt[m[0]:m[1]])
			} else if m[4] >= 0 {
				w.pin("cos", "cos-"+txt[m[4]:m[5]], rel, lineOf(txt, m[4]), txt[m[0]:m[1]])
			}
		}
	}
	for _, m := range reAMIUbuntu.FindAllStringSubmatchIndex(txt, -1) {
		w.pin("ubuntu", txt[m[4]:m[5]], rel, lineOf(txt, m[2]), "AMI ubuntu-"+txt[m[2]:m[3]])
	}
	for _, m := range reAmazonLinuxStack.FindAllStringSubmatch(txt, -1) {
		v := m[1] + m[2] + m[3] + m[4]
		w.pin("amazon-linux", v, rel, lineOf(txt, strings.Index(txt, m[0])), m[0])
	}
}

func parseTfvars(w *repoWalk, rel, _, txt string) {
	w.cloudOSRefs(rel, txt)
	w.releaseDownloads(rel, txt)
}

var reJinja = regexp.MustCompile(`\{\{[^}]*\}\}/?`)

// parseAnsibleVars: role and inventory variables that hold image
// references ("{{registry}}/grafana/grafana:4.3.2").
// ansibleRoleSlugs: role/vars folder names that name the product the role
// manages (liquibase/defaults/main.yml pins liquibase_ver: 3.1.1).
var ansibleRoleSlugs = map[string]string{
	"liquibase": "liquibase", "splunk": "splunk", "zookeeper": "zookeeper", "consul": "consul",
	"vault": "hashicorp-vault", "haproxy": "haproxy", "traefik": "traefik", "keycloak": "keycloak",
	"telegraf": "telegraf", "influxdb": "influxdb", "graylog": "graylog", "kibana": "kibana",
	"logstash": "logstash", "prometheus": "prometheus", "grafana": "grafana", "jenkins": "jenkins",
	"solr": "solr", "kafka": "apache-kafka", "spark": "apache-spark", "hadoop": "apache-hadoop",
	"cassandra": "apache-cassandra", "tomcat": "tomcat", "nginx": "nginx", "httpd": "apache-http-server",
	"apache": "apache-http-server", "memcached": "memcached", "varnish": "varnish",
	"postgresql": "postgresql", "postgres": "postgresql", "mysql": "mysql", "mariadb": "mariadb",
	"mongodb": "mongodb", "mongo": "mongodb", "rabbitmq": "rabbitmq", "redis": "redis",
	"docker": "docker-engine", "golang": "go", "nodejs": "nodejs", "node": "nodejs",
	"python": "python", "ruby": "ruby", "php": "php", "java": "java", "jdk": "java",
	"debian": "debian", "ubuntu": "ubuntu", "centos": "centos", "rhel": "rhel",
	"rockylinux": "rocky-linux", "rocky": "rocky-linux", "almalinux": "almalinux", "fedora": "fedora",
}

// parseAnsibleVars: a role's folder names the product it manages, so its
// version keys and versioned upstream filenames are that product's pins
// (liquibase_ver: 3.1.1, splunkforwarder-6.0.1-...deb; an OS role pins a
// distro codename: debian_distro: wheezy).
func parseAnsibleVars(w *repoWalk, rel, _, txt string) {
	code := reJinja.ReplaceAllString(txt, "")
	w.trackedImageRefs(rel, code)
	// group_vars pair an image name and its tag under sibling keys:
	// public_alpine_node: mhart/alpine-node + public_node_tag: 7
	w.ansibleSplitImageVars(rel, code)
	dir := strings.ToLower(path.Base(path.Dir(path.Dir(rel)))) // <slug>/defaults|vars/main.yml
	slug, ok := ansibleRoleSlugs[dir]
	if !ok {
		if s, isProduct := matrixProducts[dir]; isProduct {
			slug, ok = s, true
		}
	}
	if !ok {
		return
	}
	// the key's prefix names the role's own product (liquibase_ver), not a
	// neighbouring product pinned under this role's folder
	for _, m := range reAnsibleRoleVer.FindAllStringSubmatchIndex(code, -1) {
		if strings.EqualFold(code[m[2]:m[3]], dir) {
			w.pin(slug, code[m[4]:m[5]], rel, lineOf(code, m[2]), dir+" role "+code[m[2]:m[3]]+"_ver "+code[m[4]:m[5]])
		}
	}
	// splunkforwarder-6.0.1-189883-linux-2.6-amd64.deb: an upstream
	// release file named after the role's product
	for _, m := range reAnsibleFileVer.FindAllStringSubmatchIndex(code, -1) {
		if strings.Contains(strings.ToLower(code[m[0]:m[1]]), dir) {
			w.pin(slug, code[m[2]:m[3]], rel, lineOf(code, m[2]), dir+" "+code[m[0]:m[1]])
		}
	}
	// group_vars pair an image name and its tag under sibling keys:
	// public_alpine_node: mhart/alpine-node + public_node_tag: 7
	w.ansibleSplitImageVars(rel, code)
	if slug == "debian" || slug == "ubuntu" {
		for _, m := range reAnsibleDistro.FindAllStringSubmatchIndex(code, -1) {
			w.codename(strings.ToLower(code[m[2]:m[3]]), rel, lineOf(code, m[2]))
		}
	}
}

var (
	reAnsibleRoleVer = regexp.MustCompile(`(?m)^\s*([\w-]+)[ _-]ver(?:sion)?\s*:\s*["']?v?(\d[\w.\-+]*)`)
	reAnsibleFileVer = regexp.MustCompile(`(?m)^\s*[\w-]*(?:file|url|installer|package|archive)[\w-]*\s*:.*?[\w-]*-(\d+\.\d+(?:\.\d+)?)[\w.-]*\.(?:deb|rpm|tar\.gz|tgz|zip)`)
	reAnsibleDistro  = regexp.MustCompile(`(?m)^\s*[\w-]*(?:distro|distribution|release|suite|codename)\s*:\s*["']?([\w.]+)`)
)

var (
	reAnsibleImgKey = regexp.MustCompile(`(?m)^\s*([\w.-]+)\s*:\s*["']?([a-z0-9._\-]+/[a-z0-9._\-]+)["']?\s*$`)
	reAnsibleTagKey = regexp.MustCompile(`(?m)^\s*([\w.-]*tag)\s*:\s*["']?v?(\d[\w.\-]*)["']?\s*$`)
)

// ansibleSplitImageVars pairs sibling keys that name an image and its tag:
// public_alpine_node: mhart/alpine-node with public_node_tag: 7 pins
// nodejs 7. The link word is the tag key's word before "tag".
func (w *repoWalk) ansibleSplitImageVars(rel, code string) {
	type imgVal struct {
		base string
		line int
	}
	images := map[string][]imgVal{} // key word -> image values
	for _, m := range reAnsibleImgKey.FindAllStringSubmatchIndex(code, -1) {
		key := strings.ToLower(code[m[2]:m[3]])
		base := code[m[4]:m[5]]
		for _, word := range strings.FieldsFunc(key, func(r rune) bool { return r == '_' || r == '-' }) {
			images[word] = append(images[word], imgVal{base, lineOf(code, m[2])})
		}
	}
	for _, m := range reAnsibleTagKey.FindAllStringSubmatchIndex(code, -1) {
		key := strings.ToLower(code[m[2]:m[3]])
		ver := code[m[4]:m[5]]
		link := strings.TrimRight(strings.TrimSuffix(key, "tag"), "_-")
		if i := strings.LastIndexAny(link, "_-"); i >= 0 {
			link = link[i+1:]
		}
		if link == "" {
			continue
		}
		for _, iv := range images[link] {
			if r, ok := ident.ParseImageRef(iv.base + ":" + ver); ok {
				w.image(r, rel, iv.line, "")
			}
		}
	}
}

// parseKitchen: Test Kitchen platforms name the OS a cookbook is tested on.
func parseKitchen(w *repoWalk, rel, _, txt string) {
	for _, m := range reKitchenPlatform.FindAllStringSubmatchIndex(txt, -1) {
		osName, v := txt[m[2]:m[3]], txt[m[4]:m[5]]
		if osName == "ubuntu" && !strings.Contains(v, ".") && len(v) == 4 {
			v = v[:2] + "." + v[2:] // ubuntu-1404
		}
		if osName == "centos" || osName == "debian" {
			v = strings.SplitN(v, ".", 2)[0]
		}
		w.pin(osName, v, rel, lineOf(txt, m[2]), osName+"-"+txt[m[4]:m[5]])
	}
	for _, m := range reKitchenChef.FindAllStringSubmatchIndex(txt, -1) {
		w.pin("chef-infra-client", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "chef "+txt[m[2]:m[3]])
	}
}

var reKitchenChef = regexp.MustCompile(`(?m)^\s*(?:require_chef_omnibus|product_version|chef_version)\s*:\s*["']?(\d+(?:\.\d+){0,2})`)

var reSnapBase = regexp.MustCompile(`(?m)^base\s*:\s*core(\d\d)?\s*$`)

// parseSnapcraft: a snap's base is an Ubuntu LTS (core18 = 18.04).
var reSnapNode = regexp.MustCompile(`\bnode/(\d+)(?:\.x)?/`)

func parseSnapcraft(w *repoWalk, rel, _, txt string) {
	for _, m := range reSnapNode.FindAllStringSubmatchIndex(txt, -1) {
		w.pin("nodejs", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "build-snaps node/"+txt[m[2]:m[3]])
	}
	if m := reSnapBase.FindStringSubmatchIndex(txt); m != nil {
		v := "16"
		if m[2] >= 0 {
			v = txt[m[2]:m[3]]
		}
		w.pin("ubuntu", v+".04", rel, lineOf(txt, m[0]), "snap base")
	}
}

// Release downloads name the version they install: a GitHub release asset
// of a tracked project, the Qt installer tree, Docker's static binaries.
var releaseURLs = []struct {
	re      *regexp.Regexp
	product string
}{
	{regexp.MustCompile(`github\.com/cert-manager/cert-manager/releases/download/v(\d+\.\d+(?:\.\d+)?)`), "cert-manager"},
	{regexp.MustCompile(`github\.com/argoproj/argo-cd/(?:releases/download|raw)/v(\d+\.\d+(?:\.\d+)?)`), "argo-cd"},
	{regexp.MustCompile(`github\.com/istio/istio/releases/download/(\d+\.\d+(?:\.\d+)?)`), "istio"},
	{regexp.MustCompile(`github\.com/kyverno/kyverno/releases/download/v(\d+\.\d+(?:\.\d+)?)`), "kyverno"},
	{regexp.MustCompile(`github\.com/metallb/metallb/v?(\d+\.\d+(?:\.\d+)?)/`), "metallb"},
	{regexp.MustCompile(`download\.qt\.io/[\w/]*?/qt/(\d+\.\d+)/`), "qt"},
	{regexp.MustCompile(`/qt/(\d+\.\d+)/\d+\.\d+\.\d+/qt-opensource`), "qt"},
	{regexp.MustCompile(`(?:get|download)\.docker\.com/[\w/]*?docker-(\d+\.\d+(?:\.\d+)?)(?:-ce)?\.tgz`), "docker-engine"},
	{regexp.MustCompile(`releases\.hashicorp\.com/terraform/(\d+\.\d+\.\d+)/`), "terraform"},
	{regexp.MustCompile(`nodejs\.org/dist/v(\d+\.\d+\.\d+)/`), "nodejs"},
	{regexp.MustCompile(`mongodb-org(?:-server)?[_/](\d+\.\d+)(?:\.\d+)?`), "mongodb"},
	{regexp.MustCompile(`deb\.nodesource\.com/node_(\d+)\.x`), "nodejs"},
	{regexp.MustCompile(`releases\.nixos\.org/nix/nix-(\d+\.\d+)`), "nix"},
	{regexp.MustCompile(`\bopenssl-(\d+\.\d+\.\d+)[a-z]?\.(?:tar\.gz|tar\.xz|zip)`), "openssl"},
	{regexp.MustCompile(`(?i)\bgit-(\d+\.\d+(?:\.\d+)?)(?:\.windows|\.tar|[-\s])`), "git"},
	{regexp.MustCompile(`(?i)font-?awesome[-_ v]?(\d+\.\d+(?:\.\d+)?)\.(?:zip|tgz|tar\.gz)`), "font-awesome"},
	{regexp.MustCompile(`artifacts\.elastic\.co/downloads/elasticsearch/elasticsearch-(\d+\.\d+\.\d+)`), "elasticsearch"},
	{regexp.MustCompile(`artifacts\.elastic\.co/downloads/kibana/kibana-(\d+\.\d+\.\d+)`), "kibana"},
	{regexp.MustCompile(`artifacts\.elastic\.co/downloads/logstash/logstash-(\d+\.\d+\.\d+)`), "logstash"},
	{regexp.MustCompile(`\bkafka_\d+\.\d+-(\d+\.\d+\.\d+)\.tgz`), "apache-kafka"},
	{regexp.MustCompile(`\belasticsearch=(\d+\.\d+\.\d+)`), "elasticsearch"},
	{regexp.MustCompile(`downloads\.haskell\.org/~?ghc/(\d+\.\d+(?:\.\d+)?)/`), "ghc"},
	{regexp.MustCompile(`\bv?(1\.\d+)\.\d+\+(?:k3s|rke2r)\d+\b`), "kubernetes"},
	{regexp.MustCompile(`\bqt-everywhere-(?:opensource-)?src-(\d+\.\d+)\.\d+`), "qt"},
	{regexp.MustCompile(`\bopenSUSE-Leap-(\d+\.\d+)-`), "opensuse"},
	{regexp.MustCompile(`\bCentOS-(\d+)(?:\.\d+)?-x86_64-(?:Minimal|DVD|NetInstall)`), "centos"},
	{regexp.MustCompile(`\bdebian-(\d+)\.\d+\.\d+-(?:amd64|i386|arm64)-(?:netinst|DVD)`), "debian"},
	{regexp.MustCompile(`\bubuntu-(\d\d\.\d\d)(?:\.\d+)?-(?:server|live-server|desktop)-(?:amd64|i386|arm64)\.iso`), "ubuntu"},
	{regexp.MustCompile(`\bspark-(\d+\.\d+\.\d+)-bin-(?:hadoop|without-hadoop)`), "apache-spark"},
	{regexp.MustCompile(`grafana/loki/releases/download/v(\d+\.\d+(?:\.\d+)?)/`), "grafana-loki"},
	{regexp.MustCompile(`\brhel-(\d+)-(?:server|for-x86_64)-[\w-]*rpms\b`), "rhel"},
	{regexp.MustCompile(`\brhel-\d+-server-ose-(\d+\.\d+)-rpms\b`), "red-hat-openshift"},
	{regexp.MustCompile(`INSTALL_K3S_VERSION=["']?v(\d+\.\d+)|k3s/releases/download/v(\d+\.\d+)`), "kubernetes"},
	{regexp.MustCompile(`python\.org/ftp/python/(\d+\.\d+\.\d+)/`), "python"},
	{regexp.MustCompile(`\bvirtualbox-(\d+\.\d+)_\d+\.\d+\.\d+[\w.~-]*\.(?:deb|rpm)`), "virtualbox"},
	{regexp.MustCompile(`kernel\.ubuntu\.com/~kernel-ppa/mainline/v(\d+\.\d+)(?:\.\d+)?`), "linux"},
	{regexp.MustCompile(`(?:nvm-sh|creationix)/nvm/v(\d+\.\d+(?:\.\d+)?)/install\.sh`), "nvm"},
	{regexp.MustCompile(`go\.dev/dl/go(\d+\.\d+(?:\.\d+)?)\.`), "go"},
	{regexp.MustCompile(`golang\.org/dl/go(\d+\.\d+(?:\.\d+)?)\.`), "go"},
	{regexp.MustCompile(`dl\.google\.com/go/go(\d+\.\d+(?:\.\d+)?)\.`), "go"},
}

// ghReleaseRepos: GitHub repositories whose release assets and tag
// archives are a tracked product at the tag's version.
var ghReleaseRepos = map[string]string{
	"twbs/bootstrap": "bootstrap", "jquery/jquery": "jquery", "etcd-io/etcd": "etcd",
	"prometheus/prometheus": "prometheus", "grafana/grafana": "grafana",
	"kubernetes/kubernetes": "kubernetes", "traefik/traefik": "traefik", "containerd/containerd": "containerd",
	"goharbor/harbor": "harbor", "hashicorp/vault": "hashicorp-vault", "hashicorp/consul": "consul",
	"nginx/nginx": "nginx", "redis/redis": "redis", "nodejs/node": "nodejs", "python/cpython": "python",
	"ruby/ruby": "ruby", "php/php-src": "php", "golang/go": "go", "rust-lang/rust": "rust", "elastic/elasticsearch": "elasticsearch",
	"apache/kafka": "apache-kafka", "keycloak/keycloak": "keycloak", "mongodb/mongo": "mongodb", "envoyproxy/envoy": "envoy",
	"fluxcd/flux2": "flux",
}

var reGHRelease = regexp.MustCompile(`github\.com/([\w.\-]+/[\w.\-]+)/(?:releases/download|archive(?:/refs/tags)?)/v?(\d+\.\d+(?:\.\d+)?)\b`)

var reOcNewApp = regexp.MustCompile(`\bnew-app\b[^\n~]*?\s([\w.\-]+(?:/[\w.\-]+)+(?::[\w.\-]+)?)~`)

func (w *repoWalk) releaseDownloads(rel, txt string) {
	// oc new-app centos/python-35-centos7~https://...: the S2I builder image
	for _, m := range reOcNewApp.FindAllStringSubmatchIndex(txt, -1) {
		if r, ok := ident.ParseImageRef(txt[m[2]:m[3]]); ok {
			w.image(r, rel, lineOf(txt, m[2]), "")
		}
	}
	for _, m := range reGHRelease.FindAllStringSubmatchIndex(txt, -1) {
		if p := ghReleaseRepos[strings.ToLower(txt[m[2]:m[3]])]; p != "" {
			w.pin(p, txt[m[4]:m[5]], rel, lineOf(txt, m[4]), txt[m[0]:m[1]])
		}
	}
	for _, r := range releaseURLs {
		for _, m := range r.re.FindAllStringSubmatchIndex(txt, -1) {
			for g := 2; g+1 < len(m); g += 2 {
				if m[g] >= 0 {
					w.pin(r.product, txt[m[g]:m[g+1]], rel, lineOf(txt, m[g]), txt[m[0]:m[1]])
					break
				}
			}
		}
	}
}

var reShellVar = regexp.MustCompile(`(?m)^[ \t]*(?:export[ \t]+|local[ \t]+|readonly[ \t]+)?([A-Za-z][A-Za-z0-9_]*)=["']?([^\s"';]+)`)

// parseShellScript: install scripts CI and Dockerfiles call (conda
// install python=3.4, nvm install 8, nodesource setup_14.x, release URLs).
func parseShellScript(w *repoWalk, rel, _, txt string) {
	// NODE_VERSION=14.17.0 that the script goes on to use
	code := stripComments(txt)
	for _, m := range reShellVar.FindAllStringSubmatchIndex(code, -1) {
		name := code[m[2]:m[3]]
		if strings.Contains(code, "$"+name) || strings.Contains(code, "${"+name) {
			w.versionVar(name, code[m[4]:m[5]], rel, lineOf(code, m[2]))
		}
	}
	// aws rds create-db-instance --engine $DB_ENGINE --engine-version
	// $ENGINE_VERSION: the database engine a deploy script creates
	if m := reRDSEngineName.FindStringSubmatchIndex(code); m != nil {
		if vm := reRDSEngineVersion.FindStringSubmatchIndex(code); vm != nil && strings.Contains(code, "--engine-version") {
			if product := rdsEngines[strings.ToLower(code[m[2]:m[3]])]; product != "" {
				w.pin(product, code[vm[2]:vm[3]], rel, lineOf(code, vm[2]), "rds engine "+product+" "+code[vm[2]:vm[3]])
			}
		}
	}
	w.scriptRuntimes(rel, stripComments(txt))
	w.releaseDownloads(rel, stripComments(txt))
	w.pipInstallRefs(rel, txt)
	w.npmInstallRefs(rel, txt)
	// image refs in scripts sit in docker/kind commands or heredoc YAML
	// (image:, FROM), including a command's continuation lines; prompt text
	// and examples name no image. Blank out the other lines, keeping
	// offsets so line numbers stay right.
	lines := strings.Split(code, "\n")
	prevKeptCont := false
	for i, line := range lines {
		cont := strings.HasSuffix(strings.TrimRight(line, " \t\r"), "\\")
		if !reScriptImageContext.MatchString(line) && !prevKeptCont {
			lines[i] = strings.Repeat(" ", len(line))
			prevKeptCont = false
			continue
		}
		prevKeptCont = cont
	}
	w.trackedImageRefs(rel, strings.Join(lines, "\n"))
}

// reScriptImageContext: a line that can carry a real image reference.
var reScriptImageContext = regexp.MustCompile(`(?i)docker|podman|kubectl|kind|image\s*[:=]|container\s*[:=]|--image|\bfrom\s`)

// rdsEngines: --engine values that name a tracked database.
var rdsEngines = map[string]string{
	"mysql": "mysql", "mariadb": "mariadb", "postgres": "postgresql", "postgresql": "postgresql",
	"aurora-mysql": "mysql", "aurora-postgresql": "postgresql", "oracle": "oracle-database", "sqlserver": "mssqlserver",
}

var (
	reRDSEngineName    = regexp.MustCompile(`(?m)^\s*(?:export\s+)?DB_ENGINE\s*=\s*["']?([\w.-]+)`)
	reRDSEngineVersion = regexp.MustCompile(`(?m)^\s*(?:export\s+)?ENGINE_VERSION\s*=\s*["']?v?(\d+(?:\.\d+){0,2})`)
)

// parseVersionsEnv: a versions.env file is a pin registry: every
// <PRODUCT>_VERSION key declares what the build runs (RUST_VERSION="1.93"
// sourced by CI). No usage gate: declaring is the file's whole purpose.
func parseVersionsEnv(w *repoWalk, rel, _, txt string) {
	code := stripComments(txt)
	for _, m := range reEnvKeyVal.FindAllStringSubmatchIndex(code, -1) {
		key := strings.ToLower(code[m[2]:m[3]])
		key = strings.TrimSuffix(key, "_version")
		key = strings.TrimSuffix(key, "-version")
		product, ok := matrixProducts[key]
		if !ok {
			continue
		}
		w.pin(product, code[m[4]:m[5]], rel, lineOf(code, m[2]), code[m[2]:m[3]]+"="+code[m[4]:m[5]])
	}
}

var reEnvKeyVal = regexp.MustCompile(`(?m)^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_-]*_VERSION)\s*=\s*["']?v?(\d[\w.\-+]*)`)

// npmInstallRefs: npm install pkg@ver in a script pins the package.
var reNpmInstall = regexp.MustCompile(`\bnpm (?:i|install|add)\b([^\n]+)`)

func (w *repoWalk) npmInstallRefs(rel, txt string) {
	for _, m := range reNpmInstall.FindAllStringSubmatchIndex(txt, -1) {
		for _, tok := range strings.Fields(txt[m[2]:m[3]]) {
			if dm := reNpmPkgVer.FindStringSubmatch(tok); dm != nil {
				w.dep("npm", strings.ToLower(dm[1]), dm[2], styleNPM, rel, lineOf(txt, m[2]))
			}
		}
	}
}

var reNpmPkgVer = regexp.MustCompile(`^(?:@[\w.-]+/)?([\w.-]+)@(\d[\w.+-]*)$`)

// isTestPath: under a test directory (test/, tests/, spec/).
func isTestPath(rel string) bool {
	for _, seg := range strings.Split(strings.ToLower(path.Dir(rel)), "/") {
		if seg == "test" || seg == "tests" || seg == "spec" {
			return true
		}
	}
	return false
}

var reLeinJavac = regexp.MustCompile(`"-(?:target|source)"\s*"(\d+(?:\.\d+)?)"`)

// parseLeiningen: Clojure projects pin the Java level in :javac-options.
func parseLeiningen(w *repoWalk, rel, _, txt string) {
	if m := reLeinJavac.FindStringSubmatchIndex(txt); m != nil {
		w.pin("java", javaVersion(txt[m[2]:m[3]]), rel, lineOf(txt, m[2]), "javac "+txt[m[2]:m[3]])
	}
}

var reAntJavac = regexp.MustCompile(`(?:<javac\b[^>]*?\b(?:source|target)\s*=\s*"|name\s*=\s*"(?:ant\.build\.javac\.(?:source|target)|javac\.(?:source|target)|java\.(?:source|target)|source|target)"\s+value\s*=\s*")(\d+(?:\.\d+)?)"`)

// parseAntBuild: Ant builds pin the Java level on <javac> or in properties.
func parseAntBuild(w *repoWalk, rel, _, txt string) {
	// an Ant build beside a pom/gradle build is legacy: secondary
	seen := map[string]bool{}
	for _, m := range reAntJavac.FindAllStringSubmatchIndex(txt, -1) {
		v := javaVersion(txt[m[2]:m[3]])
		if !seen[v] {
			seen[v] = true
			w.ideJava = append(w.ideJava, idePin{path.Dir(rel), Pin{Product: "java", Version: v,
				File: loc(rel, lineOf(txt, m[2])), Raw: "ant javac " + txt[m[2]:m[3]]}})
		}
	}
}

var (
	reFlatpakRuntime = regexp.MustCompile(`"?runtime"?\s*:\s*["']?(org\.(?:kde|freedesktop|gnome)\.Platform)["']?`)
	reFlatpakVersion = regexp.MustCompile(`"?runtime-version"?\s*:\s*["']?([\w.\-]+)["']?`)
)

// parseFlatpak: a Flatpak manifest runs on a runtime: org.kde.Platform 6.7
// is Qt 6.7, 5.15-24.08 is Qt 5.15 on freedesktop-sdk 24.08.
func parseFlatpak(w *repoWalk, rel, _, txt string) {
	// KDE convention: the root .flatpak-manifest.* is the live one; other
	// manifests beside it are stale experiments
	if !strings.HasPrefix(rel, ".flatpak-manifest.") &&
		(w.files[".flatpak-manifest.json"] || w.files[".flatpak-manifest.yml"] || w.files[".flatpak-manifest.yaml"]) {
		return
	}
	rt := reFlatpakRuntime.FindStringSubmatch(txt)
	vm := reFlatpakVersion.FindStringSubmatchIndex(txt)
	if rt == nil || vm == nil {
		return
	}
	v, line := txt[vm[2]:vm[3]], lineOf(txt, vm[2])
	switch rt[1] {
	case "org.kde.Platform":
		parts := strings.SplitN(v, "-", 2)
		w.pin("qt", parts[0], rel, line, "org.kde.Platform "+v)
		if len(parts) == 2 {
			w.pin("freedesktop-sdk", parts[1], rel, line, "org.kde.Platform "+v)
		}
	case "org.freedesktop.Platform":
		w.pin("freedesktop-sdk", v, rel, line, "org.freedesktop.Platform "+v)
	}
}

var reNetBeansJavac = regexp.MustCompile(`(?m)^javac\.(?:source|target)\s*=\s*(\d+(?:\.\d+)?)`)

// parseNetBeans: nbproject/project.properties pins the Java level.
func parseNetBeans(w *repoWalk, rel, _, txt string) {
	if !strings.Contains(rel, "nbproject/") {
		return
	}
	if m := reNetBeansJavac.FindStringSubmatchIndex(txt); m != nil {
		w.pin("java", javaVersion(txt[m[2]:m[3]]), rel, lineOf(txt, m[2]), "javac.source="+txt[m[2]:m[3]])
	}
}

var (
	reEBPlatform = regexp.MustCompile(`(?m)^\s*default_platform\s*:\s*(.+)$`)
	reEBRuntime  = regexp.MustCompile(`(?i)\b(Python|Node\.js|PHP|Ruby|Java|Tomcat|Go|Corretto)\s+(\d+(?:\.\d+){0,2})`)
)

// parseElasticBeanstalk: the platform an Elastic Beanstalk app deploys on.
func parseElasticBeanstalk(w *repoWalk, rel, _, txt string) {
	products := map[string]string{"python": "python", "node.js": "nodejs", "php": "php",
		"ruby": "ruby", "java": "java", "tomcat": "tomcat", "go": "go", "corretto": "java"}
	for _, m := range reEBPlatform.FindAllStringSubmatchIndex(txt, -1) {
		plat := txt[m[2]:m[3]]
		w.cloudOSRefs(rel, plat)
		for _, r := range reEBRuntime.FindAllStringSubmatch(plat, -1) {
			w.pin(products[strings.ToLower(r[1])], r[2], rel, lineOf(txt, m[2]), strings.TrimSpace(plat))
		}
	}
}

// dropShadowedRust: a repo-root rust-toolchain file outranks the default
// toolchain CI actions set (rustup default), so their versions never build.
func (w *repoWalk) dropShadowedRust() {
	if !w.files["rust-toolchain"] && !w.files["rust-toolchain.toml"] {
		return
	}
	out := w.inv.Pins[:0]
	for _, p := range w.inv.Pins {
		if p.Product == "rust" && strings.HasPrefix(p.File, ".github/workflows/") {
			continue
		}
		out = append(out, p)
	}
	w.inv.Pins = out
}

// dropStaleFlatpaks: a Flatpak manifest on an older Qt than the CMake build
// requires (Qt5 runtime, find_package(Qt6)) cannot build the project.
func (w *repoWalk) dropStaleFlatpaks() {
	cmakeQt := ""
	for _, p := range w.inv.Pins {
		if p.Product == "qt" && strings.HasPrefix(p.Raw, "find_package(") && p.Version > cmakeQt {
			cmakeQt = p.Version
		}
	}
	if cmakeQt == "" {
		return
	}
	stale := map[string]bool{}
	for _, p := range w.inv.Pins {
		if p.Product == "qt" && strings.HasPrefix(p.Raw, "org.kde.Platform") && strings.SplitN(p.Version, ".", 2)[0] < cmakeQt {
			stale[strings.SplitN(p.File, ":", 2)[0]] = true
		}
	}
	out := w.inv.Pins[:0]
	for _, p := range w.inv.Pins {
		if !stale[strings.SplitN(p.File, ":", 2)[0]] {
			out = append(out, p)
		}
	}
	w.inv.Pins = out
}

var (
	rePantheonPHP = regexp.MustCompile(`(?m)^php_version\s*:\s*["']?(\d+\.\d+)`)
	rePantheonDB  = regexp.MustCompile(`(?m)^database\s*:\s*\n\s+version\s*:\s*["']?(\d+\.\d+)`)
)

// parsePantheon: pantheon.yml pins PHP and the MariaDB version.
func parsePantheon(w *repoWalk, rel, _, txt string) {
	if m := rePantheonPHP.FindStringSubmatchIndex(txt); m != nil {
		w.pin("php", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "php_version")
	}
	if m := rePantheonDB.FindStringSubmatchIndex(txt); m != nil {
		w.pin("mariadb", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "database version")
	}
}

var (
	reMagentoPHP = regexp.MustCompile(`(?s)<php>\s*<min>[^<]*</min>\s*<max>(\d+\.\d+)`)
)

// parseMagentoPackage: Magento Connect package.xml bounds PHP with min/max.
func parseMagentoPackage(w *repoWalk, rel, _, txt string) {
	if m := reMagentoPHP.FindStringSubmatchIndex(txt); m != nil {
		w.pin("php", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "php max "+txt[m[2]:m[3]])
	}
}
