package tools

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ShellTool executes shell commands.
type ShellTool struct{}

func (t *ShellTool) Name() string { return "shell" }
func (t *ShellTool) Description() string {
	return "Execute a shell command and return stdout, stderr, and exit code."
}
func (t *ShellTool) ParametersSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command":    map[string]any{"type": "string", "description": "The shell command to execute."},
			"timeout_ms": map[string]any{"type": "integer", "description": "Timeout in milliseconds (default: 120000)."},
		},
		"required": []string{"command"},
	}
}
func (t *ShellTool) Execute(ctx context.Context, args map[string]any, env ExecutionEnvironment) (ToolResult, error) {
	command := argString(args, "command")
	timeoutMS := argInt(args, "timeout_ms", 120000)
	timeout := time.Duration(timeoutMS) * time.Millisecond

	result := env.RunCommand(ctx, command, timeout)

	var parts []string
	if result.Stdout != "" {
		parts = append(parts, result.Stdout)
	}
	if result.Stderr != "" {
		parts = append(parts, "STDERR:\n"+result.Stderr)
	}
	if result.TimedOut {
		parts = append(parts, fmt.Sprintf("Command timed out after %.0fs", timeout.Seconds()))
	}
	output := "(no output)"
	if len(parts) > 0 {
		output = strings.Join(parts, "\n")
	}
	return ToolResult{
		Content:  output,
		IsError:  result.ExitCode != 0,
		Metadata: map[string]any{"exit_code": result.ExitCode, "timed_out": result.TimedOut},
	}, nil
}

// GlobTool finds files matching a glob pattern.
type GlobTool struct{}

func (t *GlobTool) Name() string { return "glob" }
func (t *GlobTool) Description() string {
	return "Find files matching a glob pattern (e.g. '**/*.py', 'src/**/*.ts')."
}
func (t *GlobTool) ParametersSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "The glob pattern to match files against."},
			"path":    map[string]any{"type": "string", "description": "Base directory to search in."},
		},
		"required": []string{"pattern"},
	}
}
func (t *GlobTool) Execute(ctx context.Context, args map[string]any, env ExecutionEnvironment) (ToolResult, error) {
	pattern := argString(args, "pattern")
	basePath := argString(args, "path")
	files, err := env.ListFiles(pattern, basePath)
	if err != nil {
		return ToolResult{Content: "Error: " + err.Error(), IsError: true}, nil
	}
	if len(files) == 0 {
		return ToolResult{Content: "No files matched the pattern."}, nil
	}
	return ToolResult{Content: strings.Join(files, "\n")}, nil
}

// GrepTool searches file contents using ripgrep (falling back to grep).
type GrepTool struct{}

func (t *GrepTool) Name() string { return "grep" }
func (t *GrepTool) Description() string {
	return "Search file contents using regex patterns. Returns matching lines with file paths and line numbers."
}
func (t *GrepTool) ParametersSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern":       map[string]any{"type": "string", "description": "Regex pattern to search for."},
			"path":          map[string]any{"type": "string", "description": "File or directory to search in."},
			"glob":          map[string]any{"type": "string", "description": "Glob filter for files (e.g. '*.py')."},
			"context_lines": map[string]any{"type": "integer", "description": "Number of context lines around matches."},
		},
		"required": []string{"pattern"},
	}
}
func (t *GrepTool) Execute(ctx context.Context, args map[string]any, env ExecutionEnvironment) (ToolResult, error) {
	pattern := argString(args, "pattern")
	searchPath := argString(args, "path")
	if searchPath == "" {
		searchPath = env.WorkingDirectory()
	}
	fileGlob := argString(args, "glob")
	if fileGlob == "" {
		fileGlob = "**/*"
	}
	context := argInt(args, "context_lines", 0)

	cmdParts := []string{"rg", "--line-number", "--no-heading"}
	if context > 0 {
		cmdParts = append(cmdParts, "-C", fmt.Sprint(context))
	}
	if fileGlob != "" && fileGlob != "**/*" {
		cmdParts = append(cmdParts, "--glob", fileGlob)
	}
	cmdParts = append(cmdParts, "--", pattern, searchPath)

	result := env.RunCommand(ctx, quoteArgs(cmdParts), 30*time.Second)

	if result.ExitCode == 0 && result.Stdout != "" {
		lines := strings.Split(result.Stdout, "\n")
		if len(lines) > 200 {
			return ToolResult{Content: strings.Join(lines[:200], "\n") + fmt.Sprintf("\n... (%d more lines)", len(lines)-200)}, nil
		}
		return ToolResult{Content: result.Stdout}, nil
	}
	if result.ExitCode == 1 {
		return ToolResult{Content: "No matches found."}, nil
	}
	if result.ExitCode == 2 || strings.Contains(strings.ToLower(result.Stderr), "not found") {
		// rg not available, fall back to grep
		grepParts := []string{"grep", "-rn"}
		if context > 0 {
			grepParts = append(grepParts, fmt.Sprintf("-C%d", context))
		}
		if fileGlob != "" && fileGlob != "**/*" {
			grepParts = append(grepParts, "--include", fileGlob)
		}
		grepParts = append(grepParts, "--", pattern, searchPath)
		result = env.RunCommand(ctx, quoteArgs(grepParts), 30*time.Second)
		if result.Stdout != "" {
			out := result.Stdout
			if len(out) > 50000 {
				out = out[:50000]
			}
			return ToolResult{Content: out}, nil
		}
		return ToolResult{Content: "No matches found."}, nil
	}
	return ToolResult{Content: "Search error: " + result.Stderr, IsError: true}, nil
}

func quoteArgs(parts []string) string {
	out := make([]string, len(parts))
	for i, p := range parts {
		if strings.Contains(p, " ") {
			out[i] = `"` + p + `"`
		} else {
			out[i] = p
		}
	}
	return strings.Join(out, " ")
}
