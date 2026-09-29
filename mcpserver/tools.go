package mcpserver

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/glholland/wowapi/blizzard"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---- Input types (the SDK derives each tool's input schema from these) -----

type characterRef struct {
	Realm string `json:"realm,omitempty" jsonschema:"Realm name or slug, e.g. 'Area 52'. Defaults to WOW_REALM."`
	Name  string `json:"name,omitempty" jsonschema:"Character name, exact spelling including accents. Defaults to WOW_CHARACTER."`
}

type characterArgs struct {
	characterRef
	Section string `json:"section,omitempty" jsonschema:"Which part of the profile to fetch. Default 'summary'."`
}

type encounterArgs struct {
	characterRef
	Kind      string `json:"kind,omitempty" jsonschema:"dungeons (default) or raids."`
	Expansion string `json:"expansion,omitempty" jsonschema:"Only expansions whose name contains this, e.g. 'midnight' or 'current season'. Setting it adds per-boss kill counts and last-kill dates."`
}

type mythicArgs struct {
	characterRef
	SeasonID int `json:"season_id,omitempty" jsonschema:"Mythic+ season ID. Omit for the current season (also returns this week's best runs)."`
}

type recipeProgressArgs struct {
	characterRef
	Profession   string `json:"profession" jsonschema:"Profession name (e.g. 'Tailoring') or ID (e.g. 197)."`
	SkillTierID  int    `json:"skill_tier_id,omitempty" jsonschema:"Skill tier ID, e.g. 2918 for Midnight Tailoring. Omit for the newest tier the character has learned."`
	IncludeKnown bool   `json:"include_known,omitempty" jsonschema:"Also list the known recipes by category. Default false (missing recipes only)."`
	Sources      bool   `json:"sources,omitempty" jsonschema:"Explain where each missing recipe comes from: boss drops (dungeon/raid loot tables), the item that teaches it, whether that item is tradable, and its price on the character's realm Auction House. The first call per expansion takes a few seconds."`
}

type nameArgs struct {
	Name string `json:"name,omitempty" jsonschema:"Optional. Omit to list all; give one name for full detail."`
}

type talentArgs struct {
	Class          string `json:"class,omitempty" jsonschema:"Class name, e.g. 'Mage' or 'Death Knight'. Needed when the spec name is shared (Frost, Holy, Restoration, Protection)."`
	Specialization string `json:"specialization" jsonschema:"Specialization name (e.g. 'Arcane') or ID (e.g. 62)."`
	Section        string `json:"section,omitempty" jsonschema:"Limit to one part: class, spec, hero or pvp. Omit for everything (large: ~100 talents)."`
	HeroTree       string `json:"hero_tree,omitempty" jsonschema:"Only hero trees whose name contains this, e.g. 'Sunfury'."`
}

type itemSearchArgs struct {
	Name  string `json:"name" jsonschema:"Text the item name contains, e.g. 'Arcanoweave'."`
	Limit int    `json:"limit,omitempty" jsonschema:"Max results (1-100, default 25)."`
}

type idArgs struct {
	ID int `json:"id" jsonschema:"Numeric ID."`
}

type professionArgs struct {
	ID          int `json:"id,omitempty" jsonschema:"Profession ID (e.g. Tailoring is 197). Omit to list all professions."`
	SkillTierID int `json:"skill_tier_id,omitempty" jsonschema:"Skill tier ID (an expansion's version of the profession, e.g. Midnight Tailoring). Returns its recipe categories and recipes."`
}

type priceArgs struct {
	ItemID int `json:"item_id" jsonschema:"Item ID of a commodity (cloth, herbs, ore, reagents, consumables)."`
}

type apiGetArgs struct {
	Path      string            `json:"path" jsonschema:"API path beginning with /data/wow/ or /profile/wow/, e.g. /data/wow/playable-specialization/64."`
	Namespace string            `json:"namespace" jsonschema:"static (patch data), dynamic (auctions, realms, token) or profile (characters)."`
	Query     map[string]string `json:"query,omitempty" jsonschema:"Extra query parameters."`
}

// ---- Output wrappers: structured content must be a JSON object -------------

// RaceList is the structured output of wow_races.
type RaceList struct {
	Races []blizzard.Race `json:"races"`
}

// ClassList is the structured output of wow_classes.
type ClassList struct {
	Classes []blizzard.Class `json:"classes"`
}

// ItemSearchResult is the structured output of wow_item_search.
type ItemSearchResult struct {
	Items []blizzard.ItemHit `json:"items"`
}

func annotations(title string) *mcp.ToolAnnotations {
	openWorld := true
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &openWorld}
}

// addTools registers every tool. Tools returning condensed Go types are typed
// (the SDK publishes an outputSchema and fills structuredContent); tools that
// pass Blizzard's JSON through return it as text, since its shape varies.
func addTools(s *mcp.Server, c *blizzard.Client) {
	// Character data.
	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_character",
		Description: "Get a character's profile from the Battle.net API. Sections: " + strings.Join(blizzard.SectionNames(), ", ") + ". Data updates when the character logs out.",
		Annotations: annotations("Character profile"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a characterArgs) (*mcp.CallToolResult, any, error) {
		realm, name := defaultCharacter(a.Realm, a.Name)
		return rawResult(c.Character(ctx, realm, name, a.Section))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_character_encounters",
		Description: "A character's dungeon or raid progress: per instance and difficulty, bosses killed out of total. Without an expansion filter it covers every expansion but omits individual bosses; filter (e.g. 'midnight') for boss kill counts and last-kill dates.",
		Annotations: annotations("Dungeon and raid progress"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a encounterArgs) (*mcp.CallToolResult, blizzard.EncounterProgress, error) {
		realm, name := defaultCharacter(a.Realm, a.Name)
		out, err := c.Encounters(ctx, realm, name, a.Kind, a.Expansion)
		return nil, out, err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_mythic_plus",
		Description: "A character's Mythic+ season: season name and dates, overall rating, and best runs (dungeon, key level, timed or not, duration, rating, affixes, party). For the current season it also includes this week's best runs. A character with no runs returns an empty list with a note.",
		Annotations: annotations("Mythic+ season"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a mythicArgs) (*mcp.CallToolResult, blizzard.MythicSeason, error) {
		realm, name := defaultCharacter(a.Realm, a.Name)
		out, err := c.MythicKeystoneSeason(ctx, realm, name, a.SeasonID)
		return nil, out, err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "wow_profession_recipes",
		Description: "Compare the recipes a character knows with every recipe in a profession skill tier (defaults to the newest tier they have, e.g. Midnight Tailoring). " +
			"Returns skill level, known/total counts and the missing recipes grouped by category. " +
			"Set sources=true to learn where each missing recipe comes from (boss drop, tradable or bind-on-pickup recipe item with AH price, or trainer/specialization); Blizzard's API has no vendor or quest data. Use wow_recipe for a recipe's reagents.",
		Annotations: annotations("Profession recipes"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a recipeProgressArgs) (*mcp.CallToolResult, blizzard.RecipeProgress, error) {
		realm, name := defaultCharacter(a.Realm, a.Name)
		out, err := c.ProfessionRecipes(ctx, realm, name, a.Profession, blizzard.RecipeOptions{TierID: a.SkillTierID, IncludeKnown: a.IncludeKnown, Sources: a.Sources})
		return nil, out, err
	})

	// Character creation data.
	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_races",
		Description: "Playable races for character creation: factions (Alliance/Horde/Neutral), whether it is an allied race (requires unlocking), the classes it can play, and racial abilities. Give a race name to get each racial ability's description.",
		Annotations: annotations("Playable races"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a nameArgs) (*mcp.CallToolResult, RaceList, error) {
		races, err := c.Races(ctx, a.Name)
		return nil, RaceList{Races: races}, err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_classes",
		Description: "Playable classes and their specializations with role (tank/healer/damage) and primary stat. Give a class name for spec descriptions, hero talent trees, PvP talents, resources and which races can play it. Use wow_talents for a spec's full talent tree.",
		Annotations: annotations("Classes and specializations"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a nameArgs) (*mcp.CallToolResult, ClassList, error) {
		classes, err := c.Classes(ctx, a.Name)
		return nil, ClassList{Classes: classes}, err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_talents",
		Description: "A specialization's talent tree: class talents, spec talents, its hero talent trees and PvP talents, each with description, cast/cooldown/cost, max rank, choice-node options, prerequisites and point gates. Use section and hero_tree to keep responses small. Tooltip numbers are base values, not scaled to a character.",
		Annotations: annotations("Talent tree"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a talentArgs) (*mcp.CallToolResult, blizzard.TalentTree, error) {
		out, err := c.Talents(ctx, a.Class, a.Specialization, a.Section, a.HeroTree)
		return nil, out, err
	})

	// Items, professions and prices.
	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_item_search",
		Description: "Search items by name. Returns IDs, names, quality and item level, newest items first.",
		Annotations: annotations("Item search"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a itemSearchArgs) (*mcp.CallToolResult, ItemSearchResult, error) {
		hits, err := c.SearchItems(ctx, a.Name, a.Limit)
		return nil, ItemSearchResult{Items: hits}, err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_item",
		Description: "Get full item details by ID: stats, binding, required level, sell price, spells.",
		Annotations: annotations("Item details"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a idArgs) (*mcp.CallToolResult, any, error) {
		return rawResult(c.Item(ctx, a.ID))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_profession",
		Description: "Browse professions: no ID lists all professions; an ID lists that profession's skill tiers (one per expansion); an ID plus skill_tier_id lists its recipes.",
		Annotations: annotations("Professions"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a professionArgs) (*mcp.CallToolResult, any, error) {
		return rawResult(c.Profession(ctx, a.ID, a.SkillTierID))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_recipe",
		Description: "Get a recipe by ID: reagents and quantities, and the item it crafts.",
		Annotations: annotations("Recipe"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a idArgs) (*mcp.CallToolResult, any, error) {
		return rawResult(c.Recipe(ctx, a.ID))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_commodity_price",
		Description: "Current region-wide Auction House price for a commodity: cheapest unit price, total quantity listed, and the cheapest price levels. The first call may take a while (the snapshot is large); it is then cached for 30 minutes.",
		Annotations: annotations("Commodity price"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a priceArgs) (*mcp.CallToolResult, blizzard.CommodityPrice, error) {
		out, err := c.Commodity(ctx, a.ItemID)
		return nil, out, err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_api_get",
		Description: "Call any read-only WoW Game Data or Profile API endpoint directly, for anything the other tools don't cover (talent trees, mounts, realms, WoW Token price, mythic keystone data, ...).",
		Annotations: annotations("Raw API request"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a apiGetArgs) (*mcp.CallToolResult, any, error) {
		if !strings.HasPrefix(a.Path, "/data/wow/") && !strings.HasPrefix(a.Path, "/profile/wow/") {
			return nil, nil, fmt.Errorf("path must start with /data/wow/ or /profile/wow/")
		}
		switch a.Namespace {
		case blizzard.Static, blizzard.Dynamic, blizzard.Profile:
		default:
			return nil, nil, fmt.Errorf("namespace must be static, dynamic or profile")
		}
		q := url.Values{}
		for k, v := range a.Query {
			q.Set(k, v)
		}
		return rawResult(c.Get(ctx, a.Path, a.Namespace, q))
	})
}

// rawResult returns an API response (with link noise removed) as text.
func rawResult(body []byte, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(blizzard.Slim(body))}}}, nil, nil
}
