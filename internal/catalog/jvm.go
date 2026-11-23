package catalog

import (
	"encoding/xml"
	"path"
	"regexp"
	"strings"
)

// ---- Maven ----

type pomDep struct {
	group, artifact, version string
	offset                   int64
}

// parsePom reads a pom.xml with its properties resolved: the parent, every
// dependency and plugin coordinate, and the Java level the build targets.
var (
	reXMLComment   = regexp.MustCompile(`(?s)<!--.*?-->`)
	rePackagingWar = regexp.MustCompile(`<packaging>\s*war\s*</packaging>`)
)

// blankKeepLines replaces text with spaces, keeping its newlines.
func blankKeepLines(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		return ' '
	}, s)
}

var reTomcatPlugin = regexp.MustCompile(`<artifactId>\s*tomcat([67])-maven-plugin\s*</artifactId>`)

func parsePom(w *repoWalk, rel, _, txt string) {
	// tomcat7-maven-plugin runs and embeds (exec-war) a Tomcat 7 - for war
	// modules only: tomcat7:run skips anything else. Comments blanked in place.
	if code := reXMLComment.ReplaceAllStringFunc(txt, blankKeepLines); rePackagingWar.MatchString(code) {
		if m := reTomcatPlugin.FindStringSubmatchIndex(code); m != nil {
			w.pin("tomcat", code[m[2]:m[3]], rel, lineOf(code, m[2]), "tomcat"+code[m[2]:m[3]]+"-maven-plugin")
		}
	}
	dec := xml.NewDecoder(strings.NewReader(txt))
	dec.Strict = false
	props := map[string]string{}
	propOffsets := map[string]int64{}
	var stack []string
	var deps []pomDep
	var cur *pomDep
	var javaLevels []pomDep // reuse: artifact = property name, version = value
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			switch t.Name.Local {
			case "parent", "dependency", "plugin", "extension", "artifactItem":
				cur = &pomDep{}
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			switch t.Name.Local {
			case "parent", "dependency", "plugin", "extension", "artifactItem":
				if cur != nil && cur.artifact != "" {
					deps = append(deps, *cur)
				}
				cur = nil
			}
		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			val := strings.TrimSpace(string(t))
			if val == "" {
				continue
			}
			leaf := stack[len(stack)-1]
			parent := ""
			if len(stack) > 1 {
				parent = stack[len(stack)-2]
			}
			off := dec.InputOffset()
			switch {
			case parent == "properties":
				props[leaf] = val
				propOffsets[leaf] = off
				switch leaf {
				case "java.version", "maven.compiler.source", "maven.compiler.target", "maven.compiler.release", "jdk.version", "java.release":
					javaLevels = append(javaLevels, pomDep{artifact: leaf, version: val, offset: off})
				}
			case cur != nil && (parent == "parent" || parent == "dependency" || parent == "plugin" || parent == "extension" || parent == "artifactItem"):
				switch leaf {
				case "groupId":
					cur.group = val
				case "artifactId":
					cur.artifact = val
				case "version":
					cur.version, cur.offset = val, off
				}
			case parent == "configuration" && (leaf == "release" || leaf == "source"):
				javaLevels = append(javaLevels, pomDep{artifact: leaf, version: val, offset: off})
			}
		}
	}
	own := map[string]bool{}
	for k := range props {
		own[k] = true
	}
	// properties inherited from the parent pom (../pom.xml) and the root pom
	for _, parentPom := range []string{path.Join(path.Dir(rel), "..", "pom.xml"), "pom.xml"} {
		if parentPom == rel || strings.HasPrefix(parentPom, "..") {
			continue
		}
		if ptxt, ok := w.read(parentPom); ok {
			for k, v := range pomProperties(ptxt) {
				if _, own := props[k]; !own {
					props[k] = v
				}
			}
		}
	}
	resolve := func(v string) (string, bool) {
		for i := 0; i < 5 && strings.Contains(v, "${"); i++ {
			v = rePomVar.ReplaceAllStringFunc(v, func(m string) string {
				if r, ok := props[rePomVar.FindStringSubmatch(m)[1]]; ok {
					return r
				}
				return m
			})
		}
		return v, v != "" && !strings.Contains(v, "${")
	}
	bootParent := false
	for _, d := range deps {
		if d.group == "org.springframework.boot" && (d.artifact == "spring-boot-starter-parent" || d.artifact == "spring-boot-dependencies") {
			bootParent = true
		}
	}
	// a coordinate declared twice in one pom: Maven keeps the last one
	last := map[string]int{}
	for i, d := range deps {
		last[d.group+":"+d.artifact] = i
	}
	for i, d := range deps {
		if last[d.group+":"+d.artifact] != i {
			continue
		}
		v, ok := resolve(d.version)
		if !ok || d.group == "" {
			continue
		}
		w.dep("maven", d.group+":"+d.artifact, v, stylePEP440, rel, lineAtOffset(txt, d.offset))
	}
	for _, j := range javaLevels {
		if v, ok := resolve(j.version); ok && reLeadingVersion.MatchString(v) {
			w.pin("java", javaVersion(reLeadingVersion.FindString(v)), rel, lineAtOffset(txt, j.offset), j.artifact+"="+v)
		}
	}
	// kotlin.version and friends name the product directly
	for name, product := range pomVersionProperties {
		// defined here, and used by some pom - or consumed by the Spring Boot
		// parent, which reads its managed-version overrides by name
		if v, ok := props[name]; ok && own[name] && ((bootParent && bootManaged[name]) || w.pomPropertyUsed(rel, name)) {
			if product == "spring-cloud" {
				if train, ok := springCloudBOM(v); ok {
					w.pin(product, train, rel, lineAtOffset(txt, propOffsets[name]), name+"="+v)
				}
				continue
			}
			if rv, ok := resolve(v); ok {
				w.pin(product, rv, rel, lineAtOffset(txt, propOffsets[name]), name+"="+rv)
			}
		}
	}
}

// bootManaged: properties the Spring Boot parent reads to override its
// managed versions.
var bootManaged = map[string]bool{
	"spring-framework.version": true, "spring-security.version": true, "log4j2.version": true,
	"tomcat.version": true, "jetty.version": true, "hibernate.version": true, "kotlin.version": true,
	"groovy.version": true, "elasticsearch.version": true, "kafka.version": true, "solr.version": true,
}

// pomPropertyUsed: a version property only matters when a pom it reaches
// references it: its own, a parent's (which it overrides) or a module's
// below it (which inherits it). A sibling module's same-named property is
// a different value.
func (w *repoWalk) pomPropertyUsed(rel, name string) bool {
	ref := "${" + name + "}"
	dir := path.Dir(rel)
	self := ""
	if txt, ok := w.read(rel); ok {
		self = pomArtifactID(txt)
	}
	for f := range w.files {
		if path.Base(f) != "pom.xml" {
			continue
		}
		txt, ok := w.read(f)
		if !ok || !strings.Contains(txt, ref) {
			continue
		}
		// a module whose <parent> is this pom inherits it from anywhere
		// (parent/pom.xml beside its modules)
		if d := path.Dir(f); underDir(dir, d) || underDir(d, dir) || (self != "" && pomParentID(txt) == self) {
			return true
		}
	}
	return false
}

var (
	reParentBlock = regexp.MustCompile(`(?s)<parent>.*?</parent>`)
	reArtifactID  = regexp.MustCompile(`<artifactId>\s*([^<\s]+)\s*</artifactId>`)
)

func pomArtifactID(txt string) string {
	if m := reArtifactID.FindStringSubmatch(reParentBlock.ReplaceAllString(txt, "")); m != nil {
		return m[1]
	}
	return ""
}

func pomParentID(txt string) string {
	if b := reParentBlock.FindString(txt); b != "" {
		if m := reArtifactID.FindStringSubmatch(b); m != nil {
			return m[1]
		}
	}
	return ""
}

// underDir: p is dir or inside it ("." holds everything).
func underDir(p, dir string) bool {
	return dir == "." || p == dir || strings.HasPrefix(p, dir+"/")
}

// pomVersionProperties: properties that set a tracked product's version,
// including Spring Boot's managed-version overrides.
var pomVersionProperties = map[string]string{
	"kotlin.version": "kotlin", "spring-boot.version": "spring-boot", "spring.boot.version": "spring-boot",
	"spring.version": "spring-framework", "spring-framework.version": "spring-framework",
	"spring-security.version": "spring-security", "scala.version": "scala", "quarkus.platform.version": "quarkus-framework",
	"quarkus.version": "quarkus-framework", "micronaut.version": "micronaut", "log4j2.version": "log4j",
	"log4j.version": "log4j", "tomcat.version": "tomcat", "jetty.version": "eclipse-jetty",
	"hibernate.version": "hibernate-orm", "groovy.version": "apache-groovy", "activemq.version": "apache-activemq",
	"kafka.version": "apache-kafka", "spark.version": "apache-spark", "hadoop.version": "apache-hadoop",
	"solr.version": "solr", "elasticsearch.version": "elasticsearch", "spring-cloud.version": "spring-cloud",
	// jenkins.baseline deliberately absent: it is the plugin's minimum
	// supported core (a floor), and jenkins.version derives from it.
}

var rePomVar = regexp.MustCompile(`\$\{([\w.\-]+)\}`)

var rePomProperties = regexp.MustCompile(`(?s)<properties>(.*?)</properties>`)
var rePomProperty = regexp.MustCompile(`<([\w.\-]+)>\s*([^<\s]+)\s*</([\w.\-]+)>`)

// pomProperties reads a pom's <properties> block.
func pomProperties(txt string) map[string]string {
	out := map[string]string{}
	for _, block := range rePomProperties.FindAllStringSubmatch(txt, -1) {
		for _, m := range rePomProperty.FindAllStringSubmatch(block[1], -1) {
			if m[1] == m[3] {
				out[m[1]] = m[2]
			}
		}
	}
	return out
}

func lineAtOffset(txt string, off int64) int {
	if off <= 0 || int(off) > len(txt) {
		return 0
	}
	return lineOf(txt, int(off))
}

// ---- Gradle ----

var (
	reGradleCoord       = regexp.MustCompile(`['"]([\w.\-]+):([\w.\-]+):([\w.\-${}]+)(?::[\w]+)?['"]`)
	reGradlePluginID    = regexp.MustCompile(`id\s*\(?\s*["']([\w.\-]+)["']\s*\)?\s+version\s*\(?\s*["']([^"']+)["']`)
	reGradleKotlinPl    = regexp.MustCompile(`kotlin\s*\(\s*["'][\w.\-]+["']\s*\)\s+version\s*\(?\s*["']([^"']+)["']`)
	reGradleJavaCompat  = regexp.MustCompile(`(?:source|target)Compatibility\s*=?\s*\(?\s*(?:JavaVersion\.VERSION_|["']?)(\d+(?:[._]\d+)?)`)
	reGradleToolchain   = regexp.MustCompile(`JavaLanguageVersion\.of\(\s*["']?(\d+)`)
	reGradleJvmTarget   = regexp.MustCompile(`jvmTarget\s*(?:=|\.set\()\s*(?:JvmTarget\.JVM_|["'])(\d+(?:[._]\d+)?)`)
	reGradleVarDef      = regexp.MustCompile(`(?m)^\s*(?:ext\.|def\s+|val\s+|var\s+|extra\[["']|set\(["'])?([A-Za-z_][\w]*)["']?\]?\s*(?:=|,)\s*["']([\w.\-]+)["']`)
	reGradleKotlinVar   = regexp.MustCompile(`(?:\b(?:ext\.)?kotlin_?[vV]ersion\s*=\s*|safeExtGet\(\s*['"]kotlin_?[vV]ersion['"]\s*,\s*)['"](\d+\.\d+(?:\.\d+)?)[\w.\-]*['"]`)
	reGradleWrapperTask = regexp.MustCompile(`gradleVersion\s*=\s*['"](\d+\.\d+(?:\.\d+)?)['"]`)

	// requested.group == "org.springframework" -> useVersion("6.2.15"):
	// positional pairing, not lookahead (RE2 has none by design)
	reGradleGroupCheck = regexp.MustCompile(`requested\.group\s*==\s*["']([\w.]+)["']`)
	reGradleUseVersion = regexp.MustCompile(`useVersion\(["']([\d.]+)`)

	gradleForcedGroups = map[string]string{
		"org.springframework.security": "spring-security",
		"org.springframework":          "spring-framework",
		"org.apache.tomcat.embed":      "tomcat",
		"org.jetbrains.kotlin":         "kotlin",
	}
	reGradleNodePlugin = regexp.MustCompile(`(?s)\bnode\s*\{[^}]*?\bversion\s*(?:=|\.set\()\s*['"]([\w.$\{\}]+)['"]`)
	reGradlePropLine   = regexp.MustCompile(`(?m)^\s*([A-Za-z_][\w.\-]*)\s*[=:]\s*(\S+)`)
)

var gradlePlugins = map[string]string{
	"org.springframework.boot":           "spring-boot",
	"org.jetbrains.kotlin.jvm":           "kotlin",
	"org.jetbrains.kotlin.android":       "kotlin",
	"org.jetbrains.kotlin.multiplatform": "kotlin",
	"org.jetbrains.kotlin.plugin.spring": "kotlin",
	"io.quarkus":                         "quarkus-framework",
	"io.micronaut.application":           "micronaut",
}

func (w *repoWalk) gradleVars(rel, txt string) map[string]string {
	vars := map[string]string{}
	for _, dir := range []string{path.Dir(rel), "."} {
		if props, ok := w.read(path.Join(dir, "gradle.properties")); ok {
			for _, m := range reGradlePropLine.FindAllStringSubmatch(props, -1) {
				if _, seen := vars[m[1]]; !seen {
					vars[m[1]] = m[2]
				}
			}
		}
	}
	// ext properties of enclosing build scripts reach their subprojects
	// (the repo root, or android/ in a Flutter app)
	for d := path.Dir(path.Dir(rel)); ; d = path.Dir(d) {
		for _, name := range []string{"build.gradle", "build.gradle.kts"} {
			if parent := path.Join(d, name); parent != rel {
				if ptxt, ok := w.read(parent); ok {
					for _, m := range reGradleVarDef.FindAllStringSubmatch(ptxt, -1) {
						if _, set := vars[m[1]]; !set {
							vars[m[1]] = m[2]
						}
					}
				}
			}
		}
		if d == "." || d == "/" {
			break
		}
	}
	for _, m := range reGradleVarDef.FindAllStringSubmatch(txt, -1) {
		vars[m[1]] = m[2]
	}
	return vars
}

func parseGradle(w *repoWalk, rel, _, txt string) {
	vars := w.gradleVars(rel, txt)
	expand := func(v string) (string, bool) {
		out := reVarRef.ReplaceAllStringFunc(v, func(m string) string {
			sub := reVarRef.FindStringSubmatch(m)
			name := sub[1] + sub[3]
			if r, ok := vars[name]; ok {
				return r
			}
			if i := strings.LastIndex(name, "."); i >= 0 {
				if r, ok := vars[name[i+1:]]; ok {
					return r
				}
			}
			return m
		})
		return out, !strings.Contains(out, "$")
	}
	code := stripLineComments(txt)
	for _, m := range reGradleCoord.FindAllStringSubmatchIndex(code, -1) {
		v, ok := expand(code[m[6]:m[7]])
		if !ok {
			continue
		}
		w.dep("maven", code[m[2]:m[3]]+":"+code[m[4]:m[5]], v, stylePEP440, rel, lineOf(code, m[2]))
	}
	for _, m := range reGradlePluginID.FindAllStringSubmatchIndex(code, -1) {
		if product, ok := gradlePlugins[code[m[2]:m[3]]]; ok {
			if v, ok := expand(code[m[4]:m[5]]); ok {
				w.pin(product, v, rel, lineOf(code, m[4]), code[m[2]:m[3]]+" "+v)
			}
		}
	}
	for _, m := range reGradleKotlinPl.FindAllStringSubmatchIndex(code, -1) {
		if v, ok := expand(code[m[2]:m[3]]); ok {
			w.pin("kotlin", v, rel, lineOf(code, m[2]), "kotlin plugin "+v)
		}
	}
	// ext.kotlin_version = '1.3.72', safeExtGet('kotlinVersion', '1.6.10')
	for _, m := range reGradleKotlinVar.FindAllStringSubmatchIndex(code, -1) {
		w.pin("kotlin", code[m[2]:m[3]], rel, lineOf(code, m[2]), strings.TrimSpace(code[m[0]:m[1]]))
	}
	// resolutionStrategy forces a group's artifacts onto a version:
	// requested.group == "org.springframework" -> useVersion("6.2.15")
	for _, pair := range gradleForcedVersions(code) {
		group, v := pair[0], pair[1]
		if at := strings.Index(code, `requested.group == "`+group+`"`); at >= 0 {
			if product, ok := gradleForcedGroups[strings.ToLower(group)]; ok {
				w.pin(product, v, rel, lineOf(code, at), "resolutionStrategy "+group+" "+v)
			}
		}
	}
	// wrapper { gradleVersion = '4.10' } regenerates the wrapper at that version
	if m := reGradleWrapperTask.FindStringSubmatchIndex(code); m != nil {
		w.pin("gradle", code[m[2]:m[3]], rel, lineOf(code, m[2]), "wrapper gradleVersion "+code[m[2]:m[3]])
	}
	// gradle-node-plugin downloads and runs this Node
	if m := reGradleNodePlugin.FindStringSubmatchIndex(code); m != nil {
		if v, ok := expand(code[m[2]:m[3]]); ok {
			w.pin("nodejs", v, rel, lineOf(code, m[2]), "gradle node plugin "+v)
		}
	}
	seen := map[string]bool{}
	for _, re := range []*regexp.Regexp{reGradleJavaCompat, reGradleToolchain, reGradleJvmTarget} {
		for _, m := range re.FindAllStringSubmatchIndex(code, -1) {
			v := javaVersion(strings.ReplaceAll(code[m[2]:m[3]], "_", "."))
			if seen[v] {
				continue
			}
			seen[v] = true
			w.pin("java", v, rel, lineOf(code, m[2]), strings.TrimSpace(code[m[0]:m[1]]))
		}
	}
}

// gradleForcedVersions pairs each `requested.group == "X"` check with the
// first useVersion call inside its own block (before the next group
// check). A block with no useVersion pairs with nothing — no character
// budget to misattribute across blocks, no lookahead needed.
func gradleForcedVersions(code string) [][2]string {
	type at struct {
		val string
		pos int
	}
	var groups, vers []at
	for _, m := range reGradleGroupCheck.FindAllStringSubmatchIndex(code, -1) {
		groups = append(groups, at{code[m[2]:m[3]], m[2]})
	}
	for _, m := range reGradleUseVersion.FindAllStringSubmatchIndex(code, -1) {
		vers = append(vers, at{code[m[2]:m[3]], m[2]})
	}
	var out [][2]string
	for i, g := range groups {
		end := len(code)
		if i+1 < len(groups) {
			end = groups[i+1].pos
		}
		for _, v := range vers {
			if v.pos > g.pos && v.pos < end {
				out = append(out, [2]string{g.val, v.val})
				break
			}
		}
	}
	return out
}

// propertyProducts: version property names (normalized: lowercase, no
// separators, no "version" suffix) that pin a tracked product.
var propertyProducts = map[string]string{
	"kotlin": "kotlin", "springboot": "spring-boot", "spring": "spring-framework", "springframework": "spring-framework",
	"dropwizard": "dropwizard", "hibernate": "hibernate-orm", "jetty": "eclipse-jetty", "tomcat": "tomcat",
	"groovy": "apache-groovy", "scala": "scala", "kafka": "apache-kafka", "spark": "apache-spark",
	"hadoop": "apache-hadoop", "micronaut": "micronaut", "quarkus": "quarkus-framework", "log4j": "log4j", "springcloud": "spring-cloud",
	"log4j2": "log4j", "gradle": "gradle", "elasticsearch": "elasticsearch", "solr": "solr",
}

// parseVersionProperties: gradle.properties and version*.properties keys
// named for their product (kotlin_version, dropwizard.version).
func parseVersionProperties(w *repoWalk, rel, _, txt string) {
	for _, m := range reGradlePropLine.FindAllStringSubmatchIndex(txt, -1) {
		key := strings.ToLower(txt[m[2]:m[3]])
		if !strings.HasSuffix(key, "version") {
			continue
		}
		norm := strings.NewReplacer(".", "", "_", "", "-", "").Replace(strings.TrimSuffix(key, "version"))
		p, ok := propertyProducts[norm]
		if !ok {
			continue
		}
		val := txt[m[4]:m[5]]
		if p == "spring-cloud" {
			if train, ok := springCloudBOM(val); ok {
				w.pin(p, train, rel, lineOf(txt, m[2]), txt[m[2]:m[5]])
			}
			continue
		}
		if reLeadingVersion.MatchString(val) {
			w.pin(p, val, rel, lineOf(txt, m[2]), txt[m[2]:m[5]])
		}
	}
}

var reCatalogVersion = regexp.MustCompile(`(?m)^\s*([\w.\-]+)\s*=\s*"(\d[\w.\-]*)"`)

// parseVersionCatalog reads gradle/libs.versions.toml [versions] aliases
// named for tracked products.
func parseVersionCatalog(w *repoWalk, rel, _, txt string) {
	products := map[string]string{
		"kotlin": "kotlin", "spring-boot": "spring-boot", "springBoot": "spring-boot", "springboot": "spring-boot",
		"spring-framework": "spring-framework", "java": "java", "jvm": "java", "jvmTarget": "java",
		"jdk": "java", "javaVersion": "java", "java-version": "java", "jdkVersion": "java",
	}
	pairs := tomlPairs(txt)
	versions := map[string]string{}
	for _, kv := range pairs {
		if kv.section != "versions" {
			continue
		}
		if v, ok := tomlVersion(kv.value); ok {
			versions[kv.key] = v
		}
		if p, ok := products[kv.key]; ok {
			if v, ok := tomlVersion(kv.value); ok {
				if p == "java" {
					v = javaVersion(v)
				}
				w.pin(p, v, rel, kv.line, kv.key+" = "+v)
			}
		}
	}
	// [libraries] and [plugins] entries name their coordinates and a version
	// or version.ref: spring-core, the Kotlin serialization plugin
	field := func(v, k string) string {
		if m := regexp.MustCompile(`\b` + regexp.QuoteMeta(k) + `\s*=\s*["']([^"']+)["']`).FindStringSubmatch(v); m != nil {
			return m[1]
		}
		return ""
	}
	for _, kv := range pairs {
		if kv.section != "libraries" && kv.section != "plugins" {
			continue
		}
		v := field(kv.value, "version")
		if ref := field(kv.value, "version.ref"); ref != "" {
			v = versions[ref]
		}
		if v == "" {
			continue
		}
		if kv.section == "plugins" {
			id := field(kv.value, "id")
			p := gradlePlugins[id]
			if p == "" && strings.HasPrefix(id, "org.jetbrains.kotlin.") {
				p = "kotlin"
			}
			if p != "" {
				w.runtime(p, v, stylePEP440, rel, kv.line)
			}
			continue
		}
		coord := field(kv.value, "module")
		if g, n := field(kv.value, "group"), field(kv.value, "name"); coord == "" && g != "" && n != "" {
			coord = g + ":" + n
		}
		if strings.Count(coord, ":") == 1 {
			w.dep("maven", coord, v, stylePEP440, rel, kv.line)
		}
	}
}

func stripLineComments(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if j := strings.Index(l, "//"); j >= 0 && !strings.Contains(l[:j], "http") {
			lines[i] = l[:j]
		}
	}
	return strings.Join(lines, "\n")
}

var (
	reSbtDep = regexp.MustCompile(`"([\w.\-]+)"\s*%%?%?\s*"([\w.\-]+)"\s*%\s*(?:"(\d[\w.\-]*)"|([A-Za-z_]\w*)\b)`)
	reSbtVal = regexp.MustCompile(`\b(?:lazy\s+)?val\s+(\w+)\s*=\s*"(\d[\w.\-]*)"`)
)

// parseSbtDeps: sbt dependency triples ("org.apache.spark" %% "spark-core" % "1.2.0").
func parseSbtDeps(w *repoWalk, rel, _, raw string) {
	txt := stripLineComments(raw)
	vals := map[string]string{} // val sparkVersion = "2.3.0"
	for _, m := range reSbtVal.FindAllStringSubmatch(txt, -1) {
		vals[m[1]] = m[2]
	}
	for _, m := range reSbtDep.FindAllStringSubmatchIndex(txt, -1) {
		v := ""
		if m[6] >= 0 {
			v = txt[m[6]:m[7]]
		} else {
			v = vals[txt[m[8]:m[9]]]
		}
		if v != "" {
			w.dep("maven", txt[m[2]:m[3]]+":"+txt[m[4]:m[5]], v, stylePEP440, rel, lineOf(txt, m[2]))
		}
	}
	// a minikube CI job declared in build.sbt tests one k8s server:
	// k8sServerVersion = "v1.19.11"
	for _, m := range reSbtMinikube.FindAllStringSubmatchIndex(txt, -1) {
		w.pin("kubernetes", txt[m[2]:m[3]], rel, lineOf(txt, m[2]), "minikube k8sServerVersion v"+txt[m[2]:m[3]])
	}
	// scalacOptions ++= Seq("-target:jvm-1.6"): the JVM the bytecode targets
	for _, m := range reScalacTarget.FindAllStringSubmatchIndex(txt, -1) {
		v := javaVersion(txt[m[2]:m[3]])
		w.pin("java", v, rel, lineOf(txt, m[2]), "scalac -target:jvm-"+txt[m[2]:m[3]])
	}
}

var (
	reSbtMinikube  = regexp.MustCompile(`(?i)k8sServerVersion\s*=\s*"v(\d+\.\d+)`)
	reScalacTarget = regexp.MustCompile(`(?i)-target:?jvm-?1\.(\d+)`)
)
