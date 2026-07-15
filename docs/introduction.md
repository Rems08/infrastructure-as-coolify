# iac-coolify

Declarative Infrastructure as Code for [Coolify](https://coolify.io) — YAML in,
`plan`/`apply`/`destroy` out. No HCL, no state file, no magic.

`iac-coolify` is a single-binary CLI that manages Coolify v4 resources declaratively, the
way Terraform manages cloud resources — but native to Coolify, with Kubernetes-style YAML
(`apiVersion`/`kind`/`metadata`/`spec`), a stateless-first model, and auto-generated docs.

**Status:** beta — Coolify **v4.x only**. APIs may change before `v1.0.0`.

## Supported platforms

| OS | amd64 | arm64 |
|---|---|---|
| Linux | ✅ | ✅ |
| macOS | ✅ | ✅ |
| Windows | ✅ | — |

## How this documentation is organised

This documentation follows the [Diátaxis](https://diataxis.fr/) framework:

- **Tutorials** — a hands-on getting-started lesson.
- **How-to guides** — recipes for specific tasks: installing and verifying releases,
  selecting environments, managing databases, moving resources between servers,
  interpolation, secrets, importing an existing instance, exploring interactively, and
  running in CI.
- **Reference** — the generated resource schemas (`Project`, `Environment`,
  `Application`, `Service`, `Database`, `EnvVar`).
- **Explanation** — the why behind the tool and its configuration model.
