package web

import (
	"github.com/nigelpepper/attractor/internal/agent"
	"github.com/nigelpepper/attractor/internal/pipeline"
)

// Callbacks adapts the PipelineRunner's callback hooks into hub events for a
// given run. Spread the returned struct's fields into RunnerOptions.
type Callbacks struct {
	OnNodeStart  func(node *pipeline.Node, index, total int)
	OnNodeEnd    func(nodeID, status string)
	OnEdge       func(src, target, label string)
	OnRetry      func(nodeID string, attempt, maxRetries int, delay float64)
	OnAgentEvent func(agent.Event)
}

// BridgeRunner builds the callback set that streams a run into the hub.
func BridgeRunner(hub *EventHub, runID string) Callbacks {
	return Callbacks{
		OnNodeStart: func(node *pipeline.Node, index, total int) {
			label := node.Label
			if label == "" {
				label = node.ID
			}
			hub.Publish(runID, "node_start", map[string]any{
				"node_id": node.ID,
				"label":   label,
				"type":    string(node.Type),
				"index":   index,
				"total":   total,
			})
		},
		OnNodeEnd: func(nodeID, status string) {
			hub.Publish(runID, "node_end", map[string]any{
				"node_id": nodeID,
				"outcome": status,
			})
		},
		OnEdge: func(src, target, label string) {
			hub.Publish(runID, "edge", map[string]any{
				"source": src,
				"target": target,
				"label":  label,
			})
		},
		OnRetry: func(nodeID string, attempt, maxRetries int, delay float64) {
			hub.Publish(runID, "retry", map[string]any{
				"node_id":     nodeID,
				"attempt":     attempt,
				"max_retries": maxRetries,
				"delay":       delay,
			})
		},
		OnAgentEvent: func(ev agent.Event) {
			data := map[string]any{
				"type":    string(ev.Type),
				"payload": ev.Data,
			}
			if ev.Data != nil {
				if nid, ok := ev.Data["node_id"].(string); ok && nid != "" {
					data["node_id"] = nid
				}
			}
			hub.Publish(runID, "agent_event", data)
		},
	}
}
