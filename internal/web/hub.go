// Package web provides the Attractor web UI: a live pipeline visualizer backed
// by an in-memory event hub plus a REST/SSE API over the runs/ directory.
package web

import (
	"sync"
	"time"
)

// RunEvent is a single timestamped event in a run's stream.
type RunEvent struct {
	RunID string         `json:"run_id"`
	Seq   int            `json:"seq"`
	Ts    float64        `json:"ts"`
	Kind  string         `json:"kind"`
	Data  map[string]any `json:"data"`
}

const ringSize = 2000

// runState holds the buffered history and live subscribers for one run.
type runState struct {
	buffer      []RunEvent
	subscribers map[chan RunEvent]struct{}
	nextSeq     int
	finished    bool
	graphDOT    string
	nodeState   map[string]map[string]any
}

func newRunState() *runState {
	return &runState{
		subscribers: map[chan RunEvent]struct{}{},
		nodeState:   map[string]map[string]any{},
	}
}

// EventHub is a process-wide pub/sub keyed by run_id. One hub per server; a
// running pipeline publishes into it and SSE clients subscribe.
type EventHub struct {
	mu   sync.Mutex
	runs map[string]*runState
}

// NewEventHub returns an empty hub.
func NewEventHub() *EventHub {
	return &EventHub{runs: map[string]*runState{}}
}

func (h *EventHub) run(runID string) *runState {
	st := h.runs[runID]
	if st == nil {
		st = newRunState()
		h.runs[runID] = st
	}
	return st
}

// SetGraph records the DOT source for a run so late-joining clients can render it.
func (h *EventHub) SetGraph(runID, dotSource string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.run(runID).graphDOT = dotSource
}

// ListRuns returns the run IDs known to the hub (live or recently finished).
func (h *EventHub) ListRuns() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	ids := make([]string, 0, len(h.runs))
	for id := range h.runs {
		ids = append(ids, id)
	}
	return ids
}

// Snapshot returns the current graph + per-node state for a run.
type Snapshot struct {
	Known     bool
	Finished  bool
	GraphDOT  string
	NodeState map[string]map[string]any
	NextSeq   int
}

// Snapshot returns a point-in-time view of a run for HTTP clients.
func (h *EventHub) Snapshot(runID string) Snapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	st := h.runs[runID]
	if st == nil {
		return Snapshot{}
	}
	// Deep-ish copy of node state so callers can't race the hub.
	ns := make(map[string]map[string]any, len(st.nodeState))
	for k, v := range st.nodeState {
		entry := make(map[string]any, len(v))
		for ek, ev := range v {
			entry[ek] = ev
		}
		ns[k] = entry
	}
	return Snapshot{
		Known:     true,
		Finished:  st.finished,
		GraphDOT:  st.graphDOT,
		NodeState: ns,
		NextSeq:   st.nextSeq,
	}
}

// Publish appends an event to a run and fans it out to live subscribers.
func (h *EventHub) Publish(runID, kind string, data map[string]any) RunEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	st := h.run(runID)
	ev := RunEvent{
		RunID: runID,
		Seq:   st.nextSeq,
		Ts:    float64(time.Now().UnixNano()) / 1e9,
		Kind:  kind,
		Data:  data,
	}
	st.nextSeq++
	st.buffer = append(st.buffer, ev)
	if len(st.buffer) > ringSize {
		st.buffer = st.buffer[len(st.buffer)-ringSize:]
	}
	applyToNodeState(st, ev)
	for ch := range st.subscribers {
		select {
		case ch <- ev:
		default: // slow subscriber: drop rather than block the runner
		}
	}
	return ev
}

// MarkFinished flags a run as complete and emits a terminal run_end event.
func (h *EventHub) MarkFinished(runID string) {
	h.mu.Lock()
	st := h.run(runID)
	st.finished = true
	h.mu.Unlock()
	h.Publish(runID, "run_end", map[string]any{})
}

// Subscribe registers a subscriber and returns the channel plus the buffered
// backlog from sinceSeq onward. The caller must call unsub when done.
func (h *EventHub) Subscribe(runID string, sinceSeq int) (ch chan RunEvent, backlog []RunEvent, unsub func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	st := h.run(runID)
	ch = make(chan RunEvent, 256)
	st.subscribers[ch] = struct{}{}
	for _, ev := range st.buffer {
		if ev.Seq >= sinceSeq {
			backlog = append(backlog, ev)
		}
	}
	unsub = func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(st.subscribers, ch)
	}
	return ch, backlog, unsub
}

// applyToNodeState folds an event into the per-node status snapshot so a client
// that joins late sees the right graph colours without replaying everything.
func applyToNodeState(st *runState, ev RunEvent) {
	d := ev.Data
	nid, _ := d["node_id"].(string)
	entry := func(id string) map[string]any {
		e := st.nodeState[id]
		if e == nil {
			e = map[string]any{}
			st.nodeState[id] = e
		}
		return e
	}
	switch ev.Kind {
	case "node_start":
		if nid != "" {
			e := entry(nid)
			e["status"] = "running"
			e["started_ts"] = ev.Ts
			e["index"] = d["index"]
			e["total"] = d["total"]
		}
	case "node_end":
		if nid != "" {
			e := entry(nid)
			if out, ok := d["outcome"].(string); ok && out != "" {
				e["status"] = out
			} else {
				e["status"] = "done"
			}
			e["ended_ts"] = ev.Ts
		}
	case "edge":
		if tgt, ok := d["target"].(string); ok && tgt != "" {
			e := entry(tgt)
			if _, seen := e["status"]; !seen {
				e["status"] = "queued"
			}
		}
	case "retry":
		if nid != "" {
			e := entry(nid)
			e["status"] = "retrying"
			e["attempt"] = d["attempt"]
		}
	}
}
