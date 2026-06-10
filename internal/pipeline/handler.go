package pipeline

import (
	"context"

	"github.com/nigelpepper/attractor/internal/agent"
	"github.com/nigelpepper/attractor/internal/llm"
)

// Outcome is the result of executing a node handler.
type Outcome struct {
	Status           string // success, retry, fail, partial_success
	PreferredLabel   string
	SuggestedNextIDs []string
	ContextUpdates   map[string]any
	Notes            string
}

// Handler executes a single node type.
type Handler interface {
	Execute(ctx context.Context, node *Node, pctx *PipelineContext, g *Graph, runDir string) (Outcome, error)
}

// HandlerRegistry maps node types to handlers.
type HandlerRegistry struct {
	handlers map[NodeType]Handler
}

// NewHandlerRegistry returns an empty registry.
func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{handlers: map[NodeType]Handler{}}
}

// Register installs a handler for a node type.
func (r *HandlerRegistry) Register(t NodeType, h Handler) { r.handlers[t] = h }

// Get returns the handler for a node type, or nil.
func (r *HandlerRegistry) Get(t NodeType) Handler { return r.handlers[t] }

// RegistryOptions configures the default handler registry.
type RegistryOptions struct {
	Client           *llm.Client
	Interviewer      Interviewer
	SkillRegistry    *agent.SkillRegistry
	ModelOverride    string
	ProviderOverride string
	OnAgentEvent     func(agent.Event)
}

// DefaultRegistry builds a registry with all built-in handlers.
func DefaultRegistry(opts RegistryOptions) *HandlerRegistry {
	interviewer := opts.Interviewer
	if interviewer == nil {
		interviewer = AutoApproveInterviewer{}
	}
	r := NewHandlerRegistry()
	r.Register(NodeStart, &StartHandler{})
	r.Register(NodeExit, &ExitHandler{})
	r.Register(NodeCodergen, &CodergenHandler{
		client:           opts.Client,
		skillRegistry:    opts.SkillRegistry,
		modelOverride:    opts.ModelOverride,
		providerOverride: opts.ProviderOverride,
		onAgentEvent:     opts.OnAgentEvent,
	})
	r.Register(NodeWaitHuman, &WaitHumanHandler{interviewer: interviewer})
	r.Register(NodeConditional, &ConditionalHandler{})
	r.Register(NodeParallel, &ParallelHandler{registry: r})
	r.Register(NodeTool, &ToolHandler{})
	return r
}
