package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Checkpoint captures resumable pipeline state. JSON field names match the
// Python implementation so prior runs remain loadable.
type Checkpoint struct {
	Timestamp       string         `json:"timestamp"`
	RunID           string         `json:"run_id"`
	CurrentNode     string         `json:"current_node"`
	CompletedNodes  []string       `json:"completed_nodes"`
	NodeRetries     map[string]int `json:"node_retries"`
	ContextSnapshot map[string]any `json:"context_snapshot"`
}

// SaveCheckpoint writes the checkpoint atomically (temp file + rename).
func SaveCheckpoint(cp Checkpoint, runDir string) error {
	if cp.Timestamp == "" {
		cp.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if cp.CompletedNodes == nil {
		cp.CompletedNodes = []string{}
	}
	if cp.NodeRetries == nil {
		cp.NodeRetries = map[string]int{}
	}
	if cp.ContextSnapshot == nil {
		cp.ContextSnapshot = map[string]any{}
	}
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(runDir, "*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, filepath.Join(runDir, "checkpoint.json"))
}

// LoadCheckpoint reads a checkpoint from a run directory. Returns (nil, nil)
// when no checkpoint exists.
func LoadCheckpoint(runDir string) (*Checkpoint, error) {
	path := filepath.Join(runDir, "checkpoint.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, nil
	}
	return &cp, nil
}
