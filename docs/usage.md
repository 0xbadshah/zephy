# Usage

## What to scan

| Command | Scans |
| --- | --- |
| `zephy .` | the repo in the current folder |
| `zephy ./api ../web` | several local repos |
| `zephy acme/api` | a GitHub repo, cloned to a temp folder and deleted after |
| `zephy --org acme` | every repo in a GitHub org or user account, except archived ones |
| `zephy --org gitlab.com/mygroup` | a GitLab group and its subgroups; `bitbucket.org/<workspace>` and self-hosted GitLab work the same way |
| `zephy nginx:1.24-alpine` | a container image (needs syft) |
| `zephy ./sbom.json` | an SBOM: Syft, SPDX 2.x or CycloneDX JSON |

You can mix them in one run. zephy works out what each one is:

- a folder is a repo
- a file is an SBOM
- `owner/repo` is a GitHub repo, and `host/owner/repo` a repo on another host
- `name:tag` is a container image

To say it yourself, put `repo:`, `github:`, `forge:`, `image:` or `sbom:`
in front.

## Private repos and tokens

| When you scan | zephy uses |
| --- | --- |
| a single remote repo | your git credentials: HTTPS first, then SSH |
| a GitHub org | `GITHUB_TOKEN`, or your `gh` login; public orgs need neither |
| a GitLab group | `GITLAB_TOKEN` for private groups |
| a Bitbucket workspace | `BITBUCKET_TOKEN`, as `username:token` |
| with `--check-upstream` | `GITHUB_TOKEN` (required) |

## Statuses

| Status | Meaning |
| --- | --- |
| `EOL` | past its end-of-life date |
| `EXPIRING` | reaches end-of-life within `--lookahead` (default: 1 year) |
| `DEPRECATED` | deprecated or renamed upstream |
| `ABANDONED` | no longer maintained upstream |
| `UNSUPPORTED` | outside the upstream support policy |

`--show-ok` also lists the versions that are fine. `--report-untracked`
lists the versions zephy found but couldn't check.

zephy judges a version range by the newest version it allows.
`"vue": "^2.6"` allows at most a 2.x, and Vue 2 is past end-of-life, so
zephy reports it. A floor like `>=3.6` has no upper limit, so zephy never
reports it.

## Flags

```
--format table|json|sarif   report format (default: table)
--out <file>                write the report to a file (default: stdout)
--fail-on <statuses>        exit 1 when a finding has one of these statuses:
                            eol, expiring, deprecated, abandoned, unsupported
                            (default: eol; "" never fails)
--lookahead <window>        how far ahead counts as EXPIRING: 90d, 6w, 1y
                            or a Go duration like 720h (default: 1y)
--policy <file>             config file (default: .zephy.yml, found
                            automatically)
--org <org>                 scan every repo of an org, except archived ones;
                            repeatable
--report-untracked          list the versions zephy couldn't check
--show-ok                   list the versions that are fine
--no-color                  no colors (NO_COLOR works too)
--no-tui                    plain progress lines instead of the live view
--offline                   skip the lifecycle data update (or ZEPHY_OFFLINE=1);
                            data already downloaded is still used
--refresh                   update the lifecycle data, then exit
--check-upstream            also flag dependencies whose upstream repo is
                            archived or stale: Go modules and gem, PyPI and
                            npm dependencies (needs GITHUB_TOKEN)
--stale-after <window>      no upstream activity for this long counts as
                            stale: 18w, 400d, 5y (default: 5y)
--version                   print the version and exit
```

## Exit codes

`0` nothing matched `--fail-on`, `1` a finding matched, `2` an error.
