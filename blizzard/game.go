package blizzard

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Game selects which World of Warcraft game the client talks to. Every game
// has its own API namespaces (static-us for retail, static-forever-us for
// WoW: Forever) and its own set of endpoints that actually work. The zero
// value behaves as Retail.
type Game string

const (
	Retail  Game = "retail"  // World of Warcraft (Midnight and later); the default
	Forever Game = "forever" // World of Warcraft: Forever, the Classic-style game
)

// Feature is a capability that only some games' APIs provide. Tools, commands
// and library calls check it so an unsupported request fails with a clear
// message instead of a raw 404.
type Feature int

const (
	FeatureCharacters    Feature = iota + 1 // character profiles (/profile/wow/character/...)
	FeatureMythicPlus                       // Mythic+ seasons and runs
	FeatureCommodities                      // region-wide commodity Auction House prices
	FeatureRecipeSources                    // dungeon/raid loot tables (the journal) for recipe sources
	FeatureTalentExtras                     // class, hero and PvP talents (beyond per-spec trees)
)

var featureLabels = map[Feature]string{
	FeatureCharacters:    "character lookups",
	FeatureMythicPlus:    "Mythic+",
	FeatureCommodities:   "commodity prices",
	FeatureRecipeSources: "recipe sources",
	FeatureTalentExtras:  "class, hero or PvP talents",
}

// gameInfo describes one game. Adding a Classic flavor is one more entry in
// games below: the API also serves static-classic-us, static-classic1x-us and
// static-classicann-us (flavors "classic", "classic1x", "classicann"), each
// with its own list of unsupported features.
type gameInfo struct {
	title       string             // for humans: "World of Warcraft: Forever"
	flavor      string             // namespace infix, "" for retail: static-<flavor>-us
	unsupported map[Feature]string // feature -> why this game's API lacks it
}

var games = map[Game]gameInfo{
	Retail: {title: "World of Warcraft"},
	Forever: {
		title:  "World of Warcraft: Forever",
		flavor: "forever",
		// Verified against the live API on 2026-11-05 (the day after launch);
		// the "not yet" entries are Blizzard's to fix and worth rechecking.
		unsupported: map[Feature]string{
			FeatureCharacters:    "Blizzard's API does not serve Forever character profiles yet: every lookup returns 404 and no Forever realms are listed (checked 2026-11-05)",
			FeatureMythicPlus:    "Forever has no Mythic+",
			FeatureCommodities:   "Forever has no commodity Auction House endpoint (it returns 404), and realm auctions are not published yet",
			FeatureRecipeSources: "Forever's dungeon journal is empty, so there are no boss loot tables to search",
			FeatureTalentExtras:  "Forever has only original-style per-specialization talent trees, with no class, hero or PvP talents",
		},
	},
}

// ParseGame converts a WOW_GAME value to a Game. The empty string is Retail.
func ParseGame(s string) (Game, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return Retail, nil
	}
	if _, ok := games[Game(s)]; ok {
		return Game(s), nil
	}
	return "", fmt.Errorf("unknown game %q: WOW_GAME must be one of %s", s, strings.Join(GameNames(), ", "))
}

// GameFromEnv reads WOW_GAME (see ParseGame).
func GameFromEnv() (Game, error) { return ParseGame(Env("WOW_GAME")) }

// GameNames lists the valid WOW_GAME values, sorted.
func GameNames() []string {
	names := make([]string, 0, len(games))
	for g := range games {
		names = append(names, string(g))
	}
	sort.Strings(names)
	return names
}

// Title is the game's display name, e.g. "World of Warcraft: Forever".
func (g Game) Title() string {
	if t := games[g].title; t != "" {
		return t
	}
	return games[Retail].title
}

// Namespace returns the API namespace for a kind (Static, Dynamic or Profile)
// in a region: "static-us" for retail, "static-forever-us" for Forever.
func (g Game) Namespace(kind, region string) string {
	if f := games[g].flavor; f != "" {
		return kind + "-" + f + "-" + region
	}
	return kind + "-" + region
}

// Supports reports whether the game's API provides a feature.
func (g Game) Supports(f Feature) bool {
	_, missing := games[g].unsupported[f]
	return !missing
}

// Require returns a clear error if the game's API lacks a feature, and nil
// otherwise.
func (g Game) Require(f Feature) error {
	reason, missing := games[g].unsupported[f]
	if !missing {
		return nil
	}
	return fmt.Errorf("WOW_GAME=%s does not support %s: %s", g, featureLabels[f], reason)
}

// fullNamespaceRE matches a complete namespace string such as "static-us",
// "static-forever-us" or the versioned form the API puts in hrefs,
// "static-1.60.1_70399-forever-us".
var fullNamespaceRE = regexp.MustCompile(`^(static|dynamic|profile)(-[A-Za-z0-9._]+)*-(us|eu|kr|tw|cn)$`)

// ValidNamespace reports whether ns is a namespace kind (Static, Dynamic or
// Profile) or a full namespace string, the two forms Client.Get accepts.
func ValidNamespace(ns string) bool {
	switch ns {
	case Static, Dynamic, Profile:
		return true
	}
	return fullNamespaceRE.MatchString(ns)
}

// resolveNamespace turns a kind into the configured game's namespace, and
// passes a full namespace string through unchanged.
func (c *Client) resolveNamespace(ns string) string {
	switch ns {
	case Static, Dynamic, Profile:
		return c.Game.Namespace(ns, c.Region)
	}
	return ns
}

// isForeverNamespace reports whether a full namespace belongs to Forever.
func isForeverNamespace(ns string) bool { return strings.Contains(ns, "-forever-") }
