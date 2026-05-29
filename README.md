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

# Tear the resources down (reverse dependency order)
iac-coolify destroy examples/full-project/ --dry-run      # offline preview, deletes nothing
iac-coolify destroy examples/full-project/ --auto-approve # required in CI / non-interactive

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

`destroy` deletes the declared resources in reverse dependency order (applications and
services first, then environments, then projects). Only resources that still exist remotely
are deleted, so a repeated destroy is a no-op; a `404` on delete is treated as success. Like
`apply`, it refuses a non-interactive session without `--auto-approve` and shares the same
exit codes and audit log.

## Why

| | `iac-coolify` | `coollabsio/coolify-cli` | `SierraJC/terraform-provider-coolify` |
|---|---|---|---|
| Model | declarative YAML | imperative wrapper | declarative HCL |
| `plan`/`apply`/`destroy` | ✅ `plan` + `apply` + `destroy` | ❌ | ✅ |
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

## Selecting environments

`plan`, `apply` and `destroy` operate on every environment found under the config path by
default. Use `--env <name>` to restrict the run to one or more environments, matched against
each resource's `metadata.environment`:

```bash
# Plan/apply/destroy a single environment
iac-coolify plan   infra/ --env staging
iac-coolify apply  infra/ --env production --auto-approve

# Several environments at once (repeat the flag; the result is their union)
iac-coolify apply  infra/ --env staging --env preview --auto-approve

# Compose --env with --target to act on one named resource within an environment
iac-coolify apply  infra/ --env staging --target back-office --auto-approve
```

`--env` composes with `--target` (logical resource name): a resource is selected only when it
passes **both** filters. A `--env` value that no resource declares — or a `--target`/`--env`
combination that matches nothing — exits with code 2 so a typo fails loudly instead of
silently doing nothing. Projects are cross-environment and are never filtered by `--env`.

Both folder layouts work — the filter reads `metadata.environment`, not the path:

```
# Nested per environment (see examples/beenaire)        # Flat (see examples/full-stack)
infra/                                                  infra/
  environments/staging/applications/back-office.yaml      applications/back-office.yaml   # environment: staging
  environments/production/applications/api.yaml            applications/api.yaml           # environment: production
```

## Environment interpolation

Any visible (Param) string field can reference an environment variable with
`${env:VAR}` — `metadata` names, `image.name`/`image.tag`, `fqdn`, a git `source`, an
inline `dockerfile`, `destination`, `limits`, and plain `env_vars` values. References are
resolved at load time; an unset variable is an error (no silent fallback to `""`).

```yaml
metadata:
  environment: "${env:DEPLOY_ENV}"
spec:
  image:
    tag: "${env:IMAGE_TAG}"
  fqdn: "https://${env:PUBLIC_HOST}"
```

> Param fields are **visible** in plan/apply output once resolved. Never put a secret in a
> Param field — use `value_secret` (below) for sensitive values.

## Secrets management

Secrets are an opaque `Secret` type: shown as `[REDACTED]` in every log, plan, apply output,
error, and the state cache — only their origin (`${env:NAME}`) is ever displayed. A literal
secret in YAML is rejected at parse time. The defences layer up:

| Layer | What |
|---|---|
| **L0** Opaque `Secret` type | redaction by construction (the value has no visible accessor) |
| **L1** Env interpolation | `${env:VAR}` in Param fields and `value_secret` |
| **L2** SOPS + age at rest | `${sops:path}` decrypted in memory from a colocated `secrets.enc.yaml` |
| **L4** `validate --strict` | flags a secret-like value mistakenly put in a visible `value` |
| **L5** Redacted logs/plan | guaranteed by L0; no value can reach output |
| **L6** Audit log | append-only `0600` log of who/what, with secret **origins** and a diff hash, never values |

Ways to supply a value:

```yaml
env_vars:
  - name: NODE_ENV
    value: "production"                     # visible literal
  - name: LOG_LEVEL
    value: "${env:LOG_LEVEL}"               # visible, env-resolved
  - name: DATABASE_URL
    value_secret: "${env:DATABASE_URL}"     # secret, env-sourced, REDACTED
  - name: STRIPE_KEY
    value_secret: "${sops:stripe.key}"      # secret, SOPS-decrypted, REDACTED
```

### SOPS + age secrets at rest

A `${sops:path}` reference is read from a `secrets.enc.yaml` **colocated with the manifest**
(the path is never user-supplied, so there is no traversal surface), decrypted in memory, and
shown as `[REDACTED]`. `path` is a dotted key into the decrypted document, e.g.
`databases.staging.password`. A worked example lives in
[`examples/secrets-sops/`](examples/secrets-sops/).

The committed `secrets.enc.yaml` is encrypted to a throwaway recipient, so re-encrypt it
with your own key before running the example:

```sh
# 1. Generate an age key and restrict it (iac-coolify refuses a group/other-readable key).
age-keygen -o ~/.config/sops/age/keys.txt && chmod 600 ~/.config/sops/age/keys.txt

# 2. Put your public key (age1...) in examples/secrets-sops/.sops.yaml, then re-encrypt:
printf 'databases:\n  staging:\n    password: your-secret\n' \
  | sops --encrypt --input-type yaml /dev/stdin > examples/secrets-sops/secrets.enc.yaml

# 3. Validate — the SOPS value resolves and stays REDACTED.
iac-coolify validate examples/secrets-sops/coolify.yaml
```

`iac-coolify` locates the key via `SOPS_AGE_KEY_FILE` (or the default
`~/.config/sops/age/keys.txt`). **Never commit a private age key** — the repo `.gitignore`
excludes `*.age.key`; only the encrypted file and the public recipient (`.sops.yaml`) are
safe to track.

### Viewing secret values

`iac-coolify` never reveals secret values, by design. To inspect one, go to the source —
e.g. `printenv DATABASE_URL`. Secrets stay scoped to the tool that owns them (your shell).

## CI integration

Pass `COOLIFY_API_TOKEN` (and any Cloudflare Access service-token headers) as **masked**
CI secrets, and set `IAC_COOLIFY_ACTOR` so the audit log attributes the change. When you use
SOPS, store the age private key as a masked secret (`SOPS_AGE_KEY`) and write it to an
owner-only file pointed at by `SOPS_AGE_KEY_FILE`. Run `plan`
on pull/merge requests and `apply --auto-approve` on the protected default branch.
Ready-to-copy workflows: [`examples/ci/github-actions.yml`](examples/ci/github-actions.yml)
and [`examples/ci/gitlab-ci.yml`](examples/ci/gitlab-ci.yml).

## License

[Apache-2.0](LICENSE). See [`SECURITY.md`](SECURITY.md) for responsible disclosure and
[`CONTRIBUTING.md`](CONTRIBUTING.md) to get started.

## Acknowledgements

Built on the shoulders of [coollabsio/coolify](https://github.com/coollabsio/coolify),
[charmbracelet](https://github.com/charmbracelet), [spf13/cobra](https://github.com/spf13/cobra),
and [goreleaser](https://goreleaser.com).
