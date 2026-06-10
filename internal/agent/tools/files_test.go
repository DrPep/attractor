package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Fix 6: an explicit limit=0 falls back to the default instead of reading zero
// lines.
func TestReadFileZeroLimitFallsBack(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("alpha\nbravo\ncharlie\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := NewLocalEnvironment(dir)
	tool := &ReadFileTool{}

	res, err := tool.Execute(context.Background(), map[string]any{"file_path": "f.txt", "limit": float64(0)}, env)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || res.Content == "" {
		t.Fatalf("limit=0 returned empty/error: %+v", res)
	}
	for _, want := range []string{"alpha", "bravo", "charlie"} {
		if !strings.Contains(res.Content, want) {
			t.Errorf("content missing %q: %q", want, res.Content)
		}
	}
}
