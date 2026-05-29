# The configuration model

Resources are described in a directory tree, one file per resource, using Kubernetes-style
documents (`api_version`/`kind`/`metadata`/`spec`):

```
coolify/
  coolify.yaml                                   # global config (api_url, required_coolify)
  project.yaml                                   # kind: Project
  environments/<env>/environment.yaml            # kind: Environment
  environments/<env>/applications/<app>.yaml     # kind: Application
  environments/<env>/databases/<db>.yaml         # kind: Database (8 engines)
  environments/<env>/envvars/<set>.yaml          # kind: EnvVar (shared, via env_vars_from)
  services/<svc>.yaml                            # kind: Service (docker-compose stack)
```

Supported resource kinds: **Project**, **Environment**, **Application**, **Service**,
**Database** (`postgresql`, `mysql`, `mariadb`, `mongodb`, `redis`, `keydb`, `dragonfly`,
`clickhouse`), and standalone **EnvVar** sets that an Application merges in by name
(`env_vars_from`). `apply` creates projects and environments before the applications and
services that reference them. The full field-by-field schema for each kind is in the
[reference](../reference/project.md).

## Applications and build packs

An Application is built by one of six build packs, each with its own source of truth:

- `dockerimage` — a prebuilt image (`image.name` + `image.tag`).
- `dockerfile` — either an inline `dockerfile` (Dockerfile content, no git) or a git
  `source`; exactly one of the two.
- `nixpacks`, `docker-compose`, `static`, `railpack` — a public git `source`
  (`git_repository` + `git_branch` + `ports_exposes`).

A git `source.git_repository` must use an `https://`, `http://` or `git@` URL; an inline
`dockerfile` is capped at 1 MB. The IaC build pack `docker-compose` is sent to Coolify under
its upstream spelling `dockercompose`.

> **Private git sources:** applications from a private GitHub App or deploy key are not yet
> supported (they need a separate credential resolver). Use a public repository, an inline
> Dockerfile, or a prebuilt image.

## Services

A Service is a docker-compose stack. It sources its definition from exactly one of a
`docker_compose_path` (a relative path to a compose file in your repository, which may not
resolve outside the config tree) or a `type` (a Coolify one-click template identifier such as
`gitea-with-mysql`).

## Validation at the boundary

Configuration is validated where it is parsed, not where it is used: `validate` checks YAML
syntax, semantic rules, and (with `--strict`) flags secret-like values placed in visible
fields. After validation, the internal engine trusts the typed structs — there are no
defensive fallbacks scattered through `plan` and `apply`.
