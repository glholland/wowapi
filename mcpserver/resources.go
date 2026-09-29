package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/glholland/wowapi/blizzard"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Resource URIs. Path segments are percent-encoded names, e.g.
// wow://character/Area%2052/C%C3%ABldis/equipment.
const (
	uriRaces             = "wow://races"
	uriClasses           = "wow://classes"
	uriTemplateRace      = "wow://race/{race}"
	uriTemplateClass     = "wow://class/{class}"
	uriTemplateTalents   = "wow://talents/{class}/{spec}"
	uriTemplateCharacter = "wow://character/{realm}/{name}"
	uriTemplateSection   = "wow://character/{realm}/{name}/{section}"
)

func addResources(s *mcp.Server, c *blizzard.Client) {
	s.AddResource(&mcp.Resource{
		URI:         uriRaces,
		Name:        "races",
		Title:       "Playable races",
		Description: "Every selectable race: factions, allied-race flag, classes and racial ability names.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		races, err := c.Races(ctx, "")
		return jsonResource(req.Params.URI, RaceList{Races: races}, err)
	})

	s.AddResource(&mcp.Resource{
		URI:         uriClasses,
		Name:        "classes",
		Title:       "Playable classes",
		Description: "Every class with its specializations, role and primary stat.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		classes, err := c.Classes(ctx, "")
		return jsonResource(req.Params.URI, ClassList{Classes: classes}, err)
	})

	s.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: uriTemplateRace,
		Name:        "race",
		Title:       "Race details",
		Description: "One race with its racial abilities described.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		seg, err := segments(req.Params.URI, "race", 1)
		if err != nil {
			return nil, err
		}
		races, err := c.Races(ctx, seg[0])
		return jsonResource(req.Params.URI, RaceList{Races: races}, err)
	})

	s.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: uriTemplateClass,
		Name:        "class",
		Title:       "Class details",
		Description: "One class with spec descriptions, hero talent trees, PvP talents and races.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		seg, err := segments(req.Params.URI, "class", 1)
		if err != nil {
			return nil, err
		}
		classes, err := c.Classes(ctx, seg[0])
		return jsonResource(req.Params.URI, ClassList{Classes: classes}, err)
	})

	s.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: uriTemplateTalents,
		Name:        "talents",
		Title:       "Talent tree",
		Description: "A specialization's full talent tree, e.g. wow://talents/Mage/Arcane.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		seg, err := segments(req.Params.URI, "talents", 2)
		if err != nil {
			return nil, err
		}
		tree, err := c.Talents(ctx, seg[0], seg[1], "", "")
		return jsonResource(req.Params.URI, tree, err)
	})

	characterHandler := func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		seg, err := segments(req.Params.URI, "character", 2, 3)
		if err != nil {
			return nil, err
		}
		section := "summary"
		if len(seg) == 3 {
			section = seg[2]
		}
		body, err := c.Character(ctx, seg[0], seg[1], section)
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI: req.Params.URI, MIMEType: "application/json", Text: string(blizzard.Slim(body)),
		}}}, nil
	}
	s.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: uriTemplateCharacter,
		Name:        "character",
		Title:       "Character summary",
		Description: "A character's profile summary (level, class, spec, item level, last logout).",
		MIMEType:    "application/json",
	}, characterHandler)
	s.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: uriTemplateSection,
		Name:        "character-section",
		Title:       "Character profile section",
		Description: "One section of a character's profile. Sections: " + strings.Join(blizzard.SectionNames(), ", ") + ".",
		MIMEType:    "application/json",
	}, characterHandler)
}

// segments returns the unescaped path segments of a wow://<host>/... URI,
// requiring one of the given segment counts.
func segments(uri, host string, counts ...int) ([]string, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "wow" || u.Host != host {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	for _, n := range counts {
		if len(parts) != n {
			continue
		}
		for i, p := range parts {
			if parts[i], err = url.PathUnescape(p); err != nil || parts[i] == "" {
				return nil, mcp.ResourceNotFoundError(uri)
			}
		}
		return parts, nil
	}
	return nil, mcp.ResourceNotFoundError(uri)
}

func jsonResource(uri string, v any, err error) (*mcp.ReadResourceResult, error) {
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encoding %s: %w", uri, err)
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: "application/json", Text: string(b)}}}, nil
}
