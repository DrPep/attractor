package llm

import (
	"encoding/json"
	"sort"
)

// StreamEventType discriminates streaming events.
type StreamEventType string

const (
	StreamMessageStart  StreamEventType = "message_start"
	StreamContentDelta  StreamEventType = "content_delta"
	StreamToolCallStart StreamEventType = "tool_call_start"
	StreamToolCallDelta StreamEventType = "tool_call_delta"
	StreamThinkingDelta StreamEventType = "thinking_delta"
	StreamMessageEnd    StreamEventType = "message_end"
	StreamError         StreamEventType = "error"
)

// StreamEvent is a single event in a streaming response.
type StreamEvent struct {
	Type StreamEventType

	ID    string
	Model string

	Text  string
	Index int

	ToolCallID string
	ToolName   string

	ArgumentsChunk string

	FinishReason FinishReason
	Usage        *Usage

	ErrorType    string
	ErrorMessage string
}

// MessageStartEvent builds a message_start event.
func MessageStartEvent(id, model string) StreamEvent {
	return StreamEvent{Type: StreamMessageStart, ID: id, Model: model}
}

// ContentDeltaEvent builds a content_delta event.
func ContentDeltaEvent(text string, index int) StreamEvent {
	return StreamEvent{Type: StreamContentDelta, Text: text, Index: index}
}

// ToolCallStartEvent builds a tool_call_start event.
func ToolCallStartEvent(id, name string, index int) StreamEvent {
	return StreamEvent{Type: StreamToolCallStart, ToolCallID: id, ToolName: name, Index: index}
}

// ToolCallDeltaEvent builds a tool_call_delta event.
func ToolCallDeltaEvent(chunk string, index int) StreamEvent {
	return StreamEvent{Type: StreamToolCallDelta, ArgumentsChunk: chunk, Index: index}
}

// ThinkingDeltaEvent builds a thinking_delta event.
func ThinkingDeltaEvent(text string, index int) StreamEvent {
	return StreamEvent{Type: StreamThinkingDelta, Text: text, Index: index}
}

// MessageEndEvent builds a message_end event.
func MessageEndEvent(fr FinishReason, usage *Usage) StreamEvent {
	return StreamEvent{Type: StreamMessageEnd, FinishReason: fr, Usage: usage}
}

// ErrorEvent builds an error event.
func ErrorEvent(errType, msg string) StreamEvent {
	return StreamEvent{Type: StreamError, ErrorType: errType, ErrorMessage: msg}
}

// StreamAccumulator collects stream deltas into a full Response.
type StreamAccumulator struct {
	ID           string
	Model        string
	Provider     string
	textParts    map[int][]string
	toolCalls    map[int]*accumTool
	thinking     map[int][]string
	finishReason FinishReason
	usage        Usage
}

type accumTool struct {
	id     string
	name   string
	chunks []string
}

// NewStreamAccumulator returns an initialized accumulator.
func NewStreamAccumulator() *StreamAccumulator {
	return &StreamAccumulator{
		textParts:    map[int][]string{},
		toolCalls:    map[int]*accumTool{},
		thinking:     map[int][]string{},
		finishReason: FinishStop,
	}
}

// Process folds a single event into the accumulator.
func (a *StreamAccumulator) Process(e StreamEvent) {
	switch e.Type {
	case StreamMessageStart:
		a.ID = e.ID
		a.Model = e.Model
	case StreamContentDelta:
		a.textParts[e.Index] = append(a.textParts[e.Index], e.Text)
	case StreamToolCallStart:
		a.toolCalls[e.Index] = &accumTool{id: e.ToolCallID, name: e.ToolName}
	case StreamToolCallDelta:
		if t, ok := a.toolCalls[e.Index]; ok {
			t.chunks = append(t.chunks, e.ArgumentsChunk)
		}
	case StreamThinkingDelta:
		a.thinking[e.Index] = append(a.thinking[e.Index], e.Text)
	case StreamMessageEnd:
		a.finishReason = e.FinishReason
		if e.Usage != nil {
			a.usage = *e.Usage
		}
	}
}

// ToResponse assembles the accumulated deltas into a Response.
func (a *StreamAccumulator) ToResponse() Response {
	var parts []ContentPart

	for _, idx := range sortedKeys(a.thinking) {
		parts = append(parts, ThinkingPart(joinAll(a.thinking[idx]), "", false))
	}
	for _, idx := range sortedKeys(a.textParts) {
		parts = append(parts, TextPart(joinAll(a.textParts[idx])))
	}
	toolIdx := make([]int, 0, len(a.toolCalls))
	for k := range a.toolCalls {
		toolIdx = append(toolIdx, k)
	}
	sort.Ints(toolIdx)
	for _, idx := range toolIdx {
		tc := a.toolCalls[idx]
		argsStr := joinAll(tc.chunks)
		var args any
		if argsStr != "" {
			m := map[string]any{}
			if err := json.Unmarshal([]byte(argsStr), &m); err == nil {
				args = m
			} else {
				args = argsStr
			}
		} else {
			args = map[string]any{}
		}
		parts = append(parts, ToolCallPart(tc.id, tc.name, args))
	}

	finish := a.finishReason
	if len(a.toolCalls) > 0 {
		finish = FinishToolUse
	}

	return Response{
		ID:           a.ID,
		Model:        a.Model,
		Provider:     a.Provider,
		Message:      Message{Role: RoleAssistant, Content: parts},
		FinishReason: finish,
		Usage:        a.usage,
	}
}

func sortedKeys(m map[int][]string) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

func joinAll(parts []string) string {
	out := ""
	for _, p := range parts {
		out += p
	}
	return out
}
