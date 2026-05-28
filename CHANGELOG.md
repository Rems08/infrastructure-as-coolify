# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased] - 2026-05-28

### Added (Wave 2)

- `iac-coolify plan`: Terraform-style per-field diff between the desired YAML and live
  Coolify state, with `--output=text|json` (auto-detecting TTY/CI), `--detailed-exitcode`
  (`0`/`2`/`1`), and an offline mode (all resources treated as new) when no Coolify
  URL/token is configured.
- Semantic diff engine (`internal/plan`) with Notify-only secret handling: a secret value
  change is announced without ever exposing the value, hash, or any partial.
- Live UUID resolver (`internal/state`): maps logical `(project, environment, kind, name)`
  keys to Coolify UUIDs via the documented `GET /projects`, `/projects/{uuid}/environments`
  and `/applications` endpoints, with an opt-in `--state-cache` JSON file.
- `Database` resource (8 engines: postgresql, mysql, mariadb, mongodb, redis, keydb,
  dragonfly, clickhouse) and a standalone `EnvVar` resource referenced by an Application
  through `env_vars_from`. `validate` and `docs gen` are now multi-resource aware.
- Cloudflare Access support: `CF-Access-Client-Id` / `CF-Access-Client-Secret` headers on
  every request, the secret typed as `Secret` and revealed only at the header boundary.
- OpenAPI checksum verification wired into `plan` boot (refuses a tampered pinned spec,
  tolerates its absence outside the repo).

### Added (Wave 1)

- Repository scaffold: Go module, `internal/` layout, Apache-2.0 license and OSS docs
  (`README`, `CONTRIBUTING`, `CODE_OF_CONDUCT`, `SECURITY`).
- Strict `golangci-lint` preset, `Makefile`, and GitHub Actions CI (lint on go1.23,
  test + cross-build matrix on go1.25) plus a nightly OpenAPI drift watcher.
- Opaque `secrets.Secret` type: redacted across `String`/`GoString`/`MarshalJSON`/
  `MarshalYAML`/`LogValue`, with `${env:VAR}` sourcing, literal values forbidden, and an
  AST ratchet restricting `Reveal()` to `internal/secrets` and `internal/coolify`.
- `${env:VAR}` interpolation for visible (non-secret) values.
- Minimal Coolify v4 API client (`GET /api/v1/applications`) with a Bearer token typed
  as `Secret`, bounded retries, and a pinned-OpenAPI SHA-256 verifier.
- `Application` resource type with `iac:"doc=..."` tags as the single source of truth,
  `ExactlyOneOf{value, value_secret}` validation, and a generated JSON Schema.
- `iac-coolify validate` (strict YAML parsing, semantic validation, and `--strict`
  plaintext-secret detection via regex canaries + entropy) and `iac-coolify version`.
- `iac-coolify docs gen`: Diátaxis reference generated from struct tags, guarded by an
  architecture test that fails when docs drift from the structs.
- State cache type that refuses to marshal if it ever gains a `Secret` field.

[Unreleased]: https://github.com/Rems08/infrastructure-as-coolify/commits/main
