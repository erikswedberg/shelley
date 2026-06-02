package mcp

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

// TestConnect_LiveServer exercises Connect against a real MCP server.
// Set SHELLEY_MCP_TEST_URL=http://host.sand:3845/mcp (for example) to enable.
func TestConnect_LiveServer(t *testing.T) {
	url := os.Getenv("SHELLEY_MCP_TEST_URL")
	if url == "" {
		t.Skip("set SHELLEY_MCP_TEST_URL to run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tools, cleanup := Connect(ctx, []ServerConfig{{Name: "live", URL: url}}, slog.Default())
	defer cleanup()
	if len(tools) == 0 {
		t.Fatal("no tools fetched from live server")
	}
	t.Logf("got %d tools from %s", len(tools), url)
	for _, tt := range tools {
		t.Logf("  %s", tt.Name)
	}
}
