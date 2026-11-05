package blizzard

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseGame(t *testing.T) {
	for in, want := range map[string]Game{"": Retail, "retail": Retail, " Forever ": Forever, "FOREVER": Forever} {
		if got, err := ParseGame(in); err != nil || got != want {
			t.Errorf("ParseGame(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	_, err := ParseGame("classic")
	if err == nil || !strings.Contains(err.Error(), "forever, retail") {
		t.Errorf("an unknown game should list the valid ones, got %v", err)
	}

	t.Setenv("WOW_GAME", "${user_config.game}") // an unfilled MCP bundle setting
	if g, err := GameFromEnv(); err != nil || g != Retail {
		t.Errorf("a placeholder should mean retail, got %q, %v", g, err)
	}
	t.Setenv("WOW_GAME", "forever")
	if g, err := GameFromEnv(); err != nil || g != Forever {
		t.Errorf("GameFromEnv = %q, %v", g, err)
	}
	t.Setenv("BLIZZARD_CLIENT_ID", "id")
	t.Setenv("BLIZZARD_CLIENT_SECRET", "secret")
	if c, err := NewFromEnv(); err != nil || c.Game != Forever {
		t.Errorf("NewFromEnv should pick up WOW_GAME, got %+v, %v", c, err)
	}
	t.Setenv("WOW_GAME", "nope")
	if _, err := NewFromEnv(); err == nil {
		t.Error("NewFromEnv should reject an unknown WOW_GAME")
	}
}

func TestGameNamespace(t *testing.T) {
	for _, tc := range []struct {
		game         Game
		kind, region string
		want         string
	}{
		{Retail, Static, "us", "static-us"},
		{Retail, Profile, "eu", "profile-eu"},
		{Game(""), Dynamic, "us", "dynamic-us"}, // the zero value is retail
		{Forever, Static, "us", "static-forever-us"},
		{Forever, Dynamic, "us", "dynamic-forever-us"},
		{Forever, Profile, "us", "profile-forever-us"},
	} {
		if got := tc.game.Namespace(tc.kind, tc.region); got != tc.want {
			t.Errorf("%q.Namespace(%q, %q) = %q, want %q", tc.game, tc.kind, tc.region, got, tc.want)
		}
	}
	if Forever.Title() != "World of Warcraft: Forever" || Game("").Title() != "World of Warcraft" {
		t.Errorf("unexpected titles: %q, %q", Forever.Title(), Game("").Title())
	}
}

func TestValidNamespace(t *testing.T) {
	for _, ns := range []string{"static", "dynamic", "profile", "static-us", "static-forever-us", "dynamic-classicann-us",
		"profile-classic1x-eu", "static-1.60.1_70399-forever-us"} {
		if !ValidNamespace(ns) {
			t.Errorf("%q should be valid", ns)
		}
	}
	for _, ns := range []string{"", "bogus", "static-forever", "forever-us", "static-forever-xx", "static us", "../static-us", "static-forever-us/x"} {
		if ValidNamespace(ns) {
			t.Errorf("%q should be invalid", ns)
		}
	}
}

func TestFeatureSupport(t *testing.T) {
	all := []Feature{FeatureCharacters, FeatureMythicPlus, FeatureCommodities, FeatureRecipeSources, FeatureTalentExtras}
	for _, f := range all {
		if !Retail.Supports(f) || Retail.Require(f) != nil {
			t.Errorf("retail should support feature %d", f)
		}
		err := Forever.Require(f)
		if Forever.Supports(f) || err == nil || !strings.Contains(err.Error(), "WOW_GAME=forever does not support "+featureLabels[f]) {
			t.Errorf("forever should reject feature %d with a clear error, got %v", f, err)
		}
	}
}

func TestClientUsesTheGameNamespace(t *testing.T) {
	ctx := context.Background()
	c, _ := fakeAPIFor(t, WithGame(Forever))

	// A kind resolves to the game's namespace: this is Forever data, not retail.
	body, err := c.Get(ctx, "/data/wow/playable-race/index", Static, nil)
	if err != nil || !strings.Contains(string(body), "High Order Skyborne") {
		t.Fatalf("a kind should use static-forever-us: %s %v", body, err)
	}
	// A full namespace string is used as-is, whatever the configured game.
	body, err = c.Get(ctx, "/data/wow/playable-race/index", "static-us", nil)
	if err != nil || !strings.Contains(string(body), `"id":52`) || strings.Contains(string(body), "Skyborne") {
		t.Fatalf("a full namespace should pass through: %s %v", body, err)
	}
	// An unknown namespace is a 403 whose message says so.
	_, err = c.Get(ctx, "/data/wow/playable-race/index", "static-bogus-us", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 403 || apiErr.Namespace != "static-bogus-us" || !strings.Contains(err.Error(), "does not recognize the namespace") {
		t.Errorf("want a 403 APIError naming the namespace, got %v", err)
	}
	// A recognized namespace without the data is a 404.
	_, err = c.Get(ctx, "/data/wow/realm/index", Dynamic, nil)
	if !errors.As(err, &apiErr) || apiErr.Status != 404 || apiErr.Namespace != "dynamic-forever-us" {
		t.Errorf("want a 404 from dynamic-forever-us, got %v", err)
	}
	// Character 404s explain themselves per game.
	_, err = c.Get(ctx, "/profile/wow/character/forever/celdis", Profile, nil)
	if err == nil || !strings.Contains(err.Error(), "WoW: Forever character lookups are not served") {
		t.Errorf("a Forever character 404 should explain itself: %v", err)
	}
	retail, _ := fakeAPI(t)
	_, err = retail.Get(ctx, "/profile/wow/character/lightbringer/nobody", Profile, nil)
	if err == nil || !strings.Contains(err.Error(), "character not found") {
		t.Errorf("a retail character 404 keeps its hint: %v", err)
	}
}

// Unsupported features fail before any request, so they never reach the API
// (and never show up as a raw 404).
func TestForeverRejectsRetailOnlyCalls(t *testing.T) {
	c, tokenCalls := fakeAPIFor(t, WithGame(Forever))
	ctx := context.Background()
	for name, call := range map[string]func() error{
		"character":     func() error { _, err := c.Character(ctx, "x", "y", ""); return err },
		"encounters":    func() error { _, err := c.Encounters(ctx, "x", "y", "", ""); return err },
		"mythic":        func() error { _, err := c.MythicKeystoneSeason(ctx, "x", "y", 0); return err },
		"recipes":       func() error { _, err := c.ProfessionRecipes(ctx, "x", "y", "tailoring", RecipeOptions{}); return err },
		"commodity":     func() error { _, err := c.Commodity(ctx, 1); return err },
		"talents hero":  func() error { _, err := c.Talents(ctx, "", "Elemental Combat", "hero", ""); return err },
		"talents pvp":   func() error { _, err := c.Talents(ctx, "", "Elemental Combat", "pvp", ""); return err },
		"talents class": func() error { _, err := c.Talents(ctx, "", "Elemental Combat", "class", ""); return err },
		"talents tree":  func() error { _, err := c.Talents(ctx, "", "Elemental Combat", "", "sunfury"); return err },
	} {
		err := call()
		if err == nil || !strings.Contains(err.Error(), "WOW_GAME=forever does not support") || strings.Contains(err.Error(), "HTTP 404") {
			t.Errorf("%s: want a clear unsupported error, got %v", name, err)
		}
	}
	if n := *tokenCalls; n != 0 {
		t.Errorf("unsupported calls should not touch the API, but %d token requests were made", n)
	}
	// Sources are checked separately from characters.
	if err := Forever.Require(FeatureRecipeSources); err == nil || !strings.Contains(err.Error(), "recipe sources") {
		t.Errorf("recipe sources should be unsupported: %v", err)
	}
}

func TestForeverRaces(t *testing.T) {
	c, _ := fakeAPIFor(t, WithGame(Forever))
	ctx := context.Background()

	races, err := c.Races(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Race{}
	var names []string
	for _, r := range races {
		byName[r.Name] = r
		names = append(names, r.Name)
	}
	if !reflect.DeepEqual(names, []string{"High Order Skyborne", "Human", "Orc", "Windshaper Skyborne"}) {
		t.Fatalf("Skyborne is two races (different names and factions) and must not be merged: %v", names)
	}
	high, wind := byName["High Order Skyborne"], byName["Windshaper Skyborne"]
	if !reflect.DeepEqual(high.Factions, []string{"Alliance"}) || !reflect.DeepEqual(high.IDs, []int{95}) ||
		!reflect.DeepEqual(wind.Factions, []string{"Horde"}) || !reflect.DeepEqual(wind.IDs, []int{96}) {
		t.Errorf("each Skyborne keeps its own faction and ID: %+v / %+v", high, wind)
	}
	// The Alliance Skyborne get Mage and the Horde ones Shaman.
	if !reflect.DeepEqual(high.Classes, []string{"Mage", "Warrior"}) || !reflect.DeepEqual(wind.Classes, []string{"Shaman", "Warrior"}) {
		t.Errorf("class lists should differ by faction: %v / %v", high.Classes, wind.Classes)
	}
	if high.AlliedRace || len(high.Racials) != 5 || high.Racials[0].Description != "" {
		t.Errorf("list mode: 5 racials, no descriptions: %+v", high)
	}

	one, err := c.Races(ctx, "windshaper skyborne")
	if err != nil || len(one) != 1 {
		t.Fatalf("one race: %+v %v", one, err)
	}
	for _, r := range one[0].Racials {
		if r.Description == "" {
			t.Errorf("racial %q should be described", r.Name)
		}
	}
}

func TestForeverClasses(t *testing.T) {
	c, _ := fakeAPIFor(t, WithGame(Forever))
	ctx := context.Background()

	all, err := c.Classes(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, cl := range all {
		names = append(names, cl.Name)
	}
	if !reflect.DeepEqual(names, []string{"Mage", "Shaman", "Warrior"}) {
		t.Fatalf("classes: %v", names)
	}
	shaman := all[1]
	if len(shaman.Specs) != 3 || shaman.Specs[0].Name == "" {
		t.Fatalf("a class's specs come from its record: %+v", shaman.Specs)
	}
	// Forever's spec records carry no role or primary stat; that must not
	// show up as misleading empty strings.
	b, _ := json.Marshal(shaman.Specs)
	if strings.Contains(string(b), `"role"`) || strings.Contains(string(b), `"primary_stat"`) {
		t.Errorf("empty role and primary_stat should be omitted: %s", b)
	}
	if shaman.Specs[0].Role != "" || shaman.Specs[0].PrimaryStat != "" {
		t.Errorf("unexpected role/stat: %+v", shaman.Specs[0])
	}

	one, err := c.Classes(ctx, "shaman")
	if err != nil || len(one) != 1 {
		t.Fatalf("one class: %+v %v", one, err)
	}
	if !contains(one[0].Races, "Windshaper Skyborne") || contains(one[0].Races, "High Order Skyborne") {
		t.Errorf("Shaman is playable by the Horde Skyborne only: %v", one[0].Races)
	}
	for _, sp := range one[0].Specs {
		if len(sp.HeroTrees) != 0 || len(sp.PvPTalents) != 0 {
			t.Errorf("Forever has no hero trees or PvP talents: %+v", sp)
		}
	}
}

func TestForeverTalents(t *testing.T) {
	c, _ := fakeAPIFor(t, WithGame(Forever))
	tree, err := c.Talents(context.Background(), "", "Elemental Combat", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if tree.Class != "Shaman" || tree.TreeID != 1082 || tree.SpecID != 375 {
		t.Fatalf("unexpected tree header: %+v", tree)
	}
	if len(tree.ClassTalents) != 0 || len(tree.HeroTrees) != 0 || len(tree.PvPTalents) != 0 || len(tree.Gates) != 0 {
		t.Errorf("Forever trees hold only spec talents: %+v", tree)
	}
	if len(tree.SpecTalents) != 6 {
		t.Fatalf("want the 6 fixture nodes, got %d", len(tree.SpecTalents))
	}
	multi := 0
	for _, n := range tree.SpecTalents {
		if n.Name == "" || n.Description == "" {
			t.Errorf("every talent should be named and described: %+v", n)
		}
		if n.MaxRank > 1 {
			multi++
		}
	}
	if multi == 0 {
		t.Error("original-style talents have several ranks; the fixture should show one")
	}
	if !strings.Contains(tree.Note, "original-style") {
		t.Errorf("the note should say these are original-style trees: %q", tree.Note)
	}

	// The spec section is still allowed.
	if spec, err := c.Talents(context.Background(), "Shaman", "Elemental Combat", "spec", ""); err != nil || len(spec.SpecTalents) != 6 {
		t.Errorf("section=spec should work: %+v %v", spec, err)
	}
}

func TestForeverStaticPassThroughs(t *testing.T) {
	c, _ := fakeAPIFor(t, WithGame(Forever))
	ctx := context.Background()

	hits, err := c.SearchItems(ctx, "linen", 5)
	if err != nil || len(hits) != 2 || hits[0].Name == "" {
		t.Errorf("item search: %+v %v", hits, err)
	}
	for name, call := range map[string]func() ([]byte, error){
		"item":       func() ([]byte, error) { return c.Item(ctx, 19019) },
		"recipe":     func() ([]byte, error) { return c.Recipe(ctx, 2359) },
		"profession": func() ([]byte, error) { return c.Profession(ctx, 197, 2948) },
	} {
		if body, err := call(); err != nil || len(body) == 0 {
			t.Errorf("%s: %s %v", name, body, err)
		}
	}
	body, _ := c.Item(ctx, 19019)
	if !strings.Contains(string(body), "Thunderfury") {
		t.Errorf("item 19019 should be Thunderfury: %s", body)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
