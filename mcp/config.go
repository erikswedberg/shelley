// Package mcp adds Model Context Protocol client support to Shelley.
//
// Servers are configured via the command line (--mcp name=URL, repeatable)
// or a JSON config file (--mcp-config path) whose shape mirrors the Claude
// Code / Claude Desktop convention:
//
//	{
//	  "mcpServers": {
//	    "figma": { "url": "http://host.sand:3845/mcp" },
//	    "foo":   { "url": "https://example.com/mcp",
//	              "headers": { "Authorization": "Bearer ..." } }
//	  }
//	}
//
// Only Streamable HTTP transport is supported. stdio / SSE / WebSocket
// transports are out of scope for this first cut.
package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// ServerConfig describes a single MCP server endpoint.
type ServerConfig struct {
	// Name is the user-facing identifier (also used as the tool-name prefix).
	Name string `json:"-"`
	// URL is the MCP endpoint (must be http:// or https://).
	URL string `json:"url"`
	// Headers are optional extra HTTP headers (e.g. Authorization).
	Headers map[string]string `json:"headers,omitempty"`
}

// jsonFile mirrors the {"mcpServers": {name: {url, headers, ...}}} shape.
type jsonFile struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

var nameRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,31}$`)

// LoadConfig parses zero or more --mcp flag values and an optional config
// file path into a list of ServerConfigs. Flag values override file entries
// with the same name.
func LoadConfig(flagValues []string, configPath string) ([]ServerConfig, error) {
	servers := map[string]ServerConfig{}

	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("mcp: read config %s: %w", configPath, err)
		}
		var f jsonFile
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("mcp: parse config %s: %w", configPath, err)
		}
		for name, sc := range f.MCPServers {
			sc.Name = name
			servers[name] = sc
		}
	}

	for _, v := range flagValues {
		name, url, ok := strings.Cut(v, "=")
		if !ok {
			return nil, fmt.Errorf("mcp: --mcp flag must be name=URL, got %q", v)
		}
		name = strings.TrimSpace(name)
		url = strings.TrimSpace(url)
		if name == "" || url == "" {
			return nil, fmt.Errorf("mcp: --mcp flag must be name=URL, got %q", v)
		}
		servers[name] = ServerConfig{Name: name, URL: url}
	}

	out := make([]ServerConfig, 0, len(servers))
	for _, sc := range servers {
		if !nameRE.MatchString(sc.Name) {
			return nil, fmt.Errorf("mcp: server name %q invalid; must match %s", sc.Name, nameRE)
		}
		if !strings.HasPrefix(sc.URL, "http://") && !strings.HasPrefix(sc.URL, "https://") {
			return nil, fmt.Errorf("mcp: server %q has unsupported URL %q (only http/https)", sc.Name, sc.URL)
		}
		out = append(out, sc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
