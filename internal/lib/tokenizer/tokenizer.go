// Package tokenizer provides token counting functionality
package tokenizer

import (
	"encoding/json"
	"strings"

	"omnillm/internal/providers/types"

	"github.com/rs/zerolog/log"
)

// TokenCount represents input and output token counts
type TokenCount struct {
	Input  int `json:"input"`
	Output int `json:"output"`
}

// ChatCompletionsPayload represents a simplified chat completions payload for token counting
type ChatCompletionsPayload struct {
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
	Model    string    `json:"model"`
}

type Message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // can be string or array of content parts
	Name    string      `json:"name,omitempty"`
}

type Tool struct {
	Type     string      `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// GetTokenizerFromModel determines the appropriate tokenizer for a model
func GetTokenizerFromModel(model *types.Model) string {
	if model.Capabilities != nil {
		if tokenizer, ok := model.Capabilities["tokenizer"].(string); ok {
			return tokenizer
		}
	}

	// Default tokenizer based on model name patterns
	modelID := strings.ToLower(model.ID)

	if strings.Contains(modelID, "gpt-4") || strings.Contains(modelID, "gpt-3.5") {
		return "o200k_base"
	}
	if strings.Contains(modelID, "claude") {
		return "claude"
	}
	if strings.Contains(modelID, "gemini") {
		return "gemini"
	}

	// Default fallback
	return "o200k_base"
}

// EstimateTokenCount provides a basic token count estimation
// This is a simplified version - the full implementation would use proper tokenizers
func EstimateTokenCount(payload *ChatCompletionsPayload, model *types.Model) (*TokenCount, error) {
	tokenizer := GetTokenizerFromModel(model)

	// Simple estimation based on character count
	// Real implementation would use tiktoken or similar libraries
	var totalChars int
	var inputChars int
	var outputChars int

	for _, message := range payload.Messages {
		chars := estimateMessageChars(message)
		totalChars += chars

		if message.Role == "assistant" {
			outputChars += chars
		} else {
			inputChars += chars
		}
	}

	// Add tool tokens if present
	if len(payload.Tools) > 0 {
		toolChars := estimateToolsChars(payload.Tools)
		inputChars += toolChars
		totalChars += toolChars
	}

	// Convert characters to tokens (rough approximation)
	// Different tokenizers have different char-to-token ratios
	var tokensPerChar float64
	switch tokenizer {
	case "o200k_base", "cl100k_base":
		tokensPerChar = 0.25 // GPT tokenizers typically 4 chars per token
	case "claude":
		tokensPerChar = 0.27 // Claude tokenizer is slightly more efficient
	case "gemini":
		tokensPerChar = 0.23 // Gemini tokenizer
	default:
		tokensPerChar = 0.25 // Default assumption
	}

	inputTokens := int(float64(inputChars) * tokensPerChar)
	outputTokens := int(float64(outputChars) * tokensPerChar)

	// Add some base tokens for message formatting
	inputTokens += len(payload.Messages) * 3 // ~3 tokens per message for formatting

	log.Debug().
		Str("tokenizer", tokenizer).
		Int("chars", totalChars).
		Int("input_tokens", inputTokens).
		Int("output_tokens", outputTokens).
		Msg("Estimated token count")

	return &TokenCount{
		Input:  inputTokens,
		Output: outputTokens,
	}, nil
}

func estimateMessageChars(message Message) int {
	chars := 0

	// Count role characters
	chars += len(message.Role)

	// Count name if present
	chars += len(message.Name)

	// Count content characters
	switch content := message.Content.(type) {
	case string:
		chars += len(content)
	case []interface{}:
		// Handle content parts array
		for _, part := range content {
			if partMap, ok := part.(map[string]interface{}); ok {
				if text, ok := partMap["text"].(string); ok {
					chars += len(text)
				}
				// For images, add a fixed cost
				if partType, ok := partMap["type"].(string); ok && partType == "image_url" {
					chars += 85 // Fixed cost for images as per OpenAI documentation
				}
			}
		}
	default:
		// Fallback: convert to JSON and count characters
		if jsonBytes, err := json.Marshal(content); err == nil {
			chars += len(string(jsonBytes))
		}
	}

	return chars
}

func estimateToolsChars(tools []Tool) int {
	chars := 0

	for _, tool := range tools {
		chars += len(tool.Function.Name)
		chars += len(tool.Function.Description)

		// Estimate parameters JSON length
		if tool.Function.Parameters != nil {
			if jsonBytes, err := json.Marshal(tool.Function.Parameters); err == nil {
				chars += len(string(jsonBytes))
			}
		}
	}

	return chars
}