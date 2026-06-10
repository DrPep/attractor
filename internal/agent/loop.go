package agent

import (
	"context"
	"time"

	"github.com/nigelpepper/attractor/internal/agent/tools"
	"github.com/nigelpepper/attractor/internal/llm"
)

// SessionConfig parameterizes an agent session.
type SessionConfig struct {
	MaxTurns           int
	MaxTokens          int
	Model              string
	Provider           string
	Temperature        *float64
	ReasoningEffort    string
	TruncationStrategy TruncationStrategy
	MaxContextTokens   int
}

// DefaultSessionConfig returns a config with sensible defaults for a model.
func DefaultSessionConfig(model string) SessionConfig {
	if model == "" {
		model = "claude-opus-4-7"
	}
	return SessionConfig{
		MaxTurns:           50,
		Model:              model,
		TruncationStrategy: TruncationSlidingWindow,
		MaxContextTokens:   100000,
	}
}

// TurnResult summarizes a completed turn.
type TurnResult struct {
	Messages      []llm.Message
	FinalResponse string
	ToolCallsMade int
	TurnsUsed     int
}

// AgentLoop runs the core agentic cycle: LLM → tools → repeat.
type AgentLoop struct {
	client       *llm.Client
	tools        *tools.ToolRegistry
	events       *EventBus
	loopDetector *LoopDetector
	steering     *SteeringManager
	env          tools.ExecutionEnvironment
}

// NewAgentLoop constructs an agent loop.
func NewAgentLoop(client *llm.Client, registry *tools.ToolRegistry, events *EventBus, ld *LoopDetector, steering *SteeringManager) *AgentLoop {
	return &AgentLoop{client: client, tools: registry, events: events, loopDetector: ld, steering: steering}
}

// SetExecutionEnv sets the environment tools run against.
func (a *AgentLoop) SetExecutionEnv(env tools.ExecutionEnvironment) { a.env = env }

// RunTurn runs a complete turn until the model returns a text-only response,
// the turn cap is reached, or the context is cancelled.
func (a *AgentLoop) RunTurn(ctx context.Context, history *ConversationHistory, systemPrompt string, config SessionConfig) (TurnResult, error) {
	result := TurnResult{}
	totalToolCalls := 0

	for turn := 0; turn < config.MaxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		result.TurnsUsed = turn + 1
		a.events.Emit(turnStartEvent(turn + 1))

		for _, msg := range a.steering.DrainSteering() {
			history.Add(msg)
			result.Messages = append(result.Messages, msg)
			a.events.Emit(Event{Type: EventSteeringInjected, Data: map[string]any{"text": msg.Text()}})
		}

		history.Truncate(config.TruncationStrategy, config.MaxContextTokens)

		messages := append([]llm.Message{llm.SystemMessage(systemPrompt)}, history.Messages()...)
		toolDefs := a.tools.ListDefinitions()

		req := llm.Request{
			Model:           config.Model,
			Messages:        messages,
			Provider:        config.Provider,
			Tools:           toolDefs,
			Temperature:     config.Temperature,
			MaxTokens:       config.MaxTokens,
			ReasoningEffort: config.ReasoningEffort,
		}

		a.events.Emit(llmRequestEvent(config.Model, len(messages)))

		resp, err := a.client.Complete(ctx, req)
		if err != nil {
			return result, err
		}

		a.events.Emit(llmResponseEvent(resp.Model, string(resp.FinishReason), resp.Usage.TotalTokens()))

		for _, p := range resp.Message.Content {
			if p.Kind == llm.KindThinking && p.Thinking != nil {
				a.events.Emit(thinkingEvent(p.Thinking.Text))
			}
		}

		history.Add(resp.Message)
		result.Messages = append(result.Messages, resp.Message)

		toolCalls := resp.ToolCalls()
		if len(toolCalls) == 0 {
			result.FinalResponse = resp.Text()
			a.events.Emit(turnEndEvent(turn+1, totalToolCalls))
			result.ToolCallsMade = totalToolCalls
			return result, nil
		}

		for _, tc := range toolCalls {
			args := tc.ArgsMap()
			a.events.Emit(toolCallStartEvent(tc.Name, args))
			start := time.Now()

			toolResult := a.tools.Execute(ctx, tc.Name, args, a.env)

			elapsed := float64(time.Since(start).Microseconds()) / 1000.0
			totalToolCalls++

			a.events.Emit(toolCallEndEvent(tc.Name, !toolResult.IsError, elapsed))
			if toolResult.IsError {
				a.events.Emit(toolErrorEvent(tc.Name, toolResult.Content))
			}

			toolMsg := llm.ToolResultMessage(tc.ID, toolResult.Content, toolResult.IsError)
			history.Add(toolMsg)
			result.Messages = append(result.Messages, toolMsg)

			detection := a.loopDetector.Record(tc.Name, args)
			if detection.IsLooping {
				a.events.Emit(Event{Type: EventLoopDetected, Data: map[string]any{"description": detection.Description}})
				nudge := llm.UserMessage("You seem to be stuck in a loop. Please try a different approach or explain what's blocking you.")
				history.Add(nudge)
				result.Messages = append(result.Messages, nudge)
			}
		}
	}

	result.ToolCallsMade = totalToolCalls
	result.FinalResponse = "Reached maximum turns. Last response may be incomplete."
	return result, nil
}
