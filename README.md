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

# Preview the changes against live Coolify (Terraform-style diff)
export COOLIFY_API_TOKEN=...                       # plus --coolify-url or COOLIFY_API_URL
iac-coolify plan examples/minimal/                 # runs offline (all-new) if unconfigured
iac-coolify plan examples/minimal/ --output=json --detailed-exitcode

# Generate the reference documentation from the resource structs
iac-coolify docs gen
```

`plan` resolves Coolify UUIDs from your logical `metadata` names (never written to YAML),
fetches live state, and prints a per-field diff. `--detailed-exitcode` returns `0` (no
changes), `2` (changes pending), or `1` (error). Secret values follow a Notify-only policy:
a change is announced (`resolved value changed, source ${env:X} unchanged`) but never shown.
Behind a Cloudflare Access gateway, set `CF_ACCESS_CLIENT_ID` and `CF_ACCESS_CLIENT_SECRET`.

## Why

| | `iac-coolify` | `coollabsio/coolify-cli` | `SierraJC/terraform-provider-coolify` |
|---|---|---|---|
| Model | declarative YAML | imperative wrapper | declarative HCL |
| `plan`/`apply`/`destroy` | ✅ `plan` (apply/destroy on roadmap) | ❌ | ✅ |
| State file | stateless-first | n/a | tfstate required |
| Native to Coolify | ✅ | ✅ | wraps Terraform |

## Configuration

Resources are described in a `coolify/` directory tree, one file per resource:

```
coolify/
  coolify.yaml                                   # global config (api_url, required_coolify)
  projects/<name>.yaml
  environments/<env>/applications/<app>.yaml     # kind: Application
  environments/<env>/databases/<db>.yaml         # kind: Database (8 engines)
  environments/<env>/envvars/<set>.yaml          # kind: EnvVar (shared, referenced via env_vars_from)
```

Supported resource kinds: **Application**, **Database** (`postgresql`, `mysql`, `mariadb`,
`mongodb`, `redis`, `keydb`, `dragonfly`, `clickhouse`), and standalone **EnvVar** sets that
an Application merges in by name (`env_vars_from`).

See [`examples/`](examples/) and the generated [`docs/reference/`](docs/reference/)
(`application.md`, `database.md`, `envvar.md` + JSON schemas).

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
