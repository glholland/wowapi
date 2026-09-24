package blizzard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// CharacterSections maps a friendly section name to its Profile API path suffix.
var CharacterSections = map[string]string{
	"summary":          "",
	"equipment":        "/equipment",
	"specializations":  "/specializations",
	"professions":      "/professions",
	"stats":            "/statistics",
	"reputations":      "/reputations",
	"quests":           "/quests",
	"completed_quests": "/quests/completed",
	"achievements":     "/achievements",
	"mounts":           "/collections/mounts",
	"pets":             "/collections/pets",
	"mythic_keystone":  "/mythic-keystone-profile",
	"pvp":              "/pvp-summary",
	"titles":           "/titles",
	"media":            "/character-media",
}

// SectionNames returns the valid character section names, sorted.
func SectionNames() []string {
	names := make([]string, 0, len(CharacterSections))
	for k := range CharacterSections {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// RealmSlug converts a realm display name ("Area 52", "Mal'Ganis") to its slug
// ("area-52", "malganis").
func RealmSlug(realm string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(realm)) {
		switch {
		case r == ' ' || r == '-':
			b.WriteRune('-')
		case r == '\'' || r == '’':
			// dropped
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Character fetches one section of a character's profile (see CharacterSections).
func (c *Client) Character(ctx context.Context, realm, name, section string) ([]byte, error) {
	if section == "" {
		section = "summary"
	}
	suffix, ok := CharacterSections[section]
	if !ok {
		return nil, fmt.Errorf("unknown section %q (valid: %s)", section, strings.Join(SectionNames(), ", "))
	}
	if realm == "" || name == "" {
		return nil, fmt.Errorf("realm and character name are required")
	}
	path := fmt.Sprintf("/profile/wow/character/%s/%s%s",
		url.PathEscape(RealmSlug(realm)), url.PathEscape(strings.ToLower(name)), suffix)
	return c.Get(ctx, path, Profile, nil)
}

// Item fetches an item by ID.
func (c *Client) Item(ctx context.Context, id int) ([]byte, error) {
	return c.get(ctx, fmt.Sprintf("/data/wow/item/%d", id), Static, nil, time.Hour)
}

// ItemHit is a condensed item search result.
type ItemHit struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Quality string `json:"quality,omitempty"`
	Level   int    `json:"item_level,omitempty"`
}

// SearchItems finds items whose name contains the query, newest items first.
func (c *Client) SearchItems(ctx context.Context, name string, limit int) ([]ItemHit, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	q := url.Values{}
	q.Set("name."+c.Locale, name)
	q.Set("orderby", "id:desc")
	q.Set("_pageSize", strconv.Itoa(limit))
	body, err := c.get(ctx, "/data/wow/search/item", Static, q, time.Hour)
	if err != nil {
		return nil, err
	}
	var res struct {
		Results []struct {
			Data struct {
				ID      int             `json:"id"`
				Name    json.RawMessage `json:"name"`
				Level   int             `json:"level"`
				Quality struct {
					Type string `json:"type"`
				} `json:"quality"`
			} `json:"data"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("item search response: %w", err)
	}
	hits := make([]ItemHit, 0, len(res.Results))
	for _, r := range res.Results {
		hits = append(hits, ItemHit{ID: r.Data.ID, Name: c.localized(r.Data.Name), Quality: r.Data.Quality.Type, Level: r.Data.Level})
	}
	return hits, nil
}

// localized reads a name that is either a plain string or a {"en_US": ...} map.
func (c *Client) localized(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var m map[string]string
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	if s = m[c.Locale]; s == "" {
		s = m["en_US"]
	}
	return s
}

// Profession returns the profession index (id == 0), a profession (tier == 0),
// or one skill tier of a profession (e.g. "Midnight Tailoring") with its recipes.
func (c *Client) Profession(ctx context.Context, id, tier int) ([]byte, error) {
	path := "/data/wow/profession/index"
	if id > 0 {
		path = fmt.Sprintf("/data/wow/profession/%d", id)
		if tier > 0 {
			path += fmt.Sprintf("/skill-tier/%d", tier)
		}
	}
	return c.get(ctx, path, Static, nil, time.Hour)
}

// Recipe fetches a recipe by ID (reagents, crafted item, etc.).
func (c *Client) Recipe(ctx context.Context, id int) ([]byte, error) {
	return c.get(ctx, fmt.Sprintf("/data/wow/recipe/%d", id), Static, nil, time.Hour)
}

// PriceLevel is the total quantity listed at one unit price.
type PriceLevel struct {
	UnitPrice string `json:"unit_price"`
	Copper    int64  `json:"copper"`
	Quantity  int64  `json:"quantity"`
}

// CommodityPrice summarizes region-wide Auction House listings for one commodity
// (stackable goods like cloth, herbs, reagents, consumables).
type CommodityPrice struct {
	ItemID        int          `json:"item_id"`
	Listed        bool         `json:"listed"`
	MinPrice      string       `json:"min_unit_price,omitempty"`
	TotalQuantity int64        `json:"total_quantity"`
	Cheapest      []PriceLevel `json:"cheapest_price_levels,omitempty"`
	SnapshotAge   string       `json:"snapshot_fetched_ago"`
	Note          string       `json:"note"`
}

type commodityIndex struct {
	levels  map[int][]PriceLevel // item ID -> price levels, cheapest first
	fetched time.Time
}

// Commodity returns the current Auction House price summary for a commodity.
// Blizzard refreshes the commodities snapshot roughly hourly and it is large
// (tens of MB), so it is fetched at most every 30 minutes and indexed in memory.
func (c *Client) Commodity(ctx context.Context, itemID int) (CommodityPrice, error) {
	idx, err := c.commodities(ctx)
	if err != nil {
		return CommodityPrice{}, err
	}
	out := CommodityPrice{
		ItemID:      itemID,
		SnapshotAge: time.Since(idx.fetched).Round(time.Second).String(),
		Note:        "Region-wide commodity listings. Gear and other non-stackable items are listed per realm and are not included.",
	}
	levels := idx.levels[itemID]
	if len(levels) == 0 {
		return out, nil
	}
	out.Listed = true
	out.MinPrice = levels[0].UnitPrice
	for _, l := range levels {
		out.TotalQuantity += l.Quantity
	}
	out.Cheapest = levels[:min(len(levels), 8)]
	return out, nil
}

func (c *Client) commodities(ctx context.Context) (*commodityIndex, error) {
	c.mu.Lock()
	idx := c.commodityIdx
	c.mu.Unlock()
	if idx != nil && time.Since(idx.fetched) < 30*time.Minute {
		return idx, nil
	}
	body, err := c.Get(ctx, "/data/wow/auctions/commodities", Dynamic, nil)
	if err != nil {
		return nil, err
	}
	var res struct {
		Auctions []struct {
			Item struct {
				ID int `json:"id"`
			} `json:"item"`
			Quantity  int64 `json:"quantity"`
			UnitPrice int64 `json:"unit_price"`
		} `json:"auctions"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("commodities response: %w", err)
	}
	byPrice := map[int]map[int64]int64{}
	for _, a := range res.Auctions {
		m := byPrice[a.Item.ID]
		if m == nil {
			m = map[int64]int64{}
			byPrice[a.Item.ID] = m
		}
		m[a.UnitPrice] += a.Quantity
	}
	idx = &commodityIndex{levels: make(map[int][]PriceLevel, len(byPrice)), fetched: time.Now()}
	for id, m := range byPrice {
		levels := make([]PriceLevel, 0, len(m))
		for p, q := range m {
			levels = append(levels, PriceLevel{UnitPrice: FormatGold(p), Copper: p, Quantity: q})
		}
		sort.Slice(levels, func(i, j int) bool { return levels[i].Copper < levels[j].Copper })
		idx.levels[id] = levels
	}
	c.mu.Lock()
	c.commodityIdx = idx
	c.mu.Unlock()
	return idx, nil
}

// FormatGold renders a copper amount as "12g 34s 56c".
func FormatGold(copper int64) string {
	g, s, cp := copper/10000, copper/100%100, copper%100
	switch {
	case g > 0:
		return fmt.Sprintf("%dg %02ds %02dc", g, s, cp)
	case s > 0:
		return fmt.Sprintf("%ds %02dc", s, cp)
	default:
		return fmt.Sprintf("%dc", cp)
	}
}

// Slim removes hypermedia noise ("_links" and "key": {"href": ...}) from an
// API response so it is cheaper for a model to read. Non-JSON input is
// returned unchanged.
func Slim(body []byte) []byte {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return body
	}
	out, err := json.Marshal(slim(v))
	if err != nil {
		return body
	}
	return out
}

func slim(v any) any {
	switch t := v.(type) {
	case map[string]any:
		delete(t, "_links")
		if k, ok := t["key"].(map[string]any); ok && len(k) == 1 && k["href"] != nil {
			delete(t, "key")
		}
		for k, child := range t {
			t[k] = slim(child)
		}
		return t
	case []any:
		for i, child := range t {
			t[i] = slim(child)
		}
		return t
	default:
		return v
	}
}
