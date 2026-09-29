// Command wowapi queries Blizzard's World of Warcraft API from the command line,
// or serves the same queries to an AI assistant as an MCP server over stdio.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/glholland/wowapi/blizzard"
	"github.com/glholland/wowapi/mcpserver"
)

// version is overridden at release build time with -ldflags "-X main.version=...".
var version = "0.5.0"

const usage = `wowapi - World of Warcraft Battle.net API client and MCP server

Usage:
  wowapi character [-section S] [realm] [name]   character profile (realm/name default to WOW_REALM/WOW_CHARACTER)
  wowapi encounters [-kind dungeons|raids] [-expansion E] [realm] [name]
                                                 dungeon or raid boss progress
  wowapi mythic [-season N] [realm] [name]       Mythic+ rating and best runs (default: current season)
  wowapi recipes [-tier N] [-known] [-sources] <profession> [realm] [name]
                                                 known vs. missing recipes for a profession tier;
                                                 -sources adds boss drops, recipe items and AH prices
  wowapi races [race]                            playable races (one race: racial abilities described)
  wowapi classes [class]                         classes and specs (one class: spec details, hero trees, races)
  wowapi talents [-section S] [-hero H] [class] <spec>
                                                 a spec's talent tree (section: class, spec, hero, pvp)
  wowapi search <item name>                      find item IDs by name
  wowapi item <id>                               item details
  wowapi profession [id [skill-tier-id]]         professions, skill tiers, recipes
  wowapi recipe <id>                             recipe details
  wowapi price <item id>                         Auction House commodity price
  wowapi get <static|dynamic|profile> <path> [key=value ...]
                                                 any API endpoint
  wowapi mcp [-list [-json]]                     run as an MCP server on stdio (-list: print its tools,
                                                 resources and prompts, then exit; no credentials needed)
  wowapi version

Flags for data commands:
  -raw    print the unmodified API response (default strips _links/href noise)

Environment:
  BLIZZARD_CLIENT_ID, BLIZZARD_CLIENT_SECRET   required (https://develop.battle.net/access/clients)
  WOW_REGION   us (default), eu, kr, tw
  WOW_LOCALE   en_US (default)
  WOW_REALM, WOW_CHARACTER   defaults for character lookups
`

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	case "version":
		fmt.Println(version)
		return nil
	}

	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	raw := fs.Bool("raw", false, "print the unmodified API response")
	section := fs.String("section", "summary", "character section: "+strings.Join(blizzard.SectionNames(), ", ")+"; talents: class, spec, hero or pvp")
	kind := fs.String("kind", "dungeons", "encounters: dungeons or raids")
	expansion := fs.String("expansion", "", "encounters: only expansions whose name contains this (adds boss detail)")
	season := fs.Int("season", 0, "mythic: season ID (0 = current)")
	tier := fs.Int("tier", 0, "recipes: skill tier ID (0 = newest the character has)")
	known := fs.Bool("known", false, "recipes: also list known recipes")
	list := fs.Bool("list", false, "mcp: print tools, resources and prompts, then exit")
	asJSON := fs.Bool("json", false, "mcp -list: print as JSON")
	sources := fs.Bool("sources", false, "recipes: explain where each missing recipe comes from")
	hero := fs.String("hero", "", "talents: only hero trees whose name contains this")
	fs.Parse(args)
	args = fs.Args()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if cmd == "mcp" && *list {
		// Listing never calls the API, so it works without credentials.
		return mcpserver.Describe(ctx, mcpserver.New(blizzard.New("", "", "us", "en_US"), version), os.Stdout, *asJSON)
	}
	c, err := blizzard.NewFromEnv()
	if err != nil {
		return err
	}

	print := func(body []byte, err error) error {
		if err != nil {
			return err
		}
		if !*raw {
			body = blizzard.Slim(body)
		}
		var buf bytes.Buffer
		if json.Indent(&buf, body, "", "  ") != nil {
			buf.Reset()
			buf.Write(body)
		}
		fmt.Println(buf.String())
		return nil
	}
	printJSON := func(v any, err error) error {
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(b))
		return nil
	}

	switch cmd {
	case "mcp":
		return mcpserver.Run(ctx, c, version)
	case "character":
		realm, name := resolveCharacter(args)
		return print(c.Character(ctx, realm, name, *section))
	case "encounters":
		realm, name := resolveCharacter(args)
		return printJSON(c.Encounters(ctx, realm, name, *kind, *expansion))
	case "mythic":
		realm, name := resolveCharacter(args)
		return printJSON(c.MythicKeystoneSeason(ctx, realm, name, *season))
	case "recipes":
		if len(args) == 0 {
			return fmt.Errorf("usage: wowapi recipes [-tier N] [-known] [-sources] <profession> [realm] [name]")
		}
		realm, name := resolveCharacter(args[1:])
		return printJSON(c.ProfessionRecipes(ctx, realm, name, args[0], blizzard.RecipeOptions{TierID: *tier, IncludeKnown: *known, Sources: *sources}))
	case "races":
		return printJSON(c.Races(ctx, strings.Join(args, " ")))
	case "classes":
		return printJSON(c.Classes(ctx, strings.Join(args, " ")))
	case "talents":
		if len(args) == 0 {
			return fmt.Errorf("usage: wowapi talents [-section S] [-hero H] [class] <spec>")
		}
		class, spec := strings.Join(args[:len(args)-1], " "), args[len(args)-1]
		part := *section
		if part == "summary" { // the flag's default for character; means "all" here
			part = ""
		}
		return printJSON(c.Talents(ctx, class, spec, part, *hero))
	case "search":
		if len(args) == 0 {
			return fmt.Errorf("usage: wowapi search <item name>")
		}
		return printJSON(c.SearchItems(ctx, strings.Join(args, " "), 25))
	case "item", "recipe", "price":
		if len(args) != 1 {
			return fmt.Errorf("usage: wowapi %s <id>", cmd)
		}
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("id must be a number: %q", args[0])
		}
		switch cmd {
		case "item":
			return print(c.Item(ctx, id))
		case "recipe":
			return print(c.Recipe(ctx, id))
		default:
			return printJSON(c.Commodity(ctx, id))
		}
	case "profession":
		ids := make([]int, 2)
		for i, a := range args {
			if i > 1 {
				return fmt.Errorf("usage: wowapi profession [id [skill-tier-id]]")
			}
			if ids[i], err = strconv.Atoi(a); err != nil {
				return fmt.Errorf("id must be a number: %q", a)
			}
		}
		return print(c.Profession(ctx, ids[0], ids[1]))
	case "get":
		if len(args) < 2 {
			return fmt.Errorf("usage: wowapi get <static|dynamic|profile> <path> [key=value ...]")
		}
		if !strings.HasPrefix(args[1], "/data/wow/") && !strings.HasPrefix(args[1], "/profile/wow/") {
			return fmt.Errorf("path must start with /data/wow/ or /profile/wow/, got %q (in Git Bash, set MSYS_NO_PATHCONV=1 so it isn't rewritten to a Windows path)", args[1])
		}
		q := url.Values{}
		for _, kv := range args[2:] {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				return fmt.Errorf("query parameter %q must be key=value", kv)
			}
			q.Set(k, v)
		}
		return print(c.Get(ctx, args[1], args[0], q))
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", cmd)
	}
}

// resolveCharacter resolves [realm] [name] arguments, defaulting to WOW_REALM and
// WOW_CHARACTER. An unquoted multi-word realm is everything but the last arg.
func resolveCharacter(args []string) (realm, name string) {
	realm, name = blizzard.Env("WOW_REALM"), blizzard.Env("WOW_CHARACTER")
	switch len(args) {
	case 0:
	case 1:
		name = args[0]
	default:
		realm, name = strings.Join(args[:len(args)-1], " "), args[len(args)-1]
	}
	return realm, name
}
