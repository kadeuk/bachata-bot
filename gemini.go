package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// GeminiClient wraps the Gemini API client
type GeminiClient struct {
	client *genai.Client
	model  *genai.GenerativeModel
	ctx    context.Context
}

// NewGeminiClient creates a new Gemini API client
func NewGeminiClient(apiKey string) (*GeminiClient, error) {
	ctx := context.Background()

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %v", err)
	}

	// Use Gemini 2.5 Pro - latest stable Pro model available
	// Based on ListModels output: "Stable release (June 17th, 2025) of Gemini 2.5 Pro"
	model := client.GenerativeModel("models/gemini-2.5-pro")

	// Configure model parameters
	model.SetTemperature(0.2) // Lower temperature for more consistent output
	model.SetTopK(40)
	model.SetTopP(0.95)
	model.SetMaxOutputTokens(8192) // Maximum output tokens

	return &GeminiClient{
		client: client,
		model:  model,
		ctx:    ctx,
	}, nil
}

// GenerateContent sends a prompt to Gemini and returns the response
func (gc *GeminiClient) GenerateContent(prompt string) (string, error) {
	maxRetries := 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			log.Printf("⏳ Gemini API 재시도 %d/%d...", attempt, maxRetries)
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}

		resp, err := gc.model.GenerateContent(gc.ctx, genai.Text(prompt))
		if err != nil {
			lastErr = err
			log.Printf("⚠️ Gemini API 호출 실패 (시도 %d/%d): %v", attempt, maxRetries, err)
			continue
		}

		if len(resp.Candidates) == 0 {
			lastErr = fmt.Errorf("no candidates in response")
			log.Printf("⚠️ Gemini 응답에 후보가 없음 (시도 %d/%d)", attempt, maxRetries)
			continue
		}

		if resp.Candidates[0].Content == nil {
			lastErr = fmt.Errorf("no content in candidate")
			log.Printf("⚠️ Gemini 응답에 콘텐츠가 없음 (시도 %d/%d)", attempt, maxRetries)
			continue
		}

		// Extract text from response
		var result string
		for _, part := range resp.Candidates[0].Content.Parts {
			if txt, ok := part.(genai.Text); ok {
				result += string(txt)
			}
		}

		if result == "" {
			lastErr = fmt.Errorf("empty response text")
			log.Printf("⚠️ Gemini 응답이 비어있음 (시도 %d/%d)", attempt, maxRetries)
			continue
		}

		return result, nil
	}

	return "", fmt.Errorf("Gemini API failed after %d attempts: %v", maxRetries, lastErr)
}

// GenerateContentWithSystemPrompt sends a prompt with system instructions
func (gc *GeminiClient) GenerateContentWithSystemPrompt(systemPrompt, userPrompt string) (string, error) {
	// Gemini 1.5 Pro supports system instructions
	gc.model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(systemPrompt)},
	}

	return gc.GenerateContent(userPrompt)
}

// Close closes the Gemini client
func (gc *GeminiClient) Close() error {
	return gc.client.Close()
}

// CountTokens estimates the number of tokens in a text
func (gc *GeminiClient) CountTokens(text string) (int, error) {
	resp, err := gc.model.CountTokens(gc.ctx, genai.Text(text))
	if err != nil {
		return 0, err
	}
	return int(resp.TotalTokens), nil
}
