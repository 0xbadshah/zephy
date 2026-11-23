# Glossary

The words the code uses. Use them in code, comments, reviews and issues.

**Target**: what the user asks to scan, with a scheme in front:
`repo:<dir>`, `github:<owner/repo>`, `forge:<host/path>`, `image:<ref>`,
`sbom:<file>`.

**Source**: what every target becomes (`catalog.Source`). There are four:
local repo, remote repo (shallow clone), container image (through syft)
and SBOM file.

**Inventory**: everything a source saw, before any verdict: pins, package
identities and untracked items.

**Repo file set**: the files a repo scan reads: regular files (no
symlinks), outside dependency folders and fixture paths, and git-tracked
when the target is a git checkout. Decided once, in `repoFileSet`.

**Pin**: a declared version of a product at a file location. Not judged
yet.

**Float**: the version a range reduces to when every version it allows sits
in one release line: `^2.6.11` -> `2`, `~> 5.2` -> `5`, `>=2.2,<3` -> `2`.
A float is judged against the newest release of its line. Floors (`>=3.6`)
give no pin.

**Package identity** (`PackagePin`): an ecosystem package or a container
image (`ecosystem: image`) with its version, matched against the curated
list. `ident` parses image references and purls into these, once.

**Product**: an endoflife.date slug (`python`, `nodejs`, `dotnetfx`).

**Family**: a pseudo-product judged across several products, in order.
`java` = eclipse-temurin, then openjdk-builds-from-oracle, then
oracle-jdk.

**Release**: a product's release line, sharing one EOL date (`3.7`, `20`).
The RELEASE column, and `release` in `.zephy.yml` and JSON. The dataset
calls it a cycle (`data.Cycle`), as endoflife.date does.

**Judge / Verdict**: the code that turns one pin into exactly one verdict:
Finding, OK, Suppressed, or Untracked with a reason. There is no fifth
outcome.

**Finding**: a judged pin that is past EOL (`eol`) or reaches EOL within
the lookahead (`expiring`). Curated and audit findings carry `deprecated`,
`abandoned` or `unsupported`.

**Untracked**: something seen but not checkable: no product mapping, no
product in the dataset, or no matching release. Reported with a reason,
never dropped.

**Curated list**: `known-dead.json`, packages deprecated or abandoned
upstream, each with a public source.

**Policy**: `.zephy.yml`: suppressions, extended-support dates and version
normalization patterns.

**Audit**: the code's name for `--check-upstream`, the opt-in GitHub check
for archived or stale upstream repos.

**Feed**: where fresh lifecycle data comes from (`feed.Feed`). It turns
one upstream source into zephy's dataset format; `feed.Default` is
endoflife.date.

Don't say "tracked" on its own: the code uses it for "mapped in
maps.json", "present in the dataset", "git-tracked" and the opposite of
Untracked. Say which one.
