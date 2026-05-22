package mcp

import (
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
)

// parseJSONIf parses str as JSON only if (after trimming) it begins with the
// expected sentinel ('[' for array, '{' for object). Avoids accidentally
// converting legitimate strings that happen to be JSON-parseable scalars.
func parseJSONIf(str string, want byte) (any, bool) {
	trimmed := strings.TrimSpace(str)
	if len(trimmed) == 0 || trimmed[0] != want {
		return nil, false
	}
	var v any
	if err := json.Unmarshal([]byte(trimmed), &v); err != nil {
		return nil, false
	}
	return v, true
}

// coerceArgs forgives a common LLM mistake: sending stringified scalars where
// the tool's JSON Schema declares number/integer/boolean/array/object. It
// walks the top-level properties of schema and, for each property whose
// declared type does not match the actual string-typed value, replaces it
// with the parsed value when parsing succeeds. Nested objects and arrays
// are not recursed into — over-coercion has more failure modes than it's
// worth, and tool input schemas rarely have deep structure.
//
// args must be the result of json.Unmarshal into any (i.e. map[string]any).
// Non-object args are returned unchanged. The input map is not mutated;
// callers receive a shallow copy when any field is coerced and the original
// pointer when nothing changed.
func coerceArgs(args any, schema json.RawMessage, toolName string, logger *slog.Logger) any {
	orig, ok := args.(map[string]any)
	if !ok {
		return args
	}
	var s struct {
		Properties map[string]struct {
			Type any `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &s); err != nil {
		return args
	}
	var out map[string]any // lazily-allocated copy; nil until first coercion
	set := func(key string, v any) {
		if out == nil {
			out = make(map[string]any, len(orig))
			for k, v := range orig {
				out[k] = v
			}
		}
		out[key] = v
	}
	for key, prop := range s.Properties {
		val, present := orig[key]
		if !present {
			continue
		}
		str, isStr := val.(string)
		if !isStr {
			continue
		}
		declared := schemaTypes(prop.Type)
		switch {
		case hasType(declared, "integer"):
			if n, err := strconv.ParseInt(str, 10, 64); err == nil {
				set(key, n)
				logCoerce(logger, toolName, key, str, "integer")
			}
		case hasType(declared, "number"):
			if n, err := strconv.ParseFloat(str, 64); err == nil {
				set(key, n)
				logCoerce(logger, toolName, key, str, "number")
			}
		case hasType(declared, "boolean"):
			switch str {
			case "true":
				set(key, true)
				logCoerce(logger, toolName, key, str, "boolean")
			case "false":
				set(key, false)
				logCoerce(logger, toolName, key, str, "boolean")
			}
		case hasType(declared, "array"):
			if arr, ok := parseJSONIf(str, '['); ok {
				set(key, arr)
				logCoerce(logger, toolName, key, "<stringified JSON>", "array")
			}
		case hasType(declared, "object"):
			if o, ok := parseJSONIf(str, '{'); ok {
				set(key, o)
				logCoerce(logger, toolName, key, "<stringified JSON>", "object")
			}
		}
	}
	if out == nil {
		return orig
	}
	return out
}

// schemaTypes returns the declared JSON Schema type(s) for a property.
// Type can be a string or an array of strings ("type": ["integer", "null"]).
func schemaTypes(t any) []string {
	switch v := t.(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func hasType(types []string, want string) bool {
	for _, t := range types {
		if t == want {
			return true
		}
	}
	return false
}

func logCoerce(logger *slog.Logger, tool, field, from, to string) {
	if logger == nil {
		return
	}
	logger.Debug("mcp arg coerced", "tool", tool, "field", field, "from_string", from, "to_type", to)
}
