package mcp

import (
	"encoding/json"
	"testing"
)

func TestCoerceArgs(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"properties":{
			"depth":   {"type":"number"},
			"count":   {"type":"integer"},
			"force":   {"type":"boolean"},
			"name":    {"type":"string"},
			"nullable":{"type":["integer","null"]}
		}
	}`)

	in := map[string]any{
		"depth":    "2",
		"count":    "7",
		"force":    "true",
		"name":     "42", // should NOT coerce: declared string
		"nullable": "5",
	}
	out := coerceArgs(in, schema, "test", nil).(map[string]any)

	if d, ok := out["depth"].(float64); !ok || d != 2 {
		t.Errorf("depth not coerced to number: %T %v", out["depth"], out["depth"])
	}
	if c, ok := out["count"].(int64); !ok || c != 7 {
		t.Errorf("count not coerced to integer: %T %v", out["count"], out["count"])
	}
	if b, ok := out["force"].(bool); !ok || !b {
		t.Errorf("force not coerced to bool: %T %v", out["force"], out["force"])
	}
	if n, ok := out["name"].(string); !ok || n != "42" {
		t.Errorf("name should remain string: %T %v", out["name"], out["name"])
	}
	if n, ok := out["nullable"].(int64); !ok || n != 5 {
		t.Errorf("nullable (integer|null) not coerced: %T %v", out["nullable"], out["nullable"])
	}
}

func TestCoerceArgs_BadValueLeftAlone(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"n":{"type":"integer"}}}`)
	in := map[string]any{"n": "not-a-number"}
	out := coerceArgs(in, schema, "t", nil).(map[string]any)
	if v, _ := out["n"].(string); v != "not-a-number" {
		t.Fatalf("want pass-through, got %#v", out["n"])
	}
}

func TestCoerceArgs_NonObjectPassThrough(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{}}`)
	if got := coerceArgs("hello", schema, "t", nil); got != "hello" {
		t.Fatalf("want pass-through, got %v", got)
	}
}

func TestCoerceArgs_StringifiedArrayAndObject(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"properties":{
			"nodes": {"type":"array"},
			"opts":  {"type":"object"},
			"note":  {"type":"string"}
		}
	}`)
	in := map[string]any{
		"nodes": `[{"nodeId":"1:2","fileName":"a.png"}]`,
		"opts":  `{"verbose":true}`,
		"note":  `["this should stay a string"]`,
	}
	out := coerceArgs(in, schema, "t", nil).(map[string]any)

	arr, ok := out["nodes"].([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("nodes not coerced to array: %T %v", out["nodes"], out["nodes"])
	}
	item, _ := arr[0].(map[string]any)
	if item["nodeId"] != "1:2" || item["fileName"] != "a.png" {
		t.Fatalf("array item wrong: %+v", item)
	}
	o, ok := out["opts"].(map[string]any)
	if !ok || o["verbose"] != true {
		t.Fatalf("opts not coerced to object: %T %v", out["opts"], out["opts"])
	}
	if s, _ := out["note"].(string); s == "" {
		t.Fatalf("note should stay a string")
	}
}

func TestCoerceArgs_StringNotJSON_NoArrayCoerce(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"nodes":{"type":"array"}}}`)
	in := map[string]any{"nodes": "just-a-name"}
	out := coerceArgs(in, schema, "t", nil).(map[string]any)
	if _, ok := out["nodes"].(string); !ok {
		t.Fatalf("want pass-through string, got %T", out["nodes"])
	}
}
