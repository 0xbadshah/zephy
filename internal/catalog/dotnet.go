package catalog

import (
	"encoding/json"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var (
	reTargetFramework = regexp.MustCompile(`<TargetFrameworks?\b[^>]*>([^<]+)</TargetFrameworks?>`)
	reTargetFxVersion = regexp.MustCompile(`<TargetFrameworkVersion>v?(\d+\.\d+(?:\.\d+)?)</TargetFrameworkVersion>`)
	reWebConfigFx     = regexp.MustCompile(`(?:targetFramework\s*=\s*"|sku\s*=\s*"\.NETFramework,\s*Version=v)(\d+\.\d+(?:\.\d+)?)`)
	reTfmCore         = regexp.MustCompile(`^netcoreapp(\d+\.\d+)$`)
	reTfmNet          = regexp.MustCompile(`^net(\d+\.\d+)(?:-[\w.]+)?$`)
	reTfmFramework    = regexp.MustCompile(`^net(\d)(\d)(\d?)$`)
)

// tfm maps a target framework moniker to its runtime product and version:
// netcoreapp3.1 / net6.0 -> dotnet; net45 / net472 -> .NET Framework.
// netstandard is a contract, not a runtime: no pin.
func tfm(s string) (string, string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if m := reTfmCore.FindStringSubmatch(s); m != nil {
		return "dotnet", m[1], true
	}
	if m := reTfmNet.FindStringSubmatch(s); m != nil {
		if major, _ := strconv.Atoi(strings.SplitN(m[1], ".", 2)[0]); major < 5 {
			return "dotnetfx", fxVersion(m[1]), true // net4.8: a Framework version spelled with a dot
		}
		return "dotnet", m[1], true
	}
	if m := reTfmFramework.FindStringSubmatch(s); m != nil {
		v := m[1] + "." + m[2]
		if m[3] != "" {
			v += "." + m[3]
		}
		return "dotnetfx", fxVersion(v), true
	}
	return "", "", false
}

// fxVersion: .NET Framework 3.5 is only tracked as 3.5 SP1.
// .NET Framework 2.0 and 3.0 are serviced as part of 3.5 SP1.
func fxVersion(v string) string {
	switch v {
	case "2.0", "3.0", "3.5":
		return "3.5-sp1"
	}
	return v
}

func parseDotnetProject(w *repoWalk, rel, _, txt string) {
	for _, m := range reTargetFramework.FindAllStringSubmatchIndex(txt, -1) {
		for _, t := range strings.Split(txt[m[2]:m[3]], ";") {
			if p, v, ok := tfm(t); ok {
				w.pin(p, v, rel, lineOf(txt, m[2]), strings.TrimSpace(t))
			}
		}
	}
	for _, m := range rePackageReference.FindAllStringSubmatchIndex(txt, -1) {
		w.dep("nuget", strings.ToLower(txt[m[2]:m[3]]), txt[m[4]:m[5]], stylePEP440, rel, lineOf(txt, m[2]))
	}
	for _, m := range reTargetFxVersion.FindAllStringSubmatchIndex(txt, -1) {
		if strings.Contains(txt, "<TargetFrameworkProfile>Profile") {
			break // a Portable Class Library profile, not a .NET Framework release
		}
		w.pin("dotnetfx", fxVersion(txt[m[2]:m[3]]), rel, lineOf(txt, m[2]), "TargetFrameworkVersion v"+txt[m[2]:m[3]])
	}
}

// parseGlobalJSON: the pinned SDK names its runtime line (3.1.4xx -> 3.1).
func parseGlobalJSON(w *repoWalk, rel, _, txt string) {
	var doc struct {
		SDK struct {
			Version string `json:"version"`
		} `json:"sdk"`
	}
	if json.Unmarshal([]byte(txt), &doc) != nil || doc.SDK.Version == "" {
		return
	}
	parts := strings.Split(doc.SDK.Version, ".")
	if len(parts) < 2 {
		return
	}
	w.pin("dotnet", parts[0]+"."+parts[1], rel, keyLine(txt, "version"), "sdk "+doc.SDK.Version)
}

var (
	rePackageReference = regexp.MustCompile(`<PackageReference\s+Include="([^"]+)"\s+Version="\[?([^"\],]+)`)
	reNuGetPackage     = regexp.MustCompile(`<package\s+id="([^"]+)"\s+version="([^"]+)"`)
)

// parseNuGetPackagesConfig: classic NuGet package pins.
func parseNuGetPackagesConfig(w *repoWalk, rel, _, txt string) {
	for _, m := range reNuGetPackage.FindAllStringSubmatchIndex(txt, -1) {
		w.dep("nuget", strings.ToLower(txt[m[2]:m[3]]), txt[m[4]:m[5]], stylePEP440, rel, lineOf(txt, m[2]))
	}
}

func parseWebConfig(w *repoWalk, rel, _, txt string) {
	// the project file is authoritative; a config's targetFramework or
	// supportedRuntime next to it is often stale
	if w.hasSibling(path.Dir(rel), ".csproj", ".vbproj", ".fsproj") {
		return
	}
	seen := map[string]bool{}
	for _, m := range reWebConfigFx.FindAllStringSubmatchIndex(txt, -1) {
		v := txt[m[2]:m[3]]
		if seen[v] {
			continue
		}
		seen[v] = true
		w.pin("dotnetfx", fxVersion(v), rel, lineOf(txt, m[2]), "targetFramework="+v)
	}
}

// parseProjectJSON: the pre-MSBuild .NET Core project.json lists its
// target frameworks as keys of "frameworks".
func parseProjectJSON(w *repoWalk, rel, _, txt string) {
	var doc struct {
		Frameworks map[string]json.RawMessage `json:"frameworks"`
	}
	if json.Unmarshal([]byte(txt), &doc) != nil {
		return
	}
	for t := range doc.Frameworks {
		if p, v, ok := tfm(t); ok {
			w.pin(p, v, rel, keyLine(txt, t), t)
		}
	}
}
