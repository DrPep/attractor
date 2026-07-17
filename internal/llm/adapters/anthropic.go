package adapters

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/nigelpepper/attractor/internal/llm"
)

// AnthropicAdapter talks to Anthropic's Messages API via the official SDK.
type AnthropicAdapter struct {
	client anthropic.Client
}

// NewAnthropicAdapter builds an adapter from an API key and optional base URL.
func NewAnthropicAdapter(apiKey, baseURL string) *AnthropicAdapter {
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	return &AnthropicAdapter{client: anthropic.NewClient(opts...)}
}

// ProviderName returns "anthropic".
func (a *AnthropicAdapter) ProviderName() string { return "anthropic" }

// Close is a no-op; the SDK client holds no closable resources.
func (a *AnthropicAdapter) Close() error { return nil }

func (a *AnthropicAdapter) extractSystem(msgs []llm.Message) ([]anthropic.TextBlockParam, []llm.Message) {
	var sys []string
	var remaining []llm.Message
	for _, m := range msgs {
		if m.Role == llm.RoleSystem || m.Role == llm.RoleDeveloper {
			sys = append(sys, m.Text())
		} else {
			remaining = append(remaining, m)
		}
	}
	if len(sys) == 0 {
		return nil, remaining
	}
	joined := ""
	for i, s := range sys {
		if i > 0 {
			joined += "\n\n"
		}
		joined += s
	}
	block := anthropic.TextBlockParam{Text: joined}
	block.CacheControl = anthropic.NewCacheControlEphemeralParam()
	return []anthropic.TextBlockParam{block}, remaining
}

func (a *AnthropicAdapter) translateMessages(msgs []llm.Message) []anthropic.MessageParam {
	var out []anthropic.MessageParam
	for _, m := range msgs {
		switch m.Role {
		case llm.RoleUser:
			out = append(out, anthropic.NewUserMessage(a.contentBlocks(m.Content, true)...))
		case llm.RoleAssistant:
			out = append(out, anthropic.NewAssistantMessage(a.contentBlocks(m.Content, false)...))
		case llm.RoleTool:
			var blocks []anthropic.ContentBlockParamUnion
			for _, p := range m.Content {
				if p.Kind == llm.KindToolResult && p.ToolResult != nil {
					blocks = append(blocks, anthropic.NewToolResultBlock(
						p.ToolResult.ToolCallID, contentToString(p.ToolResult.Content), p.ToolResult.IsError))
				}
			}
			out = append(out, anthropic.NewUserMessage(blocks...))
		}
	}
	return out
}

func (a *AnthropicAdapter) contentBlocks(parts []llm.ContentPart, isUser bool) []anthropic.ContentBlockParamUnion {
	var blocks []anthropic.ContentBlockParamUnion
	for _, p := range parts {
		switch p.Kind {
		case llm.KindText:
			blocks = append(blocks, anthropic.NewTextBlock(p.Text))
		case llm.KindImage:
			if p.Image != nil && isUser && p.Image.Data != nil {
				mt := p.Image.MediaType
				if mt == "" {
					mt = "image/png"
				}
				blocks = append(blocks, anthropic.NewImageBlockBase64(mt, base64Encode(p.Image.Data)))
			}
		case llm.KindToolCall:
			if p.ToolCall != nil && !isUser {
				blocks = append(blocks, anthropic.NewToolUseBlock(p.ToolCall.ID, p.ToolCall.ArgsMap(), p.ToolCall.Name))
			}
		case llm.KindThinking:
			if p.Thinking != nil && !isUser {
				blocks = append(blocks, anthropic.NewThinkingBlock(p.Thinking.Signature, p.Thinking.Text))
			}
		case llm.KindRedactedThinking:
			if p.Thinking != nil && !isUser {
				blocks = append(blocks, anthropic.NewRedactedThinkingBlock(p.Thinking.Text))
			}
		}
	}
	return blocks
}

func (a *AnthropicAdapter) translateTools(tools []llm.ToolDefinition) []anthropic.ToolUnionParam {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		schema := anthropic.ToolInputSchemaParam{}
		if props, ok := t.Parameters["properties"]; ok {
			schema.Properties = props
		}
		if req, ok := t.Parameters["required"].([]string); ok {
			schema.Required = req
		} else if reqAny, ok := t.Parameters["required"].([]any); ok {
			for _, r := range reqAny {
				if s, ok := r.(string); ok {
					schema.Required = append(schema.Required, s)
				}
			}
		}
		tp := anthropic.ToolParam{Name: t.Name, InputSchema: schema}
		if t.Description != "" {
			tp.Description = param.NewOpt(t.Description)
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &tp})
	}
	return out
}

// anthropicThinkingBudget maps a reasoning-effort level to an extended-thinking
// token budget. A zero result disables thinking. Anthropic requires the budget
// to be at least 1024 tokens. Only models on the legacy token-budget contract
// (Sonnet 4.5 and older) use this — newer ones take an effort level instead.
func anthropicThinkingBudget(effort string) int64 {
	switch normalizeEffort(effort) {
	case "", "none":
		return 0
	case "low":
		return 2048
	case "high":
		return 16384
	case "xhigh", "max":
		return 32000
	default: // "medium" and any unrecognized-but-present value
		return 8192
	}
}

// normalizeEffort canonicalizes the effort spellings the DOT pipeline accepts.
func normalizeEffort(effort string) string {
	switch e := strings.ToLower(strings.TrimSpace(effort)); e {
	case "very-high", "very_high":
		return "xhigh"
	default:
		return e
	}
}

// anthropicEffort maps a reasoning-effort level onto Anthropic's output_config
// effort levels. supportsXHigh reports whether the model accepts "xhigh", which
// arrived with Opus 4.7; models without it take the next level down rather than
// "max", which would spend more than the caller asked for.
func anthropicEffort(effort string, supportsXHigh bool) anthropic.OutputConfigEffort {
	switch normalizeEffort(effort) {
	case "low":
		return anthropic.OutputConfigEffortLow
	case "high":
		return anthropic.OutputConfigEffortHigh
	case "xhigh":
		if !supportsXHigh {
			return anthropic.OutputConfigEffortHigh
		}
		return anthropic.OutputConfigEffortXhigh
	case "max":
		return anthropic.OutputConfigEffortMax
	default: // "medium" and any unrecognized-but-present value
		return anthropic.OutputConfigEffortMedium
	}
}

// anthropicReasoning resolves a model's reasoning contract. Model IDs missing
// from the catalog default to Anthropic's current contract — adaptive thinking,
// no sampling params — so a newly released model works before it is catalogued.
func anthropicReasoning(model string) (style llm.ReasoningStyle, sampling, xhigh bool) {
	info, ok := llm.GetModelInfo(model)
	if !ok {
		return llm.ReasoningEffort, false, true
	}
	return info.Reasoning, info.SupportsSampling, info.SupportsXHighEffort
}

func (a *AnthropicAdapter) buildParams(req llm.Request) (anthropic.MessageNewParams, error) {
	sys, remaining := a.extractSystem(req.Messages)
	messages := a.translateMessages(remaining)
	if len(messages) == 0 {
		return anthropic.MessageNewParams{}, &noMessagesError{}
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}
	params := anthropic.MessageNewParams{
		Model:    anthropic.Model(req.Model),
		Messages: messages,
	}
	if len(sys) > 0 {
		params.System = sys
	}
	injectUserCacheControl(messages)
	if tools := a.translateTools(req.Tools); tools != nil {
		params.Tools = tools
	}

	// Reasoning config is model-dependent: Opus 4.7 and later take adaptive
	// thinking plus an effort level and reject a thinking token budget with a
	// 400, while older models take only the budget.
	style, sampling, xhigh := anthropicReasoning(req.Model)
	thinking := false
	switch style {
	case llm.ReasoningEffort:
		if e := normalizeEffort(req.ReasoningEffort); e != "" && e != "none" {
			thinking = true
			params.Thinking = anthropic.ThinkingConfigParamUnion{
				OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
			}
			params.OutputConfig = anthropic.OutputConfigParam{
				Effort: anthropicEffort(req.ReasoningEffort, xhigh),
			}
		}
	case llm.ReasoningTokenBudget:
		// The budget must be smaller than max_tokens, so grow max_tokens to leave
		// room for the visible response after the thinking budget.
		if budget := anthropicThinkingBudget(req.ReasoningEffort); budget > 0 {
			thinking = true
			if int64(maxTokens) <= budget {
				maxTokens = int(budget) + 8192
			}
			params.Thinking = anthropic.ThinkingConfigParamOfEnabled(budget)
		}
	}
	params.MaxTokens = int64(maxTokens)

	// Anthropic rejects a non-default temperature when thinking is on, and Opus
	// 4.7 and later reject sampling params outright.
	if req.Temperature != nil && sampling && !thinking {
		params.Temperature = param.NewOpt(*req.Temperature)
	}
	if len(req.StopSequences) > 0 {
		params.StopSequences = req.StopSequences
	}
	return params, nil
}

// injectUserCacheControl places an ephemeral cache breakpoint on the last block
// of the most recent user message — the common reusable prefix for agentic
// workloads — mirroring the Python adapter's _inject_cache_control.
func injectUserCacheControl(messages []anthropic.MessageParam) {
	cc := anthropic.NewCacheControlEphemeralParam()
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != anthropic.MessageParamRoleUser {
			continue
		}
		content := messages[i].Content
		if len(content) == 0 {
			return
		}
		block := &content[len(content)-1]
		switch {
		case block.OfText != nil:
			block.OfText.CacheControl = cc
		case block.OfToolResult != nil:
			block.OfToolResult.CacheControl = cc
		case block.OfImage != nil:
			block.OfImage.CacheControl = cc
		}
		return
	}
}

// Complete performs a completion. It streams under the hood and accumulates the
// events into a full message: the SDK refuses a plain (non-streaming) request
// whose max_tokens budget could take longer than 10 minutes (e.g. high reasoning
// effort grows max_tokens past that threshold), so streaming is the only way to
// reliably serve large-budget requests. The accumulated message is identical in
// shape to a non-streaming response.
func (a *AnthropicAdapter) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	params, err := a.buildParams(req)
	if err != nil {
		return llm.Response{}, err
	}
	stream := a.client.Messages.NewStreaming(ctx, params)
	defer stream.Close()
	var msg anthropic.Message
	for stream.Next() {
		if err := msg.Accumulate(stream.Current()); err != nil {
			return llm.Response{}, classifyAnthropicError(err)
		}
	}
	if err := stream.Err(); err != nil {
		return llm.Response{}, classifyAnthropicError(err)
	}
	return a.parseResponse(&msg), nil
}

// Stream satisfies the adapter interface via a synthetic stream over Complete.
func (a *AnthropicAdapter) Stream(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	resp, err := a.Complete(ctx, req)
	return synthStream(ctx, resp, err)
}

func (a *AnthropicAdapter) parseResponse(raw *anthropic.Message) llm.Response {
	var parts []llm.ContentPart
	for _, block := range raw.Content {
		switch block.Type {
		case "text":
			parts = append(parts, llm.TextPart(block.Text))
		case "tool_use":
			var args map[string]any
			if len(block.Input) > 0 {
				_ = json.Unmarshal(block.Input, &args)
			}
			if args == nil {
				args = map[string]any{}
			}
			parts = append(parts, llm.ToolCallPart(block.ID, block.Name, args))
		case "thinking":
			parts = append(parts, llm.ThinkingPart(block.Thinking, block.Signature, false))
		case "redacted_thinking":
			parts = append(parts, llm.ThinkingPart(block.Data, "", true))
		}
	}

	finish := llm.FinishStop
	switch raw.StopReason {
	case anthropic.StopReasonToolUse:
		finish = llm.FinishToolUse
	case anthropic.StopReasonMaxTokens:
		finish = llm.FinishMaxTokens
	}

	return llm.Response{
		ID:           raw.ID,
		Model:        string(raw.Model),
		Provider:     "anthropic",
		Message:      llm.Message{Role: llm.RoleAssistant, Content: parts},
		FinishReason: finish,
		Usage: llm.Usage{
			InputTokens:      int(raw.Usage.InputTokens),
			OutputTokens:     int(raw.Usage.OutputTokens),
			CacheReadTokens:  int(raw.Usage.CacheReadInputTokens),
			CacheWriteTokens: int(raw.Usage.CacheCreationInputTokens),
		},
	}
}
