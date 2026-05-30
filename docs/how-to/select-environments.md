# Select environments

`plan`, `apply` and `destroy` operate on every environment found under the config path by
default. Use `--env <name>` to restrict the run to one or more environments, matched against
each resource's `metadata.environment`:

```sh
# A single environment
iac-coolify plan   infra/ --env staging
iac-coolify apply  infra/ --env production --auto-approve

# Several at once (repeat the flag; the result is their union)
iac-coolify apply  infra/ --env staging --env preview --auto-approve

# Compose with --target to act on one named resource within an environment
iac-coolify apply  infra/ --env staging --target back-office --auto-approve
```

`--env` composes with `--target` (logical resource name): a resource is selected only when it
passes **both** filters. A `--env` value that no resource declares — or a `--target`/`--env`
combination that matches nothing — exits with code `2`, so a typo fails loudly instead of
silently doing nothing. Projects are cross-environment and are never filtered by `--env`.

Both folder layouts work — the filter reads `metadata.environment`, not the path:

```
# Nested per environment                       # Flat
infra/                                          infra/
  environments/staging/applications/a.yaml        applications/a.yaml   # environment: staging
  environments/production/applications/b.yaml      applications/b.yaml   # environment: production
```
