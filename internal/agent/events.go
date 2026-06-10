package agent

import (
	"log"
)

// EventType enumerates agent loop events.
type EventType string

const (
	EventTurnStart        EventType = "turn_start"
	EventTurnEnd          EventType = "turn_end"
	EventLLMRequest       EventType = "llm_request"
	EventLLMResponse      EventType = "llm_response"
	EventToolCallStart    EventType = "tool_call_start"
	EventToolCallEnd      EventType = "tool_call_end"
	EventToolError        EventType = "tool_error"
	EventThinking         EventType = "thinking"
	EventSessionStart     EventType = "session_start"
	EventSessionEnd       EventType = "session_end"
	EventSteeringInjected EventType = "steering_injected"
	EventLoopDetected     EventType = "loop_detected"
	EventHistoryTruncated EventType = "history_truncated"
	EventError            EventType = "error"
)

// Event is an agent loop notification carrying structured data.
type Event struct {
	Type EventType
	Data map[string]any
}

// EventHandler receives an event.
type EventHandler func(Event)

// EventBus dispatches events to registered handlers (synchronous).
type EventBus struct {
	handlers       map[EventType][]EventHandler
	globalHandlers []EventHandler
}

// NewEventBus returns an empty bus.
func NewEventBus() *EventBus { return &EventBus{handlers: map[EventType][]EventHandler{}} }

// On registers a handler for a specific event type.
func (b *EventBus) On(t EventType, h EventHandler) { b.handlers[t] = append(b.handlers[t], h) }

// OnAll registers a handler for every event.
func (b *EventBus) OnAll(h EventHandler) { b.globalHandlers = append(b.globalHandlers, h) }

// Emit dispatches an event to all relevant handlers, isolating panics.
func (b *EventBus) Emit(e Event) {
	handlers := append(append([]EventHandler{}, b.globalHandlers...), b.handlers[e.Type]...)
	for _, h := range handlers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Error in event handler for %s: %v", e.Type, r)
				}
			}()
			h(e)
		}()
	}
}

// --- event constructors ---

func turnStartEvent(turn int) Event {
	return Event{Type: EventTurnStart, Data: map[string]any{"turn_number": turn}}
}
func turnEndEvent(turn, toolCalls int) Event {
	return Event{Type: EventTurnEnd, Data: map[string]any{"turn_number": turn, "tool_calls": toolCalls}}
}
func llmRequestEvent(model string, messageCount int) Event {
	return Event{Type: EventLLMRequest, Data: map[string]any{"model": model, "message_count": messageCount}}
}
func llmResponseEvent(model, finishReason string, tokens int) Event {
	return Event{Type: EventLLMResponse, Data: map[string]any{"model": model, "finish_reason": finishReason, "tokens": tokens}}
}
func toolCallStartEvent(name string, args map[string]any) Event {
	return Event{Type: EventToolCallStart, Data: map[string]any{"tool_name": name, "args": args}}
}
func toolCallEndEvent(name string, success bool, durationMS float64) Event {
	return Event{Type: EventToolCallEnd, Data: map[string]any{"tool_name": name, "success": success, "duration_ms": durationMS}}
}
func toolErrorEvent(name, errMsg string) Event {
	return Event{Type: EventToolError, Data: map[string]any{"tool_name": name, "error": errMsg}}
}
func thinkingEvent(text string) Event {
	return Event{Type: EventThinking, Data: map[string]any{"text": text}}
}
