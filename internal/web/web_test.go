package web

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedRun writes a minimal on-disk run: pipeline.dot plus one node with status.
func seedRun(t *testing.T, runsDir, runID string) {
	t.Helper()
	nodeDir := filepath.Join(runsDir, runID, "greet")
	if err := os.MkdirAll(nodeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(runsDir, runID, "pipeline.dot"),
		[]byte("digraph g { greet [shape=box]; }"), 0o644))
	must(os.WriteFile(filepath.Join(nodeDir, "prompt.md"), []byte("say hi"), 0o644))
	must(os.WriteFile(filepath.Join(nodeDir, "response.md"), []byte("hi!"), 0o644))
	must(os.WriteFile(filepath.Join(nodeDir, "status.json"),
		[]byte(`{"outcome":"success","notes":"done"}`), 0o644))
}

func TestListAndGetDiskRun(t *testing.T) {
	dir := t.TempDir()
	seedRun(t, dir, "abc123")
	srv := NewServer(dir, nil)

	// list
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest("GET", "/api/runs", nil))
	if rr.Code != 200 {
		t.Fatalf("list status = %d", rr.Code)
	}
	var list struct {
		Runs []runEntry `json:"runs"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Runs) != 1 || list.Runs[0].RunID != "abc123" || !list.Runs[0].OnDisk {
		t.Fatalf("unexpected runs: %+v", list.Runs)
	}

	// get run — graph_dot from disk, node_state folded from status.json
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest("GET", "/api/runs/abc123", nil))
	if rr.Code != 200 {
		t.Fatalf("get run status = %d", rr.Code)
	}
	var run struct {
		GraphDOT  string                    `json:"graph_dot"`
		NodeState map[string]map[string]any `json:"node_state"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(run.GraphDOT, "digraph") {
		t.Errorf("graph_dot missing: %q", run.GraphDOT)
	}
	if got := run.NodeState["greet"]["status"]; got != "success" {
		t.Errorf("greet status = %v, want success", got)
	}

	// get node artifacts
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest("GET", "/api/runs/abc123/nodes/greet", nil))
	if rr.Code != 200 {
		t.Fatalf("get node status = %d", rr.Code)
	}
	var node struct {
		Prompt, Response string
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &node)
	if node.Prompt != "say hi" || node.Response != "hi!" {
		t.Errorf("node artifacts wrong: %+v", node)
	}

	// unknown run → 404
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest("GET", "/api/runs/nope", nil))
	if rr.Code != 404 {
		t.Errorf("unknown run status = %d, want 404", rr.Code)
	}
}

func TestHubNodeStateFolding(t *testing.T) {
	hub := NewEventHub()
	hub.Publish("r", "node_start", map[string]any{"node_id": "a"})
	hub.Publish("r", "node_end", map[string]any{"node_id": "a", "outcome": "fail"})
	hub.Publish("r", "edge", map[string]any{"source": "a", "target": "b"})

	snap := hub.Snapshot("r")
	if !snap.Known {
		t.Fatal("run should be known")
	}
	if got := snap.NodeState["a"]["status"]; got != "fail" {
		t.Errorf("a status = %v, want fail", got)
	}
	if got := snap.NodeState["b"]["status"]; got != "queued" {
		t.Errorf("b status = %v, want queued", got)
	}
}

func TestHubTokenAccumulation(t *testing.T) {
	hub := NewEventHub()
	emit := func(tokens int) {
		hub.Publish("r", "agent_event", map[string]any{
			"node_id": "a",
			"type":    "llm_response",
			"payload": map[string]any{"tokens": tokens},
		})
	}
	emit(100)
	emit(250)
	// A non-llm_response agent_event must not affect the token total.
	hub.Publish("r", "agent_event", map[string]any{
		"node_id": "a", "type": "tool_call_end", "payload": map[string]any{"tokens": 999},
	})

	snap := hub.Snapshot("r")
	if got := snap.NodeState["a"]["tokens"]; got != 350 {
		t.Errorf("a tokens = %v, want 350", got)
	}
}

func TestSSEStream(t *testing.T) {
	dir := t.TempDir()
	srv := NewServer(dir, nil)
	hub := srv.Hub()

	// Pre-populate one buffered event, then a terminal run_end.
	hub.Publish("live1", "node_start", map[string]any{"node_id": "a"})
	go hub.MarkFinished("live1")

	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/runs/live1/events?since=0")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q", ct)
	}

	var kinds []string
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "event: ") {
			k := strings.TrimPrefix(line, "event: ")
			kinds = append(kinds, k)
			if k == "run_end" {
				break
			}
		}
	}
	if len(kinds) == 0 || kinds[len(kinds)-1] != "run_end" {
		t.Errorf("stream kinds = %v, want trailing run_end", kinds)
	}
}
