# infrastructure-as-coolify

[![CI](https://github.com/Rems08/infrastructure-as-coolify/actions/workflows/ci.yml/badge.svg)](https://github.com/Rems08/infrastructure-as-coolify/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Rems08/infrastructure-as-coolify)](https://github.com/Rems08/infrastructure-as-coolify/releases)
[![License](https://img.shields.io/github/license/Rems08/infrastructure-as-coolify)](LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/Rems08/infrastructure-as-coolify)](go.mod)
[![Go Report Card](https://goreportcard.com/badge/github.com/Rems08/infrastructure-as-coolify)](https://goreportcard.com/report/github.com/Rems08/infrastructure-as-coolify)
[![GHCR](https://img.shields.io/badge/ghcr.io-Rems08%2Finfrastructure--as--coolify-blue?logo=docker)](https://github.com/Rems08/infrastructure-as-coolify/pkgs/container/infrastructure-as-coolify)
[![cosign](https://img.shields.io/badge/cosign-keyless-2ea44f?logo=sigstore)](https://rems08.github.io/infrastructure-as-coolify/how-to/verify-signatures.html)
[![SLSA 3](https://slsa.dev/images/gh-badge-level3.svg)](https://slsa.dev)
[![docs](https://img.shields.io/badge/docs-mdBook-1f6feb?logo=readthedocs&logoColor=white)](https://rems08.github.io/infrastructure-as-coolify/)

> Declarative Infrastructure as Code for [Coolify](https://coolify.io) — YAML in, `plan`/`apply`/`destroy` out. No HCL, no state file, no magic.

`iac-coolify` is a single-binary CLI that manages Coolify v4 resources declaratively, the
way Terraform manages cloud resources — but native to Coolify, with Kubernetes-style YAML
(`apiVersion`/`kind`/`metadata`/`spec`), a stateless-first model, auto-generated docs, and a
built-in TUI for live exploration.

**Status:** 🚧 beta — Coolify **v4.x only**. APIs may change before `v1.0.0`. The Coolify
OpenAPI commit SHA is pinned in [`testdata/openapi/COMMIT_SHA`](testdata/openapi/COMMIT_SHA),
with a nightly CI job watching upstream `v4.x` for drift.

### Supported platforms

| OS | amd64 | arm64 |
|---|---|---|
| Linux | ✅ | ✅ |
| macOS | ✅ | ✅ |
| Windows | ✅ | — |

## Install

On macOS or Linux, install through Homebrew (this repository is also the tap):

```sh
brew tap rems08/iac-coolify https://github.com/Rems08/infrastructure-as-coolify
brew install --cask iac-coolify
brew upgrade --cask iac-coolify   # update later
```

Or download a binary from the [releases page](https://github.com/Rems08/infrastructure-as-coolify/releases),
or pull the multi-arch container image:

```sh
docker pull ghcr.io/rems08/infrastructure-as-coolify:latest
```

Every option (`go install`, pinning a version) is covered in the
[install guide](https://rems08.github.io/infrastructure-as-coolify/how-to/install.html);
releases are cosign-signed with SLSA 3 provenance — see
[verify release signatures](https://rems08.github.io/infrastructure-as-coolify/how-to/verify-signatures.html).

## Quick start

```sh
# Validate your declarative config
iac-coolify validate examples/minimal/

# Preview the changes against live Coolify (Terraform-style diff)
export COOLIFY_API_TOKEN=...                       # plus --coolify-url or COOLIFY_API_URL
iac-coolify plan examples/minimal/                 # runs offline (all-new) if unconfigured
iac-coolify plan examples/minimal/ --output=json --detailed-exitcode

# Apply the changes (creates projects and environments before applications)
iac-coolify apply examples/full-project/ --dry-run # offline preview, mutates nothing
iac-coolify apply examples/full-project/           # interactive confirmation prompt
iac-coolify apply examples/full-project/ --auto-approve   # required in CI / non-interactive

# Tear the resources down (reverse dependency order)
iac-coolify destroy examples/full-project/ --auto-approve

# Browse the live instance in a TUI (drift, env vars, lifecycle actions)
iac-coolify explore ./coolify

# Generate the reference documentation from the resource structs
iac-coolify docs gen
```

`plan`, `apply` and `destroy` are idempotent and print per-field diffs; secret values are
redacted by construction and never reach any output. `--detailed-exitcode` returns `0` (no
changes), `2` (changes pending), or `1` (error). Behind a Cloudflare Access gateway, set
`CF_ACCESS_CLIENT_ID` and `CF_ACCESS_CLIENT_SECRET`.

## Documentation

The full documentation lives at **<https://rems08.github.io/infrastructure-as-coolify/>**:

- [Getting started](https://rems08.github.io/infrastructure-as-coolify/tutorials/getting-started.html) — from install to a previewed change.
- [How-to guides](https://rems08.github.io/infrastructure-as-coolify/how-to/install.html) — install, environments, databases, secrets, import, TUI, CI.
- [Reference](https://rems08.github.io/infrastructure-as-coolify/reference/application.html) — generated field-by-field schemas for every resource kind.
- [Explanation](https://rems08.github.io/infrastructure-as-coolify/explanation/why.html) — why iac-coolify and how the configuration model works.

## License

[Apache-2.0](LICENSE). See [`SECURITY.md`](SECURITY.md) for responsible disclosure and
[`CONTRIBUTING.md`](CONTRIBUTING.md) to get started.

## Acknowledgements

Built on the shoulders of [coollabsio/coolify](https://github.com/coollabsio/coolify),
[charmbracelet](https://github.com/charmbracelet), [spf13/cobra](https://github.com/spf13/cobra),
and [goreleaser](https://goreleaser.com).
