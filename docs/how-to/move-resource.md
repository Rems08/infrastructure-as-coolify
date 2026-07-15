# Move a resource to another server

The Coolify API has no in-place server move, so `destination.server` and
`destination.network` are not patchable fields. `plan` diffs them (projecting the server's
logical *name*, never its UUID) and marks any change as a recreate, distinct from an
ordinary update — in JSON the item carries `"requires_recreate": true`:

```text
-/+ Application.api must be recreated (destination changed: server "localhost" -> "hetzner-1")
    ~ Application.api.destination.server: "localhost" -> "hetzner-1"
```

`apply` refuses to update a resource whose destination changed — no partial PATCH is
emitted, so the resource can never end up half-migrated with its old server silently kept:

```text
cannot move application "api" to server "hetzner-1": destroy it first, then re-apply (Coolify has no in-place server move)
```

The supported choreography is destroy, then re-apply:

```sh
iac-coolify destroy infra/ --env staging --target api --auto-approve   # removes it from the old server
# edit destination.server in the manifest
iac-coolify apply infra/ --env staging --target api --auto-approve     # creates it on the new server
```

A `destination.server` that references a variable (`"${env:STAGING_SERVER}"`) stays stable
at `plan` time — references are resolved only at `apply`, so an unresolved reference never
shows up as a phantom recreate, and `destroy` works without the variable set.
