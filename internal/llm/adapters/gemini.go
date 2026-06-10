package adapters

import (
	"context"

	"google.golang.org/genai"

	"github.com/nigelpepper/attractor/internal/aerr"
	"github.com/nigelpepper/attractor/internal/llm"
)

// GeminiAdapter talks to the Google Gemini API via the official genai SDK.
type GeminiAdapter struct {
	apiKey  string
	client  *genai.Client
	initErr error
}

// NewGeminiAdapter builds an adapter; the underlying client is created lazily.
func NewGeminiAdapter(apiKey, baseURL string) *GeminiAdapter {
	return &GeminiAdapter{apiKey: apiKey}
}

// ProviderName returns "gemini".
func (a *GeminiAdapter) ProviderName() string { return "gemini" }

// Close is a no-op.
func (a *GeminiAdapter) Close() error { return nil }

func (a *GeminiAdapter) getClient(ctx context.Context) (*genai.Client, error) {
	if a.client == nil && a.initErr == nil {
		a.client, a.initErr = genai.NewClient(ctx, &genai.ClientConfig{
			APIKey:  a.apiKey,
			Backend: genai.BackendGeminiAPI,
		})
	}
	return a.client, a.initErr
}

func (a *GeminiAdapter) extractSystem(msgs []llm.Message) (string, []llm.Message) {
	var sys []string
	var remaining []llm.Message
	for _, m := range msgs {
		if m.Role == llm.RoleSystem || m.Role == llm.RoleDeveloper {
			sys = append(sys, m.Text())
		} else {
			remaining = append(remaining, m)
		}
	}
	joined := ""
	for i, s := range sys {
		if i > 0 {
			joined += "\n\n"
		}
		joined += s
	}
	return joined, remaining
}

func (a *GeminiAdapter) translateMessages(msgs []llm.Message) []*genai.Content {
	var out []*genai.Content
	for _, m := range msgs {
		role := "model"
		if m.Role == llm.RoleUser || m.Role == llm.RoleTool {
			role = "user"
		}
		var parts []*genai.Part
		for _, p := range m.Content {
			switch p.Kind {
			case llm.KindText:
				parts = append(parts, &genai.Part{Text: p.Text})
			case llm.KindImage:
				if p.Image != nil && p.Image.Data != nil {
					mt := p.Image.MediaType
					if mt == "" {
						mt = "image/png"
					}
					parts = append(parts, &genai.Part{InlineData: &genai.Blob{MIMEType: mt, Data: p.Image.Data}})
				}
			case llm.KindToolCall:
				if p.ToolCall != nil {
					parts = append(parts, &genai.Part{FunctionCall: &genai.FunctionCall{
						Name: p.ToolCall.Name, Args: p.ToolCall.ArgsMap()}})
				}
			case llm.KindToolResult:
				if p.ToolResult != nil {
					resp := map[string]any{}
					if s, ok := p.ToolResult.Content.(string); ok {
						resp["result"] = s
					} else if mp, ok := p.ToolResult.Content.(map[string]any); ok {
						resp = mp
					}
					parts = append(parts, &genai.Part{FunctionResponse: &genai.FunctionResponse{
						Name: p.ToolResult.ToolCallID, Response: resp}})
				}
			}
		}
		if len(parts) > 0 {
			out = append(out, &genai.Content{Role: role, Parts: parts})
		}
	}
	return out
}

func (a *GeminiAdapter) translateTools(tools []llm.ToolDefinition) []*genai.Tool {
	if len(tools) == 0 {
		return nil
	}
	var decls []*genai.FunctionDeclaration
	for _, t := range tools {
		fd := &genai.FunctionDeclaration{Name: t.Name, Description: t.Description}
		if len(t.Parameters) > 0 {
			fd.ParametersJsonSchema = t.Parameters
		}
		decls = append(decls, fd)
	}
	return []*genai.Tool{{FunctionDeclarations: decls}}
}

// Complete performs a non-streaming completion.
func (a *GeminiAdapter) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	client, err := a.getClient(ctx)
	if err != nil {
		return llm.Response{}, &aerr.ConfigurationError{Msg: "gemini client init failed: " + err.Error()}
	}

	system, remaining := a.extractSystem(req.Messages)
	contents := a.translateMessages(remaining)

	config := &genai.GenerateContentConfig{}
	if req.Temperature != nil {
		t := float32(*req.Temperature)
		config.Temperature = &t
	}
	if req.MaxTokens > 0 {
		config.MaxOutputTokens = int32(req.MaxTokens)
	}
	if len(req.StopSequences) > 0 {
		config.StopSequences = req.StopSequences
	}
	if system != "" {
		config.SystemInstruction = &genai.Content{Parts: []*genai.Part{{Text: system}}}
	}
	if tools := a.translateTools(req.Tools); tools != nil {
		config.Tools = tools
	}

	raw, err := client.Models.GenerateContent(ctx, req.Model, contents, config)
	if err != nil {
		return llm.Response{}, &aerr.ProviderError{Provider: "gemini", Msg: err.Error()}
	}
	return a.parseResponse(raw, req.Model), nil
}

// Stream satisfies the adapter interface via a synthetic stream over Complete.
func (a *GeminiAdapter) Stream(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	resp, err := a.Complete(ctx, req)
	return synthStream(ctx, resp, err)
}

func (a *GeminiAdapter) parseResponse(raw *genai.GenerateContentResponse, model string) llm.Response {
	var parts []llm.ContentPart
	finish := llm.FinishStop

	for _, cand := range raw.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				if part.Text != "" {
					parts = append(parts, llm.TextPart(part.Text))
				} else if part.FunctionCall != nil {
					args := part.FunctionCall.Args
					if args == nil {
						args = map[string]any{}
					}
					parts = append(parts, llm.ToolCallPart(genCallID(), part.FunctionCall.Name, args))
					finish = llm.FinishToolUse
				}
			}
		}
		switch cand.FinishReason {
		case genai.FinishReasonMaxTokens:
			finish = llm.FinishMaxTokens
		case genai.FinishReasonSafety:
			finish = llm.FinishContentFilter
		}
	}

	usage := llm.Usage{}
	if raw.UsageMetadata != nil {
		usage = llm.Usage{
			InputTokens:     int(raw.UsageMetadata.PromptTokenCount),
			OutputTokens:    int(raw.UsageMetadata.CandidatesTokenCount),
			CacheReadTokens: int(raw.UsageMetadata.CachedContentTokenCount),
		}
	}

	return llm.Response{
		ID:           raw.ResponseID,
		Model:        model,
		Provider:     "gemini",
		Message:      llm.Message{Role: llm.RoleAssistant, Content: parts},
		FinishReason: finish,
		Usage:        usage,
	}
}
