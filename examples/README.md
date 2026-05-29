# Examples

Each directory is a self-contained config tree you can point `iac-coolify` at. `validate`
and `plan --dry-run` run fully offline, so you can explore every example without a Coolify
instance or token.

```sh
iac-coolify validate examples/minimal/
iac-coolify plan     examples/minimal/        # offline (all-new) when unconfigured
```

| Directory | What it shows |
|---|---|
| [`minimal/`](minimal/) | Smallest config — global settings + one `dockerimage` Application. Copy-paste quickstart. See [minimal/README.md](minimal/README.md). |
| [`full-stack/`](full-stack/) | Project + Environment + three build-pack Applications (inline Dockerfile, public-git nixpacks, public-git docker-compose) + a Service. A worked create/update/destroy cycle. See [full-stack/README.md](full-stack/README.md). |
| [`full-project/`](full-project/) | Maximal coverage of every kind — Project, Environment, multi-build-pack Applications, Service, and Database. |
| [`beenaire/`](beenaire/) | A redacted snapshot of a real multi-environment deployment (applications, services, databases). |
| [`secrets-sops/`](secrets-sops/) | SOPS + age secrets at rest: `value_secret: "${sops:path}"` decrypted in memory. |
| [`env-interp/`](env-interp/) | `${env:VAR}` interpolation across Param fields. |
| [`envvar/`](envvar/) | Standalone `EnvVar` sets merged into an Application via `env_vars_from`. |
| [`ci/`](ci/) | Ready-to-copy GitHub Actions and GitLab CI pipelines. |
| [`invalid/`](invalid/) | Deliberately broken manifests used as negative fixtures by the test suite. |

Resource fields are documented in [`docs/reference/`](../docs/reference/), generated from
the Go structs (`iac-coolify docs gen`).
