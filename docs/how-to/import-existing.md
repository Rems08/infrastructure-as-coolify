# Import existing resources

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
