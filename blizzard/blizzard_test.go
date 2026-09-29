package blizzard

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRealmSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Area 52":           "area-52",
		"Mal'Ganis":         "malganis",
		"Silvermoon":        "silvermoon",
		"  Twisting Nether": "twisting-nether",
		"Azjol-Nerub":       "azjol-nerub",
	} {
		if got := RealmSlug(in); got != want {
			t.Errorf("RealmSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatGold(t *testing.T) {
	for in, want := range map[int64]string{
		5:        "5c",
		1205:     "12s 05c",
		123456:   "12g 34s 56c",
		10000000: "1000g 00s 00c",
	} {
		if got := FormatGold(in); got != want {
			t.Errorf("FormatGold(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestSlim(t *testing.T) {
	in := `{"_links":{"self":{"href":"x"}},"id":1,"realm":{"key":{"href":"y"},"name":"Area 52","id":3676},"list":[{"_links":{},"key":{"href":"z"},"id":2}]}`
	got := string(Slim([]byte(in)))
	if strings.Contains(got, "href") || strings.Contains(got, "_links") {
		t.Fatalf("Slim left link noise: %s", got)
	}
	if !strings.Contains(got, `"name":"Area 52"`) || !strings.Contains(got, `"id":2`) {
		t.Fatalf("Slim dropped real data: %s", got)
	}
}

// fakeAPI serves a token endpoint and a few API paths, recording headers.
func fakeAPI(t *testing.T) (*Client, *int32) {
	t.Helper()
	var tokenCalls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tokenCalls, 1)
		if id, secret, ok := r.BasicAuth(); !ok || id != "id" || secret != "secret" {
			http.Error(w, "bad creds", http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"access_token":"tok","expires_in":86399}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "no auth", http.StatusUnauthorized)
			return
		}
		ns := r.Header.Get("Battlenet-Namespace")
		switch {
		case r.URL.EscapedPath() == "/profile/wow/character/area-52/c%C3%A9ldis/professions" && ns == "profile-us":
			w.Write([]byte(`{"primaries":[{"profession":{"name":"Tailoring","id":197}}]}`))
		case r.URL.Path == "/data/wow/auctions/commodities" && ns == "dynamic-us":
			w.Write([]byte(`{"auctions":[
				{"item":{"id":100},"quantity":5,"unit_price":20000},
				{"item":{"id":100},"quantity":3,"unit_price":15000},
				{"item":{"id":100},"quantity":2,"unit_price":15000},
				{"item":{"id":200},"quantity":1,"unit_price":99}]}`))
		case r.URL.Path == "/data/wow/search/item" && ns == "static-us" && r.URL.Query().Get("item_class.id") == "9":
			if r.URL.Query().Get("item_subclass.id") != "8" {
				w.Write([]byte(`{"pageCount":1,"results":[]}`))
				return
			}
			w.Write([]byte(`{"pageCount":1,"results":[
				{"data":{"id":9005,"name":{"en_US":"Formula: Enchant Ring - E"}}},
				{"data":{"id":9004,"name":{"en_US":"Formula: Old Recipe"}}},
				{"data":{"id":9002,"name":{"en_US":"Formula: Enchant Ring - B"}}},
				{"data":{"id":9001,"name":{"en_US":"Formula: Enchant Ring - A"}}}]}`))
		case strings.HasPrefix(r.URL.Path, "/data/wow/item/900") && ns == "static-us":
			binding, skill := "ON_EQUIP", "Midnight Enchanting"
			switch r.URL.Path {
			case "/data/wow/item/9002":
				binding = "ON_ACQUIRE"
			case "/data/wow/item/9004":
				binding, skill = "ON_ACQUIRE", "Outland Enchanting"
			}
			w.Write([]byte(`{"preview_item":{"binding":{"type":"` + binding + `"},"requirements":{"skill":{"display_string":"Requires ` + skill + ` (50)"}}}}`))
		case r.URL.Path == "/profile/wow/character/lightbringer/enchanter/professions" && ns == "profile-us":
			w.Write([]byte(`{"character":{"name":"Enchanter"},"primaries":[{"profession":{"id":333,"name":"Enchanting"},"tiers":[
				{"tier":{"id":2909,"name":"Midnight Enchanting"},"skill_points":74,"max_skill_points":100,"known_recipes":[]}]}]}`))
		case r.URL.Path == "/data/wow/profession/333/skill-tier/2909" && ns == "static-us":
			w.Write([]byte(`{"categories":[{"name":"Ring Enchants","recipes":[
				{"id":101,"name":"Enchant Ring - A"},{"id":102,"name":"Enchant Ring - B"},{"id":103,"name":"Enchant Ring - C"},
				{"id":104,"name":"Old Recipe"},{"id":105,"name":"Enchant Ring - E"}]}]}`))
		case r.URL.Path == "/data/wow/journal-expansion/index" && ns == "static-us":
			w.Write([]byte(`{"tiers":[{"id":514,"name":"The War Within"},{"id":516,"name":"Midnight"}]}`))
		case r.URL.Path == "/data/wow/journal-expansion/516" && ns == "static-us":
			w.Write([]byte(`{"dungeons":[{"id":1300}],"raids":[]}`))
		case r.URL.Path == "/data/wow/journal-instance/1300" && ns == "static-us":
			w.Write([]byte(`{"name":"Magisters' Terrace","encounters":[{"id":2662}]}`))
		case r.URL.Path == "/data/wow/journal-encounter/2662" && ns == "static-us":
			w.Write([]byte(`{"name":"Degentrius","items":[{"item":{"id":9001}},{"item":{"id":7}}]}`))
		case r.URL.Path == "/data/wow/realm/lightbringer" && ns == "dynamic-us":
			w.Write([]byte(`{"connected_realm":{"href":"https://us.api.blizzard.com/data/wow/connected-realm/3694?namespace=dynamic-us"}}`))
		case r.URL.Path == "/data/wow/connected-realm/3694/auctions" && ns == "dynamic-us":
			w.Write([]byte(`{"auctions":[{"item":{"id":9001},"buyout":5000000},{"item":{"id":9001},"buyout":3000000},{"item":{"id":42},"buyout":1}]}`))
		case strings.HasPrefix(r.URL.Path, "/data/wow/playable-") || r.URL.Path == "/data/wow/spell/20598" ||
			r.URL.Path == "/data/wow/talent-tree/658/playable-specialization/62":
			body, ok := creationData[r.URL.Path]
			if !ok || ns != "static-us" {
				http.NotFound(w, r)
				return
			}
			w.Write([]byte(body))
		case r.URL.Path == "/data/wow/search/item" && ns == "static-us":
			if r.URL.Query().Get("name.en_US") != "cloth" {
				http.Error(w, "bad query", http.StatusBadRequest)
				return
			}
			w.Write([]byte(`{"results":[
				{"data":{"id":1,"name":{"en_US":"Linen Cloth","de_DE":"Leinenstoff"},"level":5,"quality":{"type":"COMMON"}}},
				{"data":{"id":2,"name":"Silk Cloth","level":15,"quality":{"type":"COMMON"}}}]}`))
		case r.URL.Path == "/profile/wow/character/lightbringer/mage/encounters/dungeons" && ns == "profile-us":
			w.Write([]byte(`{"character":{"name":"Mage"},"expansions":[
				{"expansion":{"id":74,"name":"Mists of Pandaria"},"instances":[{"instance":{"id":246,"name":"Scholomance"},"modes":[
					{"difficulty":{"name":"Heroic"},"status":{"name":"Complete"},"progress":{"completed_count":1,"total_count":1,"encounters":[{"completed_count":2,"encounter":{"name":"Darkmaster Gandling"},"last_kill_timestamp":1349514513000}]}}]}]},
				{"expansion":{"id":516,"name":"Midnight"},"instances":[{"instance":{"id":1300,"name":"Magisters' Terrace"},"modes":[
					{"difficulty":{"name":"Normal"},"status":{"name":"Complete"},"progress":{"completed_count":1,"total_count":1,"encounters":[{"completed_count":1,"encounter":{"name":"Degentrius"},"last_kill_timestamp":1790629465000}]}},
					{"difficulty":{},"status":{"name":"Complete"},"progress":{"completed_count":1,"total_count":1}}]}]}]}`))
		case r.URL.Path == "/profile/wow/character/lightbringer/mage/mythic-keystone-profile" && ns == "profile-us":
			w.Write([]byte(`{"character":{"name":"Mage"},"current_period":{"best_runs":[{"dungeon":{"name":"The Blinding Vale"},"keystone_level":10,"duration":1802180,"is_completed_within_time":true,"map_rating":{"rating":300.04}}]}}`))
		case r.URL.Path == "/profile/wow/character/lightbringer/mage/mythic-keystone-profile/season/18" && ns == "profile-us":
			w.Write([]byte(`{"mythic_rating":{"rating":1234.56},"best_runs":[
				{"completed_timestamp":1790458374000,"dungeon":{"name":"Low Run"},"keystone_level":5,"duration":1500000,"is_completed_within_time":true,"map_rating":{"rating":200},
				 "keystone_affixes":[{"name":"Tyrannical"}],"members":[{"character":{"name":"Mage"},"specialization":{"name":"Arcane"},"equipped_item_level":250}]},
				{"completed_timestamp":1790458374000,"dungeon":{"name":"High Run"},"keystone_level":12,"duration":1802180,"is_completed_within_time":false,"map_rating":{"rating":350}}]}`))
		case r.URL.Path == "/data/wow/mythic-keystone/season/index" && ns == "dynamic-us":
			w.Write([]byte(`{"current_season":{"id":18},"seasons":[{"id":17},{"id":18}]}`))
		case r.URL.Path == "/data/wow/mythic-keystone/season/18" && ns == "dynamic-us":
			w.Write([]byte(`{"id":18,"season_name":"Midnight Season 2","start_timestamp":1786460400000}`))
		case r.URL.Path == "/data/wow/mythic-keystone/season/17" && ns == "dynamic-us":
			w.Write([]byte(`{"id":17,"season_name":"Midnight Season 1"}`))
		case r.URL.Path == "/profile/wow/character/lightbringer/mage/professions" && ns == "profile-us":
			w.Write([]byte(`{"character":{"name":"Mage"},"primaries":[{"profession":{"id":197,"name":"Tailoring"},"tiers":[
				{"tier":{"id":2540,"name":"Classic Tailoring"},"skill_points":300,"max_skill_points":300,"known_recipes":[{"id":1,"name":"Linen Bolt"}]},
				{"tier":{"id":2918,"name":"Midnight Tailoring"},"skill_points":72,"max_skill_points":100,"known_recipes":[{"id":10,"name":"Bright Linen Bolt"},{"id":12,"name":"Courtly Helm"}]}]}],
				"secondaries":[{"profession":{"id":794,"name":"Archaeology"},"skill_points":82}]}`))
		case r.URL.Path == "/data/wow/profession/197/skill-tier/2918" && ns == "static-us":
			w.Write([]byte(`{"categories":[
				{"name":"Woven Cloth","recipes":[{"id":10,"name":"Bright Linen Bolt"},{"id":11,"name":"Arcanoweave Bolt"}]},
				{"name":"Garments","recipes":[{"id":12,"name":"Courtly Helm"}]}]}`))
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New("id", "secret", "us", "en_US")
	c.TokenURL, c.APIBase = srv.URL+"/token", srv.URL
	return c, &tokenCalls
}

func TestCharacterAndTokenReuse(t *testing.T) {
	c, tokenCalls := fakeAPI(t)
	ctx := context.Background()
	for range 2 {
		body, err := c.Character(ctx, "Area 52", "Céldis", "professions")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "Tailoring") {
			t.Fatalf("unexpected body: %s", body)
		}
	}
	if n := atomic.LoadInt32(tokenCalls); n != 1 {
		t.Errorf("token fetched %d times, want 1", n)
	}
	if _, err := c.Character(ctx, "Area 52", "Céldis", "nope"); err == nil {
		t.Error("expected error for unknown section")
	}
	var apiErr *APIError
	if _, err := c.Character(ctx, "Area 52", "Nobody", "summary"); !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Errorf("want 404 APIError, got %v", err)
	}
}

func TestCommodity(t *testing.T) {
	c, _ := fakeAPI(t)
	p, err := c.Commodity(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Listed || p.TotalQuantity != 10 || p.MinPrice != "1g 50s 00c" {
		t.Fatalf("unexpected summary: %+v", p)
	}
	if len(p.Cheapest) != 2 || p.Cheapest[0].Quantity != 5 || p.Cheapest[1].Copper != 20000 {
		t.Fatalf("unexpected price levels: %+v", p.Cheapest)
	}
	if p, _ := c.Commodity(context.Background(), 999); p.Listed {
		t.Fatalf("unlisted item reported as listed: %+v", p)
	}
}

func TestSearchItemsHandlesBothNameShapes(t *testing.T) {
	c, _ := fakeAPI(t)
	hits, err := c.SearchItems(context.Background(), "cloth", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].Name != "Linen Cloth" || hits[1].Name != "Silk Cloth" {
		t.Fatalf("unexpected hits: %+v", hits)
	}
}
