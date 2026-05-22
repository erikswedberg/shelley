package mcp

import (
	"encoding/json"
	"log/slog"
	"strconv"
)

// coerceArgs forgives a common LLM mistake: sending stringified scalars where
// the tool's JSON Schema declares number/integer/boolean. It walks the
// top-level properties of schema and, for each property whose declared type
// is number/integer/boolean and whose argument value is a string that parses
// to the right primitive, replaces the string with the parsed value. Nested
// objects and arrays are left alone (rare in tool input schemas, and
// recursive coercion has more failure modes than it's worth).
//
// args must be the result of json.Unmarshal into any (i.e. map[string]any).
// Non-object args are returned unchanged.
func coerceArgs(args any, schema json.RawMessage, toolName string, logger *slog.Logger) any {
	obj, ok := args.(map[string]any)
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
	for key, prop := range s.Properties {
		val, present := obj[key]
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
				obj[key] = n
				logCoerce(logger, toolName, key, str, "integer")
			}
		case hasType(declared, "number"):
			if n, err := strconv.ParseFloat(str, 64); err == nil {
				obj[key] = n
				logCoerce(logger, toolName, key, str, "number")
			}
		case hasType(declared, "boolean"):
			switch str {
			case "true":
				obj[key] = true
				logCoerce(logger, toolName, key, str, "boolean")
			case "false":
				obj[key] = false
				logCoerce(logger, toolName, key, str, "boolean")
			}
		}
	}
	return obj
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
