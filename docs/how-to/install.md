# Install

`iac-coolify` ships as a single static binary for Linux, macOS and Windows (amd64 and
arm64; Windows is amd64 only), plus a multi-arch container image.

## Homebrew (macOS and Linux)

This repository doubles as a Homebrew tap — the cask under `Casks/` is updated
automatically on every stable release:

```sh
brew tap rems08/iac-coolify https://github.com/Rems08/infrastructure-as-coolify
brew trust rems08/iac-coolify    # recent Homebrew requires trusting third-party taps
brew install --cask iac-coolify
iac-coolify version
```

Upgrade later with:

```sh
brew upgrade --cask iac-coolify
```

The explicit tap URL is required because the repository is not named `homebrew-*`. On macOS
the cask's post-install step clears Gatekeeper's quarantine attribute, so the binary runs
without a "cannot be opened" prompt (releases are cosign-signed, not Apple-notarized — see
[Verify release signatures](verify-signatures.md)).

## Binary download

Download the archive for your platform from the
[releases page](https://github.com/Rems08/infrastructure-as-coolify/releases), extract it,
and put `iac-coolify` on your `PATH`:

```sh
tar -xzf iac-coolify_<version>_linux_amd64.tar.gz
install -m 0755 iac-coolify /usr/local/bin/iac-coolify
```

On macOS a hand-downloaded binary is quarantined by Gatekeeper; clear it with
`xattr -d com.apple.quarantine ./iac-coolify` (the Homebrew cask does this for you).

## Container image

```sh
docker pull ghcr.io/rems08/infrastructure-as-coolify:latest
docker run --rm ghcr.io/rems08/infrastructure-as-coolify version
```

Images are multi-arch (`linux/amd64`, `linux/arm64`). Replace `:latest` with a version such
as `:0.1.6` to pin a release.

## go install

```sh
go install github.com/Rems08/infrastructure-as-coolify/cmd/iac-coolify@latest
```

A `go install` build has no release metadata, so `iac-coolify version` reports `dev`. Prefer
the cask or a release binary when the version matters (support requests, reproducibility).

## Verifying what you downloaded

Every release archive and container image is signed with cosign and ships SLSA build
provenance — see [Verify release signatures](verify-signatures.md).
