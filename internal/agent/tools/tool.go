// Package tools provides the coding agent's tool implementations, the tool
// registry, and the execution environment abstraction they run against.
package tools

import (
	"context"

	"github.com/nigelpepper/attractor/internal/llm"
)

// ToolResult is the outcome of a tool execution.
type ToolResult struct {
	Content  string
	IsError  bool
	Metadata map[string]any
}

// Tool is a capability the agent can invoke.
type Tool interface {
	Name() string
	Description() string
	ParametersSchema() map[string]any
	Execute(ctx context.Context, args map[string]any, env ExecutionEnvironment) (ToolResult, error)
}

// Definition converts a tool to its LLM-facing definition.
func Definition(t Tool) llm.ToolDefinition {
	return llm.ToolDefinition{Name: t.Name(), Description: t.Description(), Parameters: t.ParametersSchema()}
}

// ToolRegistry maps tool names to implementations.
type ToolRegistry struct {
	tools map[string]Tool
	order []string
}

// NewToolRegistry returns an empty registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: map[string]Tool{}}
}

// Register adds (or replaces) a tool.
func (r *ToolRegistry) Register(t Tool) {
	if _, exists := r.tools[t.Name()]; !exists {
		r.order = append(r.order, t.Name())
	}
	r.tools[t.Name()] = t
}

// Remove deletes a tool by name (no-op if absent).
func (r *ToolRegistry) Remove(name string) {
	if _, ok := r.tools[name]; ok {
		delete(r.tools, name)
		for i, n := range r.order {
			if n == name {
				r.order = append(r.order[:i], r.order[i+1:]...)
				break
			}
		}
	}
}

// Get returns a tool by name, or nil.
func (r *ToolRegistry) Get(name string) Tool { return r.tools[name] }

// ListTools returns all tools in registration order.
func (r *ToolRegistry) ListTools() []Tool {
	out := make([]Tool, 0, len(r.order))
	for _, n := range r.order {
		if t := r.tools[n]; t != nil {
			out = append(out, t)
		}
	}
	return out
}

// ListDefinitions returns LLM tool definitions in registration order.
func (r *ToolRegistry) ListDefinitions() []llm.ToolDefinition {
	var out []llm.ToolDefinition
	for _, t := range r.ListTools() {
		out = append(out, Definition(t))
	}
	return out
}

// Execute runs the named tool, converting panics/errors into error results.
func (r *ToolRegistry) Execute(ctx context.Context, name string, args map[string]any, env ExecutionEnvironment) ToolResult {
	t := r.tools[name]
	if t == nil {
		return ToolResult{Content: "Unknown tool: " + name, IsError: true}
	}
	res, err := t.Execute(ctx, args, env)
	if err != nil {
		return ToolResult{Content: "Tool error: " + err.Error(), IsError: true}
	}
	return res
}

// DefaultTools returns instances of all built-in tools.
func DefaultTools() []Tool {
	return []Tool{
		&ReadFileTool{}, &WriteFileTool{}, &EditFileTool{},
		&ShellTool{}, &GlobTool{}, &GrepTool{},
	}
}

// CreateDefaultRegistry returns a registry with all built-in tools.
func CreateDefaultRegistry() *ToolRegistry {
	r := NewToolRegistry()
	for _, t := range DefaultTools() {
		r.Register(t)
	}
	return r
}

// --- small typed arg helpers shared by tools ---

func argString(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func argInt(args map[string]any, key string, def int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		return def
	}
}

func argBool(args map[string]any, key string) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return false
}
