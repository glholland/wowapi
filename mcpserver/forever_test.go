package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/glholland/wowapi/blizzard"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func foreverSession(t *testing.T) *mcp.ClientSession {
	return newSessionFor(t, blizzard.WithGame(blizzard.Forever))
}

func toolNames(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}

// Under WOW_GAME=forever only the tools the Forever API can serve exist.
func TestForeverOffersOnlyWhatWorks(t *testing.T) {
	cs := foreverSession(t)
	ctx := context.Background()

	got := strings.Join(toolNames(t, cs), " ")
	want := "wow_api_get wow_classes wow_item wow_item_search wow_profession wow_races wow_recipe wow_talents"
	if got != want {
		t.Errorf("forever tools:\n got  %s\n want %s", got, want)
	}
	for _, gone := range []string{"wow_character", "wow_character_encounters", "wow_mythic_plus", "wow_profession_recipes", "wow_commodity_price"} {
		if strings.Contains(" "+got+" ", " "+gone+" ") {
			t.Errorf("%s should not be offered for forever", gone)
		}
		if _, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: gone}); err == nil {
			t.Errorf("calling %s on forever should fail: the tool is not registered", gone)
		}
	}

	var uris []string
	res, _ := cs.ListResources(ctx, nil)
	for _, r := range res.Resources {
		uris = append(uris, r.URI)
	}
	tmpl, _ := cs.ListResourceTemplates(ctx, nil)
	for _, r := range tmpl.ResourceTemplates {
		uris = append(uris, r.URITemplate)
	}
	sort.Strings(uris)
	if strings.Join(uris, " ") != "wow://class/{class} wow://classes wow://race/{race} wow://races wow://talents/{class}/{spec}" {
		t.Errorf("forever resources: %v", uris)
	}
	if _, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "wow://character/Forever/Celdis"}); err == nil {
		t.Error("character resources should not exist for forever")
	}

	prompts, _ := cs.ListPrompts(ctx, nil)
	if len(prompts.Prompts) != 1 || prompts.Prompts[0].Name != "new_character" {
		t.Errorf("forever should offer only new_character: %+v", prompts.Prompts)
	}
	init := cs.InitializeResult()
	if !strings.Contains(init.ServerInfo.Title, "World of Warcraft: Forever") ||
		!strings.Contains(init.Instructions, "does not serve this game's character profiles") {
		t.Errorf("title and instructions should describe Forever: %q / %q", init.ServerInfo.Title, init.Instructions)
	}
}

func TestForeverNewCharacterPromptOmitsRetailConcepts(t *testing.T) {
	ctx := context.Background()
	prompt := func(cs *mcp.ClientSession) (text string, p *mcp.Prompt) {
		list, err := cs.ListPrompts(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "new_character", Arguments: map[string]string{"goal": "tank for dungeons"}})
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range list.Prompts {
			if p.Name == "new_character" {
				return res.Messages[0].Content.(*mcp.TextContent).Text, p
			}
		}
		t.Fatal("no new_character prompt")
		return "", nil
	}

	text, p := prompt(foreverSession(t))
	// Everything a model reads: the message, the description and the argument docs.
	all := text + " " + p.Description
	for _, a := range p.Arguments {
		all += " " + a.Description
	}
	for _, banned := range []string{"hero", "mythic"} {
		if strings.Contains(strings.ToLower(all), banned) {
			t.Errorf("the forever new_character prompt must not mention %q: %s", banned, all)
		}
	}
	for _, want := range []string{"World of Warcraft: Forever", "tank for dungeons", "original-style", "one tree per specialization", "wow_talents"} {
		if !strings.Contains(text, want) {
			t.Errorf("forever prompt missing %q: %s", want, text)
		}
	}

	// Retail keeps both.
	retail, rp := prompt(newTestSession(t))
	for _, want := range []string{"hero talent trees", "which hero tree suits"} {
		if !strings.Contains(retail, want) {
			t.Errorf("retail prompt lost %q", want)
		}
	}
	if !strings.Contains(rp.Arguments[0].Description, "Mythic+") {
		t.Errorf("retail goal examples should still mention Mythic+: %q", rp.Arguments[0].Description)
	}
}

func TestForeverToolsReturnForeverData(t *testing.T) {
	cs := foreverSession(t)
	structured := func(tool string, args map[string]any) string {
		res := callTool(t, cs, tool, args)
		if res.IsError {
			t.Fatalf("%s %v: %s", tool, args, text(res))
		}
		b, _ := json.Marshal(res.StructuredContent)
		return string(b)
	}

	races := structured("wow_races", nil)
	for _, want := range []string{"High Order Skyborne", "Windshaper Skyborne", `"Alliance"`, `"Horde"`} {
		if !strings.Contains(races, want) {
			t.Errorf("races should include %s: %s", want, races)
		}
	}
	classes := structured("wow_classes", map[string]any{"name": "Shaman"})
	if !strings.Contains(classes, "Elemental Combat") || strings.Contains(classes, `"role"`) || strings.Contains(classes, `"primary_stat"`) {
		t.Errorf("Shaman specs, without role or primary stat: %s", classes)
	}
	talents := structured("wow_talents", map[string]any{"specialization": "Elemental Combat"})
	if !strings.Contains(talents, `"talent_tree_id":1082`) || strings.Contains(talents, "hero_talent_trees") || strings.Contains(talents, "class_talents") {
		t.Errorf("a spec-only tree: %s", talents)
	}
	// The retail-only talent sections are a clear error, not a raw 404 or an empty tree.
	if res := callTool(t, cs, "wow_talents", map[string]any{"specialization": "Elemental Combat", "section": "hero"}); !res.IsError ||
		!strings.Contains(text(res), "WOW_GAME=forever does not support class, hero or PvP talents") {
		t.Errorf("section=hero should explain itself: %s", text(res))
	}
	if !strings.Contains(callTool(t, cs, "wow_item", map[string]any{"id": 19019}).Content[0].(*mcp.TextContent).Text, "Thunderfury") {
		t.Error("wow_item should read Forever's item data")
	}
}

// wow_api_get takes a kind (resolved for the configured game) or a full namespace.
func TestRawToolAcceptsKindsAndFullNamespaces(t *testing.T) {
	forever := foreverSession(t)
	raw := func(cs *mcp.ClientSession, ns string) *mcp.CallToolResult {
		return callTool(t, cs, "wow_api_get", map[string]any{"path": "/data/wow/playable-race/index", "namespace": ns})
	}
	if res := raw(forever, "static"); res.IsError || !strings.Contains(text(res), "Skyborne") {
		t.Errorf("a kind should resolve to static-forever-us: %s", text(res))
	}
	if res := raw(forever, "static-forever-us"); res.IsError || !strings.Contains(text(res), "Skyborne") {
		t.Errorf("a full Forever namespace: %s", text(res))
	}
	if res := raw(forever, "static-us"); res.IsError || strings.Contains(text(res), "Skyborne") {
		t.Errorf("a full retail namespace passes through even on a forever server: %s", text(res))
	}
	// A retail server can reach Forever by full namespace, which is how new games get probed.
	if res := raw(newTestSession(t), "static-forever-us"); res.IsError || !strings.Contains(text(res), "Skyborne") {
		t.Errorf("retail server, forever namespace: %s", text(res))
	}
	// 404 (known namespace, no data) and 403 (unknown namespace) are told apart.
	res := callTool(t, forever, "wow_api_get", map[string]any{"path": "/data/wow/realm/index", "namespace": "dynamic"})
	if !res.IsError || !strings.Contains(text(res), "HTTP 404") {
		t.Errorf("an unpopulated endpoint is a 404: %s", text(res))
	}
	res = raw(forever, "static-bogus-us")
	if !res.IsError || !strings.Contains(text(res), "HTTP 403") || !strings.Contains(text(res), "does not recognize the namespace") {
		t.Errorf("an unknown namespace is a 403 that says so: %s", text(res))
	}
	if res := raw(forever, "not a namespace"); !res.IsError || !strings.Contains(text(res), "full namespace such as static-forever-us") {
		t.Errorf("a malformed namespace should be rejected clearly: %s", text(res))
	}
}

func TestDescribeHonorsTheGame(t *testing.T) {
	forever := blizzard.New("", "", "us", "en_US", blizzard.WithGame(blizzard.Forever))
	var buf bytes.Buffer
	if err := Describe(context.Background(), New(forever, "1.2.3"), &buf, true); err != nil {
		t.Fatal(err)
	}
	var sum Summary
	if err := json.Unmarshal(buf.Bytes(), &sum); err != nil {
		t.Fatal(err)
	}
	if len(sum.Tools) != 8 || len(sum.Resources) != 5 || len(sum.Prompts) != 1 {
		t.Errorf("forever should list 8 tools, 5 resources and 1 prompt: %d/%d/%d", len(sum.Tools), len(sum.Resources), len(sum.Prompts))
	}
}
