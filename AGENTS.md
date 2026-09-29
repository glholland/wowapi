# AGENTS.md

Guidance for AI coding agents working in this repository. Human-facing docs
are in [README.md](README.md).

## What this is

`wowapi` is a single Go binary with two front ends over the same client:

- a CLI (`main.go`) for querying Blizzard's World of Warcraft API, and
- an MCP server over stdio (`mcpserver/`, run with `wowapi mcp`) exposing
  read-only tools, resources and prompts.

## Layout

| Path | Purpose |
|---|---|
| `main.go` | CLI entry point, argument parsing, usage text, `version` (set by `-ldflags` in builds) |
| `mcpserver/server.go` | `New` / `Run`: builds the MCP server (official `github.com/modelcontextprotocol/go-sdk`) |
| `mcpserver/tools.go` | Tool definitions (`wow_*`), input types and structured output types |
| `mcpserver/resources.go` | `wow://` resources and resource templates |
| `mcpserver/prompts.go` | Prompts (`new_character`, `character_review`, `profession_plan`) |
| `mcpserver/describe.go` | `wowapi mcp -list`: lists the server through an in-memory MCP client |
| `mcpserver/server_test.go` | Protocol-level tests via in-memory transports and a fake API |
| `blizzard/client.go` | OAuth client-credentials flow, HTTP, response cache |
| `blizzard/wow.go` | Endpoint helpers (character, item, search, profession, recipe, commodities), `Slim`, `RealmSlug`, `FormatGold` |
| `blizzard/progress.go` | Condensed character progress: dungeon/raid encounters, Mythic+ seasons, known vs. missing recipes |
| `blizzard/sources.go` | Recipe sources: journal loot tables, recipe-item index, realm Auction House index |
| `blizzard/creation.go` | Character creation data: races, classes, specializations, talent trees |
| `blizzard/*_test.go` | Tests against an `httptest` fake API — no network or credentials needed |
| `Taskfile.yml` | All common commands ([taskfile.dev](https://taskfile.dev)) |
| `npm/build.mjs` | Builds the npm launcher + per-platform packages from `dist/` |
| `npm/wowapi/bin/wowapi.js` | npm launcher: finds the platform binary and runs it with inherited stdio |
| `npm/publish.sh` | Publishes the npm packages (platform packages first; skips existing versions) |
| `mcpb/build.mjs` | Builds and validates one Claude Desktop bundle (`.mcpb`) per platform; tool list comes from `wowapi mcp -list -json` |
| `.github/workflows/` | `ci.yml` (checks on push/PR), `release.yml` (on `v*` tags: GitHub release + npm) |

## Commands

Use Task rather than raw `go` commands:

- `task build` — build the binary
- `task test` — run tests
- `task check` — gofmt check + `go vet` + tests. **Run this before finishing any change.**
- `task lint` / `task vuln` — staticcheck and govulncheck (`task ci` runs everything)
- `task mcp:list` — confirm the MCP server starts and lists what you expect
- `task fmt` — format
- `task run -- <args>` — run the CLI (needs credentials in `.env`)

## Conventions

- Standard library first; the MCP SDK is the only direct dependency. Don't add
  dependencies without a clear need.
- Everything is **read-only**. Don't add tools or commands that write to any
  Blizzard or Battle.net endpoint.
- Keep the CLI and MCP surfaces in step: a new capability usually needs a
  `blizzard` method, a CLI subcommand (plus `usage` text in `main.go`), an
  MCP tool in `mcpserver/tools.go` (and a resource if it's reference data),
  and a line in the README.
- MCP tools that return condensed Go types are **typed**: use a concrete `Out`
  type in `mcp.AddTool` so the SDK publishes an `outputSchema` and fills
  `structuredContent`. Wrap lists in an object (`RaceList{Races: ...}`). Tools
  that pass Blizzard's raw JSON through return text via `rawResult`.
- MCP tool descriptions and `jsonschema` tags are read by models — keep them
  specific (IDs, defaults, examples).
- API responses go through `blizzard.Slim` to drop `_links`/`href` noise
  unless the user asks for `-raw`.
- New `blizzard` behavior gets a test using the existing `fakeAPI` helper; new
  MCP behavior gets a test in `mcpserver/server_test.go`. Tests must never hit
  the real API.

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
- Recipe names repeat across expansions ("Arcanoweave Bracers" is in Outland
  and Midnight Tailoring). Match teaching items by the skill tier they require,
  not by name alone.
- Races have one record per faction (Dracthyr, Earthen, Haranir) and Pandaren
  has unselectable faction copies; merge by name and keep `is_selectable`.
  Race records also list placeholder classes absent from the class index.
- Talent trees list every hero tree of the class; keep only the spec's own.
  Some choice nodes come back without options; they are flagged, not dropped.
- The CLI shares one flag set: don't define a flag name twice (it panics at
  startup, and `go vet` won't catch it).
- Output schemas are generated from Go types and must not be recursive (a
  type can't contain itself, even through a pointer); the server panics at
  startup otherwise. `task mcp:list` or the tests catch it.
- The SDK validates structured output against its schema on every call, and
  does not enforce required prompt arguments; prompts check them by hand.
- The npm launcher must keep stdio inherited and pass signals and exit codes
  through; MCP clients talk to the Go binary over stdin/stdout. Test packaging
  with `task npm:pack` and a global install into a temporary `--prefix`.
- Release filenames (`dist/wowapi-<version>-<goos>-<goarch>`) are parsed by
  `npm/build.mjs` and `mcpb/build.mjs`; keep them in step if `task release` changes.
- Read configuration with `blizzard.Env`, not `os.Getenv`: MCP bundle clients
  can pass unfilled settings through literally (`${user_config.realm}`), and
  `Env` treats those as unset.
- In Git Bash, `MSYS_NO_PATHCONV=1` is needed for `wowapi get /data/wow/...`.
