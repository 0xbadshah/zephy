# How it works

1. **Find versions.** zephy reads the files where versions are declared
   and records each one as a pin: product, version, file and line.
2. **Find the release.** It matches each pin to a release line in the
   [endoflife.date](https://endoflife.date) data: `3.9.18` belongs to
   Python `3.9`.
3. **Give one verdict.** Past the release's end-of-life date is EOL.
   Within `--lookahead` is EXPIRING. Later is fine. Your `.zephy.yml` can
   hide a finding or replace a date. A version with nothing to check
   against is untracked.
4. **Check packages.** A short hand-kept list flags packages that are
   deprecated or abandoned upstream. `--check-upstream` also asks GitHub
   whether each dependency's repo is archived or stale.

## Data and network

- The lifecycle data comes from endoflife.date. A copy is built into the
  binary, so zephy works offline from the first run.
- Before each scan, zephy asks endoflife.date for newer data in one
  request. If nothing changed, nothing downloads. If something did, zephy
  saves the new data (about 350 KB) to your cache folder
  (`~/Library/Caches/zephy` on macOS, `~/.cache/zephy` on Linux) and uses
  it for this scan.
- If the request fails, zephy says so in one line and scans with the data
  it has.
- `--offline` or `ZEPHY_OFFLINE=1` skips the request. `zephy --refresh`
  updates the data without scanning.
- Image scans pull the image through syft. Remote repos, `--org` and
  `--check-upstream` call the GitHub, GitLab, Bitbucket or package
  registry APIs they need.

## Design rules

1. **No zephy server.** The data ships in the binary and updates from
   endoflife.date. You need no account.
2. **No silent skips.** A version zephy can't check is reported as
   untracked (`--report-untracked`), never dropped.
3. **Messy data doesn't break it.** One bad upstream entry never hides a
   whole product.
4. **Every version gets checked.** No guessing that something "looks
   current".
5. **zephy never ships what it flags.** CI runs zephy on its own repo.
