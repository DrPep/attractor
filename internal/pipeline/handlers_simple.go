package pipeline

import "context"

// StartHandler is a no-op handler for start nodes.
type StartHandler struct{}

// Execute returns success.
func (h *StartHandler) Execute(ctx context.Context, node *Node, pctx *PipelineContext, g *Graph, runDir string) (Outcome, error) {
	return Outcome{Status: "success", Notes: "Pipeline started"}, nil
}

// ExitHandler is a no-op handler for exit nodes (gate check is done by the engine).
type ExitHandler struct{}

// Execute returns success.
func (h *ExitHandler) Execute(ctx context.Context, node *Node, pctx *PipelineContext, g *Graph, runDir string) (Outcome, error) {
	return Outcome{Status: "success", Notes: "Pipeline exit reached"}, nil
}

// ConditionalHandler is a pass-through; routing is decided by edge conditions.
type ConditionalHandler struct{}

// Execute returns success.
func (h *ConditionalHandler) Execute(ctx context.Context, node *Node, pctx *PipelineContext, g *Graph, runDir string) (Outcome, error) {
	return Outcome{Status: "success", Notes: "Conditional node - routing determined by edge conditions"}, nil
}

// WaitHumanHandler presents edge labels as choices via an interviewer.
type WaitHumanHandler struct {
	interviewer Interviewer
}

// Execute asks the interviewer to choose among outgoing edge labels.
func (h *WaitHumanHandler) Execute(ctx context.Context, node *Node, pctx *PipelineContext, g *Graph, runDir string) (Outcome, error) {
	outgoing := g.OutgoingEdges(node.ID)
	var choices []string
	for _, e := range outgoing {
		c := e.Label
		if c == "" {
			c = e.Target
		}
		if c != "" {
			choices = append(choices, c)
		}
	}
	if len(choices) == 0 {
		return Outcome{Status: "success", Notes: "No choices available"}, nil
	}

	prompt := node.Prompt()
	if prompt == "" {
		prompt = node.Label
	}
	if prompt == "" {
		prompt = "Choose next step for '" + node.ID + "'"
	}

	answer, err := h.interviewer.Ask(Question{
		Text:    prompt,
		Type:    QuestionSingleSelect,
		Choices: choices,
		Timeout: node.Timeout(),
	})
	if err != nil {
		return Outcome{Status: "fail", Notes: err.Error()}, nil
	}
	return Outcome{Status: "success", PreferredLabel: answer.Value, Notes: "Human selected: " + answer.Value}, nil
}
