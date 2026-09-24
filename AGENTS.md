# AGENTS.md

Guidance for AI coding agents working in this repository. Human-facing docs
are in [README.md](README.md).

## What this is

`wowapi` is a single Go binary with two front ends over the same client:

- a CLI (`main.go`) for querying Blizzard's World of Warcraft API, and
- an MCP server over stdio (`mcp.go`, `wowapi mcp`) exposing read-only tools.

## Layout

| Path | Purpose |
|---|---|
| `main.go` | CLI entry point, argument parsing, usage text, `version` constant |
| `mcp.go` | MCP tool definitions (`wow_*`) using `github.com/modelcontextprotocol/go-sdk` |
| `blizzard/client.go` | OAuth client-credentials flow, HTTP, response cache |
| `blizzard/wow.go` | Endpoint helpers (character, item, search, profession, recipe, commodities), `Slim`, `RealmSlug`, `FormatGold` |
| `blizzard/blizzard_test.go` | Tests against an `httptest` fake API — no network or credentials needed |
| `Taskfile.yml` | All common commands ([taskfile.dev](https://taskfile.dev)) |

## Commands

Use Task rather than raw `go` commands:

- `task build` — build the binary
- `task test` — run tests
- `task check` — gofmt check + `go vet` + tests. **Run this before finishing any change.**
- `task fmt` — format
- `task run -- <args>` — run the CLI (needs credentials in `.env`)

## Conventions

- Standard library first; the MCP SDK is the only direct dependency. Don't add
  dependencies without a clear need.
- Everything is **read-only**. Don't add tools or commands that write to any
  Blizzard or Battle.net endpoint.
- Keep the CLI and MCP surfaces in step: a new capability usually needs a
  `blizzard` method, a CLI subcommand (plus `usage` text in `main.go`), an
  MCP tool in `mcp.go`, and a line in the README.
- MCP tool descriptions and `jsonschema` tags are read by models — keep them
  specific (IDs, defaults, examples).
- API responses go through `blizzard.Slim` to drop `_links`/`href` noise
  unless the user asks for `-raw`.
- New `blizzard` behavior gets a test in `blizzard_test.go` using the existing
  `fakeAPI` helper. Tests must never hit the real API.

## Secrets

- Credentials come only from the environment (`BLIZZARD_CLIENT_ID`,
  `BLIZZARD_CLIENT_SECRET`), loaded from `.env` by Task. `.env` is git-ignored.
- Never commit `.env`, real client IDs/secrets or access tokens, and never
  print them in logs or error messages.

## Gotchas

- Character data updates on logout, not live. A 404 usually means a
  misspelled name (accents matter: `Cëldis`), wrong realm/region, or a
  character that's under level 10 or long inactive.
- Commodity AH data is region-wide and cached 30 minutes; non-stackable items
  (gear, bags) are only in per-connected-realm auctions, which are multi-megabyte.
- In Git Bash, `MSYS_NO_PATHCONV=1` is needed for `wowapi get /data/wow/...`.
