# Explore live state interactively

`iac-coolify explore` (alias `tui`) opens a terminal browser over a live Coolify instance.
It walks the project → environment → resource tree and inspects each resource. Browsing and
the drift view are read-only. It can make two kinds of write: application **lifecycle**
actions (restart/stop/start), each behind a confirmation prompt; and **write-back of an
application's desired env vars** to its YAML file. There is no create or delete in the browser
yet, and write-back never pushes to Coolify — run `iac-coolify apply` for that.

## Prerequisites

`explore` requires an interactive terminal — in a pipe or CI it exits with an error rather
than opening a UI. Credentials can come from the environment, or be entered in the
[connection wizard](#connection-wizard) when they are missing:

```bash
export COOLIFY_API_URL=https://coolify.example.com
export COOLIFY_API_TOKEN=...        # set both to skip the connection wizard
iac-coolify explore                 # browse only
iac-coolify explore ./coolify       # browse + drift against the config under ./coolify
```

## Connection wizard

When no credentials are in the environment, `explore` opens a **connection wizard** instead of
refusing to start: enter the Coolify URL, the API token, and an optional Cloudflare Access pair
(Client ID + Secret). The token and the CF-Access secret are masked as you type, and `enter`
tests the connection before opening the browser — a bad URL or token keeps the wizard open with
the (redacted) error so you can fix it. The entered values are used for the session only: they
are never written to disk and never logged. `esc` quits without connecting.

## Starting in an empty directory

When `explore` starts in a directory that holds no resource manifests, it opens an onboarding
menu instead of an empty tree:

- `[S]ync` imports the live instance into local YAML — the same reverse-engineering as
  [`iac-coolify import`](import-existing.md), writing `${env:…}` references (never secret
  values) and printing a report. After a sync the desired config is loaded straight away, so
  drift and the desired comparison work in the same session; if manifests already exist a sync
  asks before overwriting them (`[y/N]`).
- `[I]nit` scaffolds an empty root `coolify.yaml`.
- `[B]rowse` opens the live tree without any local config.
- `[Q]uit` exits.

## Keys

| Key                | Action                                                |
|--------------------|-------------------------------------------------------|
| `↑`/`↓`, `k`/`j`   | move the cursor                                       |
| `↵`                | expand/collapse a container, or open a resource       |
| `esc`/`backspace`  | close the active pane (logs, drift, detail), else collapse or jump to the parent |
| `r`                | reveal/hide masked environment-variable values        |
| `D`                | drift: compare the selected application with its config |
| `R`/`S`/`U`        | restart / stop / start the selected application (asks `[y/N]`) |
| `e`                | edit the selected desired env var                     |
| `s`/`d`            | save edits back to YAML / discard them                |
| `/`                | filter env vars by key (`esc` clears the filter)      |
| `L`                | toggle the log pane                                   |
| `q`/`ctrl+c`       | quit                                                  |

## What each resource shows

- **Service** — its live environment variables as a table. Every value is **masked**
  (`••••••`) until you press `r`; pressing it again re-masks them. Revealing is a view toggle
  on values the read API already returns in cleartext — nothing is decrypted, logged or
  written.
- **Application** — its structural fields (status, image, build pack, …) and, when a config
  path is given and a desired Application matches by `(environment, name)`, an **ENV VARS
  (desired)** section read from the YAML. A plain value is masked until you press `r`; a
  secret is shown only by its source declaration (`${env:NAME}` / `${sops:path}`) next to a
  🔒 marker. The resolved secret value is never read or displayed — `r` reveals plain values,
  never a secret. When no desired Application matches the selected name (or no config path was
  given), the section says so rather than failing. These rows can be edited and written back to
  YAML — see [Editing desired env vars](#editing-desired-env-vars).
- **Database** — its structural fields (status, type, …) only; no environment-variable table
  on the read path, so the mask toggle does not apply.

Databases that Coolify exposes only through the per-server resource listing carry no
project or environment, so they appear under a single top-level `databases` group rather
than nested under a project.

## Comparing desired and live env vars

An application's desired section is compared against its **live** env vars, joined by name and
summarised in the header as `N tracked · N only-local · N only-remote`. The comparison is by
**presence**, not value (a desired value is often a `${env:…}` reference, not a resolved value,
so a value-to-value diff would be meaningless):

- a name found on both sides is `tracked` and shows the live value beside the desired one
  (masked until `r`);
- a desired name with no live counterpart is `only-local`;
- live env vars with no desired counterpart — what is left to capture as IaC — are listed
  read-only under **ENV VARS (only on remote)** and tagged `only-remote`.

If the live env listing fails, the fields and desired section still load and the comparison is
reported as unavailable. Live values are masked by default like everything else.

Coolify scopes each live env var as buildtime, runtime, or both, so rows carry a scope tag
(`KEY [build]`, `KEY [runtime]`, `KEY [build,runtime]`). The same key in two scopes is a genuine
variant and is kept as two rows; exact `(key, scope)` duplicates are collapsed, and a collapsed
pair that disagreed on its value is flagged `⚠ conflicting values` (the value itself is never
shown). The only-remote presence count is by unique key, so a key present in both scopes counts
once.

Press `/` to filter the env lists by a key substring (case-insensitive — keys only, never
values); the header shows `filter: <query> (matched/total)` and `esc` clears it. A long
only-remote list scrolls within a fixed window with `↑ N more`/`↓ N more` markers: `↓`/`j` runs
the cursor through the editable desired rows and then scrolls the list, `↑`/`k` scrolls back.

## Drift

Press `D` on an application to compute its **drift**: the per-field difference between the
desired YAML config and the live resource. The desired and live sides are matched by the
resource's logical name (`metadata.name`), so it needs no file mapping — and it is purely a
read. Additions, updates and deletions are colour-coded; secret fields are reported by their
source declaration only and never shown in cleartext.

Drift needs a config path. Pass it as the argument to `explore` (e.g. `explore ./coolify`).
Without one — or when no desired Application matches the selected name — the pane says drift
is unavailable rather than failing.

## Lifecycle actions

`R`, `S` and `U` restart, stop and start the selected application. Each first opens a
confirmation prompt (`restart application "web"? [y/N]`) that captures every key, so a stray
press can neither trigger the action nor quit the program mid-prompt — only `y` proceeds,
`n`/`esc` cancels. Every action runs asynchronously and is recorded to the append-only audit
log (set with `--audit-log`, default `.iac-coolify/audit.log`); the entry names the action
and resource and carries no secret.

## Editing desired env vars

When an application's **ENV VARS (desired)** section is shown, `↑`/`↓` move a cursor over the
rows and `e` opens the selected one for editing in a text field that captures every key, so a
stray `q` or `s` cannot quit or save mid-typing — `↵` confirms, `esc` cancels.

- A **plain** value is pre-filled with the literal and edited freely.
- A **secret** is pre-filled with its **source declaration** (`${env:NAME}` / `${sops:path}`)
  and edited as a reference: the input must stay a `${env:}`/`${sops:}` reference or the edit
  is rejected. The browser never holds a secret's resolved value, so it can neither display
  nor write one — a secret can only ever be re-pointed at another reference.

Confirmed edits are staged in memory and the changed rows are marked `*`; nothing touches disk
until you press `s`. `s` writes the patched manifest back to its YAML file with an atomic
temp-file-then-rename and records a `write-back` entry to the audit log; `d` discards the
staged edits and restores the on-disk values. Quitting with unsaved edits asks for
confirmation first. The write-back edits the **desired** YAML only — it does not push to
Coolify, so run `iac-coolify apply` to reconcile the live instance afterwards.

## Logs

Press `L` to open a pane showing the structured log records produced during the session
(for example, how many resources the resolver found). Logs are routed through the UI rather
than printed to the terminal, so they never corrupt the full-screen view.
