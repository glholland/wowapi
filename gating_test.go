package main

import (
	"strings"
	"testing"

	"github.com/glholland/wowapi/blizzard"
)

func TestRetailRunsEveryCommand(t *testing.T) {
	flags := gateFlags{section: "hero", hero: "sunfury", sources: true}
	for _, cmd := range []string{"character", "encounters", "mythic", "recipes", "price", "talents",
		"races", "classes", "search", "item", "profession", "recipe", "get", "mcp", "version"} {
		if err := checkCommand(blizzard.Retail, cmd, flags); err != nil {
			t.Errorf("retail should run %q, got %v", cmd, err)
		}
	}
}

// WOW_GAME=forever: commands that need data Blizzard's Forever API does not
// serve fail with a clear message instead of a raw 404.
func TestForeverRejectsRetailOnlyCommands(t *testing.T) {
	for _, tc := range []struct {
		cmd   string
		flags gateFlags
		want  string
	}{
		{"character", gateFlags{section: "summary"}, "character lookups"},
		{"encounters", gateFlags{}, "character lookups"},
		{"mythic", gateFlags{}, "character lookups"},
		{"recipes", gateFlags{}, "character lookups"},
		{"recipes", gateFlags{sources: true}, "character lookups"},
		{"price", gateFlags{}, "commodity prices"},
		{"talents", gateFlags{section: "hero"}, "class, hero or PvP talents"},
		{"talents", gateFlags{section: "pvp"}, "class, hero or PvP talents"},
		{"talents", gateFlags{section: "class"}, "class, hero or PvP talents"},
		{"talents", gateFlags{section: "summary", hero: "sunfury"}, "class, hero or PvP talents"},
	} {
		err := checkCommand(blizzard.Forever, tc.cmd, tc.flags)
		if err == nil {
			t.Errorf("%s %+v: forever should be rejected", tc.cmd, tc.flags)
			continue
		}
		msg := err.Error()
		for _, part := range []string{"wowapi " + tc.cmd, "WOW_GAME=forever does not support " + tc.want, "unset WOW_GAME to use retail"} {
			if !strings.Contains(msg, part) {
				t.Errorf("%s: error should mention %q: %s", tc.cmd, part, msg)
			}
		}
		if strings.Contains(msg, "404") && !strings.Contains(msg, "returns 404") {
			t.Errorf("%s: should not surface a raw 404: %s", tc.cmd, msg)
		}
	}
}

func TestForeverRunsTheCommandsItCanServe(t *testing.T) {
	for _, tc := range []struct {
		cmd   string
		flags gateFlags
	}{
		{"races", gateFlags{}}, {"classes", gateFlags{}},
		{"talents", gateFlags{section: "summary"}}, // the flag default: the whole tree
		{"talents", gateFlags{section: "spec"}},
		{"search", gateFlags{}}, {"item", gateFlags{}}, {"profession", gateFlags{}}, {"recipe", gateFlags{}},
		{"get", gateFlags{}}, {"mcp", gateFlags{}}, {"version", gateFlags{}},
	} {
		if err := checkCommand(blizzard.Forever, tc.cmd, tc.flags); err != nil {
			t.Errorf("forever should run %q %+v, got %v", tc.cmd, tc.flags, err)
		}
	}
}
