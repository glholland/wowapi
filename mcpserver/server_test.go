package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glholland/wowapi/blizzard"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeData is served by the fake Battle.net API, keyed by path.
var fakeData = map[string]string{
	"/profile/wow/character/lightbringer/mage":      `{"_links":{"self":{"href":"x"}},"name":"Mage","level":90}`,
	"/profile/wow/character/area-52/mage/equipment": `{"equipped_items":[{"slot":{"type":"HEAD"}}]}`,
	// A race with no racial spells and no classes exercises empty lists.
	"/data/wow/playable-race/index":  `{"races":[{"id":1},{"id":2}]}`,
	"/data/wow/playable-race/1":      `{"id":1,"name":"Human","is_selectable":true,"faction":{"name":"Alliance"},"playable_classes":[{"id":8,"name":"Mage"}],"racial_spells":[{"id":5,"name":"Diplomacy"}]}`,
	"/data/wow/playable-race/2":      `{"id":2,"name":"Visitor","is_selectable":true,"faction":{"name":"Neutral"}}`,
	"/data/wow/spell/5":              `{"description":"Reputation gains increased."}`,
	"/data/wow/playable-class/index": `{"classes":[{"id":8,"name":"Mage"}]}`,
	"/data/wow/playable-class/8":     `{"id":8,"name":"Mage","power_type":{"name":"Mana"},"specializations":[{"id":62}]}`,
	"/data/wow/playable-specialization/62": `{"id":62,"name":"Arcane","playable_class":{"id":8,"name":"Mage"},"role":{"name":"Damage"},"primary_stat_type":{"name":"Intellect"},
		"hero_talent_trees":[{"id":39,"name":"Sunfury"}],
		"spec_talent_tree":{"key":{"href":"https://x/data/wow/talent-tree/658/playable-specialization/62"}}}`,
	"/data/wow/talent-tree/658/playable-specialization/62": `{"class_talent_nodes":[{"id":1,"display_row":1,"node_type":{"type":"CHOICE"},"ranks":[{"rank":1}]}],
		"spec_talent_nodes":[],"hero_talent_trees":[{"id":39,"name":"Sunfury","hero_talent_nodes":[]}]}`,
	"/profile/wow/character/lightbringer/mage/encounters/raids":        `{"expansions":[{"expansion":{"id":1,"name":"Classic"},"instances":[]}]}`,
	"/profile/wow/character/lightbringer/mage/mythic-keystone-profile": `{"character":{"name":"Mage"}}`,
	"/data/wow/mythic-keystone/season/index":                           `{"current_season":{"id":18}}`,
	"/data/wow/mythic-keystone/season/18":                              `{"season_name":"Season 2"}`,
	"/data/wow/search/item":                                            `{"results":[]}`,
	"/data/wow/auctions/commodities":                                   `{"auctions":[{"item":{"id":7},"quantity":2,"unit_price":150}]}`,
}

func newTestSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token":"tok","expires_in":86399}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if body, ok := fakeData[r.URL.Path]; ok {
			w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	})
	api := httptest.NewServer(mux)
	t.Cleanup(api.Close)

	c := blizzard.New("id", "secret", "us", "en_US")
	c.TokenURL, c.APIBase = api.URL+"/token", api.URL
	t.Setenv("WOW_REALM", "Lightbringer")
	t.Setenv("WOW_CHARACTER", "Mage")

	ctx := context.Background()
	serverT, clientT := mcp.NewInMemoryTransports()
	ss, err := New(c, "test").Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: protocol error: %v", name, err)
	}
	return res
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestToolsAdvertiseSchemas(t *testing.T) {
	cs := newTestSession(t)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	structured := map[string]bool{
		"wow_character_encounters": true, "wow_mythic_plus": true, "wow_profession_recipes": true,
		"wow_races": true, "wow_classes": true, "wow_talents": true, "wow_item_search": true, "wow_commodity_price": true,
	}
	if len(res.Tools) != 13 {
		t.Errorf("want 13 tools, got %d", len(res.Tools))
	}
	for _, tool := range res.Tools {
		if tool.InputSchema == nil {
			t.Errorf("%s has no input schema", tool.Name)
		}
		if got := tool.OutputSchema != nil; got != structured[tool.Name] {
			t.Errorf("%s: output schema present = %v, want %v", tool.Name, got, structured[tool.Name])
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.Annotations.Title == "" {
			t.Errorf("%s should be annotated read-only with a title", tool.Name)
		}
	}
}

// Structured output is validated against the schema by the SDK, so empty
// lists (nil slices) must still produce valid output.
func TestStructuredToolsValidateWithEmptyLists(t *testing.T) {
	cs := newTestSession(t)
	for _, tc := range []struct {
		tool string
		args map[string]any
		want string
	}{
		{"wow_races", nil, `"Visitor"`},
		{"wow_races", map[string]any{"name": "human"}, "Reputation gains increased."},
		{"wow_classes", map[string]any{"name": "Mage"}, `"Sunfury"`},
		{"wow_talents", map[string]any{"specialization": "Arcane"}, "does not list its options"},
		{"wow_character_encounters", map[string]any{"kind": "raids"}, `"Classic"`},
		{"wow_mythic_plus", nil, "no Mythic+ runs"},
		{"wow_item_search", map[string]any{"name": "nothing"}, `"items"`},
		{"wow_commodity_price", map[string]any{"item_id": 7}, `"1s 50c"`},
	} {
		res := callTool(t, cs, tc.tool, tc.args)
		if res.IsError {
			t.Errorf("%s %v: tool error: %s", tc.tool, tc.args, text(res))
			continue
		}
		if res.StructuredContent == nil {
			t.Errorf("%s: no structured content", tc.tool)
		}
		b, _ := json.Marshal(res.StructuredContent)
		if !bytes.Contains(b, []byte(tc.want)) {
			t.Errorf("%s %v: structured content missing %s: %s", tc.tool, tc.args, tc.want, b)
		}
	}
}

func TestRawToolAndErrors(t *testing.T) {
	cs := newTestSession(t)

	res := callTool(t, cs, "wow_character", nil)
	if res.IsError || res.StructuredContent != nil || !strings.Contains(text(res), `"level":90`) || strings.Contains(text(res), "_links") {
		t.Errorf("raw tool should return slimmed text only: %+v %s", res, text(res))
	}
	// API and validation failures come back as tool errors, not protocol errors.
	if res := callTool(t, cs, "wow_character", map[string]any{"section": "nope"}); !res.IsError || !strings.Contains(text(res), "unknown section") {
		t.Errorf("bad section should be a tool error: %s", text(res))
	}
	if res := callTool(t, cs, "wow_api_get", map[string]any{"path": "/etc/passwd", "namespace": "static"}); !res.IsError {
		t.Error("paths outside the WoW API must be rejected")
	}
	if res := callTool(t, cs, "wow_talents", map[string]any{"specialization": "Arcane", "section": "raid"}); !res.IsError {
		t.Error("bad section should be a tool error")
	}
}

func TestResources(t *testing.T) {
	cs := newTestSession(t)
	ctx := context.Background()
	for uri, want := range map[string]string{
		"wow://races":                              `"Human"`,
		"wow://race/Human":                         "Reputation gains increased.",
		"wow://classes":                            `"Arcane"`,
		"wow://talents/Mage/Arcane":                `"talent_tree_id":658`,
		"wow://character/Lightbringer/Mage":        `"level":90`,
		"wow://character/Area%2052/Mage/equipment": `"HEAD"`,
	} {
		res, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
		if err != nil {
			t.Errorf("%s: %v", uri, err)
			continue
		}
		if len(res.Contents) != 1 || res.Contents[0].MIMEType != "application/json" || !strings.Contains(res.Contents[0].Text, want) {
			t.Errorf("%s: want %s in %+v", uri, want, res.Contents)
		}
	}
	if _, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "wow://character/OnlyRealm"}); err == nil {
		t.Error("an incomplete character URI should fail")
	}
}

func TestPrompts(t *testing.T) {
	cs := newTestSession(t)
	ctx := context.Background()
	list, err := cs.ListPrompts(ctx, nil)
	if err != nil || len(list.Prompts) != 3 {
		t.Fatalf("want 3 prompts: %+v %v", list, err)
	}
	res, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "new_character", Arguments: map[string]string{"goal": "tank for Mythic+"}})
	if err != nil {
		t.Fatal(err)
	}
	msg := res.Messages[0].Content.(*mcp.TextContent).Text
	for _, want := range []string{"tank for Mythic+", "Faction: either", "wow_classes", "wow_talents"} {
		if !strings.Contains(msg, want) {
			t.Errorf("new_character prompt missing %q:\n%s", want, msg)
		}
	}
	res, err = cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "profession_plan", Arguments: map[string]string{"profession": "Tailoring", "name": "Cëldis"}})
	if err != nil || !strings.Contains(res.Messages[0].Content.(*mcp.TextContent).Text, `profession="Tailoring" and sources=true for Cëldis`) {
		t.Errorf("profession_plan prompt: %+v %v", res, err)
	}
	if _, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "profession_plan"}); err == nil {
		t.Error("a missing required argument should fail")
	}
}

func TestDescribe(t *testing.T) {
	var buf bytes.Buffer
	if err := Describe(context.Background(), New(blizzard.New("", "", "us", "en_US"), "1.2.3"), &buf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"wowapi 1.2.3", "Tools (13)", "Resources (7)", "Prompts (3)", "wow_talents                structured"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("describe output missing %q:\n%s", want, buf.String())
		}
	}
}
