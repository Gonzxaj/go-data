# Contributing to go-data

Thanks for wanting to help. Pull requests are welcome. Every PR is reviewed
by the maintainer before it is merged.

## Language

Write everything in **English**: code, comments, commit messages, PR titles
and descriptions, and issues.

## Pull request description

Every PR must explain three things:

1. **Problem** – what is wrong or missing, and how to reproduce it
   (steps, logs, screenshots for UI changes).
2. **Solution** – what you changed and why you chose that approach.
3. **Tests** – which tests you added or ran, and how you checked the change
   by hand (e.g. "ran with `docker compose up`, opened the Network tab").

Template:

```markdown
## Problem
<what is broken or missing>

## Solution
<what changed and why>

## Tests
<tests added / commands run / manual checks>
```

Keep each PR focused on one change. Unrelated fixes go in a separate PR.

## Before you open the PR

```bash
gofmt -l .        # must print nothing
go vet ./...
go test ./...
go build ./cmd/server
```

New logic (parsers, collectors, handlers, anything with branches) needs a
test next to it, following the existing `*_test.go` files in the same
package (plain `testing`, no extra test frameworks).

## Code organization

Keep the structure the project already has. Put code in the layer it belongs
to and follow the style of the files around it:

| Where | What goes there |
|---|---|
| `cmd/server/` | entrypoint only |
| `internal/app/` | wiring: builds components and connects them |
| `internal/config/` | `config.json` + environment variable loading |
| `internal/domain/` | shared types, no logic |
| `internal/collector/` | reading metrics from the system / Docker |
| `internal/api/` | HTTP handlers, one `handler_<area>.go` per area |
| `internal/web/` | HTML template rendering |
| `internal/storage/` | storage backends behind the `storage.Storage` interface |
| `internal/<feature>/` | self-contained features (`alarm`, `netmon`, `security`, `uptime`, ...) |
| `web/templates/` | `views/` per tab, `components/` for shared pieces |
| `web/static/js/` | one plain JS file per view, no build step |

Rules of thumb:

- **No new dependencies** without asking first in an issue. Prefer the
  standard library and what is already in `go.mod`.
- **No JavaScript build step.** Frontend stays plain HTML, CSS and JS.
- **New features are opt-in.** Add a config option in `internal/config/`
  with an environment variable, and document it in the README table.
- **Degrade gracefully.** If a file, mount or socket is missing, log it and
  show no data. Do not crash.
- **New API endpoints** must be documented in the README `API` section.
- Small, readable functions over clever ones. Comment the *why*, not the
  *what*.

## Commits

Use [Conventional Commits](https://www.conventionalcommits.org/), like the
existing history:

```
feat: add outbound tracking option to network monitoring
fix: handle missing /proc/net/tcp6
chore: update Go version in Dockerfile
```

## Security issues

Do not open a public issue for a vulnerability. Contact the maintainer
privately through GitHub instead.
