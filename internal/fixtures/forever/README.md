# WoW: Forever test fixtures

Trimmed copies of real responses from Blizzard's `static-forever-us` namespace
(game version `1.60.1_70399`), captured on 2026-11-05, the day after launch.
They are public game data; no credentials or tokens are involved. Files are
named after the API path: `/data/wow/playable-race/95` is
`playable-race__95.json`.

What was trimmed, so the tests stay small:

| Fixture | Real response | Kept |
|---|---|---|
| `playable-race__index` | 10 races | Skyborne (95 High Order, Alliance; 96 Windshaper, Horde), Human, Orc |
| `playable-class__index` | 9 classes | Warrior, Shaman, Mage (details and all their specializations) |
| `talent-tree__1082__playable-specialization__375` | 16 spec talent nodes | the first 6 |
| `search__item` | 42 results for "linen" | the first 2 |
| `profession__197__skill-tier__2948` | 17 recipe categories | the first 2, with 3 recipes each |

Everything else is complete. `_links` was removed; the hrefs still show the
real versioned namespace (`static-1.60.1_70399-forever-us`).

Findings these responses encode (the tests rely on them):

- Skyborne is two races with different names, factions and class lists
  (Alliance gets Mage, Horde gets Shaman), so they are not merged.
- Specialization records have no `role`, `primary_stat_type`, hero talent
  trees or PvP talents.
- A talent tree has only `spec_talent_nodes`: no class nodes, hero trees or
  restriction lines.

`dynamic-forever-us` is a valid namespace that holds no data, so the fake server
answers 404 for every path in it. `profile-forever-us` serves one real character
(`profile__celdis-witdasticc*.json`: summary and professions with the first 3
recipes per profession; links removed) by `/profile/wow/character/first-surname`, with no realm in the path; everything else, including every
realm-based path, is 404. Namespaces
Blizzard doesn't recognize get 403.

To refresh a fixture, fetch the same path with the `Battlenet-Namespace:
static-forever-us` header (for example `wowapi get static-forever-us
/data/wow/playable-race/95`) and trim it the same way.
