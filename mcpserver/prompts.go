package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/glholland/wowapi/blizzard"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A prompt is a reusable, parameterized starting message that tells the model
// which tools to use for a common question.
type promptDef struct {
	prompt *mcp.Prompt
	build  func(args map[string]string) string
}

// promptsFor returns the prompts a game can serve. The character prompts need
// character lookups, which not every game's API provides.
func promptsFor(g blizzard.Game) []promptDef {
	defs := []promptDef{newCharacterPrompt(g)}
	if g.Supports(blizzard.FeatureCharacters) {
		defs = append(defs, characterPrompts...)
	}
	return defs
}

// newCharacterPrompt plans a new character. Retail has hero talent trees and
// Mythic+; original-style games like WoW: Forever have neither, so their
// prompt leaves them out.
func newCharacterPrompt(g blizzard.Game) promptDef {
	if g.Supports(blizzard.FeatureTalentExtras) {
		return retailNewCharacter()
	}
	return originalStyleNewCharacter(g)
}

func retailNewCharacter() promptDef {
	return promptDef{
		prompt: &mcp.Prompt{
			Name:        "new_character",
			Title:       "Plan a new character",
			Description: "Recommend a race, class, specialization, hero talent tree and starting talents for a new character.",
			Arguments: []*mcp.PromptArgument{
				{Name: "goal", Description: "What the player wants, e.g. 'tank for Mythic+', 'solo questing', 'easy healer'."},
				{Name: "faction", Description: "Alliance, Horde, or leave empty for either."},
				{Name: "preferences", Description: "Anything else: melee or ranged, favorite races, playstyle, experience level."},
			},
		},
		build: func(a map[string]string) string {
			var b strings.Builder
			b.WriteString("Help me plan a new World of Warcraft character.\n\n")
			fmt.Fprintf(&b, "Goal: %s\n", orDefault(a["goal"], "not decided; suggest a few good options"))
			fmt.Fprintf(&b, "Faction: %s\n", orDefault(a["faction"], "either"))
			fmt.Fprintf(&b, "Preferences: %s\n\n", orDefault(a["preferences"], "none given"))
			b.WriteString(`Use the wowapi tools rather than memory, since game data changes every patch:
1. wow_classes (no name) to shortlist specs whose role and primary stat fit the goal.
2. wow_classes with a class name for each shortlisted class: spec descriptions, hero talent trees, resources.
3. wow_races (no name) for races that can play those classes on the chosen faction, then wow_races with a race name to compare racial abilities. Note allied races need unlocking.
4. wow_talents for the recommended spec, one section at a time (spec, then hero), to explain the core abilities and which hero tree suits the goal.

Finish with: the recommended race, class, specialization and hero tree; why they fit; one or two alternatives; and the first talents to prioritize. Say where a choice is personal taste rather than power.`)
			return b.String()
		},
	}
}

// originalStyleNewCharacter is new_character for games with one talent tree
// per specialization, no hero talents and no Mythic+ (WoW: Forever).
func originalStyleNewCharacter(g blizzard.Game) promptDef {
	return promptDef{
		prompt: &mcp.Prompt{
			Name:        "new_character",
			Title:       "Plan a new character",
			Description: "Recommend a race, class, specialization and starting talents for a new " + g.Title() + " character.",
			Arguments: []*mcp.PromptArgument{
				{Name: "goal", Description: "What the player wants, e.g. 'tank for dungeons', 'solo questing', 'easy healer'."},
				{Name: "faction", Description: "Alliance, Horde, or leave empty for either."},
				{Name: "preferences", Description: "Anything else: melee or ranged, favorite races, playstyle, experience level."},
			},
		},
		build: func(a map[string]string) string {
			var b strings.Builder
			fmt.Fprintf(&b, "Help me plan a new %s character.\n\n", g.Title())
			fmt.Fprintf(&b, "Goal: %s\n", orDefault(a["goal"], "not decided; suggest a few good options"))
			fmt.Fprintf(&b, "Faction: %s\n", orDefault(a["faction"], "either"))
			fmt.Fprintf(&b, "Preferences: %s\n\n", orDefault(a["preferences"], "none given"))
			b.WriteString(`Use the wowapi tools rather than memory, since this game's data is new and may still change:
1. wow_classes (no name) to list the classes and their specializations. This game's API has no role or primary stat data, so judge those from the class and specialization names.
2. wow_classes with a class name for each class that fits the goal: its resource and which races can play it.
3. wow_races (no name) for races that can play those classes on the chosen faction, then wow_races with a race name to compare racial abilities. Races can differ by faction in which classes they offer (the new Skyborne come in an Alliance and a Horde version).
4. wow_talents for the recommended specialization, to explain its original-style talent tree and which talents to take first.

This game's talents are original-style, with one tree per specialization. Finish with: the recommended race, class and specialization; why they fit; one or two alternatives; and the first talents to prioritize. Say where a choice is personal taste rather than power.`)
			return b.String()
		},
	}
}

// characterPrompts need character lookups (FeatureCharacters).
var characterPrompts = []promptDef{
	{
		prompt: &mcp.Prompt{
			Name:        "character_review",
			Title:       "Review a character",
			Description: "Review a character's gear, progress and professions and suggest next steps.",
			Arguments: []*mcp.PromptArgument{
				{Name: "realm", Description: "Realm name. Defaults to WOW_REALM."},
				{Name: "name", Description: "Character name, exact spelling including accents. Defaults to WOW_CHARACTER."},
			},
		},
		build: func(a map[string]string) string {
			who := "my default character (WOW_REALM / WOW_CHARACTER)"
			if a["name"] != "" {
				who = fmt.Sprintf("%s on %s", a["name"], orDefault(a["realm"], "my default realm"))
			}
			return fmt.Sprintf(`Review %s using the wowapi tools:
1. wow_character sections summary, equipment and stats: item level, weakest slots, missing enchants and empty sockets.
2. wow_character_encounters for the current expansion, and wow_mythic_plus for the current season.
3. wow_character section professions, then wow_profession_recipes (with sources=true) for their main professions.

Summarize where they stand, then list the most valuable next steps in order. Data reflects the character's last logout.`, who)
		},
	},
	{
		prompt: &mcp.Prompt{
			Name:        "profession_plan",
			Title:       "Plan profession progress",
			Description: "Plan how to level a profession and collect its missing recipes, with Auction House prices.",
			Arguments: []*mcp.PromptArgument{
				{Name: "profession", Description: "Profession name, e.g. Tailoring.", Required: true},
				{Name: "realm", Description: "Realm name. Defaults to WOW_REALM."},
				{Name: "name", Description: "Character name. Defaults to WOW_CHARACTER."},
			},
		},
		build: func(a map[string]string) string {
			return fmt.Sprintf(`Plan my %s progress. Use wow_profession_recipes with profession=%q and sources=true%s.
Group the missing recipes by how to get them: boss drops (which dungeon or raid), buyable on the Auction House (with price), bind-on-pickup items, and trainer or specialization unlocks.
Use wow_recipe and wow_commodity_price to check what the most useful missing recipes cost to craft.
End with a prioritized plan. Say clearly when Blizzard's API can't tell where a recipe comes from.`,
				a["profession"], a["profession"], characterClause(a))
		},
	},
}

func addPrompts(s *mcp.Server, g blizzard.Game) {
	for _, p := range promptsFor(g) {
		build := p.build
		s.AddPrompt(p.prompt, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			// The SDK does not enforce required arguments itself.
			for _, arg := range p.prompt.Arguments {
				if arg.Required && strings.TrimSpace(req.Params.Arguments[arg.Name]) == "" {
					return nil, fmt.Errorf("prompt %s: argument %q is required", p.prompt.Name, arg.Name)
				}
			}
			return &mcp.GetPromptResult{
				Description: p.prompt.Description,
				Messages: []*mcp.PromptMessage{{
					Role:    "user",
					Content: &mcp.TextContent{Text: build(req.Params.Arguments)},
				}},
			}, nil
		})
	}
}

func characterClause(a map[string]string) string {
	if a["name"] == "" {
		return ""
	}
	return fmt.Sprintf(" for %s on %s", a["name"], orDefault(a["realm"], "the default realm"))
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
