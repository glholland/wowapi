package blizzard

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Static game data for character creation: races, classes, specializations
// and talent trees. It changes only with patches, so it is cached for a day.
const staticTTL = 24 * time.Hour

// ---- Races ------------------------------------------------------------------

// Race is a playable race, merged across its per-faction records (the API has
// separate Alliance and Horde entries for races like Dracthyr and Earthen).
type Race struct {
	Name       string   `json:"name"`
	IDs        []int    `json:"ids"`
	Factions   []string `json:"factions"`
	AlliedRace bool     `json:"allied_race"`
	Classes    []string `json:"classes"`
	Racials    []Racial `json:"racials"`
}

// Racial is a racial ability; Description is filled only for a single race.
type Racial struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type apiRace struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Selectable bool   `json:"is_selectable"`
	Allied     bool   `json:"is_allied_race"`
	Faction    struct {
		Name string `json:"name"`
	} `json:"faction"`
	Classes []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"playable_classes"`
	Racials []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"racial_spells"`
}

// Races lists every selectable race, or with name set, that one race with its
// racial abilities described.
func (c *Client) Races(ctx context.Context, name string) ([]Race, error) {
	body, err := c.get(ctx, "/data/wow/playable-race/index", Static, nil, staticTTL)
	if err != nil {
		return nil, err
	}
	var idx struct {
		Races []struct {
			ID int `json:"id"`
		} `json:"races"`
	}
	if err := json.Unmarshal(body, &idx); err != nil {
		return nil, fmt.Errorf("race index: %w", err)
	}
	ids := make([]int, len(idx.Races))
	for i, r := range idx.Races {
		ids[i] = r.ID
	}
	var mu sync.Mutex
	raw := map[int]apiRace{}
	if err := forEach(ctx, ids, func(ctx context.Context, id int) error {
		body, err := c.get(ctx, fmt.Sprintf("/data/wow/playable-race/%d", id), Static, nil, staticTTL)
		if err != nil {
			return err
		}
		var r apiRace
		if err := json.Unmarshal(body, &r); err != nil {
			return fmt.Errorf("race %d: %w", id, err)
		}
		mu.Lock()
		raw[id] = r
		mu.Unlock()
		return nil
	}); err != nil {
		return nil, err
	}
	realClasses, err := c.classNames(ctx)
	if err != nil {
		return nil, err
	}

	merged := map[string]*Race{}
	racialIDs := map[string][]int{}
	sort.Ints(ids)
	for _, id := range ids {
		r := raw[id]
		if !r.Selectable {
			continue // e.g. the faction-specific Pandaren records
		}
		m := merged[r.Name]
		if m == nil {
			m = &Race{Name: r.Name}
			merged[r.Name] = m
		}
		m.IDs = append(m.IDs, r.ID)
		m.Factions = appendUnique(m.Factions, r.Faction.Name)
		m.AlliedRace = m.AlliedRace || r.Allied
		for _, cl := range r.Classes {
			if realClasses[cl.ID] {
				m.Classes = appendUnique(m.Classes, cl.Name)
			}
		}
		for _, s := range r.Racials {
			// "Languages" differs per faction record and isn't a real ability.
			if s.Name != "Languages" && !containsRacial(m.Racials, s.Name) {
				m.Racials = append(m.Racials, Racial{Name: s.Name})
				racialIDs[r.Name] = append(racialIDs[r.Name], s.ID)
			}
		}
	}

	var out []Race
	for _, r := range merged {
		sort.Strings(r.Classes)
		sort.Strings(r.Factions)
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if name == "" {
		return out, nil
	}

	for _, r := range out {
		if !strings.EqualFold(r.Name, strings.TrimSpace(name)) {
			continue
		}
		spellIDs := racialIDs[r.Name]
		descs := make([]string, len(spellIDs))
		if err := forEachIndex(ctx, len(spellIDs), func(ctx context.Context, i int) error {
			d, err := c.spellDescription(ctx, spellIDs[i])
			descs[i] = d
			return err
		}); err != nil {
			return nil, err
		}
		for i := range r.Racials {
			r.Racials[i].Description = descs[i]
		}
		return []Race{r}, nil
	}
	names := make([]string, len(out))
	for i, r := range out {
		names[i] = r.Name
	}
	return nil, fmt.Errorf("no playable race %q (races: %s)", name, strings.Join(names, ", "))
}

func (c *Client) spellDescription(ctx context.Context, id int) (string, error) {
	body, err := c.get(ctx, fmt.Sprintf("/data/wow/spell/%d", id), Static, nil, staticTTL)
	if err != nil {
		return "", err
	}
	var s struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &s); err != nil {
		return "", fmt.Errorf("spell %d: %w", id, err)
	}
	return cleanText(s.Description), nil
}

// ---- Classes and specializations -------------------------------------------

// Class is a playable class with its specializations.
type Class struct {
	ID          int              `json:"id"`
	Name        string           `json:"name"`
	PowerType   string           `json:"power_type"`
	ExtraPowers []string         `json:"secondary_resources,omitempty"`
	Specs       []Specialization `json:"specializations"`
	Races       []string         `json:"races,omitempty"`
}

// Specialization is one class specialization. Description, HeroTrees and
// PvPTalents are filled for a single class.
type Specialization struct {
	ID          int         `json:"id"`
	Name        string      `json:"name"`
	Role        string      `json:"role"`
	PrimaryStat string      `json:"primary_stat"`
	PowerType   string      `json:"power_type,omitempty"`
	Description string      `json:"description,omitempty"`
	HeroTrees   []string    `json:"hero_talent_trees,omitempty"`
	PvPTalents  []TalentOpt `json:"pvp_talents,omitempty"`
}

type apiSpec struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Class struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"playable_class"`
	Role struct {
		Name string `json:"name"`
	} `json:"role"`
	Stat struct {
		Name string `json:"name"`
	} `json:"primary_stat_type"`
	Power struct {
		Name string `json:"name"`
	} `json:"power_type"`
	Description struct {
		Male string `json:"male"`
	} `json:"gender_description"`
	HeroTrees []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"hero_talent_trees"`
	PvPTalents []struct {
		Talent struct {
			Name string `json:"name"`
		} `json:"talent"`
		Tooltip apiTooltip `json:"spell_tooltip"`
	} `json:"pvp_talents"`
	TalentTree struct {
		Key struct {
			Href string `json:"href"`
		} `json:"key"`
	} `json:"spec_talent_tree"`
}

func (c *Client) classIDs(ctx context.Context) ([]int, map[int]string, error) {
	body, err := c.get(ctx, "/data/wow/playable-class/index", Static, nil, staticTTL)
	if err != nil {
		return nil, nil, err
	}
	var idx struct {
		Classes []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"classes"`
	}
	if err := json.Unmarshal(body, &idx); err != nil {
		return nil, nil, fmt.Errorf("class index: %w", err)
	}
	ids := make([]int, 0, len(idx.Classes))
	names := map[int]string{}
	for _, cl := range idx.Classes {
		ids = append(ids, cl.ID)
		names[cl.ID] = cl.Name
	}
	sort.Ints(ids)
	return ids, names, nil
}

// classNames reports which class IDs are real playable classes (race records
// also list placeholder classes that are not in the class index).
func (c *Client) classNames(ctx context.Context) (map[int]bool, error) {
	ids, _, err := c.classIDs(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[int]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m, nil
}

func (c *Client) spec(ctx context.Context, id int) (apiSpec, error) {
	body, err := c.get(ctx, fmt.Sprintf("/data/wow/playable-specialization/%d", id), Static, nil, staticTTL)
	if err != nil {
		return apiSpec{}, err
	}
	var s apiSpec
	if err := json.Unmarshal(body, &s); err != nil {
		return apiSpec{}, fmt.Errorf("specialization %d: %w", id, err)
	}
	return s, nil
}

// Classes lists every class with its specializations (role, primary stat), or
// with name set, that one class with spec descriptions, hero talent trees,
// PvP talents and the races that can play it.
func (c *Client) Classes(ctx context.Context, name string) ([]Class, error) {
	ids, names, err := c.classIDs(ctx)
	if err != nil {
		return nil, err
	}
	if name != "" {
		var match []int
		for _, id := range ids {
			if strings.EqualFold(names[id], strings.TrimSpace(name)) || strconv.Itoa(id) == strings.TrimSpace(name) {
				match = append(match, id)
			}
		}
		if len(match) == 0 {
			all := make([]string, 0, len(ids))
			for _, id := range ids {
				all = append(all, names[id])
			}
			sort.Strings(all)
			return nil, fmt.Errorf("no playable class %q (classes: %s)", name, strings.Join(all, ", "))
		}
		ids = match
	}
	detail := name != ""
	out := make([]Class, len(ids))
	err = forEachIndex(ctx, len(ids), func(ctx context.Context, i int) error {
		cl, err := c.class(ctx, ids[i], detail)
		out[i] = cl
		return err
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (c *Client) class(ctx context.Context, id int, detail bool) (Class, error) {
	body, err := c.get(ctx, fmt.Sprintf("/data/wow/playable-class/%d", id), Static, nil, staticTTL)
	if err != nil {
		return Class{}, err
	}
	var res struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Power struct {
			Name string `json:"name"`
		} `json:"power_type"`
		Extra []struct {
			Name string `json:"name"`
		} `json:"additional_power_types"`
		Specs []struct {
			ID int `json:"id"`
		} `json:"specializations"`
		Races []struct {
			Name string `json:"name"`
		} `json:"playable_races"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return Class{}, fmt.Errorf("class %d: %w", id, err)
	}
	cl := Class{ID: res.ID, Name: res.Name, PowerType: res.Power.Name, Specs: make([]Specialization, len(res.Specs))}
	for _, e := range res.Extra {
		cl.ExtraPowers = append(cl.ExtraPowers, e.Name)
	}
	if detail {
		for _, r := range res.Races {
			cl.Races = appendUnique(cl.Races, r.Name)
		}
		sort.Strings(cl.Races)
	}
	err = forEachIndex(ctx, len(res.Specs), func(ctx context.Context, i int) error {
		s, err := c.spec(ctx, res.Specs[i].ID)
		if err != nil {
			return err
		}
		sp := Specialization{ID: s.ID, Name: s.Name, Role: s.Role.Name, PrimaryStat: s.Stat.Name}
		if detail {
			sp.PowerType = s.Power.Name
			sp.Description = cleanText(s.Description.Male)
			for _, h := range s.HeroTrees {
				sp.HeroTrees = append(sp.HeroTrees, h.Name)
			}
			for _, p := range s.PvPTalents {
				sp.PvPTalents = append(sp.PvPTalents, p.Tooltip.option(p.Talent.Name))
			}
		}
		cl.Specs[i] = sp
		return nil
	})
	return cl, err
}

// ---- Talent trees -----------------------------------------------------------

// TalentTree is a condensed spec talent tree: class, spec and hero talents
// with descriptions, in display order.
type TalentTree struct {
	Class        string      `json:"class"`
	Spec         string      `json:"specialization"`
	SpecID       int         `json:"specialization_id"`
	TreeID       int         `json:"talent_tree_id"`
	Gates        []string    `json:"gates,omitempty"`
	ClassTalents []Talent    `json:"class_talents,omitempty"`
	SpecTalents  []Talent    `json:"spec_talents,omitempty"`
	HeroTrees    []HeroTree  `json:"hero_talent_trees,omitempty"`
	PvPTalents   []TalentOpt `json:"pvp_talents,omitempty"`
	Note         string      `json:"note"`
}

// HeroTree is one hero talent tree available to the spec.
type HeroTree struct {
	Name    string   `json:"name"`
	Talents []Talent `json:"talents"`
}

// Talent is one node of a talent tree.
type Talent struct {
	NodeID    int         `json:"node_id"`
	Row       int         `json:"row"`
	Col       int         `json:"col"`
	Type      string      `json:"type"` // active, passive or choice
	MaxRank   int         `json:"max_rank,omitempty"`
	Free      bool        `json:"granted_free,omitempty"`
	Requires  []int       `json:"requires_node,omitempty"`
	TalentOpt             // for active/passive nodes
	Choices   []TalentOpt `json:"choices,omitempty"`
}

// TalentOpt is a talent (or one option of a choice node) and its tooltip.
type TalentOpt struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Cast        string `json:"cast,omitempty"`
	Cooldown    string `json:"cooldown,omitempty"`
	Cost        string `json:"cost,omitempty"`
	Range       string `json:"range,omitempty"`
}

type apiTooltip struct {
	Description string `json:"description"`
	CastTime    string `json:"cast_time"`
	Cooldown    string `json:"cooldown"`
	PowerCost   string `json:"power_cost"`
	Range       string `json:"range"`
}

func (t apiTooltip) option(name string) TalentOpt {
	return TalentOpt{Name: name, Description: cleanText(t.Description), Cast: t.CastTime, Cooldown: t.Cooldown, Cost: t.PowerCost, Range: t.Range}
}

type apiTalentChoice struct {
	Talent struct {
		Name string `json:"name"`
	} `json:"talent"`
	Tooltip apiTooltip `json:"spell_tooltip"`
}

type apiNode struct {
	ID       int   `json:"id"`
	Row      int   `json:"display_row"`
	Col      int   `json:"display_col"`
	LockedBy []int `json:"locked_by"`
	Type     struct {
		Type string `json:"type"`
	} `json:"node_type"`
	Ranks []struct {
		Rank          int               `json:"rank"`
		DefaultPoints int               `json:"default_points"`
		Tooltip       *apiTalentChoice  `json:"tooltip"`
		Choices       []apiTalentChoice `json:"choice_of_tooltips"`
	} `json:"ranks"`
}

func (n apiNode) condense() Talent {
	t := Talent{NodeID: n.ID, Row: n.Row, Col: n.Col, Type: strings.ToLower(n.Type.Type), MaxRank: len(n.Ranks), Requires: n.LockedBy}
	for _, r := range n.Ranks {
		if r.DefaultPoints > 0 {
			t.Free = true
		}
		if r.Tooltip != nil { // the last rank's text describes the fully-ranked talent
			t.TalentOpt = r.Tooltip.Tooltip.option(r.Tooltip.Talent.Name)
		}
		if len(r.Choices) > 0 {
			t.Choices = t.Choices[:0]
			for _, ch := range r.Choices {
				t.Choices = append(t.Choices, ch.Tooltip.option(ch.Talent.Name))
			}
		}
	}
	if t.Type == "choice" && len(t.Choices) == 0 {
		t.Name = "(choice node: Blizzard's API does not list its options)"
	}
	return t
}

func condenseNodes(nodes []apiNode) []Talent {
	out := make([]Talent, len(nodes))
	for i, n := range nodes {
		out[i] = n.condense()
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Row != out[j].Row {
			return out[i].Row < out[j].Row
		}
		return out[i].Col < out[j].Col
	})
	return out
}

var talentTreeRE = regexp.MustCompile(`/talent-tree/(\d+)/playable-specialization/`)

// resolveSpec finds a specialization by name or ID. class disambiguates names
// shared by several classes ("Frost", "Holy", "Restoration", "Protection").
func (c *Client) resolveSpec(ctx context.Context, class, spec string) (apiSpec, error) {
	spec, class = strings.TrimSpace(spec), strings.TrimSpace(class)
	if id, err := strconv.Atoi(spec); err == nil {
		return c.spec(ctx, id)
	}
	classes, err := c.Classes(ctx, "")
	if err != nil {
		return apiSpec{}, err
	}
	var matches []string
	var found *Specialization
	for _, cl := range classes {
		if class != "" && !strings.EqualFold(cl.Name, class) {
			continue
		}
		for i := range cl.Specs {
			if strings.EqualFold(cl.Specs[i].Name, spec) {
				matches = append(matches, cl.Specs[i].Name+" "+cl.Name)
				found = &cl.Specs[i]
			}
		}
	}
	switch len(matches) {
	case 0:
		var all []string
		for _, cl := range classes {
			if class != "" && !strings.EqualFold(cl.Name, class) {
				continue
			}
			for _, s := range cl.Specs {
				all = append(all, s.Name+" "+cl.Name)
			}
		}
		if len(all) == 0 {
			return apiSpec{}, fmt.Errorf("no playable class %q", class)
		}
		return apiSpec{}, fmt.Errorf("no specialization %q (options: %s)", spec, strings.Join(all, ", "))
	case 1:
		return c.spec(ctx, found.ID)
	default:
		return apiSpec{}, fmt.Errorf("specialization %q is ambiguous; give the class too (%s)", spec, strings.Join(matches, ", "))
	}
}

// Talents returns a spec's talent tree. section limits it to "class", "spec",
// "hero" or "pvp" (empty = all); hero keeps only hero trees whose name
// contains it.
func (c *Client) Talents(ctx context.Context, class, spec, section, hero string) (TalentTree, error) {
	s, err := c.resolveSpec(ctx, class, spec)
	if err != nil {
		return TalentTree{}, err
	}
	section = strings.ToLower(strings.TrimSpace(section))
	switch section {
	case "", "all":
		section = ""
	case "class", "spec", "hero", "pvp":
	default:
		return TalentTree{}, fmt.Errorf("section must be class, spec, hero or pvp, got %q", section)
	}
	out := TalentTree{
		Class:  s.Class.Name,
		Spec:   s.Name,
		SpecID: s.ID,
		Note: "Talent text uses base values; actual numbers scale with the character's stats. " +
			"granted_free talents come with the spec. requires_node lists nodes that must be taken first.",
	}
	if section == "" || section == "pvp" {
		for _, p := range s.PvPTalents {
			out.PvPTalents = append(out.PvPTalents, p.Tooltip.option(p.Talent.Name))
		}
		if section == "pvp" {
			return out, nil
		}
	}

	m := talentTreeRE.FindStringSubmatch(s.TalentTree.Key.Href)
	if m == nil {
		return TalentTree{}, fmt.Errorf("%s %s has no talent tree in the API", s.Name, s.Class.Name)
	}
	out.TreeID, _ = strconv.Atoi(m[1])
	body, err := c.get(ctx, fmt.Sprintf("/data/wow/talent-tree/%d/playable-specialization/%d", out.TreeID, s.ID), Static, nil, staticTTL)
	if err != nil {
		return TalentTree{}, err
	}
	var tree struct {
		ClassNodes []apiNode `json:"class_talent_nodes"`
		SpecNodes  []apiNode `json:"spec_talent_nodes"`
		HeroTrees  []struct {
			ID    int       `json:"id"`
			Name  string    `json:"name"`
			Nodes []apiNode `json:"hero_talent_nodes"`
		} `json:"hero_talent_trees"`
		Gates []struct {
			ForClass bool    `json:"is_for_class"`
			Points   int     `json:"required_points"`
			Row      float64 `json:"restricted_row"`
		} `json:"restriction_lines"`
	}
	if err := json.Unmarshal(body, &tree); err != nil {
		return TalentTree{}, fmt.Errorf("talent tree: %w", err)
	}
	for _, g := range tree.Gates {
		which := "spec"
		if g.ForClass {
			which = "class"
		}
		if (section == "" || section == which) && g.Points > 0 {
			out.Gates = append(out.Gates, fmt.Sprintf("%s tree: %d points spent before row %d", which, g.Points, int(g.Row+0.5)))
		}
	}
	if section == "" || section == "class" {
		out.ClassTalents = condenseNodes(tree.ClassNodes)
	}
	if section == "" || section == "spec" {
		out.SpecTalents = condenseNodes(tree.SpecNodes)
	}
	if section == "" || section == "hero" {
		// The tree lists every hero tree of the class; keep the spec's own.
		allowed := map[int]bool{}
		for _, h := range s.HeroTrees {
			allowed[h.ID] = true
		}
		for _, h := range tree.HeroTrees {
			if !allowed[h.ID] || (hero != "" && !strings.Contains(strings.ToLower(h.Name), strings.ToLower(hero))) {
				continue
			}
			out.HeroTrees = append(out.HeroTrees, HeroTree{Name: h.Name, Talents: condenseNodes(h.Nodes)})
		}
		if hero != "" && len(out.HeroTrees) == 0 {
			var names []string
			for _, h := range s.HeroTrees {
				names = append(names, h.Name)
			}
			return TalentTree{}, fmt.Errorf("%s %s has no hero tree matching %q (has: %s)", s.Name, s.Class.Name, hero, strings.Join(names, ", "))
		}
	}
	return out, nil
}

// ---- helpers ----------------------------------------------------------------

func appendUnique(list []string, s string) []string {
	for _, v := range list {
		if v == s {
			return list
		}
	}
	return append(list, s)
}

func containsRacial(list []Racial, name string) bool {
	for _, r := range list {
		if r.Name == name {
			return true
		}
	}
	return false
}

// cleanText normalizes tooltip text: CRLF to LF, trimmed.
func cleanText(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}

// forEachIndex runs fn for 0..n-1 with limited concurrency.
func forEachIndex(ctx context.Context, n int, fn func(context.Context, int) error) error {
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	return forEach(ctx, idx, fn)
}
