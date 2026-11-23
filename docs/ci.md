# CI and code scanning

zephy exits with `1` when a finding matches `--fail-on` (default: `eol`),
`2` on an error, and `0` otherwise. That's all a CI job needs.

## GitHub Actions

```yaml
name: eol
on:
  pull_request:
  schedule:
    - cron: "0 6 * * 1" # software reaches EOL without any code change

jobs:
  zephy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version: stable
      - run: go install github.com/0xbadshah/zephy/cmd/zephy@latest
      - run: zephy .
```

Each run fetches the latest lifecycle data first.
`--fail-on eol,expiring` also fails on versions that reach end-of-life
within `--lookahead`. `--fail-on ""` never fails.

## Show findings in GitHub code scanning

`--format sarif` writes a report that GitHub code scanning shows as alerts
on the file and line. Code scanning is free for public repos; private
repos need GitHub Code Security. Add this job to the workflow above:

```yaml
  zephy-sarif:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      security-events: write
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version: stable
      - run: go install github.com/0xbadshah/zephy/cmd/zephy@latest
      - run: zephy --format sarif --out zephy.sarif --fail-on "" .
      - uses: github/codeql-action/upload-sarif@v4
        with:
          sarif_file: zephy.sarif
```

`--format json` gives the same findings for your own tools.
