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

# Apply the changes (creates projects and environments before applications)
iac-coolify apply examples/full-project/ --dry-run # offline preview, mutates nothing
iac-coolify apply examples/full-project/           # interactive confirmation prompt
iac-coolify apply examples/full-project/ --auto-approve   # required in CI / non-interactive

# Generate the reference documentation from the resource structs
iac-coolify docs gen
```

`plan` resolves Coolify UUIDs from your logical `metadata` names (never written to YAML),
fetches live state, and prints a per-field diff. `--detailed-exitcode` returns `0` (no
changes), `2` (changes pending), or `1` (error). Secret values follow a Notify-only policy:
a change is announced (`resolved value changed, source ${env:X} unchanged`) but never shown.
Behind a Cloudflare Access gateway, set `CF_ACCESS_CLIENT_ID` and `CF_ACCESS_CLIENT_SECRET`.

`apply` reconciles the desired state, creating resources in dependency order (project →
environment → application). It refuses to run in a non-interactive session unless
`--auto-approve` is given, and exits `0` (success), `1` (error), or `2` (partial: some
resources changed before a failure — no rollback, the append-only `.iac-coolify/audit.log`
records each applied operation and the sources of any secrets, never their values). Every
write carries an `Idempotency-Key`, so a retried apply cannot create duplicates.

## Why

| | `iac-coolify` | `coollabsio/coolify-cli` | `SierraJC/terraform-provider-coolify` |
|---|---|---|---|
| Model | declarative YAML | imperative wrapper | declarative HCL |
| `plan`/`apply`/`destroy` | ✅ `plan` + `apply` (destroy on roadmap) | ❌ | ✅ |
| State file | stateless-first | n/a | tfstate required |
| Native to Coolify | ✅ | ✅ | wraps Terraform |

## Configuration

Resources are described in a `coolify/` directory tree, one file per resource:

```
coolify/
  coolify.yaml                                   # global config (api_url, required_coolify)
  project.yaml                                   # kind: Project
  environments/<env>/environment.yaml            # kind: Environment
  environments/<env>/applications/<app>.yaml     # kind: Application
  environments/<env>/databases/<db>.yaml         # kind: Database (8 engines)
  environments/<env>/envvars/<set>.yaml          # kind: EnvVar (shared, referenced via env_vars_from)
  services/<svc>.yaml                            # kind: Service (docker-compose stack)
```

Supported resource kinds: **Project**, **Environment**, **Application**, **Service**,
**Database** (`postgresql`, `mysql`, `mariadb`, `mongodb`, `redis`, `keydb`, `dragonfly`,
`clickhouse`), and standalone **EnvVar** sets that an Application merges in by name
(`env_vars_from`). `apply` creates projects and environments before the applications and
services that reference them.

A **Service** is a docker-compose stack. It sources its definition from exactly one of:

- `docker_compose_path` — a relative path to a `docker-compose.yml` in your repository.
  The path may not be absolute and may not resolve outside the config tree (a hostile
  compose path can never read `/etc/passwd`); the file is base64-encoded and sent to
  Coolify on `apply`.
- `type` — a Coolify one-click template identifier such as `gitea-with-mysql`.

An **Application** is built by one of six build packs, each with its own source of truth:

- `dockerimage` — a prebuilt image (`image.name` + `image.tag`).
- `dockerfile` — either an inline `dockerfile` (Dockerfile content, no git) or a git
  `source`; exactly one of the two.
- `nixpacks`, `docker-compose`, `static`, `railpack` — a public git `source`
  (`git_repository` + `git_branch` + `ports_exposes`).

A git `source.git_repository` must use an `https://`, `http://` or `git@` URL; an inline
`dockerfile` is capped at 1 MB. The IaC build pack `docker-compose` is sent to Coolify
under its upstream spelling `dockercompose`.

> **Private git sources:** applications from a private GitHub App or deploy key are not yet
> supported (they need a separate credential resolver). Use a public repository, an inline
> Dockerfile, or a prebuilt image.

> **Service domains:** Coolify binds domains per docker-compose sub-service, so a
> Service's `fqdn` is advisory metadata for now and is not applied on create.

See [`examples/`](examples/) and the generated [`docs/reference/`](docs/reference/)
(`project.md`, `environment.md`, `application.md`, `service.md`, `database.md`,
`envvar.md` + JSON schemas).

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
