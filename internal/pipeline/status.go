package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/nigelpepper/attractor/internal/llm"
)

// NodeStatus is the persisted result of a completed node.
type NodeStatus struct {
	Outcome            string         `json:"outcome"`
	PreferredNextLabel string         `json:"preferred_next_label"`
	SuggestedNextIDs   []string       `json:"suggested_next_ids"`
	ContextUpdates     map[string]any `json:"context_updates"`
	Notes              string         `json:"notes"`
	Usage              *llm.Usage     `json:"usage,omitempty"`
}

// WriteStatus writes status.json for a node.
func WriteStatus(nodeID string, status NodeStatus, runDir string) error {
	if status.Outcome == "" {
		status.Outcome = "success"
	}
	if status.SuggestedNextIDs == nil {
		status.SuggestedNextIDs = []string{}
	}
	if status.ContextUpdates == nil {
		status.ContextUpdates = map[string]any{}
	}
	dir := filepath.Join(runDir, nodeID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "status.json"), data, 0o644)
}

// ReadStatus reads status.json for a node, or (nil, nil) if absent.
func ReadStatus(nodeID, runDir string) (*NodeStatus, error) {
	data, err := os.ReadFile(filepath.Join(runDir, nodeID, "status.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var s NodeStatus
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, nil
	}
	return &s, nil
}

// WritePrompt writes prompt.md for a node.
func WritePrompt(nodeID, prompt, runDir string) error {
	dir := filepath.Join(runDir, nodeID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "prompt.md"), []byte(prompt), 0o644)
}

// WriteResponse writes response.md for a node.
func WriteResponse(nodeID, response, runDir string) error {
	dir := filepath.Join(runDir, nodeID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "response.md"), []byte(response), 0o644)
}

// EnsureRunDir creates the run directory structure.
func EnsureRunDir(runDir string) error {
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755)
}
