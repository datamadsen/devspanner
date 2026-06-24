# Releasing devspanner

Distribution is driven by a git tag. Everything below assumes the repo is pushed to
`github.com/datamadsen/devspanner`.

## Cut a release

```bash
git tag v0.1.0
git push origin v0.1.0
```

The tag push triggers [`.github/workflows/release.yml`](.github/workflows/release.yml),
which runs [GoReleaser](https://goreleaser.com). Out of the box (no extra setup) it:

- cross-compiles binaries for linux/macOS × amd64/arm64 (Windows is not supported —
  the tool relies on Unix process groups and `/proc`),
- builds `.deb` and `.rpm` packages,
- creates the **GitHub Release** with archives, `checksums.txt`, and a changelog.

Test the config locally without publishing anything:

```bash
goreleaser release --snapshot --clean   # builds into ./dist, no upload
goreleaser check                         # validate .goreleaser.yaml
```

Once a tag exists, Go users can already install with:

```bash
go install github.com/datamadsen/devspanner@latest
```

## Channels that need one-time setup

Each is a commented block in [`.goreleaser.yaml`](.goreleaser.yaml). Do the setup,
uncomment the block, and uncomment the matching `env:` line in the release workflow.

### AUR — `yay`

Two independent packages:

- **`devspanner`** (source) — maintained by hand in [`aur/`](aur/). Follow
  [`aur/README.md`](aur/README.md). No GitHub secret needed.
- **`devspanner-bin`** (prebuilt) — published by GoReleaser. Add the SSH private key
  of your AUR account as the `AUR_KEY` repo secret, then uncomment the `aurs:` block.

### Homebrew — `brew`

1. Create a public repo `datamadsen/homebrew-tap`.
2. Create a PAT with `repo` scope on it; add it as the `HOMEBREW_TAP_TOKEN` secret.
3. Uncomment the `brews:` block.

Install: `brew install datamadsen/tap/devspanner`.

### .deb / .rpm

Already built and attached to every GitHub Release — no setup. Users download and
`dpkg -i` / `rpm -i`. (Hosting a real apt/yum repo is a separate, larger effort.)

## Version stamping

`main.version` defaults to `"dev"` and is overwritten from the tag at build time
(GoReleaser ldflags, and `-X main.version=$pkgver` in the AUR PKGBUILD). Verify with
`devspanner -v`.
