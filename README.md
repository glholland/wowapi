# wowapi

A command-line client for Blizzard's World of Warcraft API that doubles as an
MCP server, so Claude (or any MCP-capable assistant) can look up your
character, items, recipes and Auction House prices directly.

## Install as an MCP server

Get Battle.net API credentials first: create a client at
<https://develop.battle.net/access/clients> (any name; the redirect URL can be
`http://localhost`) and copy the client ID and secret.

### Claude Desktop: one-click bundle

Download the `.mcpb` file for your machine from the latest
[release](https://github.com/glholland/wowapi/releases) and open it (or drag
it onto Claude Desktop's Settings → Extensions):

| Machine | File |
|---|---|
| Windows (most PCs) | `wowapi-<version>-win32-x64.mcpb` |
| Windows on ARM | `wowapi-<version>-win32-arm64.mcpb` |
| Mac with Apple silicon (M1 and later) | `wowapi-<version>-darwin-arm64.mcpb` |
| Intel Mac | `wowapi-<version>-darwin-x64.mcpb` |

Claude Desktop asks for your Battle.net client ID and secret (the secret is
stored securely) and, optionally, your region, realm and main character. No
Node.js or config files needed.

### With npm (any OS, needs Node.js 18+)

```sh
npm install -g wowapi        # or skip installing and use: npx -y wowapi
```

npm downloads only the prebuilt binary for your platform (Windows, macOS or
Linux on x64/arm64). Then register it with your MCP client.

**Claude Code:**

```sh
claude mcp add wowapi --scope user -e BLIZZARD_CLIENT_ID=... -e BLIZZARD_CLIENT_SECRET=... -e WOW_REALM=Lightbringer -e WOW_CHARACTER=Cëldis -- npx -y wowapi mcp
```

**Claude Desktop and other clients** (`claude_desktop_config.json` or equivalent):

```json
{
  "mcpServers": {
    "wowapi": {
      "command": "npx",
      "args": ["-y", "wowapi", "mcp"],
      "env": {
        "BLIZZARD_CLIENT_ID": "...",
        "BLIZZARD_CLIENT_SECRET": "...",
        "WOW_REGION": "us",
        "WOW_REALM": "Lightbringer",
        "WOW_CHARACTER": "Cëldis"
      }
    }
  }
}
```

With a global install you can use `"command": "wowapi", "args": ["mcp"]`
instead of `npx`.

### From a GitHub release (no Node.js)

Each [release](https://github.com/glholland/wowapi/releases) has a standalone
binary per platform plus `SHA256SUMS.txt`. Download the one for your machine,
put it somewhere permanent, and point your MCP client at it with the argument
`mcp`. With the GitHub CLI:

```sh
gh release download --repo glholland/wowapi --pattern "*windows-amd64.exe" --dir ~/bin
```

## Build from source

You'll need [Go](https://go.dev/dl/) 1.26+ and [Task](https://taskfile.dev/installation/)
(`winget install Task.Task`, `brew install go-task`, or `go install github.com/go-task/task/v3/cmd/task@latest`).

1. **Get API credentials.** Create a client at
   <https://develop.battle.net/access/clients>. Any name works; the redirect
   URL can be `http://localhost`. Copy the client ID and secret.

2. **Clone and set up.**

   ```sh
   git clone https://github.com/glholland/wowapi.git
   cd wowapi
   task setup
   ```

   `task setup` creates a `.env` file from `.env.example`. Open it and fill in
   `BLIZZARD_CLIENT_ID` and `BLIZZARD_CLIENT_SECRET`. Optionally set
   `WOW_REALM` and `WOW_CHARACTER` so you don't have to type them every time.

3. **Build and try it.**

   ```sh
   task build
   task character                           # your default character's summary
   task character -- -section equipment     # any section
   task run -- search arcanoweave           # find item IDs by name
   task price -- 240158                     # Auction House commodity price
   ```

4. **Connect it to Claude Code** (optional).

   ```sh
   task mcp:add
   ```

   This registers the built binary as a user-scoped MCP server using the
   values in your `.env`. Restart Claude Code and ask it about your character.

Run `task` on its own to see every available task.

## Configuration

Set these in `.env` or as real environment variables (which take precedence).

| Variable | Required | Meaning |
|---|---|---|
| `BLIZZARD_CLIENT_ID` / `BLIZZARD_CLIENT_SECRET` | yes | API credentials |
| `WOW_REGION` | no | `us` (default), `eu`, `kr`, `tw` |
| `WOW_LOCALE` | no | `en_US` (default) |
| `WOW_REALM` / `WOW_CHARACTER` | no | default character for lookups |

Character names need their exact spelling, accents included (`Cëldis`, not `Celdis`).

## CLI

```
wowapi character -section professions Lightbringer Cëldis
wowapi character -section specializations          # uses WOW_REALM / WOW_CHARACTER
wowapi search arcanoweave
wowapi item 19019
wowapi profession 197                              # Tailoring skill tiers
wowapi profession 197 <skill-tier-id>              # recipes for one expansion
wowapi recipe <id>
wowapi recipes tailoring                           # known vs. missing recipes (newest tier)
wowapi recipes -tier 2918 -known tailoring Lightbringer Cëldis
wowapi recipes -sources enchanting                 # ...plus where each missing recipe comes from
wowapi encounters                                  # dungeon progress, every expansion
wowapi encounters -kind raids -expansion midnight  # raid bosses, kill counts and dates
wowapi mythic                                      # Mythic+ rating and best runs, current season
wowapi mythic -season 17
wowapi races                                       # races: factions, allied, classes, racials
wowapi races Haranir                               # one race, racial abilities described
wowapi classes                                     # classes and specs with role and primary stat
wowapi classes Evoker                              # spec descriptions, hero trees, PvP talents, races
wowapi talents -section spec Arcane                # a spec's talents (class, spec, hero or pvp)
wowapi talents -section hero -hero sunfury Mage Arcane
wowapi talents Death Knight Frost                  # class first when the spec name is shared
wowapi price <item id>                             # AH commodity price
wowapi get static /data/wow/playable-specialization/64
wowapi get dynamic /data/wow/realm/lightbringer
```

Character sections: achievements, completed_quests, equipment, media,
mounts, mythic_keystone, pets, professions, pvp, quests, reputations,
specializations, stats, summary, titles.

Output strips the API's `_links`/`href` noise; pass `-raw` for the original.

## MCP server

`task mcp:add` does this for you. To register it by hand:

```
claude mcp add wowapi --scope user ^
  -e BLIZZARD_CLIENT_ID=... -e BLIZZARD_CLIENT_SECRET=... ^
  -e WOW_REGION=us -e WOW_REALM=Lightbringer -e WOW_CHARACTER=Cëldis ^
  -- C:\path\to\wowapi.exe mcp
```

The server is built on the official [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk).
Run `task mcp:list` to print everything it exposes (no credentials needed), or
`task mcp:inspect` to explore it in the MCP Inspector web UI (needs Node.js).

### Tools

All tools are read-only and annotated as such. **Structured** tools publish an
`outputSchema` and return typed `structuredContent` alongside the text, so
clients and models can rely on field names and types. The others pass
Blizzard's JSON through as text, since its shape varies by endpoint.

| Tool | Output | What it answers |
|---|---|---|
| `wow_character` | text | Any profile section: summary, equipment, stats, professions, reputations, achievements, ... |
| `wow_character_encounters` | structured | Dungeon or raid progress per difficulty; filter by expansion for boss kills |
| `wow_mythic_plus` | structured | Mythic+ rating and best runs for a season, plus this week's runs |
| `wow_profession_recipes` | structured | Recipes a character knows vs. is missing in a profession tier; with `sources`, where to get each one |
| `wow_races` | structured | Playable races: factions, allied race, classes, racial abilities |
| `wow_classes` | structured | Classes and specs: role, primary stat, description, hero trees, PvP talents |
| `wow_talents` | structured | A spec's class, spec, hero and PvP talents with descriptions and choice options |
| `wow_item_search` | structured | Find items by name |
| `wow_item` | text | Item details |
| `wow_profession`, `wow_recipe` | text | Profession tiers and recipe lists; a recipe's reagents |
| `wow_commodity_price` | structured | Region-wide Auction House price for stackable goods |
| `wow_api_get` | text | Any other `/data/wow/` or `/profile/wow/` endpoint |

### Resources

For clients that attach context directly. Names in URIs are percent-encoded
(`wow://character/Area%2052/C%C3%ABldis`).

| URI | Contents |
|---|---|
| `wow://races`, `wow://race/{race}` | All races, or one with racial abilities described |
| `wow://classes`, `wow://class/{class}` | All classes and specs, or one class in detail |
| `wow://talents/{class}/{spec}` | A spec's full talent tree |
| `wow://character/{realm}/{name}` | A character's profile summary |
| `wow://character/{realm}/{name}/{section}` | One profile section, e.g. `equipment` |

### Prompts

Ready-made starting points that tell the model which tools to use:

| Prompt | Arguments | Purpose |
|---|---|---|
| `new_character` | goal, faction, preferences | Recommend a race, class, spec, hero tree and first talents |
| `character_review` | realm, name | Review gear, progress and professions; suggest next steps |
| `profession_plan` | profession (required), realm, name | Level a profession and collect missing recipes, with AH prices |

## Development

| Task | What it does |
|---|---|
| `task build` | Build `wowapi` / `wowapi.exe`, stamped with the git version |
| `task release` | Cross-compile stripped binaries for Windows, Linux and macOS (amd64 + arm64) into `dist/` |
| `task version` | Show the version builds will carry and the built binary's |
| `task install` | `go install` into your Go bin directory |
| `task test` | Run the tests (no network or credentials needed) |
| `task check` | gofmt check, `go vet`, tests — run before committing |
| `task lint` | staticcheck |
| `task vuln` | govulncheck: known vulnerabilities in code and dependencies |
| `task cover` | Tests with per-function coverage |
| `task ci` | check + lint + vuln + release, in one go |
| `task fmt` | Format the code |
| `task tidy` | `go mod tidy` |
| `task deps:outdated` | List dependencies with newer versions |
| `task deps:update` | Update dependencies, then re-run `check` |
| `task mcp:list` | Print the MCP server's tools, resources and prompts |
| `task mcp:inspect` | Open the MCP Inspector against the built server |
| `task mcp:add` | Register the server with Claude Code using `.env` |
| `task clean` | Remove build output, `dist/` and coverage files |

| `task npm:pack` | Build the npm packages and tarballs into `npm/dist` (`NPM_VERSION=0.5.0`) |
| `task mcpb:pack` | Build and validate the Claude Desktop bundles into `dist/` (`MCPB_VERSION=0.5.0`) |

Linting and vulnerability tools run through `go run`, so there is nothing
extra to install. Builds are versioned from `git describe`, so tag releases
(`git tag v0.5.0`) to get clean version numbers.

### Releasing

Push a version tag and [the release workflow](.github/workflows/release.yml)
does the rest:

```sh
git tag v0.5.0
git push origin v0.5.0
```

It re-runs the checks, cross-compiles, builds a Claude Desktop bundle per
platform, creates a GitHub release with the binaries, bundles and checksums,
and publishes to npm: one package per platform
(`wowapi-win32-x64`, `wowapi-darwin-arm64`, ...) plus the `wowapi` launcher
that depends on them. A tag like `v0.6.0-rc.1` becomes a pre-release and goes
to npm's `next` tag. Publishing needs an `NPM_TOKEN` repository secret (an npm
automation or granular access token); without it the packages are built but
not published. [CI](.github/workflows/ci.yml) runs the same checks on every
push and pull request.

See [AGENTS.md](AGENTS.md) for notes aimed at AI coding assistants.

## Things to know

- Character data updates when you **log out**, not live.
- Commodity prices are region-wide and cover stackable goods only; the
  snapshot is large, so it's fetched at most every 30 minutes.
- Gear, bags and other non-stackable items are listed per realm. Get them from
  `/data/wow/connected-realm/{id}/auctions` (find the ID via
  `/data/wow/realm/{slug}`); the response is several megabytes.
- The API can't see your bags, bank or your own auctions.
- Recipe sources (`-sources`) come from dungeon/raid loot tables and the items
  that teach recipes, priced on your realm's Auction House. Blizzard's API has
  no vendor, trainer, quest or specialization data, so those recipes are
  labelled by elimination ("bind-on-pickup item" or "no item teaches this").
  The first lookup per expansion takes a few seconds; results are cached.
- In Git Bash, set `MSYS_NO_PATHCONV=1` before `wowapi get ...` so the API path
  isn't rewritten into a Windows path.
