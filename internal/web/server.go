package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed static
var staticFS embed.FS

// Server bundles the runs directory, the live event hub, and the HTTP handler.
type Server struct {
	runsDir string
	hub     *EventHub
	mux     *http.ServeMux
}

// NewServer builds a web server over runsDir using hub for live runs. If hub is
// nil a fresh one is created (history-only mode).
func NewServer(runsDir string, hub *EventHub) *Server {
	if hub == nil {
		hub = NewEventHub()
	}
	s := &Server{runsDir: runsDir, hub: hub, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Hub exposes the underlying event hub so callers can bridge a runner into it.
func (s *Server) Hub() *EventHub { return s.hub }

// ListenAndServe starts the HTTP server on addr (host:port) and blocks until
// the context is cancelled, then shuts it down gracefully.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{Addr: addr, Handler: s}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/runs", s.handleListRuns)
	s.mux.HandleFunc("GET /api/runs/{run_id}", s.handleGetRun)
	s.mux.HandleFunc("GET /api/runs/{run_id}/nodes/{node_id}", s.handleGetNode)
	s.mux.HandleFunc("POST /api/runs/{run_id}/nodes/{node_id}/steer", s.handleSteer)
	s.mux.HandleFunc("GET /api/runs/{run_id}/events", s.handleEvents)

	sub, _ := fs.Sub(staticFS, "static")
	s.mux.Handle("/", http.FileServer(http.FS(sub)))
}

// ── JSON helpers ────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// ── handlers ────────────────────────────────────────────────────────────────

type runEntry struct {
	RunID  string `json:"run_id"`
	Live   bool   `json:"live"`
	OnDisk bool   `json:"on_disk"`
}

func (s *Server) handleListRuns(w http.ResponseWriter, _ *http.Request) {
	disk := s.listDiskRuns()
	diskSet := map[string]bool{}
	for _, id := range disk {
		diskSet[id] = true
	}
	live := map[string]bool{}
	for _, id := range s.hub.ListRuns() {
		// Treat a run as live only while it hasn't finished.
		if snap := s.hub.Snapshot(id); snap.Known && !snap.Finished {
			live[id] = true
		}
	}

	var liveIDs, diskOnly []string
	for id := range live {
		liveIDs = append(liveIDs, id)
	}
	sort.Strings(liveIDs)
	for _, id := range disk {
		if !live[id] {
			diskOnly = append(diskOnly, id)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(diskOnly)))

	runs := make([]runEntry, 0, len(liveIDs)+len(diskOnly))
	for _, id := range liveIDs {
		runs = append(runs, runEntry{RunID: id, Live: true, OnDisk: diskSet[id]})
	}
	for _, id := range diskOnly {
		runs = append(runs, runEntry{RunID: id, Live: false, OnDisk: true})
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	snap := s.hub.Snapshot(runID)
	runDir := filepath.Join(s.runsDir, runID)

	graphDOT := snap.GraphDOT
	if graphDOT == "" {
		graphDOT = s.readGraphDOT(runDir)
	}

	nodeState := snap.NodeState
	if nodeState == nil {
		nodeState = map[string]map[string]any{}
	}
	// Fold in on-disk statuses for nodes not tracked live.
	for nid, disk := range s.readDiskStatuses(runDir) {
		entry := nodeState[nid]
		if entry == nil {
			entry = map[string]any{}
			nodeState[nid] = entry
		}
		if _, ok := entry["status"]; !ok {
			entry["status"] = disk.status
		}
		if _, ok := entry["tokens"]; !ok && disk.tokens > 0 {
			entry["tokens"] = disk.tokens
		}
	}

	if graphDOT == "" && len(nodeState) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]any{"detail": "unknown run " + runID})
		return
	}

	// A run the hub isn't actively tracking is historical, hence finished — so
	// the client doesn't open a stream that would never see a run_end.
	live := snap.Known && !snap.Finished
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":     runID,
		"live":       live,
		"finished":   !live,
		"graph_dot":  graphDOT,
		"node_state": nodeState,
		"next_seq":   snap.NextSeq,
	})
}

func (s *Server) handleGetNode(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	nodeID := r.PathValue("node_id")
	nodeDir := filepath.Join(s.runsDir, runID, nodeID)
	if _, err := os.Stat(nodeDir); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"detail": "no artifacts for " + nodeID})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"node_id":  nodeID,
		"prompt":   readText(filepath.Join(nodeDir, "prompt.md")),
		"response": readText(filepath.Join(nodeDir, "response.md")),
		"status":   readJSON(filepath.Join(nodeDir, "status.json")),
	})
}

// handleSteer injects a corrective feedback message into a running node's agent
// session. It succeeds only while that node has a live session registered.
func (s *Server) handleSteer(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	nodeID := r.PathValue("node_id")

	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "invalid JSON body"})
		return
	}
	if strings.TrimSpace(body.Message) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "message is required"})
		return
	}

	if !s.hub.Steer(runID, nodeID, body.Message) {
		writeJSON(w, http.StatusConflict, map[string]any{"detail": "node " + nodeID + " is not accepting feedback"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued"})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	runID := r.PathValue("run_id")
	since := 0
	if v := r.URL.Query().Get("since"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			since = n
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	ch, backlog, unsub := s.hub.Subscribe(runID, since)
	defer unsub()

	send := func(ev RunEvent) bool {
		payload, _ := json.Marshal(ev)
		if _, err := fmt.Fprintf(w, "event: %s\nid: %d\ndata: %s\n\n", ev.Kind, ev.Seq, payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	for _, ev := range backlog {
		if !send(ev) {
			return
		}
	}

	ctx := r.Context()
	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-ch:
			if !send(ev) {
				return
			}
			if ev.Kind == "run_end" {
				return
			}
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// ── disk helpers ────────────────────────────────────────────────────────────

func (s *Server) listDiskRuns() []string {
	entries, err := os.ReadDir(s.runsDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

func (s *Server) readGraphDOT(runDir string) string {
	for _, name := range []string{"pipeline.dot", "graph.dot", "source.dot"} {
		if txt := readText(filepath.Join(runDir, name)); txt != "" {
			return txt
		}
	}
	return ""
}

// diskStatus is the subset of a node's persisted status.json the run view needs.
type diskStatus struct {
	status string
	tokens int
}

func (s *Server) readDiskStatuses(runDir string) map[string]diskStatus {
	out := map[string]diskStatus{}
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data := readJSON(filepath.Join(runDir, e.Name(), "status.json"))
		if data == nil {
			continue
		}
		ds := diskStatus{status: "done"}
		if outcome, ok := data["outcome"].(string); ok && outcome != "" {
			ds.status = outcome
		}
		if usage, ok := data["usage"].(map[string]any); ok {
			in, _ := toInt(usage["input_tokens"])
			outT, _ := toInt(usage["output_tokens"])
			ds.tokens = in + outT
		}
		out[e.Name()] = ds
	}
	return out
}

func readText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func readJSON(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var v map[string]any
	if json.Unmarshal(data, &v) != nil {
		return nil
	}
	return v
}
