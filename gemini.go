package main

import (
	"context"
	"fmt"
	"log"
	"strings"
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

// PostProcessNumbers uses AI to convert remaining Korean numbers to Arabic numbers
// This is a context-aware post-processing step after sequence-based conversion
func (gc *GeminiClient) PostProcessNumbers(content string) (string, error) {
	prompt := `당신은 한국어 자막의 숫자 표기를 교정하는 전문가입니다.

**임무:**
SRT 자막에서 한국어로 표기된 숫자(원, 투, 쓰리, 포, 파, 식, 세븐, 에잇 등)를 문맥을 보고 아라비아 숫자로 변환하세요.

**중요한 규칙:**
1. **타임코드는 절대 변경하지 마세요** - 원본 그대로 유지
2. **자막 번호는 절대 변경하지 마세요** - 원본 그대로 유지
3. **SRT 형식을 정확히 유지하세요** (번호, 타임코드, 텍스트, 빈 줄)
4. **문맥을 보고 판단하세요:**
   - 카운트/숫자인 경우: 변환 (예: "원 투 쓰리" → "1 2 3", "세븐에잇" → "7 8")
   - 일반 단어의 일부인 경우: 변환 안 함 (예: "어떤 식으로" → "어떤 식으로", "익스텐션" → "익스텐션")

**변환 예시:**
- "원 투 쓰리 포" → "1 2 3 4" (카운트)
- "세븐에잇에" → "7 8에" (카운트)
- "파 식 세븐 에잇" → "5 6 7 8" (카운트)
- "어떤 식으로" → "어떤 식으로" (일반 단어, 변환 안 함)
- "익스텐션" → "익스텐션" (일반 단어, 변환 안 함)
- "6에 왼쪽을" → "6에 왼쪽을" (이미 숫자, 유지)

**출력:**
수정된 전체 SRT 파일을 출력하세요. 마크다운 코드 블록 없이 순수 SRT 형식만 출력하세요.

**원본 SRT:**
` + content

	result, err := gc.GenerateContent(prompt)
	if err != nil {
		return content, fmt.Errorf("AI 숫자 후처리 실패: %v", err)
	}

	// Clean response
	result = strings.TrimSpace(result)
	result = strings.TrimPrefix(result, "```srt")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	return result, nil
}
