// Package fixtures serves trimmed copies of real Blizzard API responses to
// tests, so no test ever touches the network. Test fake servers call
// Respond first for the namespaces it knows, then fall back to their own
// retail fixtures.
//
// See forever/README.md for how the WoW: Forever responses were captured.
package fixtures

import (
	"embed"
	"net/http"
	"net/url"
	"strings"
)

//go:embed forever/*.json
var foreverFS embed.FS

// NotFoundBody is what Blizzard returns for a missing resource in a namespace
// it recognizes.
const NotFoundBody = `{"code":404,"type":"BLZWEBAPI00000404","detail":"Not Found"}`

// ForbiddenBody is what Blizzard returns for a namespace it does not
// recognize.
const ForbiddenBody = `{"code":403,"type":"BLZWEBAPI00000403","detail":"Forbidden"}`

// Namespaces the fake servers recognize; anything else gets a 403, as it does
// from the real API.
var known = map[string]bool{
	"static-us": true, "dynamic-us": true, "profile-us": true,
	"static-forever-us": true, "dynamic-forever-us": true, "profile-forever-us": true,
}

// Known reports whether the real API would recognize the namespace.
func Known(ns string) bool { return known[ns] }

// Respond answers a request for a WoW: Forever namespace. It reports ok=false
// when ns is not a Forever namespace, so the caller can serve it itself.
//
// Mirroring the API on 2026-11-05: static-forever-us has game data, while
// dynamic-forever-us and profile-forever-us are valid namespaces that hold no
// data yet, so every path in them is a 404.
func Respond(ns, path string, query url.Values) (status int, body []byte, ok bool) {
	switch ns {
	case "dynamic-forever-us", "profile-forever-us":
		return http.StatusNotFound, []byte(NotFoundBody), true
	case "static-forever-us":
	default:
		return 0, nil, false
	}
	if b, found := foreverFile(path, query); found {
		return http.StatusOK, b, true
	}
	return http.StatusNotFound, []byte(NotFoundBody), true
}

// foreverFile maps /data/wow/playable-race/95 to forever/playable-race__95.json.
func foreverFile(path string, query url.Values) ([]byte, bool) {
	rest, ok := strings.CutPrefix(path, "/data/wow/")
	if !ok {
		return nil, false
	}
	// The one canned search: items named "linen".
	if rest == "search/item" && query.Get("name.en_US") != "linen" {
		return nil, false
	}
	b, err := foreverFS.ReadFile("forever/" + strings.ReplaceAll(rest, "/", "__") + ".json")
	return b, err == nil
}
