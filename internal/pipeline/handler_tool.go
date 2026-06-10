package pipeline

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ToolHandler executes a shell command from the node's command/prompt attribute.
type ToolHandler struct{}

// Execute runs the node's command in the run directory.
func (h *ToolHandler) Execute(ctx context.Context, node *Node, pctx *PipelineContext, g *Graph, runDir string) (Outcome, error) {
	command := node.Attrs["command"]
	if command == "" {
		command = node.Prompt()
	}
	if command == "" {
		return Outcome{Status: "fail", Notes: "No command specified"}, nil
	}

	goal := pctx.GetString("graph.goal")
	command = strings.ReplaceAll(command, "$goal", goal)

	timeout := 120 * time.Second
	if t := node.Timeout(); t != nil {
		timeout = *t
	}

	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, "/bin/sh", "-c", command)
	cmd.Dir = runDir
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	_ = WriteResponse(node.ID, stdout.String()+stderr.String(), runDir)

	if cctx.Err() == context.DeadlineExceeded {
		return Outcome{Status: "fail", Notes: fmt.Sprintf("Command timed out after %.0fs", timeout.Seconds())}, nil
	}
	if err == nil {
		return Outcome{
			Status:         "success",
			Notes:          "Command completed (exit 0)",
			ContextUpdates: map[string]any{node.ID + ".output": strings.TrimSpace(stdout.String())},
		}, nil
	}

	exitCode := -1
	if ee, ok := err.(*exec.ExitError); ok {
		exitCode = ee.ExitCode()
	}
	errOut := stderr.String()
	if len(errOut) > 200 {
		errOut = errOut[:200]
	}
	return Outcome{Status: "fail", Notes: fmt.Sprintf("Command failed (exit %d): %s", exitCode, errOut)}, nil
}
