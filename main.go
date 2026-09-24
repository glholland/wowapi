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
)

const version = "0.1.0"

const usage = `wowapi - World of Warcraft Battle.net API client and MCP server

Usage:
  wowapi character [-section S] [realm] [name]   character profile (realm/name default to WOW_REALM/WOW_CHARACTER)
  wowapi search <item name>                      find item IDs by name
  wowapi item <id>                               item details
  wowapi profession [id [skill-tier-id]]         professions, skill tiers, recipes
  wowapi recipe <id>                             recipe details
  wowapi price <item id>                         Auction House commodity price
  wowapi get <static|dynamic|profile> <path> [key=value ...]
                                                 any API endpoint
  wowapi mcp                                     run as an MCP server on stdio
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
	section := fs.String("section", "summary", "character section: "+strings.Join(blizzard.SectionNames(), ", "))
	fs.Parse(args)
	args = fs.Args()

	c, err := blizzard.NewFromEnv()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

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
		return runMCP(ctx, c)
	case "character":
		realm, name := os.Getenv("WOW_REALM"), os.Getenv("WOW_CHARACTER")
		switch len(args) {
		case 0:
		case 1:
			name = args[0]
		case 2:
			realm, name = args[0], args[1]
		default:
			// Unquoted multi-word realm: everything but the last arg.
			realm, name = strings.Join(args[:len(args)-1], " "), args[len(args)-1]
		}
		return print(c.Character(ctx, realm, name, *section))
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
