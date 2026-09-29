package blizzard

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestEncounters(t *testing.T) {
	c, _ := fakeAPI(t)
	ctx := context.Background()

	all, err := c.Encounters(ctx, "Lightbringer", "Mage", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Expansions) != 2 || all.Expansions[0].Name != "Midnight" {
		t.Fatalf("want 2 expansions, newest first: %+v", all.Expansions)
	}
	mt := all.Expansions[0].Instances[0]
	if len(mt.Modes) != 1 || mt.Modes[0].Progress != "1/1" || mt.Modes[0].Bosses != nil {
		t.Fatalf("unfiltered: want one mode without boss detail, got %+v", mt.Modes)
	}

	mid, err := c.Encounters(ctx, "Lightbringer", "Mage", "dungeon", "midnight")
	if err != nil {
		t.Fatal(err)
	}
	if len(mid.Expansions) != 1 {
		t.Fatalf("filter: want 1 expansion, got %+v", mid.Expansions)
	}
	b := mid.Expansions[0].Instances[0].Modes[0].Bosses
	if len(b) != 1 || b[0].Name != "Degentrius" || b[0].Kills != 1 || b[0].LastKill == "" {
		t.Fatalf("filter: unexpected bosses %+v", b)
	}

	none, err := c.Encounters(ctx, "Lightbringer", "Mage", "dungeons", "legion")
	if err != nil || len(none.Expansions) != 0 || !strings.Contains(none.Note, "Midnight") {
		t.Fatalf("no-match filter should list available expansions: %+v %v", none, err)
	}
	if _, err := c.Encounters(ctx, "Lightbringer", "Mage", "arenas", ""); err == nil {
		t.Error("expected error for bad kind")
	}
}

func TestMythicKeystoneSeason(t *testing.T) {
	c, _ := fakeAPI(t)
	ctx := context.Background()

	cur, err := c.MythicKeystoneSeason(ctx, "Lightbringer", "Mage", 0)
	if err != nil {
		t.Fatal(err)
	}
	if cur.SeasonID != 18 || cur.SeasonName != "Midnight Season 2" || cur.Rating != 1234.6 {
		t.Fatalf("unexpected season summary: %+v", cur)
	}
	if len(cur.Runs) != 2 || cur.Runs[0].Dungeon != "High Run" || cur.Runs[0].Duration != "30:02" || cur.Runs[0].Timed {
		t.Fatalf("runs should be sorted by rating with m:ss durations: %+v", cur.Runs)
	}
	if got := cur.Runs[1].Party; len(got) != 1 || got[0] != "Mage (Arcane, 250)" {
		t.Fatalf("unexpected party: %v", got)
	}
	if len(cur.ThisWeek) != 1 || cur.ThisWeek[0].Rating != 300 {
		t.Fatalf("unexpected weekly runs: %+v", cur.ThisWeek)
	}

	old, err := c.MythicKeystoneSeason(ctx, "Lightbringer", "Mage", 17)
	if err != nil {
		t.Fatalf("a season with no runs should not be an error: %v", err)
	}
	if len(old.Runs) != 0 || old.Note == "" || old.ThisWeek != nil || old.SeasonName != "Midnight Season 1" {
		t.Fatalf("unexpected empty-season result: %+v", old)
	}

	var apiErr *APIError
	if _, err := c.MythicKeystoneSeason(ctx, "Lightbringer", "Nobody", 0); !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Errorf("missing character should be a 404, got %v", err)
	}
}

func TestProfessionRecipes(t *testing.T) {
	c, _ := fakeAPI(t)
	ctx := context.Background()

	p, err := c.ProfessionRecipes(ctx, "Lightbringer", "Mage", "tailoring", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if p.TierID != 2918 || p.Skill != "72/100" || p.Known != 2 || p.Total != 3 {
		t.Fatalf("should default to the newest tier: %+v", p)
	}
	if len(p.Missing) != 1 || p.Missing[0].Category != "Woven Cloth" || p.Missing[0].Recipes[0].Name != "Arcanoweave Bolt" {
		t.Fatalf("unexpected missing recipes: %+v", p.Missing)
	}
	if p.KnownList != nil || len(p.OtherTiers) != 1 {
		t.Fatalf("known list should be omitted and other tiers listed: %+v", p)
	}

	byID, err := c.ProfessionRecipes(ctx, "Lightbringer", "Mage", "197", 2918, true)
	if err != nil || len(byID.KnownList) != 2 {
		t.Fatalf("lookup by ID with known recipes: %+v %v", byID, err)
	}
	if _, err := c.ProfessionRecipes(ctx, "Lightbringer", "Mage", "mining", 0, false); err == nil || !strings.Contains(err.Error(), "Tailoring") {
		t.Errorf("unknown profession should list what the character has, got %v", err)
	}
	if _, err := c.ProfessionRecipes(ctx, "Lightbringer", "Mage", "tailoring", 9999, false); err == nil {
		t.Error("expected error for a tier the character lacks")
	}
}
