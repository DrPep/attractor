// Package llm provides the unified multi-provider LLM client and data models.
package llm

import "encoding/json"

// Role is the author of a message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleDeveloper Role = "developer"
)

// ContentKind discriminates the payload carried by a ContentPart.
type ContentKind string

const (
	KindText             ContentKind = "text"
	KindImage            ContentKind = "image"
	KindAudio            ContentKind = "audio"
	KindDocument         ContentKind = "document"
	KindToolCall         ContentKind = "tool_call"
	KindToolResult       ContentKind = "tool_result"
	KindThinking         ContentKind = "thinking"
	KindRedactedThinking ContentKind = "redacted_thinking"
)

// ImageData holds an image referenced by URL or inline bytes.
type ImageData struct {
	URL       string `json:"url,omitempty"`
	Data      []byte `json:"data,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// ToolCallData is a request by the model to invoke a tool.
type ToolCallData struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments any    `json:"arguments"` // map[string]any or string
	Type      string `json:"type,omitempty"`
}

// ArgsMap returns the tool call arguments as a map, decoding from a JSON string
// when necessary. Returns an empty map when arguments cannot be interpreted.
func (t ToolCallData) ArgsMap() map[string]any {
	switch v := t.Arguments.(type) {
	case map[string]any:
		return v
	case string:
		m := map[string]any{}
		if v != "" {
			_ = json.Unmarshal([]byte(v), &m)
		}
		return m
	default:
		return map[string]any{}
	}
}

// ToolResultData is the outcome of a tool execution.
type ToolResultData struct {
	ToolCallID     string `json:"tool_call_id"`
	Content        any    `json:"content"` // string or map[string]any
	IsError        bool   `json:"is_error,omitempty"`
	ImageData      []byte `json:"image_data,omitempty"`
	ImageMediaType string `json:"image_media_type,omitempty"`
}

// ThinkingData is extended/redacted reasoning emitted by a model.
type ThinkingData struct {
	Text      string `json:"text,omitempty"`
	Signature string `json:"signature,omitempty"`
	Redacted  bool   `json:"redacted,omitempty"`
}

// ContentPart is one block within a Message.
type ContentPart struct {
	Kind       ContentKind     `json:"kind"`
	Text       string          `json:"text,omitempty"`
	Image      *ImageData      `json:"image,omitempty"`
	ToolCall   *ToolCallData   `json:"tool_call,omitempty"`
	ToolResult *ToolResultData `json:"tool_result,omitempty"`
	Thinking   *ThinkingData   `json:"thinking,omitempty"`
}

// TextPart builds a text content part.
func TextPart(text string) ContentPart { return ContentPart{Kind: KindText, Text: text} }

// ToolCallPart builds a tool-call content part.
func ToolCallPart(id, name string, args any) ContentPart {
	return ContentPart{Kind: KindToolCall, ToolCall: &ToolCallData{ID: id, Name: name, Arguments: args}}
}

// ToolResultPart builds a tool-result content part.
func ToolResultPart(toolCallID string, content any, isError bool) ContentPart {
	return ContentPart{Kind: KindToolResult, ToolResult: &ToolResultData{ToolCallID: toolCallID, Content: content, IsError: isError}}
}

// ThinkingPart builds a thinking (or redacted thinking) content part.
func ThinkingPart(text, signature string, redacted bool) ContentPart {
	kind := KindThinking
	if redacted {
		kind = KindRedactedThinking
	}
	return ContentPart{Kind: kind, Thinking: &ThinkingData{Text: text, Signature: signature, Redacted: redacted}}
}

// Message is a single conversational turn.
type Message struct {
	Role       Role          `json:"role"`
	Content    []ContentPart `json:"content"`
	Name       string        `json:"name,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

// Text concatenates all text parts of the message.
func (m Message) Text() string {
	out := ""
	for _, p := range m.Content {
		if p.Kind == KindText {
			out += p.Text
		}
	}
	return out
}

// ToolCalls returns all tool-call parts in the message.
func (m Message) ToolCalls() []ToolCallData {
	var out []ToolCallData
	for _, p := range m.Content {
		if p.Kind == KindToolCall && p.ToolCall != nil {
			out = append(out, *p.ToolCall)
		}
	}
	return out
}

// SystemMessage builds a system message.
func SystemMessage(text string) Message {
	return Message{Role: RoleSystem, Content: []ContentPart{TextPart(text)}}
}

// UserMessage builds a user message.
func UserMessage(text string) Message {
	return Message{Role: RoleUser, Content: []ContentPart{TextPart(text)}}
}

// AssistantMessage builds an assistant message.
func AssistantMessage(text string) Message {
	return Message{Role: RoleAssistant, Content: []ContentPart{TextPart(text)}}
}

// ToolResultMessage builds a tool-result message.
func ToolResultMessage(toolCallID, content string, isError bool) Message {
	return Message{
		Role:       RoleTool,
		Content:    []ContentPart{ToolResultPart(toolCallID, content, isError)},
		ToolCallID: toolCallID,
	}
}

// ToolDefinition describes a tool exposed to the model.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ToolChoice constrains tool selection (auto, none, required, or a named tool).
type ToolChoice struct {
	Mode string `json:"mode"`
	Name string `json:"name,omitempty"`
}

// ResponseFormat requests a particular output format.
type ResponseFormat struct {
	Type       string         `json:"type"`
	JSONSchema map[string]any `json:"json_schema,omitempty"`
}

// FinishReason explains why generation stopped.
type FinishReason string

const (
	FinishStop          FinishReason = "stop"
	FinishToolUse       FinishReason = "tool_use"
	FinishMaxTokens     FinishReason = "max_tokens"
	FinishContentFilter FinishReason = "content_filter"
	FinishError         FinishReason = "error"
)

// Usage tracks token consumption for a request.
type Usage struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
}

// TotalTokens returns the sum of input and output tokens.
func (u Usage) TotalTokens() int { return u.InputTokens + u.OutputTokens }

// Warning is a non-fatal advisory returned with a response.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Request is a provider-agnostic completion request.
type Request struct {
	Model           string            `json:"model"`
	Messages        []Message         `json:"messages"`
	Provider        string            `json:"provider,omitempty"`
	Tools           []ToolDefinition  `json:"tools,omitempty"`
	ToolChoice      *ToolChoice       `json:"tool_choice,omitempty"`
	ResponseFormat  *ResponseFormat   `json:"response_format,omitempty"`
	Temperature     *float64          `json:"temperature,omitempty"`
	TopP            *float64          `json:"top_p,omitempty"`
	MaxTokens       int               `json:"max_tokens,omitempty"`
	StopSequences   []string          `json:"stop_sequences,omitempty"`
	ReasoningEffort string            `json:"reasoning_effort,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	ProviderOptions map[string]any    `json:"provider_options,omitempty"`
}

// Response is a provider-agnostic completion response.
type Response struct {
	ID           string       `json:"id"`
	Model        string       `json:"model"`
	Provider     string       `json:"provider"`
	Message      Message      `json:"message"`
	FinishReason FinishReason `json:"finish_reason"`
	Usage        Usage        `json:"usage"`
	Warnings     []Warning    `json:"warnings,omitempty"`
}

// Text returns the response's concatenated text.
func (r Response) Text() string { return r.Message.Text() }

// ToolCalls returns the response's tool calls.
func (r Response) ToolCalls() []ToolCallData { return r.Message.ToolCalls() }
