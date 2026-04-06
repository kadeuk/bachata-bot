package main

// AIClient defines the interface for AI clients (Gemini, DeepSeek, etc.)
type AIClient interface {
	// GenerateContent sends a prompt and returns the response
	GenerateContent(prompt string) (string, error)
	
	// GenerateContentWithSystemPrompt sends a prompt with system instructions
	GenerateContentWithSystemPrompt(systemPrompt, userPrompt string) (string, error)
	
	// PostProcessNumbers uses AI to convert Korean numbers to Arabic numbers
	PostProcessNumbers(content string) (string, error)
	
	// CountTokens estimates the number of tokens in a text
	CountTokens(text string) (int, error)
	
	// Close closes the client
	Close() error
}

// GeminiClient implements AIClient
var _ AIClient = (*GeminiClient)(nil)

// DeepSeekClient implements AIClient
var _ AIClient = (*DeepSeekClient)(nil)