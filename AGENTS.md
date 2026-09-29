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
| `.goreleaser.yaml` | GoReleaser: builds, bare-binary archives, per-platform MCP bundle hooks, checksums, GitHub release |
| `npm/build.mjs` | Builds the npm launcher + per-platform packages from GoReleaser's `dist/artifacts.json` |
| `npm/wowapi/bin/wowapi.js` | npm launcher: finds the platform binary and runs it with inherited stdio |
| `npm/publish.sh` | Publishes the npm packages (platform packages first; skips existing versions) |
| `mcpb/build.mjs` | Claude Desktop bundles: `--prepare` (GoReleaser before hook: tool list from `wowapi mcp -list -json`, mcpb CLI) and one bundle per build (post hook) into `build/mcpb/` |
| `.github/workflows/` | `ci.yml` (PR/push checks on 3 OSes), `pr-title.yml`, `release-please.yml` (release PRs), `release.yml` (build + publish; tag push or called by release-please), `vuln.yml` (weekly) |
| `release-please-config.json`, `.release-please-manifest.json` | Versioning and changelog config; the manifest holds the current version |

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
- PR titles must be Conventional Commits (`feat:`, `fix:`, `refactor:`, ...):
  PRs are squash-merged and release-please reads the titles. Don't edit
  `CHANGELOG.md`, the version in `main.go` (marked `x-release-please-version`)
  or `.release-please-manifest.json` by hand; the release PR does that.
- The npm launcher must keep stdio inherited and pass signals and exit codes
  through; MCP clients talk to the Go binary over stdin/stdout. Test packaging
  with `task npm:pack` and a global install into a temporary `--prefix`.
- Releases go through GoReleaser (`task release` locally, snapshot only).
  `npm/build.mjs` finds binaries via `dist/artifacts.json`, and the MCP bundle
  hooks write to `build/mcpb/`, which the checksum and release sections of
  `.goreleaser.yaml` pick up; keep those paths in step. npm publishing stays
  outside GoReleaser (its npm support is Pro-only and uses a postinstall
  download instead of per-platform packages).
- npm platform packages are scoped (`@gholland/wowapi-<os>-<cpu>`); only the
  launcher `wowapi` is unscoped. Several unscoped look-alike names published
  quickly by a new account trip npm's spam detection.
- npm publishing uses trusted publishing (OIDC), not a token. npm checks the
  *calling* workflow's filename, so each package trusts both
  `release-please.yml` and `release.yml`, and both need `id-token: write`.
  Renaming either workflow breaks publishing until npm's settings are updated.
  It only works from a public repo; there is deliberately no token fallback.
- Read configuration with `blizzard.Env`, not `os.Getenv`: MCP bundle clients
  can pass unfilled settings through literally (`${user_config.realm}`), and
  `Env` treats those as unset.
- In Git Bash, `MSYS_NO_PATHCONV=1` is needed for `wowapi get /data/wow/...`.
