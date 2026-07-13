// Package claudetool provides tools for Claude AI models.
//
// When adding, removing, or modifying tools in this package,
// remember to update the tool display template in termui/termui.go
// to ensure proper tool output formatting.
package claudetool

import (
	"context"
	"fmt"

	"shelley.exe.dev/llm"
)

// FlexBool is a bool that also accepts string values "true" and "false" in JSON.
// LLMs sometimes send boolean values as strings.
type FlexBool bool

func (b *FlexBool) UnmarshalJSON(data []byte) error {
	switch string(data) {
	case "true", `"true"`:
		*b = true
	case "false", `"false"`:
		*b = false
	default:
		return fmt.Errorf("invalid bool value: %s", data)
	}
	return nil
}

func WithWorkingDir(ctx context.Context, wd string) context.Context {
	return llm.WithWorkingDir(ctx, wd)
}

func WorkingDir(ctx context.Context) string {
	return llm.WorkingDir(ctx)
}

func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return llm.WithSessionID(ctx, sessionID)
}

func SessionID(ctx context.Context) string {
	return llm.SessionID(ctx)
}

// WithToolProgress returns a context with the given ToolProgressFunc.
func WithToolProgress(ctx context.Context, fn llm.ToolProgressFunc) context.Context {
	return llm.WithToolProgress(ctx, fn)
}

// GetToolProgress retrieves the ToolProgressFunc from the context, or nil.
func GetToolProgress(ctx context.Context) llm.ToolProgressFunc {
	return llm.GetToolProgress(ctx)
}

// WithToolUseID returns a context with the given tool use ID.
func WithToolUseID(ctx context.Context, id string) context.Context {
	return llm.WithToolUseID(ctx, id)
}

func ToolUseID(ctx context.Context) string {
	return llm.ToolUseID(ctx)
}

type todoChangeCtxKeyType string

const todoChangeCtxKey todoChangeCtxKeyType = "todoChange"

// WithTodoChangeCallback returns a context with a callback invoked when todos change.
func WithTodoChangeCallback(ctx context.Context, fn func()) context.Context {
	return context.WithValue(ctx, todoChangeCtxKey, fn)
}

// NotifyTodoChange calls the todo change callback if set.
func NotifyTodoChange(ctx context.Context) {
	if fn, ok := ctx.Value(todoChangeCtxKey).(func()); ok && fn != nil {
		fn()
	}
}
