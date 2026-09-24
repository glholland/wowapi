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
		case r.URL.Path == "/data/wow/search/item" && ns == "static-us":
			if r.URL.Query().Get("name.en_US") != "cloth" {
				http.Error(w, "bad query", http.StatusBadRequest)
				return
			}
			w.Write([]byte(`{"results":[
				{"data":{"id":1,"name":{"en_US":"Linen Cloth","de_DE":"Leinenstoff"},"level":5,"quality":{"type":"COMMON"}}},
				{"data":{"id":2,"name":"Silk Cloth","level":15,"quality":{"type":"COMMON"}}}]}`))
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
