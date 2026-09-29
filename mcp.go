package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/glholland/wowapi/blizzard"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type characterArgs struct {
	Realm   string `json:"realm,omitempty" jsonschema:"Realm name or slug, e.g. 'Area 52'. Defaults to WOW_REALM."`
	Name    string `json:"name,omitempty" jsonschema:"Character name. Defaults to WOW_CHARACTER."`
	Section string `json:"section,omitempty" jsonschema:"Which part of the profile to fetch. Default 'summary'."`
}

type encounterArgs struct {
	Realm     string `json:"realm,omitempty" jsonschema:"Realm name or slug. Defaults to WOW_REALM."`
	Name      string `json:"name,omitempty" jsonschema:"Character name, exact spelling including accents. Defaults to WOW_CHARACTER."`
	Kind      string `json:"kind,omitempty" jsonschema:"dungeons (default) or raids."`
	Expansion string `json:"expansion,omitempty" jsonschema:"Only expansions whose name contains this, e.g. 'midnight' or 'current season'. Setting it adds per-boss kill counts and last-kill dates."`
}

type mythicArgs struct {
	Realm    string `json:"realm,omitempty" jsonschema:"Realm name or slug. Defaults to WOW_REALM."`
	Name     string `json:"name,omitempty" jsonschema:"Character name, exact spelling including accents. Defaults to WOW_CHARACTER."`
	SeasonID int    `json:"season_id,omitempty" jsonschema:"Mythic+ season ID. Omit for the current season (also returns this week's best runs)."`
}

type recipeProgressArgs struct {
	Realm        string `json:"realm,omitempty" jsonschema:"Realm name or slug. Defaults to WOW_REALM."`
	Name         string `json:"name,omitempty" jsonschema:"Character name, exact spelling including accents. Defaults to WOW_CHARACTER."`
	Profession   string `json:"profession" jsonschema:"Profession name (e.g. 'Tailoring') or ID (e.g. 197)."`
	SkillTierID  int    `json:"skill_tier_id,omitempty" jsonschema:"Skill tier ID, e.g. 2918 for Midnight Tailoring. Omit for the newest tier the character has learned."`
	IncludeKnown bool   `json:"include_known,omitempty" jsonschema:"Also list the known recipes by category. Default false (missing recipes only)."`
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

func runMCP(ctx context.Context, c *blizzard.Client) error {
	s := mcp.NewServer(&mcp.Implementation{Name: "wowapi", Title: "World of Warcraft (Battle.net API)", Version: version}, &mcp.ServerOptions{
		Instructions: "Read-only access to Blizzard's official World of Warcraft API for region " + c.Region + ". " +
			"Character data reflects the character's last logout, not live in-game state. " +
			"Use wow_item_search to turn item names into IDs before calling wow_item or wow_commodity_price.",
	})
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(true)}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_character",
		Description: "Get a character's profile from the Battle.net API. Sections: " + strings.Join(blizzard.SectionNames(), ", ") + ". Data updates when the character logs out.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a characterArgs) (*mcp.CallToolResult, any, error) {
		realm, name := firstNonEmpty(a.Realm, os.Getenv("WOW_REALM")), firstNonEmpty(a.Name, os.Getenv("WOW_CHARACTER"))
		return rawResult(c.Character(ctx, realm, name, a.Section))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_character_encounters",
		Description: "A character's dungeon or raid progress: per instance and difficulty, bosses killed out of total. Without an expansion filter it covers every expansion but omits individual bosses; filter (e.g. 'midnight') for boss kill counts and last-kill dates.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a encounterArgs) (*mcp.CallToolResult, any, error) {
		realm, name := firstNonEmpty(a.Realm, os.Getenv("WOW_REALM")), firstNonEmpty(a.Name, os.Getenv("WOW_CHARACTER"))
		return jsonResult(c.Encounters(ctx, realm, name, a.Kind, a.Expansion))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_mythic_plus",
		Description: "A character's Mythic+ season: season name and dates, overall rating, and best runs (dungeon, key level, timed or not, duration, rating, affixes, party). For the current season it also includes this week's best runs. A character with no runs returns an empty list with a note.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a mythicArgs) (*mcp.CallToolResult, any, error) {
		realm, name := firstNonEmpty(a.Realm, os.Getenv("WOW_REALM")), firstNonEmpty(a.Name, os.Getenv("WOW_CHARACTER"))
		return jsonResult(c.MythicKeystoneSeason(ctx, realm, name, a.SeasonID))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_profession_recipes",
		Description: "Compare the recipes a character knows with every recipe in a profession skill tier (defaults to the newest tier they have, e.g. Midnight Tailoring). Returns skill level, known/total counts and the missing recipes grouped by category. Use wow_recipe for a recipe's reagents.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a recipeProgressArgs) (*mcp.CallToolResult, any, error) {
		realm, name := firstNonEmpty(a.Realm, os.Getenv("WOW_REALM")), firstNonEmpty(a.Name, os.Getenv("WOW_CHARACTER"))
		return jsonResult(c.ProfessionRecipes(ctx, realm, name, a.Profession, a.SkillTierID, a.IncludeKnown))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_item_search",
		Description: "Search items by name. Returns IDs, names, quality and item level, newest items first.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a itemSearchArgs) (*mcp.CallToolResult, any, error) {
		hits, err := c.SearchItems(ctx, a.Name, a.Limit)
		return jsonResult(hits, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_item",
		Description: "Get full item details by ID: stats, binding, required level, sell price, spells.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a idArgs) (*mcp.CallToolResult, any, error) {
		return rawResult(c.Item(ctx, a.ID))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_profession",
		Description: "Browse professions: no ID lists all professions; an ID lists that profession's skill tiers (one per expansion); an ID plus skill_tier_id lists its recipes.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a professionArgs) (*mcp.CallToolResult, any, error) {
		return rawResult(c.Profession(ctx, a.ID, a.SkillTierID))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_recipe",
		Description: "Get a recipe by ID: reagents and quantities, and the item it crafts.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a idArgs) (*mcp.CallToolResult, any, error) {
		return rawResult(c.Recipe(ctx, a.ID))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_commodity_price",
		Description: "Current region-wide Auction House price for a commodity: cheapest unit price, total quantity listed, and the cheapest price levels. The first call may take a while (the snapshot is large); it is then cached for 30 minutes.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a priceArgs) (*mcp.CallToolResult, any, error) {
		p, err := c.Commodity(ctx, a.ItemID)
		return jsonResult(p, err)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wow_api_get",
		Description: "Call any read-only WoW Game Data or Profile API endpoint directly, for anything the other tools don't cover (talent trees, specializations, mounts, realms, WoW Token price, mythic keystone data, ...).",
		Annotations: readOnly,
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

	return s.Run(ctx, &mcp.StdioTransport{})
}

func rawResult(body []byte, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(blizzard.Slim(body))}}}, nil, nil
}

func jsonResult(v any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
