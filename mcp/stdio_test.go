package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"shelley.exe.dev/llm"
)

// TestMain doubles as a stdio MCP server when SHELLEY_MCP_FIXTURE=1.
// This lets us spawn the test binary itself as an MCP server and exercise
// the full stdio path without any external dependency (no npx, no network).
func TestMain(m *testing.M) {
	if os.Getenv("SHELLEY_MCP_FIXTURE") == "1" {
		runFixtureServer()
		return
	}
	os.Exit(m.Run())
}

func runFixtureServer() {
	srv := sdk.NewServer(&sdk.Implementation{Name: "shelley-test", Version: "0.0.1"}, nil)
	srv.AddTool(&sdk.Tool{
		Name:        "echo",
		Description: "Echo the input back as text content.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{"type": "string"},
			},
		},
	}, func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		var args struct{ Text string }
		if req.Params.Arguments != nil {
			_ = json.Unmarshal(req.Params.Arguments, &args)
		}
		// Write a line to stderr to confirm drainStderr forwards it.
		fmt.Fprintln(os.Stderr, "fixture got call: "+args.Text)
		return &sdk.CallToolResult{
			Content: []sdk.Content{&sdk.TextContent{Text: "echo: " + args.Text}},
		}, nil
	})
	srv.AddTool(&sdk.Tool{
		Name:        "sleep",
		Description: "Block forever (used by timeout tests).",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}, func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err := srv.Run(context.Background(), &sdk.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func TestConnect_StdioFixture(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg := ServerConfig{
		Name:    "fix",
		Command: exe,
		Args:    []string{"-test.run=TestMain"},
		Env:     map[string]string{"SHELLEY_MCP_FIXTURE": "1"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tools, cleanup := Connect(ctx, []ServerConfig{cfg}, slog.Default())
	defer cleanup()
	var echo *llm.Tool
	for _, tt := range tools {
		if tt.Name == "fix__echo" {
			echo = tt
		}
	}
	if echo == nil {
		t.Fatalf("fix__echo tool not found in %d tools", len(tools))
	}
	out := echo.Run(ctx, json.RawMessage(`{"text":"hello"}`))
	if out.Error != nil {
		t.Fatalf("run: %v", out.Error)
	}
	if len(out.LLMContent) == 0 || out.LLMContent[0].Text != "echo: hello" {
		t.Fatalf("unexpected content: %+v", out.LLMContent)
	}
}

// TestConnect_StdioMissingCommand ensures a missing binary doesn't crash
// the Connect path: it should log a warning and return zero tools.
func TestConnect_StdioMissingCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := ServerConfig{Name: "nope", Command: "definitely-not-a-real-binary-shelley"}
	tools, cleanup := Connect(ctx, []ServerConfig{cfg}, slog.Default())
	defer cleanup()
	if len(tools) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(tools))
	}
	// Sanity: exec.LookPath confirms the binary really is missing.
	if _, err := exec.LookPath(cfg.Command); err == nil {
		t.Fatal("test fixture: pick a name that isn't on PATH")
	}
}

// TestCallTool_Timeout verifies callMCP returns a clean error when the
// server never responds, instead of hanging forever.
func TestCallTool_Timeout(t *testing.T) {
	orig := callTimeout
	callTimeout = 200 * time.Millisecond
	t.Cleanup(func() { callTimeout = orig })

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg := ServerConfig{
		Name:    "fix",
		Command: exe,
		Args:    []string{"-test.run=TestMain"},
		Env:     map[string]string{"SHELLEY_MCP_FIXTURE": "1"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tools, cleanup := Connect(ctx, []ServerConfig{cfg}, slog.Default())
	defer cleanup()
	var sleep *llm.Tool
	for _, tt := range tools {
		if tt.Name == "fix__sleep" {
			sleep = tt
		}
	}
	if sleep == nil {
		t.Fatal("fix__sleep tool not found")
	}
	t0 := time.Now()
	out := sleep.Run(ctx, json.RawMessage(`{}`))
	elapsed := time.Since(t0)
	if out.Error == nil {
		t.Fatal("expected error from timeout, got success")
	}
	if !strings.Contains(out.Error.Error(), "timed out") {
		t.Fatalf("expected timeout error, got %v", out.Error)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("expected fast return on timeout, took %s", elapsed)
	}
}
