package catalog

import (
	"regexp"
	"strings"

	"github.com/0xbadshah/zephy/internal/ident"
	"gopkg.in/yaml.v3"
)

// CI and platform files declare the runtimes a project builds, tests and
// deploys on: setup actions and matrices, runner images, Travis language
// lists, Lambda and App Engine runtimes, CodeBuild runtime-versions.

func yamlRoot(txt string) *yaml.Node {
	var doc yaml.Node
	if yaml.Unmarshal([]byte(txt), &doc) != nil || len(doc.Content) == 0 {
		// tab-indented YAML is invalid but CI systems that use their own
		// parsers still run it: retry with tabs as two spaces
		if !strings.Contains(txt, "\t") {
			return nil
		}
		doc = yaml.Node{}
		if yaml.Unmarshal([]byte(strings.ReplaceAll(txt, "\t", "  ")), &doc) != nil || len(doc.Content) == 0 {
			// mixed tabs and spaces: keep the lines whose indentation
			// is clean (blanking the rest keeps line numbers), so the
			// top-level keys still read
			keep := strings.Split(txt, "\n")
			for i, l := range keep {
				if ind := len(l) - len(strings.TrimLeft(l, " \t")); strings.Contains(l[:ind], "\t") {
					keep[i] = ""
				}
			}
			doc = yaml.Node{}
			if yaml.Unmarshal([]byte(strings.Join(keep, "\n")), &doc) != nil || len(doc.Content) == 0 {
				return nil
			}
		}
	}
	return doc.Content[0]
}

// yamlGet returns the value node for key in a mapping node.
func yamlGet(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

// yamlScalars flattens a scalar or a sequence of scalars.
func yamlScalars(n *yaml.Node) []*yaml.Node {
	if n == nil {
		return nil
	}
	switch n.Kind {
	case yaml.ScalarNode:
		return []*yaml.Node{n}
	case yaml.SequenceNode:
		var out []*yaml.Node
		for _, c := range n.Content {
			out = append(out, yamlScalars(c)...)
		}
		return out
	}
	return nil
}

// yamlWalk visits every key/value pair in the document.
func yamlWalk(n *yaml.Node, visit func(key string, val *yaml.Node, parent *yaml.Node)) {
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			visit(n.Content[i].Value, n.Content[i+1], n)
			yamlWalk(n.Content[i+1], visit)
		}
	case yaml.SequenceNode, yaml.DocumentNode:
		for _, c := range n.Content {
			yamlWalk(c, visit)
		}
	}
}

var reMatrixRef = regexp.MustCompile(`\$\{\{\s*(?:matrix(?:\.([\w.-]+)|\[['"]([\w-]+)['"]\])|(env\.[\w-]+))\s*\}\}`)

// setupActions maps setup actions to the input naming the runtime version.
var setupActions = []struct{ prefix, input, product string }{
	{"actions/setup-node", "node-version", "nodejs"},
	{"actions/setup-python", "python-version", "python"},
	{"actions/setup-java", "java-version", "java"},
	{"actions/setup-go", "go-version", "go"},
	{"ruby/setup-ruby", "ruby-version", "ruby"},
	{"actions/setup-ruby", "ruby-version", "ruby"},
	{"actions/setup-dotnet", "dotnet-version", "dotnet"},
	{"shivammathur/setup-php", "php-version", "php"},
	{"nanasess/setup-php", "php-version", "php"},
	{"hashicorp/setup-terraform", "terraform_version", "terraform"},
	{"erlef/setup-beam", "elixir-version", "elixir"},
	{"erlef/setup-beam", "otp-version", "erlang"},
	{"erlef/setup-elixir", "elixir-version", "elixir"},
	{"erlef/setup-elixir", "otp-version", "erlang"},
	{"actions/setup-elixir", "elixir-version", "elixir"},
	{"actions/setup-elixir", "otp-version", "erlang"},
	{"denoland/setup-deno", "deno-version", "deno"},
	{"oven-sh/setup-bun", "bun-version", "bun"},
	{"conda-incubator/setup-miniconda", "python-version", "python"},
	{"actions/setup-haskell", "ghc-version", "ghc"},
	{"haskell/actions/setup", "ghc-version", "ghc"},
	{"julia-actions/setup-julia", "version", "julia"},
	{"graalvm/setup-graalvm", "java-version", "java"},
	{"astral-sh/setup-uv", "python-version", "python"},
	{"delaguardo/setup-graalvm", "graalvm", "graalvm-ce"},
	{"gradle/actions/setup-gradle", "gradle-version", "gradle"},
	{"jurplel/install-qt-action", "version", "qt"},
	{"actions-rs/toolchain", "toolchain", "rust"},
	{"dtolnay/rust-toolchain", "toolchain", "rust"},
	{"jurplel/install-qt-action", "version", "qt"},
	{"gradle/gradle-build-action", "gradle-version", "gradle"},
	{"delaguardo/setup-graalvm", "graalvm-version", "graalvm-ce"},
	{"pnpm/action-setup", "version", "pnpm"},
}

var reRustToolchainRef = regexp.MustCompile(`^dtolnay/rust-toolchain@(\d+\.\d+(?:\.\d+)?)$`)

var reMatrixGHC = regexp.MustCompile(`^ghc-(\d+\.\d+)(?:\.\d+)?$`)

// reNodeLTS: node_js: - lts/carbon.
var reNodeLTS = regexp.MustCompile(`^lts/(\w+)$`)

var matrixProducts = map[string]string{
	"typo3": "typo3", "django": "django", "rails": "rails", "laravel": "laravel", "symfony": "symfony",
	"drupal": "drupal", "wordpress": "wordpress", "wp": "wordpress", "magento": "magento", "postgres": "postgresql",
	"postgresql": "postgresql", "mysql": "mysql", "mariadb": "mariadb", "redis": "redis", "mongodb": "mongodb",
	"elasticsearch": "elasticsearch", "kubernetes": "kubernetes", "k8s": "kubernetes", "numpy": "numpy", "etcd": "etcd",
	"angular": "angular", "electron": "electron", "spring-boot": "spring-boot", "otp": "erlang", "elixir": "elixir",
	"python": "python", "py": "python", "node": "nodejs", "nodejs": "nodejs", "ruby": "ruby", "php": "php",
	"go": "go", "golang": "go", "java": "java", "jdk": "java", "dotnet": "dotnet", "rust": "rust",
	"mediawiki": "mediawiki", "mw": "mediawiki", "nextcloud": "nextcloud", "moodle": "moodle", "joomla": "joomla",
}

func parseGitHubWorkflow(w *repoWalk, rel, _, txt string) {
	root := yamlRoot(txt)
	jobs := yamlGet(root, "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode {
		return
	}
	env := map[string]*yaml.Node{}
	collectEnv := func(n *yaml.Node) {
		if n == nil || n.Kind != yaml.MappingNode {
			return
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i+1].Kind == yaml.ScalarNode {
				env[n.Content[i].Value] = n.Content[i+1]
			}
		}
	}
	collectEnv(yamlGet(root, "env"))
	for i := 1; i < len(jobs.Content); i += 2 {
		job := jobs.Content[i]
		collectEnv(yamlGet(job, "env"))
		matrix := workflowMatrix(yamlGet(yamlGet(job, "strategy"), "matrix"))
		for k, v := range env {
			if _, ok := matrix["env."+k]; !ok {
				matrix["env."+k] = []*yaml.Node{v}
			}
		}
		values := func(n *yaml.Node) []*yaml.Node {
			var out []*yaml.Node
			for _, s := range yamlScalars(n) {
				v := strings.TrimSpace(s.Value)
				refs := reMatrixRef.FindAllStringSubmatch(v, -1)
				switch {
				case len(refs) == 1:
					// ${{ matrix.python }} or ubuntu-${{ matrix.ubuntu }}: one
					// value per matrix entry, spliced into the text around it
					for _, mv := range matrix[refs[0][1]+refs[0][2]+refs[0][3]] {
						out = append(out, &yaml.Node{Kind: yaml.ScalarNode, Line: mv.Line,
							Value: reMatrixRef.ReplaceAllLiteralString(v, mv.Value)})
					}
				case !strings.Contains(v, "${{"):
					out = append(out, s)
				}
			}
			return out
		}
		for _, s := range values(yamlGet(job, "runs-on")) {
			w.runner(s.Value, rel, s.Line)
		}
		// a reusable workflow call passes runtime choices as inputs
		// (uses: org/actions/... with: php: '8.1')
		if u := yamlGet(job, "uses"); u != nil && u.Kind == yaml.ScalarNode {
			if withN := yamlGet(job, "with"); withN != nil && withN.Kind == yaml.MappingNode {
				for i := 0; i+1 < len(withN.Content); i += 2 {
					if product, ok := matrixProducts[strings.ToLower(withN.Content[i].Value)]; ok {
						for _, v := range yamlScalars(withN.Content[i+1]) {
							w.runtime(product, v.Value, styleNPM, rel, v.Line)
						}
					}
				}
			}
		}
		if c := yamlGet(job, "container"); c != nil && c.Kind == yaml.ScalarNode {
			for _, s := range values(c) {
				if r, ok := ident.ParseImageRef(s.Value); ok {
					w.image(r, rel, s.Line, "")
				}
			}
		}
		w.setupSteps(yamlGet(job, "steps"), values, rel)
		// matrix dimensions named for a product (django: ["2.2", "3.2"],
		// TYPO3: ['11', '12']) declare the versions the job tests against
		for key, vals := range matrix {
			// a dimension no step reads declares nothing
			// (python-version: '3.10' with no matrix reference)
			if !strings.Contains(txt, "matrix."+key) &&
				!strings.Contains(txt, "matrix."+strings.TrimPrefix(key, "env.")) &&
				!strings.Contains(txt, "matrix['"+key+"']") &&
				!strings.Contains(txt, "matrix[\""+key+"\"]") {
				continue
			}
			// OS targets as matrix values: fedora41, centos7, ubuntu20.04
			for _, v := range vals {
				if m := reMatrixGHC.FindStringSubmatch(strings.TrimSpace(v.Value)); m != nil {
					w.pin("ghc", m[1], rel, v.Line, v.Value) // haskell-ci: compiler: ghc-9.4.8
				}
				if m := reOSToken.FindStringSubmatch(strings.ToLower(strings.TrimSpace(v.Value))); m != nil {
					if p := osTokenProducts[m[1]]; p != "" {
						w.pin(p, m[2], rel, v.Line, v.Value)
					}
				}
			}
			product, ok := matrixProducts[strings.TrimSuffix(strings.TrimSuffix(strings.ToLower(key), "-version"), "_version")]
			if !ok {
				continue
			}
			// pip install Django~=${{ matrix.django }}: each value is a
			// compatible-release spec (~=3.0 installs the newest 3.x)
			compat := regexp.MustCompile(`~=\s*["']?\$\{\{\s*matrix\.` + regexp.QuoteMeta(key) + `\s*\}\}`).MatchString(txt)
			for _, v := range vals {
				if compat {
					w.runtime(product, "~="+strings.TrimSpace(v.Value), stylePEP440, rel, v.Line)
					continue
				}
				w.runtime(product, v.Value, styleNPM, rel, v.Line)
			}
		}
		// include entries can pair a product with its version across two
		// dims: db-type: mysql + db-version: 5.7 (the CI service image is
		// built from both)
		if inc := yamlGet(yamlGet(yamlGet(job, "strategy"), "matrix"), "include"); inc != nil && inc.Kind == yaml.SequenceNode {
			for _, item := range inc.Content {
				if item.Kind != yaml.MappingNode {
					continue
				}
				kv := map[string]*yaml.Node{}
				for x := 0; x+1 < len(item.Content); x += 2 {
					kv[item.Content[x].Value] = item.Content[x+1]
				}
				for name, v := range kv {
					lk := strings.ToLower(name)
					if !strings.HasSuffix(lk, "-version") || v.Kind != yaml.ScalarNode {
						continue
					}
					prefix := strings.TrimSuffix(lk, "-version")
					t, ok := kv[prefix+"-type"]
					if !ok || t.Kind != yaml.ScalarNode {
						continue
					}
					product, mapped := matrixProducts[strings.ToLower(t.Value)]
					if !mapped {
						continue
					}
					// only when the workflow reads the version dim
					if !strings.Contains(txt, "matrix."+name) {
						continue
					}
					w.runtime(product, v.Value, styleNPM, rel, v.Line)
				}
			}
		}
	}
	// a runner label written to $GITHUB_OUTPUT is what a later
	// runs-on: ${{ steps.x.outputs.y }} resolves to
	for _, m := range reOutputRunner.FindAllStringSubmatchIndex(txt, -1) {
		w.runner(txt[m[2]:m[3]], rel, lineOf(txt, m[2]))
	}
	// version vars assigned in run: blocks: USE_GO_VERSION="1.14" echoed
	// to $GITHUB_OUTPUT and read by a setup step pin the runtime
	code := stripComments(txt)
	for _, m := range reShellVar.FindAllStringSubmatchIndex(code, -1) {
		name := code[m[2]:m[3]]
		if strings.Contains(code, "$"+name) || strings.Contains(code, "${"+name) {
			w.versionVar(name, code[m[4]:m[5]], rel, lineOf(code, m[2]))
		}
	}
}

// reOutputRunner: platform=macos-12 >> $GITHUB_OUTPUT
var reOutputRunner = regexp.MustCompile(`(?m)["']?(macos-[\d.]+|ubuntu-\d+(?:\.\d+)?|windows-\d{4})["']?\s*>>\s*\$GITHUB_OUTPUT`)

// setupSteps reads setup-action versions from a list of steps; values
// resolves matrix/env expressions (nil: literals only).
func (w *repoWalk) setupSteps(steps *yaml.Node, values func(*yaml.Node) []*yaml.Node, rel string) {
	if steps == nil {
		return
	}
	if values == nil {
		values = func(n *yaml.Node) []*yaml.Node {
			var out []*yaml.Node
			for _, s := range yamlScalars(n) {
				if !strings.Contains(s.Value, "${{") {
					out = append(out, s)
				}
			}
			return out
		}
	}
	{
		for _, step := range steps.Content {
			uses := yamlGet(step, "uses")
			with := yamlGet(step, "with")
			if uses != nil {
				// dtolnay/rust-toolchain@1.84.0: the ref is the toolchain
				if m := reRustToolchainRef.FindStringSubmatch(uses.Value); m != nil {
					w.pin("rust", m[1], rel, uses.Line, uses.Value)
				}
			}
			if uses == nil || with == nil {
				continue
			}
			for _, a := range setupActions {
				if !strings.HasPrefix(strings.ToLower(uses.Value), a.prefix) {
					continue
				}
				for _, s := range values(yamlGet(with, a.input)) {
					v := strings.TrimSpace(s.Value)
					if a.product == "ruby" {
						v = strings.TrimPrefix(v, "ruby-") // setup-ruby accepts ruby-3.0.2
					}
					if a.product == "python" {
						v = strings.TrimPrefix(v, "cpython-")
					}
					w.runtime(a.product, v, styleNPM, rel, s.Line)
				}
			}
		}
	}
}

// workflowMatrix collects matrix values per key: lists, and include entries.
func workflowMatrix(m *yaml.Node) map[string][]*yaml.Node {
	out := map[string][]*yaml.Node{}
	if m == nil || m.Kind != yaml.MappingNode {
		return out
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		key, val := m.Content[i].Value, m.Content[i+1]
		switch key {
		case "exclude":
		case "include":
			for _, entry := range val.Content {
				if entry.Kind != yaml.MappingNode {
					continue
				}
				for j := 0; j+1 < len(entry.Content); j += 2 {
					k, v := entry.Content[j].Value, entry.Content[j+1]
					out[k] = append(out[k], yamlScalars(v)...)
					if v.Kind == yaml.MappingNode { // include: [{platform: {os: macos-13}}]
						for x := 0; x+1 < len(v.Content); x += 2 {
							sub := k + "." + v.Content[x].Value
							out[sub] = append(out[sub], yamlScalars(v.Content[x+1])...)
						}
					}
				}
			}
		default:
			out[key] = append(out[key], yamlScalars(val)...)
			// config: [{os: ubuntu-16.04, r: release}] -> matrix.config.os
			for _, item := range val.Content {
				if item.Kind == yaml.MappingNode {
					for j := 0; j+1 < len(item.Content); j += 2 {
						sub := key + "." + item.Content[j].Value
						out[sub] = append(out[sub], yamlScalars(item.Content[j+1])...)
					}
				}
			}
		}
	}
	return out
}

var (
	reRunnerUbuntu  = regexp.MustCompile(`(?i)^ubuntu-(\d+\.\d+)$`)
	reAzureXcode    = regexp.MustCompile(`(?i)^xcode\d+-macos-?(\d+\.\d+)$`)
	reRunnerMac     = regexp.MustCompile(`(?i)^macos[- ](\d+(?:\.\d+)?)(?:-[a-z0-9]+)?$`)
	reRunnerWindows = regexp.MustCompile(`(?i)^(?:windows-|vs\d+-win)(\d{4})$`)
)

// runner records a hosted CI runner label: the OS it runs.
func (w *repoWalk) runner(label, rel string, line int) {
	label = strings.TrimSpace(label)
	switch {
	case reRunnerUbuntu.MatchString(label):
		w.pin("ubuntu", reRunnerUbuntu.FindStringSubmatch(label)[1], rel, line, label)
	case reRunnerMac.MatchString(label):
		w.pin("macos", reRunnerMac.FindStringSubmatch(label)[1], rel, line, label)
	case reAzureXcode.MatchString(label):
		w.pin("macos", reAzureXcode.FindStringSubmatch(label)[1], rel, line, label) // xcode9-macos10.13
	case reRunnerWindows.MatchString(label):
		w.pin("windows-server", reRunnerWindows.FindStringSubmatch(label)[1], rel, line, label)
	}
}

var travisDists = map[string]string{
	"precise": "12.04", "trusty": "14.04", "xenial": "16.04", "bionic": "18.04", "focal": "20.04", "jammy": "22.04", "noble": "24.04",
}

var reJDKName = regexp.MustCompile(`(?i)^(?:oracle|open)?jdk-?(\d+)$`)

func parseTravis(w *repoWalk, rel, _, txt string) {
	root := yamlRoot(txt)
	if root == nil {
		return
	}
	scopes := []*yaml.Node{root}
	for _, k := range []string{"matrix", "jobs"} {
		for _, inc := range []string{"include", "allow_failures"} {
			if n := yamlGet(yamlGet(root, k), inc); n != nil {
				scopes = append(scopes, n.Content...)
			}
		}
	}
	langs := map[string]string{
		"node_js": "nodejs", "python": "python", "rvm": "ruby", "php": "php", "go": "go",
		"dotnet": "dotnet", "elixir": "elixir", "otp_release": "erlang", "julia": "julia",
	}
	for _, scope := range scopes {
		for key, product := range langs {
			for _, s := range yamlScalars(yamlGet(scope, key)) {
				v := s.Value
				if key == "node_js" {
					if m := reNodeLTS.FindStringSubmatch(v); m != nil {
						if codename, ok := nodeCodenames[strings.ToLower(m[1])]; ok {
							v = codename // lts/carbon = Node 8
						}
					}
				}
				w.runtime(product, v, styleNPM, rel, s.Line)
			}
		}
		for _, s := range yamlScalars(yamlGet(scope, "jdk")) {
			if m := reJDKName.FindStringSubmatch(s.Value); m != nil {
				w.pin("java", javaVersion(m[1]), rel, s.Line, s.Value)
			}
		}
		addons := []*yaml.Node{yamlGet(scope, "addons")}
		if a := addons[0]; a != nil && a.Kind == yaml.SequenceNode {
			addons = a.Content // addons: [- mariadb: '10.2']
		}
		for _, a := range addons {
			for key, product := range map[string]string{"postgresql": "postgresql", "mariadb": "mariadb", "mysql": "mysql", "firefox": "firefox"} {
				for _, s := range yamlScalars(yamlGet(a, key)) {
					w.runtime(product, s.Value, styleNPM, rel, s.Line)
				}
			}
		}
		for _, s := range yamlScalars(yamlGet(scope, "osx_image")) {
			if v, ok := travisOSXImage(s.Value); ok {
				w.pin("macos", v, rel, s.Line, "osx_image: "+s.Value)
			}
		}
		for _, s := range yamlScalars(yamlGet(scope, "dist")) {
			if v, ok := travisDists[strings.ToLower(s.Value)]; ok {
				w.pin("ubuntu", v, rel, s.Line, "dist: "+s.Value)
			}
		}
		// env matrix entries carry pins as KEY=VALUE tokens (quotes may
		// hold spaces): python=3.6, DJANGO="django >=1.8,<1.9",
		// DJANGO=Django==1.6.1, DJANGO=https://host/download/1.7c1/tarball/
		envs := []*yaml.Node{}
		if e := yamlGet(scope, "env"); e != nil {
			if e.Kind == yaml.MappingNode {
				envs = append(envs, yamlScalars(yamlGet(e, "matrix"))...)
			} else {
				envs = append(envs, yamlScalars(e)...)
			}
		}
		for _, s := range envs {
			for _, kv := range reEnvEntry.FindAllStringSubmatch(s.Value, -1) {
				name, val := kv[1], ""
				for _, g := range kv[2:5] {
					if g != "" {
						val = g // the quoted content, or the bare token
						break
					}
				}
				key := strings.ToLower(strings.TrimSpace(name))
				// an env var no step consumes declares nothing
				// (DJANGO=1.8 counts only when a step reads $DJANGO)
				if !strings.Contains(txt, "$"+name) && !strings.Contains(txt, "${"+name) {
					continue
				}
				product, isProduct := matrixProducts[key]
				if isProduct && reBareVersion.MatchString(val) {
					w.runtime(product, val, styleNPM, rel, s.Line)
					continue
				}
				if m := reTravisPipSpec.FindStringSubmatch(val); m != nil {
					w.dep("pypi", strings.ToLower(m[1]), m[2], stylePEP440, rel, s.Line)
					continue
				}
				if isProduct {
					if m := reTravisURLVersion.FindStringSubmatch(val); m != nil {
						w.runtime(product, m[1], stylePEP440, rel, s.Line)
					}
				}
			}
		}
	}
}

// reEnvEntry: KEY=VALUE where the value may be a quoted string holding
// spaces (DJANGO="django >=1.8,<1.9").
var reEnvEntry = regexp.MustCompile(`([A-Za-z_][\w-]*)=(?:"([^"]*)"|'([^']*)'|(\S+))`)

// reBareVersion: python=3.6 in a Travis env matrix entry.
var reBareVersion = regexp.MustCompile(`^v?\d+(?:\.\d+)+$`)

// reTravisPipSpec: DJANGO=Django==1.6.1, DJANGO="django >=1.8,<1.9" in an
// env entry: a package plus a version spec.
var reTravisPipSpec = regexp.MustCompile(`(?i)^([A-Za-z0-9_.\-]+)\s*(==\s*[\d.]+|>=\s*[\d.]+.*)$`)

// reTravisURLVersion: a version inside a download URL the key names.
var reTravisURLVersion = regexp.MustCompile(`/(?:download|releases?)/v?(\d+\.\d+(?:\.\d+)?(?:c\d+)?)/`)

// parseGitLabCI: services and images in their mapping forms
// (- name: postgres:9.6), which the line-based image scan does not see.
func parseGitLabCI(w *repoWalk, rel, _, txt string) {
	yamlWalk(yamlRoot(txt), func(key string, val *yaml.Node, _ *yaml.Node) {
		switch key {
		case "services":
			for _, item := range val.Content {
				n := item
				if item.Kind == yaml.MappingNode {
					n = yamlGet(item, "name")
				}
				if n != nil && n.Kind == yaml.ScalarNode {
					if r, ok := ident.ParseImageRef(n.Value); ok {
						w.image(r, rel, n.Line, "")
					}
				}
			}
		case "image":
			if n := yamlGet(val, "name"); n != nil && n.Kind == yaml.ScalarNode {
				if r, ok := ident.ParseImageRef(n.Value); ok {
					w.image(r, rel, n.Line, "")
				}
			}
		}
	})
}

var (
	rePkg2AppImageDist = regexp.MustCompile(`(?m)^ingredients:[ \t]*\n(?:[ \t]+[^\n]*\n)*?[ \t]+dist:[ \t]*["']?(\w+)`)
	reLambdaRuntime    = regexp.MustCompile(`^(nodejs\d+(?:\.\d+|\.x)|python\d\.\d+|java\d+(?:\.al2(?:023)?)?|ruby\d\.\d+|dotnetcore\d\.\d|dotnet\d+(?:\.0)?|go1\.x)$`)
	reGAERuntime       = regexp.MustCompile(`^(python|nodejs|go|java|php|ruby)(\d+)$`)
	rePlatformType     = regexp.MustCompile(`^(?:pantheon-)?(php|python|nodejs|node|ruby|golang|go|java|mysql|mariadb|postgresql|postgres|redis|elasticsearch|solr|mongodb|mongo|rabbitmq|opensearch|memcached|varnish|nginx|apache|tomcat):(\d+(?:\.\d+)*)$`)
)

// parseYamlRuntimeKeys reads runtime declarations any YAML can carry:
// Lambda runtimes (serverless, SAM, CloudFormation), App Engine runtimes,
// CodeBuild runtime-versions, Azure Pipelines vmImage, Platform.sh types.
func parseYamlRuntimeKeys(w *repoWalk, rel, name, txt string) {
	w.releaseDownloads(rel, stripComments(txt)) // tarballs and repos Ansible tasks install
	if m := rePkg2AppImageDist.FindStringSubmatchIndex(txt); m != nil {
		// pkg2appimage recipes build from a distribution's packages
		code := strings.ToLower(txt[m[2]:m[3]])
		if v, ok := ubuntuCodenames[code]; ok {
			w.pin("ubuntu", v, rel, lineOf(txt, m[2]), "dist: "+code)
		} else if v, ok := debianCodenames[code]; ok {
			w.pin("debian", v, rel, lineOf(txt, m[2]), "dist: "+code)
		}
	}
	if !strings.Contains(txt, "untime") && !strings.Contains(txt, "vmImage") && !strings.Contains(txt, "type:") && !strings.Contains(txt, "kubernetesVersion") &&
		!strings.Contains(strings.ToUpper(txt), "_VER") && !strings.Contains(strings.ToUpper(txt), "VERSION") && !strings.Contains(txt, "suites") &&
		!strings.Contains(txt, "TOXENV") {
		return
	}
	platformSh := name == ".platform.app.yaml" || strings.Contains(rel, ".platform/") || strings.HasPrefix(name, ".lando")
	root := yamlRoot(txt)
	w.yamlVersionVars(root, rel)
	yamlWalk(root, func(key string, val *yaml.Node, parent *yaml.Node) {
		switch {
		case key == "suites" && strings.Contains(rel, "gitian"):
			for _, s := range yamlScalars(val) {
				w.codename(s.Value, rel, s.Line)
			}
		case key == "runtime" || key == "Runtime":
			if val.Kind != yaml.ScalarNode {
				return
			}
			v := strings.TrimSpace(val.Value)
			if reLambdaRuntime.MatchString(v) {
				w.pin("aws-lambda", v, rel, val.Line, key+": "+v)
			} else if m := reGAERuntime.FindStringSubmatch(v); m != nil {
				w.gaeRuntime(m[1], m[2], rel, val.Line)
			}
		case key == "runtime-versions" && val.Kind == yaml.MappingNode:
			for i := 0; i+1 < len(val.Content); i += 2 {
				lang, ver := val.Content[i].Value, val.Content[i+1]
				w.codebuildRuntime(lang, ver.Value, rel, ver.Line)
			}
		case key == "image" && val.Kind == yaml.ScalarNode && strings.Contains(val.Value, "codebuild/"):
			w.cloudOSRefs(rel, val.Value)
		case key == "vmImage" && val.Kind == yaml.ScalarNode:
			w.runner(strings.Trim(val.Value, `'"`), rel, val.Line)
		case key == "kubernetesVersion" && val.Kind == yaml.ScalarNode:
			w.runtime("kubernetes", strings.TrimPrefix(val.Value, "v"), styleNPM, rel, val.Line) // kubeadm ClusterConfiguration
		case key == "setup_remote_docker" && val.Kind == yaml.MappingNode:
			// CircleCI remote Docker engine: version: docker24 / 20.10.18
			if v := yamlGet(val, "version"); v != nil && v.Kind == yaml.ScalarNode {
				w.runtime("docker-engine", strings.TrimPrefix(v.Value, "docker"), styleNPM, rel, v.Line)
			}
		case key == "task" && val.Kind == yaml.ScalarNode:
			w.azureTask(val.Value, parent, rel)
		case key == "type" && platformSh && val.Kind == yaml.ScalarNode:
			if m := rePlatformType.FindStringSubmatch(val.Value); m != nil {
				product := map[string]string{"golang": "go", "go": "go", "mysql": "mysql", "varnish": "", "node": "nodejs",
					"postgres": "postgresql", "mongo": "mongodb", "apache": "apache-http-server"}[m[1]]
				if product == "" && m[1] != "varnish" {
					product = m[1]
				}
				if m[1] == "mysql" && !strings.HasPrefix(name, ".lando") {
					product = "mariadb" // Platform.sh's mysql service is MariaDB
				}
				if product != "" {
					w.pin(product, m[2], rel, val.Line, val.Value)
				}
			}
		}
	})
}

// gaeRuntime: App Engine spells versions without dots (python27, go111).
func (w *repoWalk) gaeRuntime(lang, digits, rel string, line int) {
	var version string
	switch lang {
	case "go":
		if len(digits) < 2 {
			return
		}
		version = digits[:1] + "." + digits[1:]
	case "nodejs", "java":
		version = digits
	default: // python27 -> 2.7, python310 -> 3.10, php72 -> 7.2
		if len(digits) < 2 {
			return
		}
		version = digits[:1] + "." + digits[1:]
	}
	product := map[string]string{"nodejs": "nodejs", "python": "python", "go": "go", "java": "java", "php": "php", "ruby": "ruby"}[lang]
	w.pin(product, version, rel, line, "runtime: "+lang+digits)
}

var reCodebuildJava = regexp.MustCompile(`(?i)^(?:corretto|openjdk|java)(\d+)$`)

func (w *repoWalk) codebuildRuntime(lang, ver, rel string, line int) {
	if strings.HasPrefix(ver, "$") {
		return
	}
	ver = strings.TrimSpace(ver)
	switch lang {
	case "java":
		if m := reCodebuildJava.FindStringSubmatch(ver); m != nil {
			w.pin("java", m[1], rel, line, lang+": "+ver)
		}
	case "nodejs", "python", "php", "ruby", "dotnet":
		w.runtime(lang, ver, styleNPM, rel, line)
	case "golang":
		w.runtime("go", ver, styleNPM, rel, line)
	}
}

var (
	reToxEnvlist  = regexp.MustCompile(`(?m)^\s*envlist\s*=\s*((?:.*\n?)(?:[ \t]+.*\n?)*)`)
	reToxPy       = regexp.MustCompile(`^py(\d)[.-]?(\d+)$`)                            // py35, py3.5
	reBasepython  = regexp.MustCompile(`(?m)^\s*(?:[\w.{}-]+:\s*)?python(\d\.\d+)\s*$`) // py3.5: python3.5
	reBraceExpand = regexp.MustCompile(`\{([^{}]*)\}`)
)

// parseTox reads test environments: py27, py36 run on those Pythons.
func parseTox(w *repoWalk, rel, _, txt string) {
	if m := reToxEnvlist.FindStringSubmatchIndex(txt); m != nil {
		line := lineOf(txt, m[2])
		for _, env := range expandBraces(txt[m[2]:m[3]]) {
			for _, factor := range strings.Split(env, "-") {
				if f := reToxPy.FindStringSubmatch(factor); f != nil {
					w.pin("python", f[1]+"."+f[2], rel, line, "tox env "+env)
				}
			}
		}
	}
	for _, m := range reBasepython.FindAllStringSubmatchIndex(txt, -1) {
		w.pin("python", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "basepython")
	}
	w.toxDeps(rel, txt)
}

// expandBraces expands tox's py{27,36}-django{22,30} generative lists.
func expandBraces(s string) []string {
	var out []string
	// split on top-level commas/newlines, respecting braces
	depth, start := 0, 0
	var items []string
	for i, r := range s {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
		case ',', '\n':
			if depth == 0 {
				items = append(items, s[start:i])
				start = i + 1
			}
		}
	}
	items = append(items, s[start:])
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" || strings.HasPrefix(it, "#") {
			continue
		}
		out = append(out, expandOne(it)...)
	}
	return out
}

func expandOne(s string) []string {
	m := reBraceExpand.FindStringSubmatchIndex(s)
	if m == nil {
		return []string{s}
	}
	var out []string
	for _, alt := range strings.Split(s[m[2]:m[3]], ",") {
		out = append(out, expandOne(s[:m[0]]+strings.TrimSpace(alt)+s[m[1]:])...)
	}
	return out
}

var reNetlifyVar = regexp.MustCompile(`(?m)^\s*(NODE_VERSION|PYTHON_VERSION|RUBY_VERSION|GO_VERSION|PHP_VERSION)\s*=\s*["']?([^"'\s]+)`)

func parseNetlify(w *repoWalk, rel, _, txt string) {
	products := map[string]string{"NODE_VERSION": "nodejs", "PYTHON_VERSION": "python", "RUBY_VERSION": "ruby", "GO_VERSION": "go", "PHP_VERSION": "php"}
	for _, m := range reNetlifyVar.FindAllStringSubmatchIndex(txt, -1) {
		w.runtime(products[txt[m[2]:m[3]]], txt[m[4]:m[5]], styleNPM, rel, lineOf(txt, m[2]))
	}
}

var (
	reDevcontainerImage   = regexp.MustCompile(`"image"\s*:\s*"([^"]+)"`)
	reDevcontainerFeature = regexp.MustCompile(`"[^"]*devcontainers/features/(node|python|java|go|ruby|php|dotnet|terraform|rust)(?::\d+)?"\s*:\s*\{[^}]*?"version"\s*:\s*"([^"]+)"`)
)

func parseDevcontainer(w *repoWalk, rel, _, txt string) {
	for _, m := range reDevcontainerImage.FindAllStringSubmatchIndex(txt, -1) {
		if r, ok := ident.ParseImageRef(txt[m[2]:m[3]]); ok {
			w.image(r, rel, lineOf(txt, m[2]), "")
		}
	}
	products := map[string]string{"node": "nodejs", "go": "go", "java": "java", "python": "python", "ruby": "ruby", "php": "php", "dotnet": "dotnet", "terraform": "terraform", "rust": "rust"}
	for _, m := range reDevcontainerFeature.FindAllStringSubmatchIndex(txt, -1) {
		w.runtime(products[txt[m[2]:m[3]]], txt[m[4]:m[5]], styleNPM, rel, lineOf(txt, m[4]))
	}
}

var reJenkinsImage = regexp.MustCompile(`(?:\bimage\s*\(?\s*|docker\.image\(\s*|\bdocker\s+)['"]([^'"\s$]+:[^'"\s$]+)['"]`)

func parseJenkinsfile(w *repoWalk, rel, _, txt string) {
	for _, m := range reJenkinsImage.FindAllStringSubmatchIndex(txt, -1) {
		if r, ok := ident.ParseImageRef(txt[m[2]:m[3]]); ok {
			w.image(r, rel, lineOf(txt, m[2]), "")
		}
	}
}

var (
	reVagrantBox     = regexp.MustCompile(`\.vm\.box\s*=\s*["']([^"']+)["']`)
	reBoxCodename    = regexp.MustCompile(`^(?:(?:ubuntu|hashicorp|debian)/)?([a-z]+)(?:32|64)$`)
	reBoxDistroVer   = regexp.MustCompile(`^(?:bento|generic|geerlingguy|centos|debian|ubuntu|roboxes|rockylinux|almalinux|fedora)/(ubuntu|centos|debian|fedora|rocky|rockylinux|almalinux|oracle)-?(\d+(?:\.\d+)?)`)
	reBoxCentOS      = regexp.MustCompile(`^centos/(\d+)$`)
	reBoxGenericUbu  = regexp.MustCompile(`^generic/ubuntu(\d\d)(\d\d)$`)
	reBoxGenericDist = regexp.MustCompile(`^generic/(debian|centos|fedora|rocky|alma)(\d+)$`)
)

var boxDistros = map[string]string{
	"ubuntu": "ubuntu", "centos": "centos", "debian": "debian", "fedora": "fedora",
	"rocky": "rocky-linux", "rockylinux": "rocky-linux", "almalinux": "almalinux", "alma": "almalinux", "oracle": "oracle-linux",
}

func parseVagrantfile(w *repoWalk, rel, _, raw string) {
	txt := stripComments(raw) // a commented-out box is not the box
	for _, m := range reVagrantBox.FindAllStringSubmatchIndex(txt, -1) {
		box, line := strings.ToLower(txt[m[2]:m[3]]), lineOf(txt, m[2])
		switch {
		case reBoxCodename.MatchString(box):
			code := reBoxCodename.FindStringSubmatch(box)[1]
			if v, ok := ubuntuCodenames[code]; ok {
				w.pin("ubuntu", v, rel, line, box)
			} else if v, ok := debianCodenames[code]; ok {
				w.pin("debian", v, rel, line, box)
			}
		case reBoxGenericUbu.MatchString(box):
			s := reBoxGenericUbu.FindStringSubmatch(box)
			w.pin("ubuntu", s[1]+"."+s[2], rel, line, box)
		case reBoxGenericDist.MatchString(box):
			s := reBoxGenericDist.FindStringSubmatch(box)
			w.pin(boxDistros[s[1]], s[2], rel, line, box)
		case reBoxCentOS.MatchString(box):
			w.pin("centos", reBoxCentOS.FindStringSubmatch(box)[1], rel, line, box)
		case reBoxDistroVer.MatchString(box):
			s := reBoxDistroVer.FindStringSubmatch(box)
			v := s[2]
			if s[1] == "centos" || s[1] == "rocky" || s[1] == "rockylinux" || s[1] == "almalinux" || s[1] == "oracle" {
				v = strings.SplitN(v, ".", 2)[0]
			}
			w.pin(boxDistros[s[1]], v, rel, line, box)
		}
	}
}

// azureTasks: Azure Pipelines tool installers and the input naming the version.
var azureTasks = map[string]struct{ input, product string }{
	"nodetool":           {"versionSpec", "nodejs"},
	"usenode":            {"version", "nodejs"},
	"usepythonversion":   {"versionSpec", "python"},
	"userubyversion":     {"versionSpec", "ruby"},
	"javatoolinstaller":  {"versionSpec", "java"},
	"gotool":             {"version", "go"},
	"usedotnet":          {"version", "dotnet"},
	"terraforminstaller": {"terraformVersion", "terraform"},
}

func (w *repoWalk) azureTask(task string, step *yaml.Node, rel string) {
	name := strings.ToLower(strings.SplitN(task, "@", 2)[0])
	t, ok := azureTasks[name]
	if !ok {
		return
	}
	if v := yamlGet(yamlGet(step, "inputs"), t.input); v != nil && v.Kind == yaml.ScalarNode {
		w.runtime(t.product, v.Value, styleNPM, rel, v.Line)
	}
}
