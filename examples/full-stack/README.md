# Full-stack example — create, update, destroy

A complete tree: a Project, one Environment, three Applications each built by a different
build pack, and a Service (docker-compose stack).

```
full-stack/
  coolify.yaml                                   # kind: Project (beenaire)
  environments/production/environment.yaml       # kind: Environment
  applications/dockerfile-inline.yaml            # build_pack: dockerfile (inline)
  applications/nixpacks-public.yaml              # build_pack: nixpacks (public git)
  applications/dockercompose-public.yaml         # build_pack: docker-compose (public git)
  services/observability.yaml                    # kind: Service
  services/docker-compose.yml                    # the stack the Service deploys
```

The manifests use a flat layout (Applications grouped under `applications/`) with the owning
environment set per file in `metadata.environment`. The `--env` filter reads that field, not
the path, so both flat and nested layouts work.

## Create

```sh
export COOLIFY_API_TOKEN=... COOLIFY_API_URL=https://coolify.example.com
iac-coolify apply examples/full-stack/ --dry-run          # offline preview, mutates nothing
iac-coolify apply examples/full-stack/ --auto-approve     # required in CI / non-interactive
```

`apply` creates resources in dependency order: the Project and Environment first, then the
Applications and Service that reference them. Every write carries an `Idempotency-Key`, so a
retried apply cannot create duplicates.

## Update

Edit any field — say the image tag of an Application or an env var — and re-apply. `plan`
shows a per-field diff; a second apply with no edits reports no changes.

```sh
iac-coolify plan  examples/full-stack/ --env production
iac-coolify apply examples/full-stack/ --auto-approve
```

## Destroy

```sh
iac-coolify destroy examples/full-stack/ --dry-run        # offline preview, deletes nothing
iac-coolify destroy examples/full-stack/ --auto-approve
```

`destroy` deletes in reverse dependency order (Applications and Service, then Environment,
then Project). Only resources that still exist remotely are deleted, so a repeated destroy
is a no-op.
