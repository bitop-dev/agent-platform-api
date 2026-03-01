package handlers

import (
	"github.com/gofiber/fiber/v2"
)

// Model is a supported LLM model.
type Model struct {
	ID           string  `json:"id"`
	Provider     string  `json:"provider"`
	DisplayName  string  `json:"display_name"`
	ContextWindow int    `json:"context_window"`
	InputCost    float64 `json:"input_cost_per_1m"`
	OutputCost   float64 `json:"output_cost_per_1m"`
	SupportsTools bool   `json:"supports_tools"`
	IsReasoning  bool    `json:"is_reasoning"`
}

// SupportedModels returns the list of models the platform supports.
var SupportedModels = []Model{
	{ID: "gpt-4o", Provider: "openai", DisplayName: "GPT-4o", ContextWindow: 128000, InputCost: 2.50, OutputCost: 10.0, SupportsTools: true},
	{ID: "gpt-4o-mini", Provider: "openai", DisplayName: "GPT-4o Mini", ContextWindow: 128000, InputCost: 0.15, OutputCost: 0.60, SupportsTools: true},
	{ID: "gpt-5", Provider: "openai", DisplayName: "GPT-5", ContextWindow: 256000, InputCost: 10.0, OutputCost: 30.0, SupportsTools: true, IsReasoning: true},
	{ID: "o3", Provider: "openai", DisplayName: "O3", ContextWindow: 200000, InputCost: 10.0, OutputCost: 40.0, SupportsTools: true, IsReasoning: true},
	{ID: "o4-mini", Provider: "openai", DisplayName: "O4 Mini", ContextWindow: 200000, InputCost: 1.10, OutputCost: 4.40, SupportsTools: true, IsReasoning: true},
	{ID: "claude-sonnet-4-20250514", Provider: "anthropic", DisplayName: "Claude Sonnet 4", ContextWindow: 200000, InputCost: 3.0, OutputCost: 15.0, SupportsTools: true},
	{ID: "claude-sonnet-4.6", Provider: "anthropic", DisplayName: "Claude Sonnet 4.6", ContextWindow: 200000, InputCost: 3.0, OutputCost: 15.0, SupportsTools: true},
	{ID: "claude-opus-4-20250514", Provider: "anthropic", DisplayName: "Claude Opus 4", ContextWindow: 200000, InputCost: 15.0, OutputCost: 75.0, SupportsTools: true},
	{ID: "claude-haiku-3.5", Provider: "anthropic", DisplayName: "Claude Haiku 3.5", ContextWindow: 200000, InputCost: 0.80, OutputCost: 4.0, SupportsTools: true},
	{ID: "llama3.1", Provider: "ollama", DisplayName: "Llama 3.1 (local)", ContextWindow: 128000, InputCost: 0, OutputCost: 0, SupportsTools: true},
	{ID: "deepseek-r1", Provider: "openai", DisplayName: "DeepSeek R1", ContextWindow: 128000, InputCost: 0.55, OutputCost: 2.19, SupportsTools: true, IsReasoning: true},
}

// ListModels returns all supported models (public endpoint for frontend).
func ListModels(c *fiber.Ctx) error {
	provider := c.Query("provider")
	if provider == "" {
		return c.JSON(fiber.Map{"models": SupportedModels})
	}

	var filtered []Model
	for _, m := range SupportedModels {
		if m.Provider == provider {
			filtered = append(filtered, m)
		}
	}
	return c.JSON(fiber.Map{"models": filtered})
}
