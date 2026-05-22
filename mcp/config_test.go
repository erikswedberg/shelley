package mcp

import (
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
