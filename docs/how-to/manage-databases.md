# Manage databases

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
# Create or update the declared databases (ordered after project + environment)
iac-coolify apply infra/ --env staging --auto-approve

# Restrict to one database by logical name
iac-coolify apply infra/ --target pg-core --auto-approve

# Delete databases (reverse dependency order; a full teardown of configs, volumes and networks)
iac-coolify destroy infra/ --env staging --auto-approve
```

The `password` is an opaque `Secret`: it is mapped to the engine-specific credential field
(`postgres_password`, `redis_password`, `mysql_password`, …) and revealed only at the HTTP
boundary, never in plan/apply output, errors or the audit log. The `apply` diff compares
`image`, `public`/`public_port`, the `destination` pair and CPU/memory limits; a credential
never field-diffs, so rotating a password is an explicit future operation rather than
reconciled drift. A `destination` change is a recreate, not an update — see
[Move a resource to another server](move-resource.md).

> **MongoDB passwords on create:** the pinned Coolify v4 spec exposes no password field on
> the MongoDB create endpoint (only `mongo_initdb_root_username`), so a MongoDB `password`
> is not sent at creation time. Set it afterwards through Coolify.
