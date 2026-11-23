# Install

## Download a binary

Download the archive for your OS and CPU from the
[latest release](https://github.com/0xbadshah/zephy/releases/latest),
unpack it and put `zephy` on your `PATH`. There are builds for Linux,
macOS and Windows, on amd64 and arm64.

Each release has a signed build provenance attestation. It proves an
archive was built from this repo by its release workflow:

```sh
gh attestation verify <archive> --repo 0xbadshah/zephy
```

macOS quarantines binaries downloaded in a browser. To clear that:

```sh
xattr -d com.apple.quarantine zephy
```

## With Go

You need Go 1.26 or newer.

```sh
go install github.com/0xbadshah/zephy/cmd/zephy@latest
```

## From source

```sh
git clone https://github.com/0xbadshah/zephy
cd zephy
go build -o zephy ./cmd/zephy
```

`zephy --version` prints the version you have.

## Optional tools

- [syft](https://github.com/anchore/syft) to scan container images.
- `git` to scan remote repos.
