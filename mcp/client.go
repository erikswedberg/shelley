package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"shelley.exe.dev/llm"
	"shelley.exe.dev/version"
)

// connectTimeout caps how long we'll wait to set up the MCP session.
const connectTimeout = 10 * time.Second

// headerRoundTripper adds static headers to every outbound request.
type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range h.headers {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}
	return h.base.RoundTrip(req)
}

// session is one live MCP server connection.
type session struct {
	cfg ServerConfig
	sdk *sdk.ClientSession
}

func (s *session) close() {
	if s == nil || s.sdk == nil {
		return
	}
	_ = s.sdk.Close()
}

// connect establishes one MCP session over Streamable HTTP.
func connect(ctx context.Context, cfg ServerConfig, logger *slog.Logger) (*session, error) {
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	hc := &http.Client{}
	if len(cfg.Headers) > 0 {
		hc.Transport = &headerRoundTripper{base: http.DefaultTransport, headers: cfg.Headers}
	}
	tr := &sdk.StreamableClientTransport{
		Endpoint:             cfg.URL,
		HTTPClient:           hc,
		DisableStandaloneSSE: true, // we don't yet handle server-initiated messages
	}
	cli := sdk.NewClient(&sdk.Implementation{
		Name:    "shelley",
		Version: version.Version,
	}, nil)
	sess, err := cli.Connect(ctx, tr, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp(%s): connect: %w", cfg.Name, err)
	}
	if logger != nil {
		logger.Info("mcp connected", "server", cfg.Name, "url", cfg.URL)
	}
	return &session{cfg: cfg, sdk: sess}, nil
}

// Connect attempts to connect to every configured server. Failures are logged
// but do not abort: the returned []*llm.Tool contains tools from whichever
// servers came up. The returned cleanup must be called to terminate sessions.
func Connect(ctx context.Context, servers []ServerConfig, logger *slog.Logger) (tools []*llm.Tool, cleanup func()) {
	if len(servers) == 0 {
		return nil, func() {}
	}
	var (
		mu       sync.Mutex
		sessions []*session
		wg       sync.WaitGroup
	)
	wg.Add(len(servers))
	results := make([][]*llm.Tool, len(servers))
	for i, cfg := range servers {
		go func(i int, cfg ServerConfig) {
			defer wg.Done()
			s, err := connect(ctx, cfg, logger)
			if err != nil {
				if logger != nil {
					logger.Warn("mcp connect failed", "server", cfg.Name, "err", err)
				}
				return
			}
			ts, err := buildTools(ctx, s, logger)
			if err != nil {
				if logger != nil {
					logger.Warn("mcp list tools failed", "server", cfg.Name, "err", err)
				}
				s.close()
				return
			}
			mu.Lock()
			sessions = append(sessions, s)
			results[i] = ts
			mu.Unlock()
		}(i, cfg)
	}
	wg.Wait()

	for _, r := range results {
		tools = append(tools, r...)
	}
	cleanup = func() {
		for _, s := range sessions {
			s.close()
		}
	}
	return tools, cleanup
}

// buildTools fetches the tool list for one session and wraps each as an
// llm.Tool. Tool names are prefixed with the server name to prevent
// collisions, e.g. figma's get_code becomes figma__get_code.
func buildTools(ctx context.Context, s *session, logger *slog.Logger) ([]*llm.Tool, error) {
	listCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := s.sdk.ListTools(listCtx, nil)
	if err != nil {
		return nil, err
	}
	out := make([]*llm.Tool, 0, len(res.Tools))
	for _, t := range res.Tools {
		schema, err := sanitizeSchema(t.InputSchema)
		if err != nil {
			if logger != nil {
				logger.Warn("mcp tool skipped: bad schema", "server", s.cfg.Name, "tool", t.Name, "err", err)
			}
			continue
		}
		name := s.cfg.Name + "__" + t.Name
		fullName := t.Name // captured for closure
		desc := strings.TrimSpace(t.Description)
		if t.Title != "" {
			if desc == "" {
				desc = t.Title
			} else {
				desc = t.Title + "\n\n" + desc
			}
		}
		sess := s
		out = append(out, &llm.Tool{
			Name:        name,
			Description: desc,
			InputSchema: schema,
			Run: func(ctx context.Context, input json.RawMessage) llm.ToolOut {
				return callMCP(ctx, sess, fullName, input)
			},
		})
	}
	return out, nil
}

// sanitizeSchema converts whatever the MCP server gave us into a json.RawMessage
// that llm.MustSchema would accept (type=object with a properties key).
func sanitizeSchema(in any) (json.RawMessage, error) {
	if in == nil {
		return json.RawMessage(`{"type":"object","properties":{}}`), nil
	}
	var obj map[string]any
	switch v := in.(type) {
	case map[string]any:
		obj = v
	case json.RawMessage:
		if err := json.Unmarshal(v, &obj); err != nil {
			return nil, err
		}
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &obj); err != nil {
			return nil, err
		}
	}
	if obj == nil {
		obj = map[string]any{}
	}
	if t, ok := obj["type"].(string); !ok || t != "object" {
		obj["type"] = "object"
	}
	if _, ok := obj["properties"]; !ok {
		obj["properties"] = map[string]any{}
	}
	// Drop $schema: some LLM providers reject unknown top-level keys.
	delete(obj, "$schema")
	return json.Marshal(obj)
}

func callMCP(ctx context.Context, s *session, toolName string, input json.RawMessage) llm.ToolOut {
	var args any
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return llm.ToolOut{Error: fmt.Errorf("invalid arguments: %w", err)}
		}
	}
	res, err := s.sdk.CallTool(ctx, &sdk.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	if err != nil {
		return llm.ToolOut{Error: fmt.Errorf("mcp call %s.%s: %w", s.cfg.Name, toolName, err)}
	}
	llmContent := contentFromMCP(res.Content)
	if len(llmContent) == 0 {
		// Some servers reply with no content blocks on success; surface
		// structured content as JSON text so the model has something to read.
		if res.StructuredContent != nil {
			if b, err := json.Marshal(res.StructuredContent); err == nil {
				llmContent = append(llmContent, llm.Content{Type: llm.ContentTypeText, Text: string(b)})
			}
		}
		if len(llmContent) == 0 {
			llmContent = []llm.Content{{Type: llm.ContentTypeText, Text: "(no content)"}}
		}
	}
	if res.IsError {
		var msg strings.Builder
		for _, c := range llmContent {
			if c.Text != "" {
				if msg.Len() > 0 {
					msg.WriteString("\n")
				}
				msg.WriteString(c.Text)
			}
		}
		if msg.Len() == 0 {
			msg.WriteString("mcp tool returned isError=true")
		}
		return llm.ToolOut{Error: fmt.Errorf("%s", msg.String())}
	}
	return llm.ToolOut{LLMContent: llmContent}
}

// contentFromMCP translates MCP content blocks into Shelley llm.Content blocks.
// Tool-result image content is encoded as llm.ContentTypeText with MediaType +
// base64 Data; the Anthropic adapter detects MediaType to emit it as image.
func contentFromMCP(in []sdk.Content) []llm.Content {
	out := make([]llm.Content, 0, len(in))
	for _, c := range in {
		switch v := c.(type) {
		case *sdk.TextContent:
			out = append(out, llm.Content{Type: llm.ContentTypeText, Text: v.Text})
		case *sdk.ImageContent:
			if len(v.Data) == 0 {
				continue
			}
			mt := v.MIMEType
			if mt == "" {
				mt = "image/png"
			}
			out = append(out, llm.Content{
				Type:      llm.ContentTypeText,
				MediaType: mt,
				Data:      base64.StdEncoding.EncodeToString(v.Data),
			})
		default:
			// Fall back to JSON-marshalling unknown content blocks so the model
			// still sees something.
			if b, err := json.Marshal(v); err == nil {
				out = append(out, llm.Content{Type: llm.ContentTypeText, Text: string(b)})
			}
		}
	}
	return out
}
