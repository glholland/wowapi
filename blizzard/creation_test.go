package blizzard

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// creationData is served by fakeAPI for race, class, spec and talent paths.
var creationData = map[string]string{
	"/data/wow/playable-race/index": `{"races":[{"id":1},{"id":24},{"id":25},{"id":52},{"id":70}]}`,
	"/data/wow/playable-race/1": `{"id":1,"name":"Human","is_selectable":true,"is_allied_race":false,"faction":{"name":"Alliance"},
		"playable_classes":[{"id":8,"name":"Mage"},{"id":14,"name":"Adventurer"}],
		"racial_spells":[{"id":20598,"name":"The Human Spirit"},{"id":79738,"name":"Languages"}]}`,
	"/data/wow/playable-race/24": `{"id":24,"name":"Pandaren","is_selectable":true,"faction":{"name":"Neutral"},
		"playable_classes":[{"id":8,"name":"Mage"}],"racial_spells":[{"id":107072,"name":"Epicurean"}]}`,
	"/data/wow/playable-race/25": `{"id":25,"name":"Pandaren","is_selectable":false,"faction":{"name":"Alliance"},
		"playable_classes":[{"id":8,"name":"Mage"}],"racial_spells":[{"id":107072,"name":"Epicurean"}]}`,
	"/data/wow/playable-race/52": `{"id":52,"name":"Dracthyr","is_selectable":true,"faction":{"name":"Alliance"},
		"playable_classes":[{"id":13,"name":"Evoker"}],"racial_spells":[{"id":1,"name":"Soar"}]}`,
	"/data/wow/playable-race/70": `{"id":70,"name":"Dracthyr","is_selectable":true,"faction":{"name":"Horde"},
		"playable_classes":[{"id":13,"name":"Evoker"},{"id":8,"name":"Mage"}],"racial_spells":[{"id":1,"name":"Soar"}]}`,
	"/data/wow/spell/20598": `{"id":20598,"description":"You gain 2% more of all secondary stats.\r\n"}`,

	"/data/wow/playable-class/index": `{"classes":[{"id":8,"name":"Mage"},{"id":13,"name":"Evoker"},{"id":6,"name":"Death Knight"}]}`,
	"/data/wow/playable-class/8": `{"id":8,"name":"Mage","power_type":{"name":"Mana"},"additional_power_types":[{"name":"Arcane Charges"}],
		"specializations":[{"id":62},{"id":64}],"playable_races":[{"name":"Pandaren"},{"name":"Human"},{"name":"Pandaren"}]}`,
	"/data/wow/playable-class/13": `{"id":13,"name":"Evoker","power_type":{"name":"Mana"},"specializations":[{"id":1467}]}`,
	"/data/wow/playable-class/6":  `{"id":6,"name":"Death Knight","power_type":{"name":"Runic Power"},"specializations":[{"id":251}]}`,
	"/data/wow/playable-specialization/62": `{"id":62,"name":"Arcane","playable_class":{"id":8,"name":"Mage"},"role":{"name":"Damage"},
		"primary_stat_type":{"name":"Intellect"},"power_type":{"name":"Mana"},"gender_description":{"male":"Manipulates raw Arcane magic.\r\n\r\nPreferred Weapon: Staff"},
		"hero_talent_trees":[{"id":39,"name":"Sunfury"},{"id":40,"name":"Spellslinger"}],
		"pvp_talents":[{"talent":{"name":"Ice Wall"},"spell_tooltip":{"description":"Conjures an Ice Wall.","cooldown":"1.5 min cooldown"}}],
		"spec_talent_tree":{"key":{"href":"https://us.api.blizzard.com/data/wow/talent-tree/658/playable-specialization/62?namespace=static-us"}}}`,
	"/data/wow/playable-specialization/64":   `{"id":64,"name":"Frost","playable_class":{"id":8,"name":"Mage"},"role":{"name":"Damage"},"primary_stat_type":{"name":"Intellect"}}`,
	"/data/wow/playable-specialization/251":  `{"id":251,"name":"Frost","playable_class":{"id":6,"name":"Death Knight"},"role":{"name":"Damage"},"primary_stat_type":{"name":"Strength"}}`,
	"/data/wow/playable-specialization/1467": `{"id":1467,"name":"Devastation","playable_class":{"id":13,"name":"Evoker"},"role":{"name":"Damage"},"primary_stat_type":{"name":"Intellect"}}`,

	"/data/wow/talent-tree/658/playable-specialization/62": `{
		"class_talent_nodes":[
			{"id":1,"display_row":1,"display_col":4,"node_type":{"type":"ACTIVE"},"ranks":[{"rank":1,"default_points":1,
				"tooltip":{"talent":{"name":"Prismatic Barrier"},"spell_tooltip":{"description":"Shields\r\nyou.","cast_time":"Instant","power_cost":"900 Mana"}}}]},
			{"id":2,"display_row":3,"display_col":1,"locked_by":[1],"node_type":{"type":"PASSIVE"},"ranks":[
				{"rank":1,"tooltip":{"talent":{"name":"Master of Time"},"spell_tooltip":{"description":"Cooldown reduced by 5 sec."}}},
				{"rank":2,"tooltip":{"talent":{"name":"Master of Time"},"spell_tooltip":{"description":"Cooldown reduced by 10 sec."}}}]},
			{"id":3,"display_row":2,"display_col":9,"node_type":{"type":"CHOICE"},"ranks":[{"rank":1}]}],
		"spec_talent_nodes":[
			{"id":10,"display_row":1,"display_col":18,"node_type":{"type":"CHOICE"},"ranks":[{"rank":1,"choice_of_tooltips":[
				{"talent":{"name":"Attuned Familiar"},"spell_tooltip":{"description":"Splinters."}},
				{"talent":{"name":"Shifting Shards"},"spell_tooltip":{"description":"More splinters."}}]}]}],
		"hero_talent_trees":[
			{"id":39,"name":"Sunfury","hero_talent_nodes":[{"id":20,"display_row":1,"display_col":1,"node_type":{"type":"PASSIVE"},
				"ranks":[{"rank":1,"tooltip":{"talent":{"name":"Spellfire Spheres"},"spell_tooltip":{"description":"Spheres."}}}]}]},
			{"id":41,"name":"Frostfire","hero_talent_nodes":[{"id":30,"display_row":1,"display_col":1,"node_type":{"type":"PASSIVE"},"ranks":[{"rank":1}]}]}],
		"restriction_lines":[{"is_for_class":true,"required_points":8,"restricted_row":4.5}]}`,
}

func TestRaces(t *testing.T) {
	c, _ := fakeAPI(t)
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
	if !reflect.DeepEqual(names, []string{"Dracthyr", "Human", "Pandaren"}) {
		t.Fatalf("want merged, sorted, selectable races; got %v", names)
	}
	if d := byName["Dracthyr"]; !reflect.DeepEqual(d.Factions, []string{"Alliance", "Horde"}) || !reflect.DeepEqual(d.Classes, []string{"Evoker", "Mage"}) || len(d.Racials) != 1 {
		t.Errorf("Dracthyr should merge both faction records: %+v", d)
	}
	if p := byName["Pandaren"]; !reflect.DeepEqual(p.IDs, []int{24}) || !reflect.DeepEqual(p.Factions, []string{"Neutral"}) {
		t.Errorf("unselectable Pandaren records should be dropped: %+v", p)
	}
	h := byName["Human"]
	if !reflect.DeepEqual(h.Classes, []string{"Mage"}) {
		t.Errorf("placeholder classes not in the class index should be dropped: %v", h.Classes)
	}
	if len(h.Racials) != 1 || h.Racials[0].Name != "The Human Spirit" || h.Racials[0].Description != "" {
		t.Errorf("list mode: Languages dropped, no descriptions: %+v", h.Racials)
	}

	one, err := c.Races(ctx, "human")
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 1 || one[0].Racials[0].Description != "You gain 2% more of all secondary stats." {
		t.Errorf("single race should describe racials: %+v", one)
	}
	if _, err := c.Races(ctx, "orc"); err == nil || !strings.Contains(err.Error(), "Dracthyr") {
		t.Errorf("unknown race should list options, got %v", err)
	}
}

func TestClasses(t *testing.T) {
	c, _ := fakeAPI(t)
	ctx := context.Background()

	all, err := c.Classes(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].Name != "Death Knight" || all[2].Name != "Mage" {
		t.Fatalf("want 3 classes sorted by name: %+v", all)
	}
	mage := all[2]
	if len(mage.Specs) != 2 || mage.Specs[0].Role != "Damage" || mage.Specs[0].PrimaryStat != "Intellect" {
		t.Fatalf("specs should carry role and stat: %+v", mage.Specs)
	}
	if mage.Specs[0].Description != "" || mage.Races != nil {
		t.Errorf("list mode should omit details: %+v", mage)
	}

	one, err := c.Classes(ctx, "mage")
	if err != nil {
		t.Fatal(err)
	}
	arcane := one[0].Specs[0]
	if !reflect.DeepEqual(one[0].Races, []string{"Human", "Pandaren"}) || !reflect.DeepEqual(one[0].ExtraPowers, []string{"Arcane Charges"}) {
		t.Errorf("detail mode should dedupe races and list resources: %+v", one[0])
	}
	if arcane.Description != "Manipulates raw Arcane magic.\n\nPreferred Weapon: Staff" || !reflect.DeepEqual(arcane.HeroTrees, []string{"Sunfury", "Spellslinger"}) || len(arcane.PvPTalents) != 1 {
		t.Errorf("detail mode should describe specs: %+v", arcane)
	}
	if _, err := c.Classes(ctx, "bard"); err == nil || !strings.Contains(err.Error(), "Evoker") {
		t.Errorf("unknown class should list options, got %v", err)
	}
}

func TestTalents(t *testing.T) {
	c, _ := fakeAPI(t)
	ctx := context.Background()

	if _, err := c.Talents(ctx, "", "Frost", "", ""); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("Frost without a class should be ambiguous, got %v", err)
	}
	if _, err := c.Talents(ctx, "Mage", "Holy", "", ""); err == nil || !strings.Contains(err.Error(), "Arcane Mage") {
		t.Errorf("unknown spec should list the class's specs, got %v", err)
	}

	tree, err := c.Talents(ctx, "mage", "arcane", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if tree.TreeID != 658 || tree.Class != "Mage" || len(tree.PvPTalents) != 1 {
		t.Fatalf("unexpected tree header: %+v", tree)
	}
	if !reflect.DeepEqual(tree.Gates, []string{"class tree: 8 points spent before row 5"}) {
		t.Errorf("unexpected gates: %v", tree.Gates)
	}
	ct := tree.ClassTalents
	if len(ct) != 3 || ct[0].NodeID != 1 || ct[1].NodeID != 3 || ct[2].NodeID != 2 {
		t.Fatalf("class talents should be in row order: %+v", ct)
	}
	if !ct[0].Free || ct[0].Description != "Shields\nyou." || ct[0].Cost != "900 Mana" {
		t.Errorf("free talent with cleaned text expected: %+v", ct[0])
	}
	if ct[2].MaxRank != 2 || ct[2].Description != "Cooldown reduced by 10 sec." || !reflect.DeepEqual(ct[2].Requires, []int{1}) {
		t.Errorf("multi-rank talent should use its max-rank text and list prerequisites: %+v", ct[2])
	}
	if ct[1].Type != "choice" || !strings.Contains(ct[1].Name, "does not list") {
		t.Errorf("empty choice node should be flagged: %+v", ct[1])
	}
	if ch := tree.SpecTalents[0].Choices; len(ch) != 2 || ch[1].Name != "Shifting Shards" {
		t.Errorf("choice node options expected: %+v", tree.SpecTalents[0])
	}
	if len(tree.HeroTrees) != 1 || tree.HeroTrees[0].Name != "Sunfury" {
		t.Errorf("only the spec's own hero trees should be kept: %+v", tree.HeroTrees)
	}

	pvp, err := c.Talents(ctx, "", "62", "pvp", "")
	if err != nil || len(pvp.PvPTalents) != 1 || pvp.ClassTalents != nil || pvp.TreeID != 0 {
		t.Errorf("pvp section by spec ID should skip the tree: %+v %v", pvp, err)
	}
	hero, err := c.Talents(ctx, "Mage", "Arcane", "hero", "sun")
	if err != nil || len(hero.HeroTrees) != 1 || hero.ClassTalents != nil || hero.Gates != nil {
		t.Errorf("hero section: %+v %v", hero, err)
	}
	if _, err := c.Talents(ctx, "Mage", "Arcane", "hero", "spellslinger"); err == nil {
		t.Error("a hero tree missing from the tree data should be an error")
	}
	if _, err := c.Talents(ctx, "Mage", "Arcane", "raid", ""); err == nil {
		t.Error("bad section should be an error")
	}
}
