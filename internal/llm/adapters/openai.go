package adapters

import (
	"context"
	"encoding/json"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	"github.com/nigelpepper/attractor/internal/llm"
)

// OpenAIAdapter talks to OpenAI's Responses API via the official SDK.
type OpenAIAdapter struct {
	client openai.Client
}

// NewOpenAIAdapter builds an adapter from credentials and optional overrides.
func NewOpenAIAdapter(apiKey, baseURL, orgID string) *OpenAIAdapter {
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	if orgID != "" {
		opts = append(opts, option.WithOrganization(orgID))
	}
	return &OpenAIAdapter{client: openai.NewClient(opts...)}
}

// ProviderName returns "openai".
func (a *OpenAIAdapter) ProviderName() string { return "openai" }

// Close is a no-op.
func (a *OpenAIAdapter) Close() error { return nil }

func (a *OpenAIAdapter) translateMessages(msgs []llm.Message) responses.ResponseInputParam {
	var items responses.ResponseInputParam
	easy := func(role responses.EasyInputMessageRole, text string) {
		items = append(items, responses.ResponseInputItemUnionParam{
			OfMessage: &responses.EasyInputMessageParam{
				Role:    role,
				Content: responses.EasyInputMessageContentUnionParam{OfString: param.NewOpt(text)},
			},
		})
	}
	for _, m := range msgs {
		switch m.Role {
		case llm.RoleSystem:
			easy(responses.EasyInputMessageRoleSystem, m.Text())
		case llm.RoleDeveloper:
			easy(responses.EasyInputMessageRoleDeveloper, m.Text())
		case llm.RoleUser:
			easy(responses.EasyInputMessageRoleUser, m.Text())
		case llm.RoleAssistant:
			if txt := m.Text(); txt != "" {
				easy(responses.EasyInputMessageRoleAssistant, txt)
			}
			for _, p := range m.Content {
				if p.Kind == llm.KindToolCall && p.ToolCall != nil {
					args := argsToJSON(p.ToolCall.Arguments)
					items = append(items, responses.ResponseInputItemParamOfFunctionCall(args, p.ToolCall.ID, p.ToolCall.Name))
				}
			}
		case llm.RoleTool:
			for _, p := range m.Content {
				if p.Kind == llm.KindToolResult && p.ToolResult != nil {
					items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(
						p.ToolResult.ToolCallID, contentToString(p.ToolResult.Content)))
				}
			}
		}
	}
	return items
}

func (a *OpenAIAdapter) translateTools(tools []llm.ToolDefinition) []responses.ToolUnionParam {
	if len(tools) == 0 {
		return nil
	}
	out := make([]responses.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		fn := responses.FunctionToolParam{
			Name:       t.Name,
			Parameters: t.Parameters,
			Strict:     param.NewOpt(false),
		}
		if t.Description != "" {
			fn.Description = param.NewOpt(t.Description)
		}
		out = append(out, responses.ToolUnionParam{OfFunction: &fn})
	}
	return out
}

// Complete performs a non-streaming completion.
func (a *OpenAIAdapter) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(req.Model),
		Input: responses.ResponseNewParamsInputUnion{OfInputItemList: a.translateMessages(req.Messages)},
	}
	if tools := a.translateTools(req.Tools); tools != nil {
		params.Tools = tools
	}
	if req.Temperature != nil {
		params.Temperature = param.NewOpt(*req.Temperature)
	}
	if req.MaxTokens > 0 {
		params.MaxOutputTokens = param.NewOpt(int64(req.MaxTokens))
	}
	if req.ReasoningEffort != "" && req.ReasoningEffort != "none" {
		params.Reasoning = shared.ReasoningParam{Effort: shared.ReasoningEffort(req.ReasoningEffort)}
	}

	raw, err := a.client.Responses.New(ctx, params)
	if err != nil {
		return llm.Response{}, classifyOpenAIError(err)
	}
	return a.parseResponse(raw), nil
}

// Stream satisfies the adapter interface via a synthetic stream over Complete.
func (a *OpenAIAdapter) Stream(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	resp, err := a.Complete(ctx, req)
	return synthStream(ctx, resp, err)
}

func (a *OpenAIAdapter) parseResponse(raw *responses.Response) llm.Response {
	var parts []llm.ContentPart
	finish := llm.FinishStop
	for _, item := range raw.Output {
		switch item.Type {
		case "message":
			msg := item.AsMessage()
			for _, c := range msg.Content {
				if c.Type == "output_text" {
					parts = append(parts, llm.TextPart(c.Text))
				}
			}
		case "function_call":
			fc := item.AsFunctionCall()
			var args any
			m := map[string]any{}
			if fc.Arguments != "" && json.Unmarshal([]byte(fc.Arguments), &m) == nil {
				args = m
			} else {
				args = fc.Arguments
			}
			callID := fc.CallID
			if callID == "" {
				callID = fc.ID
			}
			parts = append(parts, llm.ToolCallPart(callID, fc.Name, args))
			finish = llm.FinishToolUse
		case "reasoning":
			r := item.AsReasoning()
			for _, s := range r.Summary {
				if s.Text != "" {
					parts = append(parts, llm.ThinkingPart(s.Text, "", false))
				}
			}
		}
	}

	return llm.Response{
		ID:           raw.ID,
		Model:        string(raw.Model),
		Provider:     "openai",
		Message:      llm.Message{Role: llm.RoleAssistant, Content: parts},
		FinishReason: finish,
		Usage: llm.Usage{
			InputTokens:     int(raw.Usage.InputTokens),
			OutputTokens:    int(raw.Usage.OutputTokens),
			CacheReadTokens: int(raw.Usage.InputTokensDetails.CachedTokens),
		},
	}
}

func argsToJSON(args any) string {
	switch v := args.(type) {
	case string:
		return v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "{}"
		}
		return string(b)
	}
}
