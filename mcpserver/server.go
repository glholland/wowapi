// Package mcpserver exposes the Battle.net World of Warcraft client as a Model
// Context Protocol server: read-only tools (with typed structured output where
// the data is condensed), resources for character and game data, and prompts
// for common questions.
package mcpserver

import (
	"context"

	"github.com/glholland/wowapi/blizzard"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// New builds the MCP server with every tool, resource and prompt the
// client's game can serve (see blizzard.Feature): a game whose API lacks
// character lookups gets no character tools, and so on.
func New(c *blizzard.Client, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "wowapi",
		Title:   c.Game.Title() + " (Battle.net API)",
		Version: version,
	}, &mcp.ServerOptions{Instructions: instructions(c)})
	addTools(s, c)
	addResources(s, c)
	addPrompts(s, c.Game)
	return s
}

func instructions(c *blizzard.Client) string {
	if c.Game == blizzard.Forever {
		return "Read-only access to Blizzard's " + c.Game.Title() + " API for region " + c.Region + " (namespaces *-forever-" + c.Region + "). " +
			"Forever characters have a first name and a surname and no realm: pass name='Celdis Witdasticc' (realm is ignored; WOW_CHARACTER sets a default). " +
			"A character appears in the API after it has logged in, and data reflects the last logout. " +
			"Blizzard's API has no Mythic+ or Auction House data for this game yet, so no Mythic+ or price tools are offered. " +
			"Use wow_races, wow_classes and wow_talents (one talent tree per specialization) to plan a character; the new_character prompt walks through it. " +
			"Use wow_item_search to turn item names into IDs before calling wow_item. " +
			"wow_api_get reaches any other endpoint, in this game's namespaces or any full namespace. " +
			"Resources under wow:// expose the same data for clients that attach context directly."
	}
	if !c.Game.Supports(blizzard.FeatureCharacters) {
		return "Read-only access to Blizzard's " + c.Game.Title() + " API for region " + c.Region + " (namespaces *-" + string(c.Game) + "-" + c.Region + "). " +
			"Blizzard's API does not serve this game's character profiles, Mythic+ or Auction House data yet, so no character, Mythic+ or price tools are offered. " +
			"Use wow_races, wow_classes and wow_talents (one talent tree per specialization) to plan a new character; the new_character prompt walks through it. " +
			"Use wow_item_search to turn item names into IDs before calling wow_item. " +
			"wow_api_get reaches any other endpoint, in this game's namespaces or any full namespace. " +
			"Resources under wow:// expose the same data for clients that attach context directly."
	}
	return "Read-only access to Blizzard's official World of Warcraft API for region " + c.Region + ". " +
		"Character data reflects the character's last logout, not live in-game state. " +
		"Character names need their exact spelling, accents included. " +
		"Use wow_item_search to turn item names into IDs before calling wow_item or wow_commodity_price. " +
		"For new-character planning use wow_classes, wow_races and wow_talents; the new_character prompt walks through it. " +
		"Resources under wow:// expose the same data for clients that attach context directly."
}

// Run serves MCP over stdio until the context is cancelled or stdin closes.
func Run(ctx context.Context, c *blizzard.Client, version string) error {
	return New(c, version).Run(ctx, &mcp.StdioTransport{})
}

// defaultCharacter fills realm and name from WOW_REALM / WOW_CHARACTER.
// WoW: Forever has no realms, so the realm is dropped and the name is the
// "First Surname" pair.
func defaultCharacter(g blizzard.Game, realm, name string) (string, string) {
	if g == blizzard.Forever {
		if name == "" {
			name = blizzard.Env("WOW_CHARACTER")
		}
		return "", name
	}
	if realm == "" {
		realm = blizzard.Env("WOW_REALM")
	}
	if name == "" {
		name = blizzard.Env("WOW_CHARACTER")
	}
	return realm, name
}
