package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// ReadFileTool reads file contents with line numbers.
type ReadFileTool struct{}

func (t *ReadFileTool) Name() string { return "read_file" }
func (t *ReadFileTool) Description() string {
	return "Read the contents of a file. Returns file content with line numbers."
}
func (t *ReadFileTool) ParametersSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{"type": "string", "description": "Absolute or relative path to the file to read."},
			"offset":    map[string]any{"type": "integer", "description": "Line number to start reading from (1-based)."},
			"limit":     map[string]any{"type": "integer", "description": "Maximum number of lines to read."},
		},
		"required": []string{"file_path"},
	}
}
func (t *ReadFileTool) Execute(ctx context.Context, args map[string]any, env ExecutionEnvironment) (ToolResult, error) {
	filePath := argString(args, "file_path")
	offset := argInt(args, "offset", 1)
	limit := argInt(args, "limit", 2000)

	content, err := env.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ToolResult{Content: "File not found: " + filePath, IsError: true}, nil
		}
		return ToolResult{Content: "Error reading file: " + err.Error(), IsError: true}, nil
	}

	lines := strings.Split(content, "\n")
	// Python splitlines drops a trailing empty element from a final newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	start := offset - 1
	if start < 0 {
		start = 0
	}
	end := start + limit
	if end > len(lines) {
		end = len(lines)
	}
	var b strings.Builder
	for i := start; i < end; i++ {
		if i > start {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%6d\t%s", i+1, lines[i])
	}
	return ToolResult{Content: b.String()}, nil
}

// WriteFileTool writes content to a file.
type WriteFileTool struct{}

func (t *WriteFileTool) Name() string { return "write_file" }
func (t *WriteFileTool) Description() string {
	return "Write content to a file. Creates parent directories if needed."
}
func (t *WriteFileTool) ParametersSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{"type": "string", "description": "Absolute or relative path to write to."},
			"content":   map[string]any{"type": "string", "description": "The content to write to the file."},
		},
		"required": []string{"file_path", "content"},
	}
}
func (t *WriteFileTool) Execute(ctx context.Context, args map[string]any, env ExecutionEnvironment) (ToolResult, error) {
	filePath := argString(args, "file_path")
	content := argString(args, "content")
	if err := env.WriteFile(filePath, content); err != nil {
		return ToolResult{Content: "Error writing file: " + err.Error(), IsError: true}, nil
	}
	return ToolResult{Content: "Successfully wrote to " + filePath}, nil
}

// EditFileTool performs exact string replacement.
type EditFileTool struct{}

func (t *EditFileTool) Name() string { return "edit_file" }
func (t *EditFileTool) Description() string {
	return "Perform exact string replacement in a file. The old_string must be unique in the file unless replace_all is true."
}
func (t *EditFileTool) ParametersSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path":   map[string]any{"type": "string", "description": "Path to the file to edit."},
			"old_string":  map[string]any{"type": "string", "description": "The exact string to find and replace."},
			"new_string":  map[string]any{"type": "string", "description": "The replacement string."},
			"replace_all": map[string]any{"type": "boolean", "description": "Replace all occurrences (default: false)."},
		},
		"required": []string{"file_path", "old_string", "new_string"},
	}
}
func (t *EditFileTool) Execute(ctx context.Context, args map[string]any, env ExecutionEnvironment) (ToolResult, error) {
	filePath := argString(args, "file_path")
	oldString := argString(args, "old_string")
	newString := argString(args, "new_string")
	replaceAll := argBool(args, "replace_all")

	content, err := env.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ToolResult{Content: "File not found: " + filePath, IsError: true}, nil
		}
		return ToolResult{Content: "Error reading file: " + err.Error(), IsError: true}, nil
	}

	count := strings.Count(content, oldString)
	if count == 0 {
		return ToolResult{Content: "old_string not found in " + filePath, IsError: true}, nil
	}

	var newContent string
	if !replaceAll {
		if count > 1 {
			return ToolResult{Content: fmt.Sprintf(
				"old_string appears %d times in %s. Use replace_all=true or provide more context to make it unique.",
				count, filePath), IsError: true}, nil
		}
		newContent = strings.Replace(content, oldString, newString, 1)
	} else {
		newContent = strings.ReplaceAll(content, oldString, newString)
	}

	if err := env.WriteFile(filePath, newContent); err != nil {
		return ToolResult{Content: "Error writing file: " + err.Error(), IsError: true}, nil
	}
	return ToolResult{Content: fmt.Sprintf("Replaced %d occurrence(s) in %s", count, filePath)}, nil
}
