package mcpserver

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Describe connects an in-memory MCP client to s and writes what it exposes:
// tools (marking those with structured output), resources, resource
// templates and prompts. It exercises the real protocol handshake, so it also
// verifies the server starts cleanly.
func Describe(ctx context.Context, s *mcp.Server, w io.Writer) error {
	serverT, clientT := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, serverT, nil)
	if err != nil {
		return err
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "wowapi-describe"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		return err
	}
	defer cs.Close()

	init := cs.InitializeResult()
	fmt.Fprintf(w, "%s %s (protocol %s)\n", init.ServerInfo.Name, init.ServerInfo.Version, init.ProtocolVersion)

	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "\nTools (%d):\n", len(tools.Tools))
	for _, t := range tools.Tools {
		kind := "text"
		if t.OutputSchema != nil {
			kind = "structured"
		}
		fmt.Fprintf(w, "  %-26s %-10s %s\n", t.Name, kind, firstSentence(t.Description))
	}

	res, err := cs.ListResources(ctx, nil)
	if err != nil {
		return err
	}
	tmpl, err := cs.ListResourceTemplates(ctx, nil)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "\nResources (%d):\n", len(res.Resources)+len(tmpl.ResourceTemplates))
	for _, r := range res.Resources {
		fmt.Fprintf(w, "  %-42s %s\n", r.URI, r.Description)
	}
	for _, r := range tmpl.ResourceTemplates {
		fmt.Fprintf(w, "  %-42s %s\n", r.URITemplate, r.Description)
	}

	ps, err := cs.ListPrompts(ctx, nil)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "\nPrompts (%d):\n", len(ps.Prompts))
	for _, p := range ps.Prompts {
		var args []string
		for _, a := range p.Arguments {
			if a.Required {
				args = append(args, a.Name+"*")
			} else {
				args = append(args, a.Name)
			}
		}
		fmt.Fprintf(w, "  %-18s (%s) %s\n", p.Name, strings.Join(args, ", "), p.Description)
	}
	return nil
}

func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}
