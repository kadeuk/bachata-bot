package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// DeepSeekClient wraps the DeepSeek API client
type DeepSeekClient struct {
	apiKey   string
	client   *http.Client
	baseURL  string
	model    string
	ctx      context.Context
}

// DeepSeekRequest represents the API request structure
type DeepSeekRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

// DeepSeekResponse represents the API response structure
type DeepSeekResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Choice represents a response choice
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage represents token usage
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// NewDeepSeekClient creates a new DeepSeek API client
func NewDeepSeekClient(apiKey string) (*DeepSeekClient, error) {
	// Use DeepSeek-V3 as default model (best for Korean text processing)
	// Alternatives: "deepseek-chat", "deepseek-coder", "deepseek-math"
	model := "deepseek-chat" // DeepSeek-V3 chat model
	
	return &DeepSeekClient{
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 120 * time.Second}, // Longer timeout for processing
		baseURL: "https://api.deepseek.com/v1",
		model:   model,
		ctx:     context.Background(),
	}, nil
}

// GenerateContent sends a prompt to DeepSeek and returns the response
func (dc *DeepSeekClient) GenerateContent(prompt string) (string, error) {
	return dc.GenerateContentWithSystemPrompt("", prompt)
}

// GenerateContentWithSystemPrompt sends a prompt with system instructions
func (dc *DeepSeekClient) GenerateContentWithSystemPrompt(systemPrompt, userPrompt string) (string, error) {
	maxRetries := 3
	var lastErr error

	// Prepare messages
	messages := []Message{}
	
	if systemPrompt != "" {
		messages = append(messages, Message{
			Role:    "system",
			Content: systemPrompt,
		})
	}
	
	messages = append(messages, Message{
		Role:    "user",
		Content: userPrompt,
	})

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			log.Printf("⏳ DeepSeek API 재시도 %d/%d...", attempt, maxRetries)
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}

		// Prepare request body
		requestBody := DeepSeekRequest{
			Model:    dc.model,
			Messages: messages,
			Stream:   false,
		}

		jsonData, err := json.Marshal(requestBody)
		if err != nil {
			lastErr = fmt.Errorf("failed to marshal request: %v", err)
			continue
		}

		// Create HTTP request
		req, err := http.NewRequestWithContext(dc.ctx, "POST", dc.baseURL+"/chat/completions", bytes.NewBuffer(jsonData))
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %v", err)
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+dc.apiKey)
		req.Header.Set("Accept", "application/json")

		// Send request
		resp, err := dc.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed: %v", err)
			log.Printf("⚠️ DeepSeek API 호출 실패 (시도 %d/%d): %v", attempt, maxRetries, err)
			continue
		}
		defer resp.Body.Close()

		// Read response
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %v", err)
			log.Printf("⚠️ DeepSeek 응답 읽기 실패 (시도 %d/%d): %v", attempt, maxRetries, err)
			continue
		}

		// Check HTTP status
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
			log.Printf("⚠️ DeepSeek API 오류 (시도 %d/%d, 상태 %d): %s", attempt, maxRetries, resp.StatusCode, string(body))
			continue
		}

		// Parse response
		var apiResp DeepSeekResponse
		if err := json.Unmarshal(body, &apiResp); err != nil {
			lastErr = fmt.Errorf("failed to parse response: %v", err)
			log.Printf("⚠️ DeepSeek 응답 파싱 실패 (시도 %d/%d): %v", attempt, maxRetries, err)
			continue
		}

		// Extract content
		if len(apiResp.Choices) == 0 {
			lastErr = fmt.Errorf("no choices in response")
			log.Printf("⚠️ DeepSeek 응답에 선택지가 없음 (시도 %d/%d)", attempt, maxRetries)
			continue
		}

		content := apiResp.Choices[0].Message.Content
		if content == "" {
			lastErr = fmt.Errorf("empty response content")
			log.Printf("⚠️ DeepSeek 응답이 비어있음 (시도 %d/%d)", attempt, maxRetries)
			continue
		}

		log.Printf("✅ DeepSeek API 호출 성공 (토큰 사용: 프롬프트=%d, 완료=%d, 총=%d)",
			apiResp.Usage.PromptTokens, apiResp.Usage.CompletionTokens, apiResp.Usage.TotalTokens)
		
		return content, nil
	}

	return "", fmt.Errorf("DeepSeek API failed after %d attempts: %v", maxRetries, lastErr)
}

// PostProcessNumbers uses AI to convert remaining Korean numbers to Arabic numbers
func (dc *DeepSeekClient) PostProcessNumbers(content string) (string, error) {
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

	result, err := dc.GenerateContent(prompt)
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

// CountTokens estimates the number of tokens in a text
// Note: DeepSeek API doesn't have a separate count tokens endpoint
// We'll use a simple approximation for now
func (dc *DeepSeekClient) CountTokens(text string) (int, error) {
	// Approximate token count: 1 token ≈ 4 characters for Korean/English mixed text
	// This is a rough estimate; actual tokenization may vary
	return len(text) / 4, nil
}

// Close closes the DeepSeek client (no-op for HTTP client)
func (dc *DeepSeekClient) Close() error {
	// HTTP client doesn't need explicit closing
	return nil
}