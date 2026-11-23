package catalog

import (
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/0xbadshah/zephy/internal/ident"
)

// parser is one file shape: a name/path matcher and the parse that emits
// every pin and package identity the file holds. Every parser whose match
// accepts a file runs on it (a workflow file is both an image source and
// a runtime-version source).
type parser struct {
	match func(rel, name string) bool
	parse func(w *repoWalk, rel, name, txt string)
}

var parsers = []parser{
	// containers
	{isDockerfile, parseDockerfile},
	{func(_, n string) bool {
		// no extension, or a version suffix (DF_fabric-tools_1.0.1)
		ext := strings.TrimLeft(path.Ext(n), ".0123456789")
		return ext == "" && !isDockerfile("", n) && n != "makefile" && n != "gemfile"
	}, parseDockerfileByContent},
	{isImageYaml, parseYamlImages},
	{named("devcontainer.json", ".devcontainer.json"), parseDevcontainer},
	{named("jenkinsfile"), parseJenkinsfile},
	{named("vagrantfile"), parseVagrantfile},
	// CI and platform runtime declarations
	{isWorkflow, parseGitHubWorkflow},
	{func(rel, _ string) bool { return rel == ".travis.yml" || rel == ".travis.yaml" }, parseTravis}, // runs from the root only
	{isCIFile, parseCIScripts},
	{named("makefile", "gnumakefile"), parseMakefile},
	{func(_, n string) bool { return strings.HasSuffix(n, ".mk") }, parseMakefile},
	{named("dab.conf"), parseDabConf},
	{named("appveyor.yml", ".appveyor.yml"), parseAppveyor},
	{isModelSource, parseModelIDs},
	{isModelSource, parseCDKRuntimes},
	{func(_, n string) bool { return strings.HasSuffix(n, ".sql") }, parseSQLRuntimes},
	{func(_, n string) bool {
		return strings.HasSuffix(n, ".bat") || strings.HasSuffix(n, ".cmd") || strings.HasSuffix(n, ".ps1")
	}, parseWindowsScript},
	{func(_, n string) bool { return strings.HasSuffix(n, ".json") || strings.HasSuffix(n, ".pkr.hcl") }, parsePackerTemplate},
	{named("wercker.yml"), parseWercker},
	{named("ember-try.js"), parseEmberTry},
	{func(_, n string) bool { return strings.HasSuffix(n, ".txt") }, parseRequirementsLike},
	{named("versions.env", "version.env", "versions.properties"), parseVersionsEnv},
	{func(_, n string) bool { return strings.HasSuffix(n, ".gemspec") }, parseGemspec},
	{func(_, n string) bool { return strings.HasSuffix(n, ".sqlproj") }, parseSqlproj},
	{func(rel, n string) bool { // Capistrano: set :rbenv_ruby, '2.2.2'
		return n == "deploy.rb" || strings.HasPrefix(rel, "config/deploy/") && strings.HasSuffix(n, ".rb")
	}, func(w *repoWalk, rel, _, txt string) { w.scriptRuntimes(rel, txt) }},
	{func(rel, n string) bool {
		return strings.HasSuffix(rel, ".moon/toolchain.yml") || strings.HasSuffix(rel, ".moon/toolchains.yml")
	}, parseMoonToolchain},
	{func(_, n string) bool { return strings.HasSuffix(n, ".vcxproj") }, parseVcxproj},
	{func(_, n string) bool {
		return strings.HasSuffix(n, ".wdl") || strings.HasSuffix(n, ".nf") || n == "nextflow.config" || strings.HasSuffix(n, ".cwl")
	}, parseWorkflowLang},
	{named("bower.json", "component.json"), parseBower},
	{named("packages.config"), parseNuGetPackagesConfig},
	{func(rel, n string) bool {
		return strings.Contains(rel, "depends/packages/") && strings.HasSuffix(n, ".mk")
	}, parseDependsMk},
	{named("circle.yml"), parseCircle1},
	{named("snapcraft.yaml"), parseSnapcraft},
	{named("pantheon.yml", "pantheon.upstream.yml"), parsePantheon},
	{named("package.xml"), parseMagentoPackage},
	{func(rel, n string) bool {
		return n == "version.php" && strings.HasSuffix(rel, "wp-includes/version.php")
	}, single(reWPVersion, "wordpress")},
	{named(".scrutinizer.yml", ".scrutinizer.yaml"), parseScrutinizer},
	{named(".cirrus.yml", ".cirrus.yaml"), parseCirrus},
	{named("manifest.mf"), parseManifestMF},
	{named("org.eclipse.jdt.core.prefs"), parseJDTPrefs},
	{func(rel, n string) bool { return n == "misc.xml" && strings.Contains(rel, ".idea/") }, parseIdeaMisc},
	{func(_, n string) bool {
		return strings.HasSuffix(n, ".def") || n == "singularity" || strings.HasPrefix(n, "singularity.")
	}, parseSingularity},
	{func(_, n string) bool { return strings.HasSuffix(n, ".tfvars") }, parseTfvars},
	{func(_, n string) bool { return strings.HasPrefix(n, ".kitchen") && isYaml("", n) }, parseKitchen},
	{func(_, n string) bool {
		return strings.HasSuffix(n, ".template") || strings.HasSuffix(n, ".yaml") || strings.HasSuffix(n, ".yml") || strings.HasSuffix(n, ".json")
	}, parseCloudFormation},
	{func(_, n string) bool { return strings.HasSuffix(n, ".nix") }, parseNix},
	{named(".readthedocs.yml", ".readthedocs.yaml", "readthedocs.yml", "readthedocs.yaml"), parseReadTheDocs},
	{isGitLabCI, parseGitLabCI},
	{isYaml, parseYamlRuntimeKeys},
	{named("tox.ini"), parseTox},
	{named("netlify.toml"), parseNetlify},
	// language runtimes and their package ecosystems
	{named("package.json"), parsePackageJSON},
	{named("composer.json"), parseComposerJSON},
	{func(_, n string) bool { return n == "gemfile.lock" || strings.HasSuffix(n, ".gemfile.lock") }, parseGemfileLock},
	{func(_, n string) bool { return n == "gemfile" || n == "gems.rb" || strings.HasSuffix(n, ".gemfile") }, parseGemfile}, // + Appraisal gemfiles/*.gemfile
	{named("project.clj"), parseLeiningen},
	{named("build.xml"), parseAntBuild},
	{named("mix.exs"), parseMixExs},
	{func(rel, n string) bool {
		d := path.Base(path.Dir(rel))
		return (d == "group_vars" || d == "host_vars" || d == "vars" || d == "defaults") && isYaml(rel, n)
	}, parseAnsibleVars},
	{func(_, n string) bool { return strings.HasSuffix(n, ".sh") || strings.HasSuffix(n, ".bash") }, parseShellScript},
	{func(rel, n string) bool {
		return n == "build.sbt" || (strings.HasSuffix(n, ".scala") && strings.HasPrefix(path.Base(path.Dir(rel)), "project"))
	}, parseSbtDeps},
	{named("project.properties"), parseNetBeans},
	{func(rel, _ string) bool { return strings.Contains(rel, ".elasticbeanstalk/") }, parseElasticBeanstalk},
	{func(_, n string) bool {
		return (strings.HasPrefix(n, "org.") || strings.HasPrefix(n, "com.") || strings.HasPrefix(n, "io.") || strings.Contains(n, "flatpak")) &&
			(strings.HasSuffix(n, ".json") || strings.HasSuffix(n, ".yml") || strings.HasSuffix(n, ".yaml"))
	}, parseFlatpak},
	{isRequirements, parseRequirements},
	{named("pipfile"), parsePipfile},
	{named("pyproject.toml"), parsePyproject},
	{named("setup.py"), parseSetupPy},
	{isImageYaml, parseCondaEnvByContent},
	{named("go.mod"), parseGoMod},
	{named("pom.xml"), parsePom},
	{isGradle, parseGradle},
	{func(_, n string) bool {
		return n == "gradle.properties" || (strings.HasPrefix(n, "version") && strings.HasSuffix(n, ".properties"))
	}, parseVersionProperties},
	{named("gradle-wrapper.properties"), single(reGradleWrap, "gradle")},
	{named("maven-wrapper.properties"), single(reMavenWrap, "apache-maven")},
	{named("libs.versions.toml"), parseVersionCatalog},
	{named("build.sbt"), single(reScalaVersion, "scala")},
	{isDotnetProject, parseDotnetProject},
	{named("global.json"), parseGlobalJSON},
	{named("project.json"), parseProjectJSON},
	{named("web.config", "app.config"), parseWebConfig},
	{named(".terraform.lock.hcl"), parseTerraformLock},
	{func(_, n string) bool { return strings.HasSuffix(n, ".tf") }, parseTerraform},
	// version files
	{named(".ruby-version"), versionFile(reVersion, "ruby")},
	{named(".rvmrc"), single(reRvmrc, "ruby")},
	{named(".python-version"), parsePythonVersionFile},
	{named(".nvmrc", ".node-version"), parseNvmrc},
	{named(".java-version"), versionFileFunc(func(v string) (string, string) { return "java", javaVersion(v) })},
	{named(".go-version"), versionFile(reVersion, "go")},
	{named(".terraform-version"), versionFile(reVersion, "terraform")},
	{named(".php-version"), versionFile(reVersion, "php")},
	{named("runtime.txt"), versionFile(reRuntimeTxt, "python")},
	{named(".tool-versions"), parseToolVersions},
	{named("mise.toml", ".mise.toml", ".rtx.toml", "rtx.toml"), parseMise},
	{named("module.yaml", "project.yaml"), parseAmper},
	{named(".sdkmanrc"), parseSdkmanrc},
	{func(_, n string) bool {
		// rust-toolchain(.toml), and pinned-toolchain.toml that flakes read
		return n == "rust-toolchain" || strings.HasSuffix(n, "toolchain.toml")
	}, parseRustToolchain},
	{named("projectversion.txt"), single(reUnityEditor, "unity")},
	{named("project.godot"), parseGodotProject},
	{named("stack.yaml"), parseStackYaml},
	{named("buildozer.spec"), parseBuildozer},
	{named("cmakelists.txt"), parseCMake},
	{named(".pre-commit-config.yaml", ".pre-commit-config.yml"), parsePreCommit},
	{named("action.yml", "action.yaml"), parseActionMetadata},
}

var (
	reGradleWrap   = regexp.MustCompile(`gradle-(\d+\.\d+(?:\.\d+)?)-`)
	reMavenWrap    = regexp.MustCompile(`apache-maven-(\d+\.\d+(?:\.\d+)?)-`)
	reScalaVersion = regexp.MustCompile(`(?m)^\s*(?:ThisBuild\s*/\s*)?scalaVersion\s*:=\s*"(\d+\.\d+(?:\.\d+)?)"`)
	reVersion      = regexp.MustCompile(`(\d+\.\d+(?:\.\d+)?)`)
	reLineVersion  = regexp.MustCompile(`^\s*(?:python-?|cpython-)?(\d+\.\d+(?:\.\d+)?)\b`) // not anaconda3-2020.02
	reNodeVersion  = regexp.MustCompile(`^\s*(?:node-?)?v?(\d+(?:\.\d+){0,2})(?:\.[xX*])*\s*$`)
	reRuntimeTxt   = regexp.MustCompile(`python-(\d+\.\d+(?:\.\d+)?)|^\s*(\d\.\d+(?:\.\d+)?)\s*$`)
	reRvmrc        = regexp.MustCompile(`(?m)^\s*rvm\s+(?:use\s+)?(?:ruby-)?(\d+\.\d+(?:\.\d+)?)`)
	reWPVersion    = regexp.MustCompile(`\$wp_version\s*=\s*'(\d+\.\d+(?:\.\d+)?)'`)
	reUnityEditor  = regexp.MustCompile(`(?m)^m_EditorVersion:\s*(\d+\.\d+(?:\.\d+)?)`)
)

func named(names ...string) func(string, string) bool {
	return func(_, n string) bool {
		for _, x := range names {
			if n == x {
				return true
			}
		}
		return false
	}
}

var reLeadingFrom = regexp.MustCompile(`^(?:\s*(?:#[^\n]*|ARG [^\n]*)?\n)*\s*FROM\s+\S`)

// parseDockerfileByContent: Dockerfiles under custom names (DF_debian,
// build/base) are recognized by a leading FROM.
func parseDockerfileByContent(w *repoWalk, rel, name, txt string) {
	if len(txt) < 64<<10 && reLeadingFrom.MatchString(txt) {
		parseDockerfile(w, rel, name, txt)
	}
}

func isDockerfile(_, n string) bool {
	return strings.HasPrefix(n, "dockerfile") || strings.HasSuffix(n, ".dockerfile") || strings.HasPrefix(n, "containerfile") ||
		strings.HasSuffix(n, ".dockerimage")
}

func isYaml(rel, n string) bool {
	if strings.HasSuffix(n, ".yaml") || strings.HasSuffix(n, ".yml") {
		return true
	}
	// Ansible inventory vars are YAML without an extension: group_vars/all
	d := path.Base(path.Dir(rel))
	return (d == "group_vars" || d == "host_vars") && !strings.Contains(n, ".")
}

// isImageYaml: any YAML that is not a package-manager lockfile.
func isImageYaml(rel, n string) bool {
	return isYaml(rel, n) && n != "pnpm-lock.yaml" && n != "yarn.lock" && !strings.HasSuffix(n, "lock.yaml")
}

// isWorkflow: GitHub runs workflows from the repository root only; a
// vendored project's .github/workflows never executes.
func isWorkflow(rel, n string) bool {
	return strings.HasPrefix(rel, ".github/workflows/") && isYaml(rel, n)
}

func isGitLabCI(rel, n string) bool {
	return n == ".gitlab-ci.yml" || n == ".gitlab-ci.yaml" || (strings.Contains(rel, ".gitlab/") && isYaml(rel, n))
}

func isRequirements(rel, n string) bool {
	if n == "requirements" || n == "requirements-dev" || n == "requirements_dev" {
		return true // no extension
	}
	txt := strings.HasSuffix(n, ".txt") || strings.HasSuffix(n, ".in")
	return (strings.Contains(n, "requirement") && txt) ||
		(strings.HasPrefix(n, "constraints") && strings.HasSuffix(n, ".txt")) ||
		(strings.Contains(strings.ToLower(rel), "requirements/") && txt) // requirements/base.txt
}

func isCondaEnv(rel, n string) bool {
	return (strings.HasPrefix(n, "environment") || strings.HasPrefix(n, "conda")) && isYaml(rel, n)
}

func isGradle(_, n string) bool {
	return strings.HasSuffix(n, ".gradle") || strings.HasSuffix(n, ".gradle.kts")
}

func isDotnetProject(_, n string) bool {
	for _, ext := range []string{".csproj", ".fsproj", ".vbproj", ".props", ".targets"} {
		if strings.HasSuffix(n, ext) {
			return true
		}
	}
	return false
}

// single emits one lifecycle pin from the first capture of re.
func single(re *regexp.Regexp, product string) func(*repoWalk, string, string, string) {
	return func(w *repoWalk, rel, _, txt string) {
		if m := re.FindStringSubmatchIndex(txt); m != nil {
			w.pin(product, txt[m[2]:m[3]], rel, lineOf(txt, m[2]), rel)
		}
	}
}

// versionFile emits the version a whole-file pin (.ruby-version) holds.
func versionFile(re *regexp.Regexp, product string) func(*repoWalk, string, string, string) {
	return func(w *repoWalk, rel, _, txt string) {
		if m := re.FindStringSubmatch(strings.TrimSpace(stripComments(txt))); m != nil {
			for _, v := range m[1:] { // the first alternative that matched
				if v != "" {
					w.pin(product, v, rel, 0, rel)
					break
				}
			}
		}
	}
}

// versionFileLines: pyenv-style files may list several versions.
func versionFileLines(re *regexp.Regexp, product string) func(*repoWalk, string, string, string) {
	return func(w *repoWalk, rel, _, txt string) {
		for i, line := range strings.Split(txt, "\n") {
			if m := re.FindStringSubmatch(stripComments(line)); m != nil {
				w.pin(product, m[1], rel, i+1, rel)
			}
		}
	}
}

func versionFileFunc(f func(string) (string, string)) func(*repoWalk, string, string, string) {
	return func(w *repoWalk, rel, _, txt string) {
		if m := reVersion.FindStringSubmatch(stripComments(txt)); m != nil {
			p, v := f(m[1])
			w.pin(p, v, rel, 0, rel)
		} else if m := regexp.MustCompile(`\b(\d+)\b`).FindStringSubmatch(stripComments(txt)); m != nil {
			p, v := f(m[1])
			w.pin(p, v, rel, 0, rel)
		}
	}
}

func stripComments(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// parseNvmrc: a version, or an LTS codename (lts/dubnium = Node 10).
func parseNvmrc(w *repoWalk, rel, name, txt string) {
	line := strings.ToLower(strings.TrimSpace(stripComments(txt)))
	if code, ok := strings.CutPrefix(line, "lts/"); ok {
		if v, ok := nodeCodenames[code]; ok {
			w.pin("nodejs", v, rel, 0, line)
		}
		return
	}
	versionFile(reNodeVersion, "nodejs")(w, rel, name, txt)
}

// lineOf maps a byte offset in text to a 1-based line number.
func lineOf(text string, offset int) int {
	return strings.Count(text[:offset], "\n") + 1
}

// dep records a declared dependency: its package identity, and - when the
// package is itself a tracked product - a lifecycle pin for the version
// its specifier pins (exact, or the release line a range is confined to).
func (w *repoWalk) dep(eco, name, spec string, style specStyle, rel string, line int) {
	spec = strings.TrimSpace(spec)
	if eco == "npm" && strings.HasPrefix(spec, "npm:") {
		// an alias ("vue2": "npm:vue@^2.6.14") depends on the aliased package
		if at := strings.LastIndex(spec, "@"); at > 4 {
			name, spec = strings.ToLower(spec[4:at]), spec[at+1:]
		}
	}
	if eco == "bower" {
		if i := strings.LastIndex(spec, "#"); i >= 0 {
			spec = spec[i+1:] // "driftyco/ionic-bower#1.0.0-rc.1"
		}
	}
	if eco == "maven" && strings.HasPrefix(name, "org.springframework.cloud:spring-cloud-dependencies") {
		if train, ok := springCloudBOM(spec); ok { // codename trains: Finchley.SR2
			w.pin("spring-cloud", train, rel, line, name+" "+spec)
			return
		}
	}
	v, ok := specVersion(spec, style)
	if !ok {
		return
	}
	if eco == "maven" && strings.HasPrefix(name, "org.springframework.cloud:") && strings.Contains(name, "dependencies") {
		if train, ok := springCloudBOM(spec); ok {
			w.pin("spring-cloud", train, rel, line, name+" "+spec)
		}
	}
	if eco == "maven" && strings.HasPrefix(name, "org.springframework.cloud:") && !strings.Contains(name, "dependencies") {
		// component versions map to release trains: 2.0 = Finchley,
		// 3.0 = 2020.0, 4.1 = 2023.0
		if train, ok := springCloudTrain(v); ok {
			w.pin("spring-cloud", train, rel, line, name+" "+spec)
		}
	}
	if eco == "composer" && name == "magento/framework" {
		// magento/framework 100.1 ships in Magento 2.1, 101.0 in 2.2, ...
		major, _ := strconv.Atoi(strings.SplitN(v, ".", 2)[0])
		minor := 0
		if p := strings.Split(v, "."); len(p) > 1 {
			minor, _ = strconv.Atoi(p[1])
		}
		if major >= 100 {
			mv := map[int]string{100: "2.1", 101: "2.2", 102: "2.3", 103: "2.4"}[major]
			if major == 100 && minor == 0 {
				mv = "2.0"
			}
			if mv != "" {
				w.pin("magento", mv, rel, line, "magento/framework "+spec)
			}
		}
	}
	if eco == "maven" && strings.HasPrefix(name, "org.webjars:") {
		// a webjar is a web library repackaged for JVM builds:
		// org.webjars:bootstrap 4.0.0 is Bootstrap 4
		if slug, ok := librarySlug("npm", name[len("org.webjars:"):]); ok {
			w.pin(slug, v, rel, line, name+" "+spec)
		}
	}
	w.inv.Packages = append(w.inv.Packages, PackagePin{Ecosystem: eco, Name: name, Version: v, File: loc(rel, line)})
	if slug, ok := librarySlug(eco, name); ok {
		w.pin(slug, v, rel, line, eco+":"+name+" "+spec)
	}
}

// runtime records a runtime declared by a version specifier (engines,
// CI matrices, requires-python): floors declare nothing.
func (w *repoWalk) runtime(product, spec string, style specStyle, rel string, line int) {
	if product == "ruby" && strings.HasPrefix(strings.TrimSpace(spec), "jruby-") {
		product, spec = "jruby", strings.TrimPrefix(strings.TrimSpace(spec), "jruby-") // setup-ruby/rvm: jruby-9.3
	}
	if v, ok := specVersion(strings.TrimSpace(spec), style); ok {
		if product == "java" {
			v = javaVersion(v)
		}
		w.pin(product, v, rel, line, spec)
	}
}

var (
	reYamlImage  = regexp.MustCompile(`^\s*-?\s*(?:[\w-]*[iI]mage|repository)\s*:\s*["']?((?:\$\{[^}]*\}|[^\s"'#{])+)`)
	reYamlReg    = regexp.MustCompile(`^registry\s*:\s*["']?([^\s"'#]+)`)
	reYamlTag    = regexp.MustCompile(`^tag\s*:\s*["']?([A-Za-z0-9_.\-]+)`)
	reTopKey     = regexp.MustCompile(`^([A-Za-z][\w.\-/]*):`)
	reEnabledOff = regexp.MustCompile(`^\s*enabled:\s*false\s*$`)
)

var (
	reCircleParamRef     = regexp.MustCompile(`<<\s*parameters\.([\w-]+)\s*>>`)
	reCircleParamDefault = regexp.MustCompile(`(?m)^(\s+)([\w-]+):\s*\n(?:\s+(?:type|description):.*\n)*\s+default:\s*["']?([\w.\-]+)`)
)

// circleParamDefaults substitutes CircleCI executor/job parameter
// defaults (circleci/node:<< parameters.image >> with default "10").
func circleParamDefaults(txt string) string {
	if !strings.Contains(txt, "<< parameters.") && !strings.Contains(txt, "<<parameters.") {
		return txt
	}
	defaults := map[string]string{}
	for _, m := range reCircleParamDefault.FindAllStringSubmatch(txt, -1) {
		defaults[m[2]] = m[3]
	}
	return reCircleParamRef.ReplaceAllStringFunc(txt, func(ref string) string {
		if v, ok := defaults[reCircleParamRef.FindStringSubmatch(ref)[1]]; ok {
			return v
		}
		return ref
	})
}

func parseYamlImages(w *repoWalk, rel, name, txt string) {
	if strings.HasPrefix(rel, ".circleci/") {
		txt = circleParamDefaults(txt)
	}
	var disabled map[string]bool
	if strings.HasPrefix(name, "values") {
		st := w.helmFor(path.Dir(rel))
		disabled = st.overlays
		if name == "values.yaml" || name == "values.yml" {
			disabled = st.own
		}
	}
	parseImageYaml(w, rel, txt, disabled)
}

// parseImageYaml extracts image references from compose, kubernetes, helm
// values and CI YAML:
// - one-line forms: image: postgres:14
// - helm split pairs: repository: postgres / tag: "13"
// - chart triples: registry: + image: + tag: across three lines
// When disabled is non-nil, top-level components switched off with
// "enabled: false" are recorded there.
func parseImageYaml(w *repoWalk, rel, txt string, disabled map[string]bool) {
	const maxPairDistance = 15 // lines between repository:/image: and tag:
	var pendingRepo, pendingRegistry string
	pendingLine := 0
	topKey := ""
	for i, line := range strings.Split(txt, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if m := reTopKey.FindStringSubmatch(line); m != nil {
			topKey = m[1]
		}
		if disabled != nil && topKey != "" && reEnabledOff.MatchString(line) {
			disabled[topKey] = true
		}
		if m := reYamlImage.FindStringSubmatch(trimmed); m != nil {
			pendingRepo, pendingRegistry, pendingLine = "", "", 0
			ref := m[1]
			if strings.Contains(ref, "${") {
				// compose interpolation: postgres:${PG_VERSION:-12} -> the default
				if expanded, ok := expandVars(ref, nil); ok {
					ref = expanded
				}
			}
			r, ok := ident.ParseImageRef(ref)
			if !ok {
				continue
			}
			if r.Tag != "" || r.Digest != "" {
				w.image(r, rel, i+1, topKey)
			} else {
				pendingRepo, pendingLine = ref, i // tag: expected on a nearby line
			}
			continue
		}
		if m := reYamlReg.FindStringSubmatch(trimmed); m != nil {
			pendingRegistry = strings.TrimSuffix(m[1], "/")
			continue
		}
		if pendingRepo != "" && i-pendingLine <= maxPairDistance {
			if m := reYamlTag.FindStringSubmatch(trimmed); m != nil {
				full := pendingRepo
				if pendingRegistry != "" {
					full = pendingRegistry + "/" + pendingRepo
				}
				if r, ok := ident.ParseImageRef(full + ":" + m[1]); ok {
					w.image(r, rel, pendingLine+1, topKey)
				}
				pendingRepo, pendingRegistry = "", ""
			}
		}
	}
}

// springCloudTrain maps a Spring Cloud component version to its release
// train; trains before 2020.0 (Hoxton and older, components < 3.0) are
// all past EOL and reported against the oldest tracked train.
func springCloudTrain(v string) (string, bool) {
	p := strings.Split(v, ".")
	if len(p) < 2 {
		return "", false
	}
	major, _ := strconv.Atoi(p[0])
	minor, _ := strconv.Atoi(p[1])
	switch {
	case major < 3:
		return "2019", true // pre-2020.0 train: below the dead 2020.0
	case major == 3:
		return "202" + strconv.Itoa(minor) + ".0", true // 3.0 = 2020.0, 3.1 = 2021.0
	case major == 4:
		return "202" + strconv.Itoa(minor+2) + ".0", true // 4.0 = 2022.0, 4.3 = 2025.0
	}
	return "", false
}

var reCloudTrain = regexp.MustCompile(`^(?i)(angel|brixton|camden|dalston|edgware|finchley|greenwich|hoxton|ilford)\b|^(20\d\d\.\d+)`)

// springCloudBOM reads a release-train version: codenames (Finchley.SR2)
// predate the tracked year-numbered trains; 2021.0.3 is train 2021.0.
func springCloudBOM(v string) (string, bool) {
	m := reCloudTrain.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return "", false
	}
	if m[1] != "" {
		return "2019", true // every codename train ended before 2020.0
	}
	return m[2], true
}
