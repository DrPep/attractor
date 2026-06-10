package adapters

import (
	"os"

	"github.com/nigelpepper/attractor/internal/llm"
)

// FromEnv builds a Client by auto-discovering providers from environment
// variables, registering only those with credentials present. Mirrors
// Client.from_env in the Python implementation.
func FromEnv(middleware ...llm.Middleware) *llm.Client {
	client := llm.NewClient(nil, middleware...)

	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		client.RegisterProvider("openai", NewOpenAIAdapter(
			key, os.Getenv("OPENAI_BASE_URL"), os.Getenv("OPENAI_ORG_ID")))
	}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		client.RegisterProvider("anthropic", NewAnthropicAdapter(
			key, os.Getenv("ANTHROPIC_BASE_URL")))
	}
	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
		client.RegisterProvider("gemini", NewGeminiAdapter(
			key, os.Getenv("GEMINI_BASE_URL")))
	}
	return client
}
