package main

import (
	"fmt"

	"github.com/glholland/wowapi/blizzard"
)

// gateFlags are the command-line flags that change which features a command
// needs.
type gateFlags struct {
	section string // talents: class, spec, hero or pvp ("summary" is the character default, meaning all)
	hero    string // talents: hero tree filter
	sources bool   // recipes: also look up where each recipe comes from
}

// commandNeeds lists the game features a CLI command depends on.
func commandNeeds(cmd string, f gateFlags) []blizzard.Feature {
	switch cmd {
	case "character", "encounters":
		return []blizzard.Feature{blizzard.FeatureCharacters}
	case "mythic":
		return []blizzard.Feature{blizzard.FeatureCharacters, blizzard.FeatureMythicPlus}
	case "recipes":
		needs := []blizzard.Feature{blizzard.FeatureCharacters}
		if f.sources {
			needs = append(needs, blizzard.FeatureRecipeSources)
		}
		return needs
	case "price":
		return []blizzard.Feature{blizzard.FeatureCommodities}
	case "talents":
		switch f.section {
		case "class", "hero", "pvp":
			return []blizzard.Feature{blizzard.FeatureTalentExtras}
		}
		if f.hero != "" {
			return []blizzard.Feature{blizzard.FeatureTalentExtras}
		}
	}
	return nil
}

// checkCommand returns a clear error if the game's API cannot serve the
// command. It needs no credentials or network, so it runs before the client is
// built.
func checkCommand(g blizzard.Game, cmd string, f gateFlags) error {
	for _, need := range commandNeeds(cmd, f) {
		if err := g.Require(need); err != nil {
			return fmt.Errorf("wowapi %s: %w (unset WOW_GAME to use retail)", cmd, err)
		}
	}
	return nil
}
