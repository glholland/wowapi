# wowapi

A command-line client for Blizzard's World of Warcraft API that doubles as an
MCP server, so Claude (or any MCP-capable assistant) can look up your
character, items, recipes and Auction House prices directly.

## Getting Started

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

Tools (all read-only):

| Tool | What it answers |
|---|---|
| `wow_character` | Any profile section: summary, equipment, stats, professions, reputations, achievements, ... |
| `wow_character_encounters` | Dungeon or raid progress per difficulty; filter by expansion for boss kills |
| `wow_mythic_plus` | Mythic+ rating and best runs for a season, plus this week's runs |
| `wow_profession_recipes` | Recipes a character knows vs. is missing in a profession tier; with `sources`, where to get each one |
| `wow_races` | Playable races: factions, allied race, classes, racial abilities |
| `wow_classes` | Classes and specs: role, primary stat, description, hero trees, PvP talents |
| `wow_talents` | A spec's class, spec, hero and PvP talents with descriptions and choice options |
| `wow_item_search`, `wow_item` | Find items by name; item details |
| `wow_profession`, `wow_recipe` | Profession tiers and recipe lists; a recipe's reagents |
| `wow_commodity_price` | Region-wide Auction House price for stackable goods |
| `wow_api_get` | Any other `/data/wow/` or `/profile/wow/` endpoint |

## Development

| Task | What it does |
|---|---|
| `task build` | Build `wowapi` / `wowapi.exe` |
| `task test` | Run the tests |
| `task check` | gofmt check, `go vet`, tests — run before committing |
| `task fmt` | Format the code |
| `task install` | `go install` into your Go bin directory |
| `task clean` | Remove build output |

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
