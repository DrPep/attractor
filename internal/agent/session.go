package agent

import (
	"context"
	"errors"

	"github.com/nigelpepper/attractor/internal/agent/tools"
	"github.com/nigelpepper/attractor/internal/llm"
)

// SessionState is the lifecycle state of a session.
type SessionState string

const (
	StateIdle          SessionState = "idle"
	StateProcessing    SessionState = "processing"
	StateAwaitingInput SessionState = "awaiting_input"
	StateClosed        SessionState = "closed"
)

// Session is the main entry point for the coding agent.
type Session struct {
	client   *llm.Client
	profile  ProviderProfile
	env      tools.ExecutionEnvironment
	config   SessionConfig
	state    SessionState
	history  *ConversationHistory
	events   *EventBus
	detector *LoopDetector
	steering *SteeringManager
	tools    *tools.ToolRegistry
	loop     *AgentLoop
}

// SessionOptions configures a new session; zero values fall back to defaults.
type SessionOptions struct {
	Profile      *ProviderProfile
	Environment  tools.ExecutionEnvironment
	Config       *SessionConfig
	ToolRegistry *tools.ToolRegistry
}

// NewSession builds a session.
func NewSession(client *llm.Client, opts SessionOptions) *Session {
	profile := ProfileForAnthropic("")
	if opts.Profile != nil {
		profile = *opts.Profile
	}
	env := opts.Environment
	if env == nil {
		env = tools.NewLocalEnvironment("")
	}
	config := DefaultSessionConfig(profile.Model)
	if opts.Config != nil {
		config = *opts.Config
	}
	if profile.ProviderName != "" && config.Provider == "" {
		config.Provider = profile.ProviderName
	}
	registry := opts.ToolRegistry
	if registry == nil {
		registry = tools.CreateDefaultRegistry()
	}

	s := &Session{
		client:   client,
		profile:  profile,
		env:      env,
		config:   config,
		state:    StateIdle,
		history:  NewConversationHistory(),
		events:   NewEventBus(),
		detector: NewLoopDetector(20, 3),
		steering: NewSteeringManager(),
		tools:    registry,
	}
	s.loop = NewAgentLoop(client, registry, s.events, s.detector, s.steering)
	s.loop.SetExecutionEnv(env)
	return s
}

// State returns the session state.
func (s *Session) State() SessionState { return s.state }

// Config returns the session config.
func (s *Session) Config() SessionConfig { return s.config }

// History returns the conversation history.
func (s *Session) History() *ConversationHistory { return s.history }

// Submit adds user input and runs the agent loop to completion.
func (s *Session) Submit(ctx context.Context, userInput string) (TurnResult, error) {
	if s.state == StateClosed {
		return TurnResult{}, errors.New("session is closed")
	}
	if s.state == StateProcessing {
		return TurnResult{}, errors.New("session is already processing")
	}
	s.state = StateProcessing
	s.events.Emit(Event{Type: EventSessionStart, Data: map[string]any{"input": userInput}})
	defer func() { s.state = StateIdle }()

	s.history.Add(llm.UserMessage(userInput))
	return s.loop.RunTurn(ctx, s.history, s.profile.SystemPrompt, s.config)
}

// Steer injects a steering message between tool rounds.
func (s *Session) Steer(message string) { s.steering.Steer(message) }

// OnEvent registers an event handler for a specific type.
func (s *Session) OnEvent(t EventType, h EventHandler) { s.events.On(t, h) }

// OnAllEvents registers a handler for all events.
func (s *Session) OnAllEvents(h EventHandler) { s.events.OnAll(h) }

// Close ends the session.
func (s *Session) Close() {
	s.state = StateClosed
	s.detector.Reset()
}
