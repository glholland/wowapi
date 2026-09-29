package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Summary is what an MCP server exposes, as seen by a client.
type Summary struct {
	Name      string       `json:"name"`
	Version   string       `json:"version"`
	Protocol  string       `json:"protocol_version"`
	Tools     []ToolInfo   `json:"tools"`
	Resources []NamedURI   `json:"resources"`
	Prompts   []PromptInfo `json:"prompts"`
}

// ToolInfo describes one tool.
type ToolInfo struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description"`
	Structured  bool   `json:"structured_output"`
}

// NamedURI describes a resource or resource template.
type NamedURI struct {
	URI         string `json:"uri"`
	Description string `json:"description"`
}

// PromptInfo describes one prompt.
type PromptInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Arguments   []string `json:"arguments,omitempty"`
	Required    []string `json:"required_arguments,omitempty"`
}

// Summarize connects an in-memory MCP client to s and collects what it
// exposes. It exercises the real protocol handshake, so it also verifies the
// server starts cleanly.
func Summarize(ctx context.Context, s *mcp.Server) (Summary, error) {
	serverT, clientT := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, serverT, nil)
	if err != nil {
		return Summary{}, err
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "wowapi-describe"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		return Summary{}, err
	}
	defer cs.Close()

	init := cs.InitializeResult()
	out := Summary{Name: init.ServerInfo.Name, Version: init.ServerInfo.Version, Protocol: init.ProtocolVersion}

	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		return Summary{}, err
	}
	for _, t := range tools.Tools {
		info := ToolInfo{Name: t.Name, Description: t.Description, Structured: t.OutputSchema != nil}
		if t.Annotations != nil {
			info.Title = t.Annotations.Title
		}
		out.Tools = append(out.Tools, info)
	}

	res, err := cs.ListResources(ctx, nil)
	if err != nil {
		return Summary{}, err
	}
	for _, r := range res.Resources {
		out.Resources = append(out.Resources, NamedURI{URI: r.URI, Description: r.Description})
	}
	tmpl, err := cs.ListResourceTemplates(ctx, nil)
	if err != nil {
		return Summary{}, err
	}
	for _, r := range tmpl.ResourceTemplates {
		out.Resources = append(out.Resources, NamedURI{URI: r.URITemplate, Description: r.Description})
	}

	ps, err := cs.ListPrompts(ctx, nil)
	if err != nil {
		return Summary{}, err
	}
	for _, p := range ps.Prompts {
		info := PromptInfo{Name: p.Name, Description: p.Description}
		for _, a := range p.Arguments {
			info.Arguments = append(info.Arguments, a.Name)
			if a.Required {
				info.Required = append(info.Required, a.Name)
			}
		}
		out.Prompts = append(out.Prompts, info)
	}
	return out, nil
}

// Describe writes a Summary of s as readable text, or as JSON when asJSON is
// set (used to generate the MCP bundle manifest).
func Describe(ctx context.Context, s *mcp.Server, w io.Writer, asJSON bool) error {
	sum, err := Summarize(ctx, s)
	if err != nil {
		return err
	}
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(sum)
	}

	fmt.Fprintf(w, "%s %s (protocol %s)\n", sum.Name, sum.Version, sum.Protocol)
	fmt.Fprintf(w, "\nTools (%d):\n", len(sum.Tools))
	for _, t := range sum.Tools {
		kind := "text"
		if t.Structured {
			kind = "structured"
		}
		fmt.Fprintf(w, "  %-26s %-10s %s\n", t.Name, kind, firstSentence(t.Description))
	}
	fmt.Fprintf(w, "\nResources (%d):\n", len(sum.Resources))
	for _, r := range sum.Resources {
		fmt.Fprintf(w, "  %-42s %s\n", r.URI, r.Description)
	}
	fmt.Fprintf(w, "\nPrompts (%d):\n", len(sum.Prompts))
	for _, p := range sum.Prompts {
		args := make([]string, 0, len(p.Arguments))
		for _, a := range p.Arguments {
			if contains(p.Required, a) {
				a += "*"
			}
			args = append(args, a)
		}
		fmt.Fprintf(w, "  %-18s (%s) %s\n", p.Name, strings.Join(args, ", "), p.Description)
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}
