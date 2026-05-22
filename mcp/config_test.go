package mcp

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_FlagOnly(t *testing.T) {
	servers, err := LoadConfig([]string{"figma=http://host.sand:3845/mcp"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 || servers[0].Name != "figma" || servers[0].URL != "http://host.sand:3845/mcp" {
		t.Fatalf("unexpected: %+v", servers)
	}
	if !servers[0].IsHTTP() || servers[0].IsStdio() {
		t.Fatalf("transport wrong: %+v", servers[0])
	}
}

func TestLoadConfig_FileAndFlagMerge(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(p, []byte(`{"mcpServers":{"foo":{"url":"https://a/"},"bar":{"url":"https://b/","headers":{"X":"1"}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Flag overrides file entry for "foo".
	servers, err := LoadConfig([]string{"foo=https://override/"}, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 {
		t.Fatalf("want 2, got %d: %+v", len(servers), servers)
	}
	var foo, bar *ServerConfig
	for i := range servers {
		switch servers[i].Name {
		case "foo":
			foo = &servers[i]
		case "bar":
			bar = &servers[i]
		}
	}
	if foo == nil || foo.URL != "https://override/" {
		t.Errorf("foo not overridden: %+v", foo)
	}
	if bar == nil || bar.Headers["X"] != "1" {
		t.Errorf("bar headers not preserved: %+v", bar)
	}
}

func TestLoadConfig_BadName(t *testing.T) {
	if _, err := LoadConfig([]string{"1bad=http://x/"}, ""); err == nil {
		t.Fatal("want error for invalid name")
	}
	if _, err := LoadConfig([]string{"ok=ftp://x/"}, ""); err == nil {
		t.Fatal("want error for non-http URL")
	}
	if _, err := LoadConfig([]string{"noequals"}, ""); err == nil {
		t.Fatal("want error for missing =")
	}
}

func TestLoadConfig_Stdio(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mcp.json")
	write := func(body string) {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write(`{"mcpServers":{"f":{"command":"npx","args":["-y","foo"],"env":{"TOK":"${MY_PAT}"}}}}`)
	getenv := func(k string) string {
		if k == "MY_PAT" {
			return "secret"
		}
		return ""
	}
	servers, err := loadConfig(nil, p, getenv)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 {
		t.Fatalf("want 1 server, got %d", len(servers))
	}
	s := servers[0]
	if !s.IsStdio() || s.IsHTTP() {
		t.Fatalf("wrong transport: %+v", s)
	}
	if s.Command != "npx" || len(s.Args) != 2 {
		t.Fatalf("bad command/args: %+v", s)
	}
	if s.Env["TOK"] != "secret" {
		t.Fatalf("env not expanded: %+v", s.Env)
	}
}

func TestLoadConfig_StdioMissingEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(p, []byte(`{"mcpServers":{"f":{"command":"x","env":{"TOK":"${NOPE}"}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	servers, err := loadConfig(nil, p, func(string) string { return "" })
	var me *MissingEnvError
	if !errors.As(err, &me) {
		t.Fatalf("want MissingEnvError, got %v", err)
	}
	if len(servers) != 1 || servers[0].Env["TOK"] != "" {
		t.Fatalf("want empty TOK, got %+v", servers)
	}
}

func TestLoadConfig_TransportConflict(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(p, []byte(`{"mcpServers":{"f":{"url":"http://x/","command":"npx"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(nil, p); err == nil {
		t.Fatal("want error for both url and command")
	}
	if err := os.WriteFile(p, []byte(`{"mcpServers":{"f":{}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(nil, p); err == nil {
		t.Fatal("want error for neither url nor command")
	}
}
