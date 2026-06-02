// Package mcp adds Model Context Protocol client support to Shelley.
//
// Servers are configured via the command line (--mcp name=URL, repeatable,
// HTTP only) or a JSON config file (--mcp-config path) whose shape mirrors
// the Claude Code / Claude Desktop convention. Two transports are supported:
//
//	{
//	  "mcpServers": {
//	    "figma":     { "url": "http://host.sand:3845/mcp" },
//	    "example":   { "url": "https://example.com/mcp",
//	                  "headers": { "Authorization": "Bearer ${MY_TOKEN}" } },
//	    "framelink": { "command": "npx",
//	                  "args":    ["-y", "figma-developer-mcp", "--stdio"],
//	                  "env":     { "FIGMA_API_KEY": "${FIGMA_API_KEY}" } }
//	  }
//	}
//
// ${VAR} occurrences in headers values and env values are expanded from the
// launcher's environment. Missing variables substitute to empty and log a
// warning at connect time.
package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// ServerConfig describes a single MCP server. It must have either URL
// (HTTP transport) or Command (stdio transport) set, but not both.
type ServerConfig struct {
	// Name is the user-facing identifier (also used as the tool-name prefix).
	Name string `json:"-"`

	// URL is the MCP endpoint for HTTP transport (must be http:// or https://).
	URL string `json:"url,omitempty"`
	// Headers are optional extra HTTP headers for HTTP transport.
	Headers map[string]string `json:"headers,omitempty"`

	// Command is the program to spawn for stdio transport.
	Command string `json:"command,omitempty"`
	// Args are passed to Command.
	Args []string `json:"args,omitempty"`
	// Env are extra environment variables for the child process.
	// Values support ${VAR} expansion from the launcher's environment.
	Env map[string]string `json:"env,omitempty"`
}

// IsStdio reports whether this server uses stdio transport.
func (s ServerConfig) IsStdio() bool { return s.Command != "" }

// IsHTTP reports whether this server uses HTTP transport.
func (s ServerConfig) IsHTTP() bool { return s.URL != "" }

// jsonFile mirrors {"mcpServers": {name: ServerConfig}}.
type jsonFile struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

var (
	nameRE   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,31}$`)
	expandRE = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
)

// expandEnv expands ${VAR} occurrences in s from getenv. Missing vars
// substitute to empty string and are appended to missing.
func expandEnv(s string, getenv func(string) string, missing *[]string) string {
	return expandRE.ReplaceAllStringFunc(s, func(match string) string {
		key := match[2 : len(match)-1]
		v := getenv(key)
		if v == "" && missing != nil {
			*missing = append(*missing, key)
		}
		return v
	})
}

// LoadConfig parses zero or more --mcp flag values and an optional config
// file path into a list of ServerConfigs. Flag values override file entries
// with the same name. ${VAR} expansion uses os.Getenv.
func LoadConfig(flagValues []string, configPath string) ([]ServerConfig, error) {
	return loadConfig(flagValues, configPath, os.Getenv)
}

func loadConfig(flagValues []string, configPath string, getenv func(string) string) ([]ServerConfig, error) {
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

	var missingAll []string
	out := make([]ServerConfig, 0, len(servers))
	for _, sc := range servers {
		if !nameRE.MatchString(sc.Name) {
			return nil, fmt.Errorf("mcp: server name %q invalid; must match %s", sc.Name, nameRE)
		}
		switch {
		case sc.URL != "" && sc.Command != "":
			return nil, fmt.Errorf("mcp: server %q has both url and command; pick one", sc.Name)
		case sc.URL == "" && sc.Command == "":
			return nil, fmt.Errorf("mcp: server %q has neither url nor command", sc.Name)
		case sc.URL != "":
			if !strings.HasPrefix(sc.URL, "http://") && !strings.HasPrefix(sc.URL, "https://") {
				return nil, fmt.Errorf("mcp: server %q has unsupported URL %q (only http/https)", sc.Name, sc.URL)
			}
			if len(sc.Headers) > 0 {
				expanded := make(map[string]string, len(sc.Headers))
				for k, v := range sc.Headers {
					expanded[k] = expandEnv(v, getenv, &missingAll)
				}
				sc.Headers = expanded
			}
		case sc.Command != "":
			if len(sc.Env) > 0 {
				expanded := make(map[string]string, len(sc.Env))
				for k, v := range sc.Env {
					expanded[k] = expandEnv(v, getenv, &missingAll)
				}
				sc.Env = expanded
			}
		}
		out = append(out, sc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(missingAll) > 0 {
		// Non-fatal: return the configs anyway. Caller (cmd/shelley) logs.
		return out, &MissingEnvError{Vars: missingAll}
	}
	return out, nil
}

// MissingEnvError is returned when ${VAR} expansion encountered undefined
// variables. The Vars field lists each missing variable (possibly with
// duplicates if referenced multiple times). The returned ServerConfig list
// is still valid; the caller may choose to warn and continue.
type MissingEnvError struct {
	Vars []string
}

func (e *MissingEnvError) Error() string {
	return fmt.Sprintf("mcp: undefined env vars in config: %s", strings.Join(e.Vars, ", "))
}
