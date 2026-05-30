# Explore live state interactively

`iac-coolify explore` (alias `tui`) opens a read-only terminal browser over a live Coolify
instance. It walks the project → environment → resource tree and inspects each resource. It
**never mutates anything**: there is no create, update, delete, start, stop or restart in
the browser.

## Prerequisites

`explore` browses the remote instance, so — unlike `plan` — it has no offline mode and
requires credentials:

```bash
export COOLIFY_API_URL=https://coolify.example.com
export COOLIFY_API_TOKEN=...
iac-coolify explore
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
| `L`                | toggle the log pane                                   |
| `q`/`ctrl+c`       | quit                                                  |

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
