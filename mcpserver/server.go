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

// New builds the MCP server with every tool, resource and prompt registered.
func New(c *blizzard.Client, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "wowapi",
		Title:   "World of Warcraft (Battle.net API)",
		Version: version,
	}, &mcp.ServerOptions{
		Instructions: "Read-only access to Blizzard's official World of Warcraft API for region " + c.Region + ". " +
			"Character data reflects the character's last logout, not live in-game state. " +
			"Character names need their exact spelling, accents included. " +
			"Use wow_item_search to turn item names into IDs before calling wow_item or wow_commodity_price. " +
			"For new-character planning use wow_classes, wow_races and wow_talents; the new_character prompt walks through it. " +
			"Resources under wow:// expose the same data for clients that attach context directly.",
	})
	addTools(s, c)
	addResources(s, c)
	addPrompts(s)
	return s
}

// Run serves MCP over stdio until the context is cancelled or stdin closes.
func Run(ctx context.Context, c *blizzard.Client, version string) error {
	return New(c, version).Run(ctx, &mcp.StdioTransport{})
}

// defaultCharacter fills realm and name from WOW_REALM / WOW_CHARACTER.
func defaultCharacter(realm, name string) (string, string) {
	if realm == "" {
		realm = blizzard.Env("WOW_REALM")
	}
	if name == "" {
		name = blizzard.Env("WOW_CHARACTER")
	}
	return realm, name
}
