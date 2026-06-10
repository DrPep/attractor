package agent

import (
	"encoding/json"
	"fmt"

	"github.com/nigelpepper/attractor/internal/llm"
)

// TruncationStrategy selects how history is trimmed to fit the context window.
type TruncationStrategy string

const (
	TruncationNone          TruncationStrategy = "none"
	TruncationSlidingWindow TruncationStrategy = "sliding_window"
	TruncationHeadTail      TruncationStrategy = "head_tail"
)

func toolCallIDs(m llm.Message) map[string]bool {
	out := map[string]bool{}
	for _, p := range m.Content {
		if p.Kind == llm.KindToolCall && p.ToolCall != nil {
			out[p.ToolCall.ID] = true
		}
	}
	return out
}

func toolResultIDs(m llm.Message) map[string]bool {
	out := map[string]bool{}
	for _, p := range m.Content {
		if p.Kind == llm.KindToolResult && p.ToolResult != nil {
			out[p.ToolResult.ToolCallID] = true
		}
	}
	return out
}

func subset(a, b map[string]bool) bool {
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// healOrphanToolPairs drops messages that would leave tool_use/tool_result
// pairs broken, iterating to a fixed point. Mirrors the Python implementation.
func healOrphanToolPairs(messages []llm.Message) []llm.Message {
	current := append([]llm.Message{}, messages...)
	for {
		n := len(current)
		producedBefore := make([]map[string]bool, n)
		seen := map[string]bool{}
		for i, m := range current {
			cp := map[string]bool{}
			for k := range seen {
				cp[k] = true
			}
			producedBefore[i] = cp
			for k := range toolCallIDs(m) {
				seen[k] = true
			}
		}

		resultsAfter := make([]map[string]bool, n)
		seen2 := map[string]bool{}
		for i := n - 1; i >= 0; i-- {
			cp := map[string]bool{}
			for k := range seen2 {
				cp[k] = true
			}
			resultsAfter[i] = cp
			for k := range toolResultIDs(current[i]) {
				seen2[k] = true
			}
		}

		dropped := false
		var out []llm.Message
		for i, m := range current {
			rIDs := toolResultIDs(m)
			if len(rIDs) > 0 && !subset(rIDs, producedBefore[i]) {
				dropped = true
				continue
			}
			cIDs := toolCallIDs(m)
			isLast := i == n-1
			if len(cIDs) > 0 && !isLast && !subset(cIDs, resultsAfter[i]) {
				dropped = true
				continue
			}
			out = append(out, m)
		}
		if !dropped {
			return out
		}
		current = out
	}
}

// estimateTokens roughly estimates token count (~4 chars per token).
func estimateTokens(messages []llm.Message) int {
	total := 0
	for _, m := range messages {
		for _, p := range m.Content {
			switch p.Kind {
			case llm.KindText:
				total += len(p.Text) / 4
			case llm.KindToolCall:
				if p.ToolCall != nil {
					var argStr string
					if s, ok := p.ToolCall.Arguments.(string); ok {
						argStr = s
					} else {
						b, _ := json.Marshal(p.ToolCall.Arguments)
						argStr = string(b)
					}
					total += (len(p.ToolCall.Name) + len(argStr)) / 4
				}
			case llm.KindToolResult:
				if p.ToolResult != nil {
					total += len(fmt.Sprint(p.ToolResult.Content)) / 4
				}
			}
		}
	}
	return total
}

// ConversationHistory manages messages with truncation support.
type ConversationHistory struct {
	messages []llm.Message
}

// NewConversationHistory returns an empty history.
func NewConversationHistory() *ConversationHistory { return &ConversationHistory{} }

// Messages returns the current message slice.
func (h *ConversationHistory) Messages() []llm.Message { return h.messages }

// Add appends a message.
func (h *ConversationHistory) Add(m llm.Message) { h.messages = append(h.messages, m) }

// TokenEstimate returns the rough token count of the history.
func (h *ConversationHistory) TokenEstimate() int { return estimateTokens(h.messages) }

// Truncate trims history if it exceeds maxTokens. Returns true if it changed.
func (h *ConversationHistory) Truncate(strategy TruncationStrategy, maxTokens int) bool {
	if strategy == TruncationNone {
		return false
	}
	if h.TokenEstimate() <= maxTokens {
		return false
	}
	switch strategy {
	case TruncationSlidingWindow:
		return h.truncateSlidingWindow(maxTokens)
	case TruncationHeadTail:
		return h.truncateHeadTail(maxTokens)
	}
	return false
}

func (h *ConversationHistory) splitSystem() (system, nonSystem []llm.Message) {
	for _, m := range h.messages {
		if m.Role == llm.RoleSystem {
			system = append(system, m)
		} else {
			nonSystem = append(nonSystem, m)
		}
	}
	return
}

func (h *ConversationHistory) truncateSlidingWindow(maxTokens int) bool {
	system, nonSystem := h.splitSystem()
	budget := maxTokens - estimateTokens(system)

	var kept []llm.Message
	running := 0
	for i := len(nonSystem) - 1; i >= 0; i-- {
		mt := estimateTokens([]llm.Message{nonSystem[i]})
		if running+mt > budget {
			break
		}
		kept = append([]llm.Message{nonSystem[i]}, kept...)
		running += mt
	}
	if len(kept) == 0 && len(nonSystem) > 0 {
		kept = []llm.Message{nonSystem[len(nonSystem)-1]}
	}
	kept = healOrphanToolPairs(kept)

	if len(kept) < len(nonSystem) {
		summary := llm.SystemMessage("[Earlier conversation truncated to fit context window]")
		h.messages = append(append(append([]llm.Message{}, system...), summary), kept...)
		return true
	}
	return false
}

func (h *ConversationHistory) truncateHeadTail(maxTokens int) bool {
	system, nonSystem := h.splitSystem()
	if len(nonSystem) <= 4 {
		return false
	}
	budget := maxTokens - estimateTokens(system) - 50

	head := nonSystem[:2]
	tailBudget := budget - estimateTokens(head)

	var tail []llm.Message
	running := 0
	for i := len(nonSystem) - 1; i >= 2; i-- {
		mt := estimateTokens([]llm.Message{nonSystem[i]})
		if running+mt > tailBudget {
			break
		}
		tail = append([]llm.Message{nonSystem[i]}, tail...)
		running += mt
	}

	combined := healOrphanToolPairs(append(append([]llm.Message{}, head...), tail...))
	if len(combined) < len(nonSystem) {
		summary := llm.SystemMessage(fmt.Sprintf("[%d messages truncated]", len(nonSystem)-len(combined)))
		h.messages = append(append(append([]llm.Message{}, system...), summary), combined...)
		return true
	}
	return false
}
