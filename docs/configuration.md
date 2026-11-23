# Configuration

zephy reads `.zephy.yml` from the first local repo it scans, or else from
the current folder. `--policy <file>` points it to another file. Every
section is optional.

```yaml
suppress:
  # an upgrade is planned: hide Node 18 until then
  - product: nodejs
    release: "18"
    reason: "upgrade planned for Q1"
  # a service on its way out: ignore everything in it
  - path: "legacy/**"
    reason: "legacy service retired in March"

extends:
  # you pay for extended support: your date replaces the public one
  - product: postgresql
    release: "13"
    until: "2028-11-13"

normalize:
  # a version zephy can't read: the capture group keeps the version
  - '^(\d+\.\d+\.\d+)p\d+'   # 3.4.3p32 -> 3.4.3
```

## suppress

Hides findings that match every field you set. A field you leave out
matches anything.

- `product`: the PRODUCT column, like `nodejs`.
- `release`: the RELEASE column. `18` covers every Node 18 version.
- `version`: the exact version in the VERSION column.
- `path`: the start of a file path. `legacy/` and `legacy/**` both match
  everything under `legacy/`. Test fixtures need no rule: zephy skips
  them.
- `reason`: why the rule exists, for whoever reads the file. zephy prints
  how many findings it hid, not which.

## extends

Replaces the public end-of-life date for one release, for example when
you pay for extended support. `product`, `release` and `until` are all
required.

## normalize

Regex patterns for version strings zephy can't read. Each pattern has one
capture group, and zephy uses what it captures. Reach for this when
`--report-untracked` shows a version that matches no release.
