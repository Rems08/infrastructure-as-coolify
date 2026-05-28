# infrastructure-as-coolify

> Declarative Infrastructure as Code for [Coolify](https://coolify.io) — YAML in, `plan`/`apply`/`destroy` out. No HCL, no state file, no magic.

`iac-coolify` is a single-binary CLI that manages Coolify v4 resources declaratively, the
way Terraform manages cloud resources — but native to Coolify, with Kubernetes-style YAML
(`apiVersion`/`kind`/`metadata`/`spec`), a stateless-first model, auto-generated docs, and a
future Bubble Tea TUI for live exploration.

**Status:** 🚧 beta — Coolify **v4.x only**. APIs may change before `v1.0.0`.

The Coolify **OpenAPI commit SHA** is pinned in
[`testdata/openapi/COMMIT_SHA`](testdata/openapi/COMMIT_SHA) (currently `5a27427`), with a
SHA-256 sidecar verified at boot. A nightly CI job watches upstream `v4.x` for drift.

### Supported platforms

| OS | amd64 | arm64 |
|---|---|---|
| Linux | ✅ | ✅ |
| macOS | ✅ | ✅ |
| Windows | ✅ | — |

## Quick start

```sh
# Validate your declarative config
iac-coolify validate examples/minimal/

# Generate the reference documentation from the resource structs
iac-coolify docs gen
```

## Why

| | `iac-coolify` | `coollabsio/coolify-cli` | `SierraJC/terraform-provider-coolify` |
|---|---|---|---|
| Model | declarative YAML | imperative wrapper | declarative HCL |
| `plan`/`apply`/`destroy` | ✅ (roadmap) | ❌ | ✅ |
| State file | stateless-first | n/a | tfstate required |
| Native to Coolify | ✅ | ✅ | wraps Terraform |

## Configuration

Resources are described in a `coolify/` directory tree, one file per resource:

```
coolify/
  coolify.yaml                                   # global config (api_url, required_coolify)
  projects/<name>.yaml
  environments/<env>/applications/<app>.yaml
```

See [`examples/`](examples/) and the generated [`docs/reference/`](docs/reference/).

## Viewing secret values

`iac-coolify` never reveals secret values, by design. To inspect a value:

- env-sourced: `printenv DATABASE_URL_STAGING`
- SOPS-sourced (Wave 4+): `sops -d coolify/environments/staging/db.enc.yaml`

Secrets stay scoped to the tool that owns them (your shell, SOPS).

## License

[Apache-2.0](LICENSE). See [`SECURITY.md`](SECURITY.md) for responsible disclosure and
[`CONTRIBUTING.md`](CONTRIBUTING.md) to get started.

## Acknowledgements

Built on the shoulders of [coollabsio/coolify](https://github.com/coollabsio/coolify),
[charmbracelet](https://github.com/charmbracelet), [spf13/cobra](https://github.com/spf13/cobra),
and [goreleaser](https://goreleaser.com).
