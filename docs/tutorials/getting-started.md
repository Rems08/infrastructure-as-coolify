# Getting started

This tutorial takes you from an empty shell to a previewed change against a Coolify
instance, using the bundled `examples/minimal/` configuration.

## 1. Install

Download a binary for your platform from the
[releases page](https://github.com/Rems08/infrastructure-as-coolify/releases), or pull the
container image:

```sh
docker pull ghcr.io/rems08/infrastructure-as-coolify:latest
docker run --rm ghcr.io/rems08/infrastructure-as-coolify version
```

Images are multi-arch (`linux/amd64`, `linux/arm64`). Replace `:latest` with a version such
as `:0.1.1` to pin a release.

## 2. Validate a configuration

Validation is fully offline — it needs no Coolify connection:

```sh
iac-coolify validate examples/minimal/
```

You should see `Validated 1 application: web (no issues)`.

## 3. Preview the changes

`plan` resolves Coolify UUIDs from your logical `metadata` names (never written to YAML),
fetches live state, and prints a per-field diff. Without a configured Coolify URL/token it
runs offline, treating every resource as new:

```sh
iac-coolify plan examples/minimal/

# Against a live instance:
export COOLIFY_API_TOKEN=...                  # plus --coolify-url or COOLIFY_API_URL
iac-coolify plan examples/minimal/ --output=json --detailed-exitcode
```

`--detailed-exitcode` returns `0` (no changes), `2` (changes pending), or `1` (error) — handy
in CI.

## 4. Apply and tear down

```sh
# Create resources in dependency order (project -> environment -> application)
iac-coolify apply examples/full-project/ --dry-run        # offline preview, mutates nothing
iac-coolify apply examples/full-project/ --auto-approve   # required in CI / non-interactive

# Remove them in reverse dependency order
iac-coolify destroy examples/full-project/ --auto-approve
```

A repeated `apply` on an unchanged state is a no-op, and a repeated `destroy` is too (a `404`
on delete is treated as success).

## Next steps

- [Select environments](../how-to/select-environments.md) to scope a run.
- [Manage secrets](../how-to/secrets.md) so sensitive values never reach output.
- Browse the [reference](../reference/application.md) for every field.
