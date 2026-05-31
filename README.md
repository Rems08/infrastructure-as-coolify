# infrastructure-as-coolify

[![CI](https://github.com/Rems08/infrastructure-as-coolify/actions/workflows/ci.yml/badge.svg)](https://github.com/Rems08/infrastructure-as-coolify/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Rems08/infrastructure-as-coolify)](https://github.com/Rems08/infrastructure-as-coolify/releases)
[![License](https://img.shields.io/github/license/Rems08/infrastructure-as-coolify)](LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/Rems08/infrastructure-as-coolify)](go.mod)
[![Go Report Card](https://goreportcard.com/badge/github.com/Rems08/infrastructure-as-coolify)](https://goreportcard.com/report/github.com/Rems08/infrastructure-as-coolify)
[![GHCR](https://img.shields.io/badge/ghcr.io-Rems08%2Finfrastructure--as--coolify-blue?logo=docker)](https://github.com/Rems08/infrastructure-as-coolify/pkgs/container/infrastructure-as-coolify)
[![cosign](https://img.shields.io/badge/cosign-keyless-2ea44f?logo=sigstore)](#verifying-release-signatures)
[![SLSA 3](https://slsa.dev/images/gh-badge-level3.svg)](https://slsa.dev)
[![docs](https://img.shields.io/badge/docs-mdBook-1f6feb?logo=readthedocs&logoColor=white)](https://rems08.github.io/infrastructure-as-coolify/)

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

## Install

Download a binary for your platform from the [releases page](https://github.com/Rems08/infrastructure-as-coolify/releases),
or pull the container image:

```sh
docker pull ghcr.io/rems08/infrastructure-as-coolify:latest
docker run --rm ghcr.io/rems08/infrastructure-as-coolify version
```

Images are multi-arch (`linux/amd64`, `linux/arm64`). Replace `:latest` with a version such
as `:0.1.0-rc.2` to pin a release.

## Verifying release signatures

Every release is signed with [cosign](https://docs.sigstore.dev/cosign/overview/) keyless
(no long-lived key — the signature is bound to this repository's GitHub Actions identity) and
ships [SLSA build level 3](https://slsa.dev/spec/v1.0/levels#build-l3) provenance. Set
`TAG` to the release you downloaded, e.g. `TAG=v0.1.0-rc.2`.

```sh
# Verify a downloaded archive against its cosign bundle
cosign verify-blob \
  --bundle iac-coolify_${TAG#v}_linux_amd64.tar.gz.bundle \
  --certificate-identity-regexp 'https://github.com/Rems08/infrastructure-as-coolify/.github/workflows/release.yml@.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  iac-coolify_${TAG#v}_linux_amd64.tar.gz

# Verify the container image
cosign verify ghcr.io/rems08/infrastructure-as-coolify:${TAG#v} \
  --certificate-identity-regexp 'https://github.com/Rems08/infrastructure-as-coolify/.github/workflows/release.yml@.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com

# Verify the SLSA provenance attached to the release
slsa-verifier verify-artifact \
  --provenance-path multiple.intoto.jsonl \
  --source-uri github.com/Rems08/infrastructure-as-coolify \
  --source-tag "$TAG" \
  iac-coolify_${TAG#v}_linux_amd64.tar.gz
```

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

## Managing databases

`apply` creates, updates and deletes managed databases alongside applications and services.
A database is declared per engine; the engine selects the Coolify create endpoint:

```yaml
api_version: iac-coolify/v1
kind: Database
metadata:
  name: pg-core
  project: beenaire
  environment: staging
spec:
  engine: postgresql        # postgresql|mysql|mariadb|mongodb|redis|keydb|dragonfly|clickhouse
  image: postgres:18-alpine
  destination:
    server: localhost
    network: coolify
  password: "${env:PG_PASSWORD}"   # opaque Secret; mapped to the engine's credential field
```

```sh
# Create or update the declared databases (creation is ordered after project + environment)
iac-coolify apply infra/ --env staging --auto-approve

# Restrict to one database by logical name
iac-coolify apply infra/ --target pg-core --auto-approve

# Delete databases (reverse dependency order; a full teardown of configs, volumes and networks)
iac-coolify destroy infra/ --env staging --auto-approve
```

The `password` is an opaque `Secret`: it is mapped to the engine-specific credential field
(`postgres_password`, `redis_password`, `mysql_password`, …) and revealed only at the HTTP
boundary, never in plan/apply output, errors or the audit log. The `apply` diff compares
`image`, `public`/`public_port` and CPU/memory limits; a credential never field-diffs, so
rotating a password is an explicit future operation rather than reconciled drift.

> **MongoDB passwords on create:** the pinned Coolify v4 spec exposes no password field on
> the MongoDB create endpoint (only `mongo_initdb_root_username`), so a MongoDB `password`
> is not sent at creation time. Set it afterwards through Coolify.

## Environment interpolation

Any visible (Param) string field can reference an environment variable with
`${env:VAR}` — `metadata` names, `image.name`/`image.tag`, `fqdn`, a git `source`, an
inline `dockerfile`, `destination`, `limits`, and plain `env_vars` values. References are
resolved at `apply`, just before the value is pushed; an unset variable is then an error (no
silent fallback to `""`). `validate`, `plan` and `explore` keep references unresolved and need
no variable set.

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

`${env:…}` references (secrets and visible values alike) resolve at `apply`, not at load:
`validate`, `plan` and `explore` keep them unresolved and need no variable set, while `apply`
binds them and fails with a clear, resource-named error if one is missing. `${sops:…}` is
decrypted at load. `validate --strict` therefore no longer flags an unset secret env var.

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

## Importing existing resources

`iac-coolify import [dir]` reverse-engineers a live Coolify instance into local YAML
manifests, so you can adopt an instance that was built by hand. It runs non-interactively
(CI-friendly): point it at the instance with the same credentials the other commands use,
and it scaffolds a `coolify.yaml` root plus an `environments/<env>/{applications,databases}/`
tree under `dir` (default: the current directory).

```bash
export COOLIFY_API_URL=https://coolify.example.com
export COOLIFY_API_TOKEN=...                  # both required; import has no offline mode
iac-coolify import ./coolify                  # scaffold the whole instance
iac-coolify import ./coolify --env staging    # only the staging environment
iac-coolify import ./coolify --force          # overwrite existing manifests
```

Import never writes a secret value. Every application environment variable becomes a
`${env:KEY}` reference and every database password a `${env:NAME_PASSWORD}` reference; the
final report lists the keys to populate in your own `.env`. Verify the result with
`iac-coolify validate ./coolify` and `iac-coolify plan ./coolify` — a faithfully imported
resource plans as no change.

| Flag | Meaning |
|------|---------|
| `--default-network` | Docker network written on every resource (default `coolify`; the API does not expose it) |
| `--env` | Import only one or more environment names (repeatable) |
| `--force` | Overwrite existing manifests instead of refusing |

Two limits are reported, not silently dropped:

- **Services are not imported.** The API exposes only a service's name and environment, not
  enough to rebuild its spec (tracked at [coollabsio/coolify#10449](https://github.com/coollabsio/coolify/issues/10449)).
- **Git-based applications are partial.** Coolify does not expose the source repository URL,
  so a placeholder is written and the application is listed as needing a manual edit before
  it will `validate`. Applications built from a docker image import completely.

By default `import` refuses to overwrite any existing file, listing every collision so a
re-run never clobbers local edits; pass `--force` to overwrite.

## Exploring interactively

`iac-coolify explore` (alias `tui`) opens a terminal browser over the live Coolify
instance — walk the project → environment → resource tree on the left and inspect the
selected resource on the right. Pass an optional config path to compare desired YAML against
live state (drift) and to run application lifecycle actions behind a confirm prompt.

```bash
export COOLIFY_API_URL=https://coolify.example.com
export COOLIFY_API_TOKEN=...        # set both to skip the connection wizard
iac-coolify explore                 # browse only
iac-coolify explore ./coolify       # browse + drift against the config in ./coolify
```

When no credentials are in the environment, `explore` opens a **connection wizard** instead of
refusing to start: enter the Coolify URL, the API token, and an optional Cloudflare Access pair
(Client ID + Secret). The token and the CF-Access secret are masked as you type, and `enter`
tests the connection before opening the browser — a bad URL or token keeps the wizard open with
the (redacted) error so you can fix it. The entered values are used for the session only: they
are never written to disk and never logged. `esc` quits without connecting.

When `explore` starts in a directory that holds no resource manifests, it opens an onboarding
menu instead of an empty tree: `[S]ync` imports the live instance into local YAML (the same
reverse-engineering as `iac-coolify import`, writing `${env:…}` references — never secret
values — and printing a report), `[I]nit` scaffolds an empty root `coolify.yaml`, `[B]rowse`
opens the live tree without any local config, and `[Q]uit` exits. After a sync the desired
config is loaded straight away, so drift and the desired comparison work in the same session;
if manifests already exist a sync asks before overwriting them (`[y/N]`).

| Key        | Action                                            |
|------------|---------------------------------------------------|
| `↑`/`↓` `k`/`j` | move the cursor                              |
| `↵`        | expand/collapse a container, or open a resource   |
| `esc`/`backspace` | close the active pane (logs, drift, detail), else collapse or jump to the parent |
| `r`        | reveal/hide masked environment-variable values    |
| `D`        | drift: compare the selected application against its config |
| `R`/`S`/`U` | restart / stop / start the selected application (asks `[y/N]`) |
| `e`/`s`/`d` | edit the selected desired env var / save edits to YAML / discard them |
| `/`        | filter env vars by key (`esc` clears the filter)  |
| `L`        | toggle the log pane                               |
| `q`/`ctrl+c` | quit                                            |

A service shows its live environment variables as a table; every value is **masked by
default** and only shown after you press `r`. An application shows its structural fields and,
when a config path is given and a desired Application matches by `(environment, name)`, an
**ENV VARS (desired)** section read from the YAML: plain values are masked until `r`, and a
secret is shown only by its source declaration (`${env:…}`/`${sops:…}`) — its resolved value
is never read or displayed. Databases show their structural fields only.

An application's desired section is compared against its **live** env vars, joined by name and
summarised in the header as `N tracked · N only-local · N only-remote`. The comparison is by
**presence**, not value (a desired value is often a `${env:…}` reference, not a resolved value,
so a value-to-value diff would be meaningless): a name found on both sides is `tracked` and
shows the live value beside the desired one (masked until `r`); a desired name with no live
counterpart is `only-local`. Live env vars with no desired counterpart — what is left to
capture as IAC — are listed read-only under **ENV VARS (only on remote)** and tagged
`only-remote`. If the live env listing fails, the fields and desired section still load and the
comparison is reported as unavailable. Live values are masked by default like everything else.

Coolify scopes each live env var as buildtime, runtime, or both, so rows carry a scope tag
(`KEY [build]`, `KEY [runtime]`, `KEY [build,runtime]`). The same key in two scopes is a genuine
variant and is kept as two rows; exact `(key, scope)` duplicates are collapsed, and a collapsed
pair that disagreed on its value is flagged `⚠ conflicting values` (the value itself is never
shown). The only-remote presence count is by unique key, so a key present in both scopes counts
once. Press `/` to filter the env lists by a key substring (case-insensitive — keys only, never
values); the header shows `filter: <query> (matched/total)` and `esc` clears it. A long
only-remote list scrolls within a fixed window with `↑ N more`/`↓ N more` markers: `↓`/`j` runs
the cursor through the editable desired rows and then scrolls the list, `↑`/`k` scrolls back.

Press `D` on an application to see its **drift** — the per-field difference between the
desired config and the live resource, matched by logical name. The drift view is read-only;
secret fields are never shown in cleartext. It needs a config path; without one it says so.

`R`/`S`/`U` trigger an application **lifecycle** action. Each asks for confirmation first
(`[y/N]`) so a stray key cannot restart a service, and each action is recorded to the
append-only audit log (`--audit-log`, default `.iac-coolify/audit.log`). `explore` requires an
interactive terminal — in a pipe or CI it exits with an error rather than opening a UI.

In an application's **ENV VARS (desired)** section, `↑`/`↓` select a row and `e` opens it for
editing. A plain value is edited as-is; a secret is edited as its **source declaration** — the
input must stay a `${env:…}`/`${sops:…}` reference, so a secret can never be turned into a
literal and its resolved value is never read or written. Edited rows are marked `*` and held
unsaved until you press `s`, which writes the patched manifest back to its YAML file (atomic
write, recorded to the audit log); `d` discards the pending edits. Quitting with unsaved edits
asks for confirmation. The write-back updates the **desired** YAML only — run `iac-coolify
apply` to push it to Coolify.

## CI integration

Pass `COOLIFY_API_TOKEN` (and any Cloudflare Access service-token headers) as **masked**
CI secrets, and set `IAC_COOLIFY_ACTOR` so the audit log attributes the change. When you use
SOPS, store the age private key as a masked secret (`SOPS_AGE_KEY`) and write it to an
owner-only file pointed at by `SOPS_AGE_KEY_FILE`. Run `plan`
on pull/merge requests and `apply --auto-approve` on the protected default branch.

### GitHub Actions

Use the composite action — the version is selected by the action ref (it runs the published
container image as a Linux Docker action):

```yaml
jobs:
  plan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: Rems08/infrastructure-as-coolify@v1
        with:
          command: plan                 # plan | apply | destroy | validate
          path: coolify/
          env: staging                  # one or more, separated by spaces or commas
          api-token: ${{ secrets.COOLIFY_API_TOKEN }}
```

### GitLab CI

Include the reusable template and extend the jobs you need; `apply` runs on the default branch
and tags, `destroy` is manual:

```yaml
include:
  - remote: "https://raw.githubusercontent.com/Rems08/infrastructure-as-coolify/v0.1.0-rc.2/.gitlab/templates/iac-coolify.yml"

plan:
  extends: .iac-coolify-plan
apply:
  extends: .iac-coolify-apply
destroy:
  extends: .iac-coolify-destroy
```

Set `IAC_COOLIFY_PATH` and `IAC_COOLIFY_ARGS` (e.g. `--env staging`) as CI variables, and store
`COOLIFY_API_TOKEN` as a masked, protected variable.

For a runner without the action or template — or to pin via `go install` — the raw reference
workflows remain at [`examples/ci/github-actions.yml`](examples/ci/github-actions.yml) and
[`examples/ci/gitlab-ci.yml`](examples/ci/gitlab-ci.yml). Full documentation lives at the
[doc site](https://rems08.github.io/infrastructure-as-coolify/).

## License

[Apache-2.0](LICENSE). See [`SECURITY.md`](SECURITY.md) for responsible disclosure and
[`CONTRIBUTING.md`](CONTRIBUTING.md) to get started.

## Acknowledgements

Built on the shoulders of [coollabsio/coolify](https://github.com/coollabsio/coolify),
[charmbracelet](https://github.com/charmbracelet), [spf13/cobra](https://github.com/spf13/cobra),
and [goreleaser](https://goreleaser.com).
