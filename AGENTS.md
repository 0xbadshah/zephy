# zephy

Go CLI that finds end-of-life software in repos, container images and SBOMs.
Flow: `internal/catalog` finds version pins, `internal/match` judges each one
against the embedded endoflife.date data in `internal/data`, `internal/scan`
collects the findings and `internal/report` writes them as a table, JSON or
SARIF. `cmd/zephy/main.go` wires the flags and the TUI (`internal/ui`).

## Before calling a change done

Run all three. CI runs the same.

```sh
go vet ./...
go test ./...
go build -o zephy ./cmd/zephy && ./zephy --fail-on eol,expiring repo:.
```

The last one is the dogfood check: zephy scans its own repo and must exit 0.
When a test or source file quotes an end-of-life version on purpose, add a
`suppress` entry with a `reason` to `.zephy.yml`.

Run a single test with `go test ./internal/catalog -run TestGoldenWalk`.

## Detection

- New file format: add a matcher and parser to the `parsers` table in
  `internal/catalog/parsers.go`, a case in `internal/catalog/parsers_test.go`
  and a line in `docs/supported-files.md`.
- When a parser change alters what the walker finds, update the expected pins
  in `internal/catalog/golden_test.go` in the same change.
- Names map to endoflife.date slugs in `internal/catalog/maps.json`. Use slugs
  that exist in `internal/data`; `maps_test.go` checks this.
- A version zephy cannot verify goes to Untracked with a reason, so it shows
  under `--report-untracked`.
- Each entry in `internal/curated/known-dead.json` links to a public upstream
  statement (deprecation notice, archived repo, EOL announcement). Entries
  without one are dropped on load.

## Contracts

- Exit codes: 0 clean, 1 findings matched by `--fail-on`, 2 operational error.
- zephy works offline from the embedded dataset. Before each scan it
  updates the cached copy with one conditional request; a failed update
  prints one line and the scan goes on.
- New data sources plug in as a `Feed` in `internal/data/feed` that
  produces zephy's own format (`data.Dataset`); `feed.Default` picks the
  one in use.
- Follow the design rules in `docs/how-it-works.md` and the terms in
  `docs/glossary.md`.
- When the table output changes, re-record the README demo with
  `vhs assets/demo.tape`.

## Refreshing the embedded dataset

`go generate ./internal/data` rewrites `internal/data/dataset.json` from
`feed.Default`. The `data` workflow does this weekly and commits it.

## Commits and releases

- Use Conventional Commits. release-please turns them into versions: `fix:`
  is a patch, `feat:` a minor, `feat!:` a major release. Versions, tags and
  the changelog come from its release PR.
- After changing any `uses:` in `.github/workflows/`, run `gh actions-lock`
  to regenerate `.github/workflows/actions.lock`.
