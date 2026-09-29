package blizzard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RecipeSource explains where a recipe comes from, as far as the Game Data
// APIs can tell. They know boss loot tables and the items that teach recipes,
// but not vendors, trainers, quests or specialization unlocks.
type RecipeSource struct {
	// Type is one of: boss_drop, tradable_item, bind_on_pickup_item, no_item.
	Type          string     `json:"type"`
	Summary       string     `json:"summary"`
	TaughtBy      *RecipeRef `json:"taught_by,omitempty"`
	RequiresSkill string     `json:"requires_skill,omitempty"`
	DroppedBy     []string   `json:"dropped_by,omitempty"` // "Boss (Instance)"
	Tradable      bool       `json:"tradable,omitempty"`
	AHListings    int        `json:"ah_listings,omitempty"`
	AHMinBuyout   string     `json:"ah_min_buyout,omitempty"`
}

// recipeItemSubclass maps a profession ID to the Recipe item class (9)
// subclass holding its patterns, formulas, schematics, etc.
var recipeItemSubclass = map[int]int{
	165: 1,  // Leatherworking
	197: 2,  // Tailoring
	202: 3,  // Engineering
	164: 4,  // Blacksmithing
	185: 5,  // Cooking
	171: 6,  // Alchemy
	333: 8,  // Enchanting
	356: 9,  // Fishing
	755: 10, // Jewelcrafting
	773: 11, // Inscription
}

// tierExpansionAliases maps the region word in a skill tier name ("Outland
// Tailoring") to the journal expansion name ("Burning Crusade").
var tierExpansionAliases = map[string]string{
	"outland":      "burning crusade",
	"northrend":    "wrath of the lich king",
	"pandaria":     "mists of pandaria",
	"draenor":      "warlords of draenor",
	"kul tiran":    "battle for azeroth",
	"zandalari":    "battle for azeroth",
	"dragon isles": "dragonflight",
	"khaz algar":   "the war within",
}

// taughtItem is a recipe-class item that teaches a recipe.
type taughtItem struct {
	ID   int
	Name string
}

// recipeItems indexes every recipe item for a profession by the name of the
// recipe it teaches ("Pattern: Arcanoweave Lining" -> "arcanoweave lining"),
// newest item first.
func (c *Client) recipeItems(ctx context.Context, professionID int) (map[string][]taughtItem, error) {
	sub, ok := recipeItemSubclass[professionID]
	if !ok {
		return nil, nil // e.g. Mining or Herbalism: nothing teaches recipes
	}
	q := url.Values{}
	q.Set("item_class.id", "9")
	q.Set("item_subclass.id", strconv.Itoa(sub))
	q.Set("orderby", "id:desc")
	q.Set("_pageSize", "1000")
	idx := map[string][]taughtItem{}
	for page := 1; ; page++ {
		q.Set("_page", strconv.Itoa(page))
		body, err := c.get(ctx, "/data/wow/search/item", Static, q, 24*time.Hour)
		if err != nil {
			return nil, err
		}
		var res struct {
			PageCount int `json:"pageCount"`
			Results   []struct {
				Data struct {
					ID   int             `json:"id"`
					Name json.RawMessage `json:"name"`
				} `json:"data"`
			} `json:"results"`
		}
		if err := json.Unmarshal(body, &res); err != nil {
			return nil, fmt.Errorf("recipe item search: %w", err)
		}
		for _, r := range res.Results {
			name := c.localized(r.Data.Name)
			_, taught, ok := strings.Cut(name, ": ")
			if !ok {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(taught))
			idx[key] = append(idx[key], taughtItem{ID: r.Data.ID, Name: name})
		}
		if page >= res.PageCount {
			break
		}
	}
	return idx, nil
}

// journalDrops maps item IDs to the bosses that drop them ("Boss (Instance)")
// across every dungeon and raid of one journal expansion.
func (c *Client) journalDrops(ctx context.Context, expansionID int) (map[int][]string, error) {
	c.mu.Lock()
	if d, ok := c.dropIdx[expansionID]; ok {
		c.mu.Unlock()
		return d, nil
	}
	c.mu.Unlock()

	body, err := c.get(ctx, fmt.Sprintf("/data/wow/journal-expansion/%d", expansionID), Static, nil, 24*time.Hour)
	if err != nil {
		return nil, err
	}
	var exp struct {
		Dungeons []struct{ ID int } `json:"dungeons"`
		Raids    []struct{ ID int } `json:"raids"`
	}
	if err := json.Unmarshal(body, &exp); err != nil {
		return nil, fmt.Errorf("journal expansion: %w", err)
	}
	var instances []int
	for _, d := range append(exp.Dungeons, exp.Raids...) {
		instances = append(instances, d.ID)
	}

	type encounterRef struct {
		ID       int
		Instance string
	}
	var (
		mu         sync.Mutex
		encounters []encounterRef
	)
	err = forEach(ctx, instances, func(ctx context.Context, id int) error {
		body, err := c.get(ctx, fmt.Sprintf("/data/wow/journal-instance/%d", id), Static, nil, 24*time.Hour)
		if err != nil {
			return err
		}
		var inst struct {
			Name       string `json:"name"`
			Encounters []struct {
				ID int `json:"id"`
			} `json:"encounters"`
		}
		if err := json.Unmarshal(body, &inst); err != nil {
			return fmt.Errorf("journal instance %d: %w", id, err)
		}
		mu.Lock()
		for _, e := range inst.Encounters {
			encounters = append(encounters, encounterRef{ID: e.ID, Instance: inst.Name})
		}
		mu.Unlock()
		return nil
	})
	if err != nil {
		return nil, err
	}

	drops := map[int][]string{}
	ids := make([]int, len(encounters))
	instanceOf := map[int]string{}
	for i, e := range encounters {
		ids[i] = e.ID
		instanceOf[e.ID] = e.Instance
	}
	err = forEach(ctx, ids, func(ctx context.Context, id int) error {
		body, err := c.get(ctx, fmt.Sprintf("/data/wow/journal-encounter/%d", id), Static, nil, 24*time.Hour)
		if err != nil {
			return err
		}
		var enc struct {
			Name  string `json:"name"`
			Items []struct {
				Item struct {
					ID int `json:"id"`
				} `json:"item"`
			} `json:"items"`
		}
		if err := json.Unmarshal(body, &enc); err != nil {
			return fmt.Errorf("journal encounter %d: %w", id, err)
		}
		src := fmt.Sprintf("%s (%s)", enc.Name, instanceOf[id])
		mu.Lock()
		for _, it := range enc.Items {
			drops[it.Item.ID] = append(drops[it.Item.ID], src)
		}
		mu.Unlock()
		return nil
	})
	if err != nil {
		return nil, err
	}
	for id := range drops {
		sort.Strings(drops[id])
	}

	c.mu.Lock()
	c.dropIdx[expansionID] = drops
	c.mu.Unlock()
	return drops, nil
}

// journalExpansionFor finds the journal expansion matching a skill tier name
// such as "Midnight Tailoring" or "Outland Enchanting". It returns 0 if none.
func (c *Client) journalExpansionFor(ctx context.Context, tierName, professionName string) (int, string, error) {
	region := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(tierName, professionName)))
	if alias, ok := tierExpansionAliases[region]; ok {
		region = alias
	}
	body, err := c.get(ctx, "/data/wow/journal-expansion/index", Static, nil, 24*time.Hour)
	if err != nil {
		return 0, "", err
	}
	var idx struct {
		Tiers []struct {
			ID   int             `json:"id"`
			Name json.RawMessage `json:"name"`
		} `json:"tiers"`
	}
	if err := json.Unmarshal(body, &idx); err != nil {
		return 0, "", fmt.Errorf("journal expansion index: %w", err)
	}
	for _, t := range idx.Tiers {
		if name := c.localized(t.Name); strings.ToLower(name) == region {
			return t.ID, name, nil
		}
	}
	return 0, "", nil
}

// itemDetail is the subset of an item we need to classify a recipe source.
type itemDetail struct {
	Binding string // ON_ACQUIRE, ON_EQUIP, TO_ACCOUNT, ... ("" = none)
	Skill   string // "Requires Midnight Tailoring (50)"
}

func (c *Client) itemDetail(ctx context.Context, id int) (itemDetail, error) {
	body, err := c.Item(ctx, id)
	if err != nil {
		return itemDetail{}, err
	}
	var it struct {
		Preview struct {
			Binding struct {
				Type string `json:"type"`
			} `json:"binding"`
			Requirements struct {
				Skill struct {
					Display string `json:"display_string"`
				} `json:"skill"`
			} `json:"requirements"`
		} `json:"preview_item"`
	}
	if err := json.Unmarshal(body, &it); err != nil {
		return itemDetail{}, fmt.Errorf("item %d: %w", id, err)
	}
	return itemDetail{Binding: it.Preview.Binding.Type, Skill: it.Preview.Requirements.Skill.Display}, nil
}

// realmListing is the realm Auction House summary for one item.
type realmListing struct {
	Count     int
	MinBuyout int64
}

var connectedRealmRE = regexp.MustCompile(`/connected-realm/(\d+)`)

// realmAuctions indexes a realm's (non-commodity) Auction House listings by
// item ID. The snapshot is several megabytes, so it is kept for 30 minutes.
func (c *Client) realmAuctions(ctx context.Context, realm string) (map[int]realmListing, error) {
	body, err := c.get(ctx, "/data/wow/realm/"+url.PathEscape(RealmSlug(realm)), Dynamic, nil, 24*time.Hour)
	if err != nil {
		return nil, err
	}
	var r struct {
		ConnectedRealm struct {
			Href string `json:"href"`
		} `json:"connected_realm"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("realm: %w", err)
	}
	m := connectedRealmRE.FindStringSubmatch(r.ConnectedRealm.Href)
	if m == nil {
		return nil, fmt.Errorf("realm %q: no connected realm in response", realm)
	}
	id, _ := strconv.Atoi(m[1])

	c.mu.Lock()
	if a, ok := c.auctionIdx[id]; ok && time.Since(a.fetched) < 30*time.Minute {
		c.mu.Unlock()
		return a.items, nil
	}
	c.mu.Unlock()

	body, err = c.Get(ctx, fmt.Sprintf("/data/wow/connected-realm/%d/auctions", id), Dynamic, nil)
	if err != nil {
		return nil, err
	}
	var res struct {
		Auctions []struct {
			Item struct {
				ID int `json:"id"`
			} `json:"item"`
			Buyout int64 `json:"buyout"`
		} `json:"auctions"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("realm auctions: %w", err)
	}
	items := map[int]realmListing{}
	for _, a := range res.Auctions {
		l := items[a.Item.ID]
		l.Count++
		if a.Buyout > 0 && (l.MinBuyout == 0 || a.Buyout < l.MinBuyout) {
			l.MinBuyout = a.Buyout
		}
		items[a.Item.ID] = l
	}
	c.mu.Lock()
	c.auctionIdx[id] = realmAuctionIndex{items: items, fetched: time.Now()}
	c.mu.Unlock()
	return items, nil
}

type realmAuctionIndex struct {
	items   map[int]realmListing
	fetched time.Time
}

// recipeSources fills in Source for every missing recipe. realm is used to
// price tradable recipe items on that realm's Auction House.
func (c *Client) recipeSources(ctx context.Context, realm string, professionID int, professionName, tierName string, missing []RecipeCategory) (string, error) {
	items, err := c.recipeItems(ctx, professionID)
	if err != nil {
		return "", err
	}
	var notes []string
	drops := map[int][]string{}
	expID, expName, err := c.journalExpansionFor(ctx, tierName, professionName)
	if err != nil {
		return "", err
	}
	if expID != 0 {
		if drops, err = c.journalDrops(ctx, expID); err != nil {
			return "", err
		}
	} else {
		notes = append(notes, fmt.Sprintf("no dungeon journal expansion matches %q, so boss drops were not checked", tierName))
	}

	// Look up every item whose name matches a missing recipe. Older expansions
	// reuse recipe names ("Arcanoweave Bracers" exists in Outland Tailoring
	// too), so a candidate only counts if it requires this skill tier.
	seen := map[int]bool{}
	var ids []int
	for _, cat := range missing {
		for _, r := range cat.Recipes {
			for _, it := range items[strings.ToLower(r.Name)] {
				if !seen[it.ID] {
					seen[it.ID] = true
					ids = append(ids, it.ID)
				}
			}
		}
	}
	details := make(map[int]itemDetail, len(ids))
	var mu sync.Mutex
	if err := forEach(ctx, ids, func(ctx context.Context, id int) error {
		d, err := c.itemDetail(ctx, id)
		if err != nil {
			return err
		}
		mu.Lock()
		details[id] = d
		mu.Unlock()
		return nil
	}); err != nil {
		return "", err
	}

	type job struct {
		recipe *RecipeRef
		item   taughtItem
	}
	var jobs []job
	tierLower := strings.ToLower(tierName)
	for ci := range missing {
		for ri := range missing[ci].Recipes {
			r := &missing[ci].Recipes[ri]
			var pick *taughtItem
			for _, it := range items[strings.ToLower(r.Name)] { // newest first
				skill := strings.ToLower(details[it.ID].Skill)
				if skill == "" || strings.Contains(skill, tierLower) {
					pick = &it
					break
				}
			}
			if pick == nil {
				r.Source = &RecipeSource{Type: "no_item", Summary: "No item teaches this: learned from the trainer, a specialization unlock or a quest."}
				continue
			}
			jobs = append(jobs, job{recipe: r, item: *pick})
		}
	}

	var ah map[int]realmListing
	for _, j := range jobs {
		if tradable(details[j.item.ID].Binding) {
			if ah, err = c.realmAuctions(ctx, realm); err != nil {
				notes = append(notes, "could not load realm Auction House: "+err.Error())
				ah = map[int]realmListing{}
			}
			break
		}
	}

	for _, j := range jobs {
		d := details[j.item.ID]
		src := &RecipeSource{
			TaughtBy:      &RecipeRef{ID: j.item.ID, Name: j.item.Name},
			RequiresSkill: strings.TrimPrefix(d.Skill, "Requires "),
			DroppedBy:     drops[j.item.ID],
			Tradable:      tradable(d.Binding),
		}
		var parts []string
		switch {
		case len(src.DroppedBy) > 0:
			src.Type = "boss_drop"
			parts = append(parts, fmt.Sprintf("%s drops from %s.", j.item.Name, strings.Join(src.DroppedBy, ", ")))
		case src.Tradable:
			src.Type = "tradable_item"
			parts = append(parts, fmt.Sprintf("Taught by %s, which can be traded.", j.item.Name))
		default:
			src.Type = "bind_on_pickup_item"
			parts = append(parts, fmt.Sprintf("Taught by %s (binds on pickup): a vendor, quest, treasure or world drop; the API doesn't say which.", j.item.Name))
		}
		if src.Tradable {
			if l := ah[j.item.ID]; l.Count > 0 {
				src.AHListings, src.AHMinBuyout = l.Count, FormatGold(l.MinBuyout)
				parts = append(parts, fmt.Sprintf("%d listed on %s from %s.", l.Count, realm, src.AHMinBuyout))
			} else if ah != nil {
				parts = append(parts, fmt.Sprintf("None listed on %s right now.", realm))
			}
		}
		src.Summary = strings.Join(parts, " ")
		j.recipe.Source = src
	}

	note := "Sources come from dungeon/raid loot tables and recipe items. Vendors, trainers, quests and specialization unlocks are not in Blizzard's API."
	if expName != "" {
		note += fmt.Sprintf(" Boss drops checked across %s dungeons and raids.", expName)
	}
	if len(notes) > 0 {
		note += " Note: " + strings.Join(notes, "; ") + "."
	}
	return note, nil
}

// tradable reports whether an item with this binding can be sold to other
// players (not bind-on-pickup or account-bound).
func tradable(binding string) bool {
	switch binding {
	case "ON_ACQUIRE", "TO_ACCOUNT", "TO_BNETACCOUNT", "QUEST":
		return false
	}
	return true
}

// forEach runs fn for every ID with limited concurrency and returns the first error.
func forEach(ctx context.Context, ids []int, fn func(context.Context, int) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sem := make(chan struct{}, 8)
	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
	)
	for _, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(id int) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := fn(ctx, id); err != nil {
				once.Do(func() { firstErr = err; cancel() })
			}
		}(id)
	}
	wg.Wait()
	return firstErr
}
