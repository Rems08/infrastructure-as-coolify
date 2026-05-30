# Explore live state interactively

`iac-coolify explore` (alias `tui`) opens a terminal browser over a live Coolify instance.
It walks the project → environment → resource tree and inspects each resource. Browsing and
the drift view are read-only; the only writes it can make are application **lifecycle**
actions (restart/stop/start), and each one asks for confirmation first. There is no create,
update, delete, or environment-variable editing in the browser yet.

## Prerequisites

`explore` browses the remote instance, so — unlike `plan` — it has no offline mode and
requires credentials:

```bash
export COOLIFY_API_URL=https://coolify.example.com
export COOLIFY_API_TOKEN=...
iac-coolify explore             # browse only
iac-coolify explore ./coolify   # browse + drift against the config under ./coolify
```

If either variable is missing, or you are running in a pipe or in CI (no interactive
terminal), `explore` exits with an explanatory error instead of opening a UI.

## Keys

| Key                | Action                                                |
|--------------------|-------------------------------------------------------|
| `↑`/`↓`, `k`/`j`   | move the cursor                                       |
| `↵`                | expand/collapse a container, or open a resource       |
| `esc`/`backspace`  | collapse the node, or jump to its parent              |
| `r`                | reveal/hide a service's environment-variable values   |
| `D`                | drift: compare the selected application with its config |
| `R`/`S`/`U`        | restart / stop / start the selected application (asks `[y/N]`) |
| `L`                | toggle the log pane                                   |
| `q`/`ctrl+c`       | quit                                                  |

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
and resource and carries no secret. Editing environment variables and writing changes back
to YAML are not in the browser yet.

## What each resource shows

- **Service** — its environment variables as a table. Every value is **masked** (`••••••`)
  until you press `r`; pressing it again re-masks them. Revealing is a view toggle on values
  the read API already returns in cleartext — nothing is decrypted, logged or written.
- **Application** and **Database** — their structural fields (status, image, build pack,
  type, …). They carry no environment-variable table on the read path, so the mask toggle
  does not apply to them.

Databases that Coolify exposes only through the per-server resource listing carry no
project or environment, so they appear under a single top-level `databases` group rather
than nested under a project.

## Logs

Press `L` to open a pane showing the structured log records produced during the session
(for example, how many resources the resolver found). Logs are routed through the UI rather
than printed to the terminal, so they never corrupt the full-screen view.
