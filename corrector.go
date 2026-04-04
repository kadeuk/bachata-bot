package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/google/generative-ai-go/genai"
)

// CorrectionSuggestion represents a single correction suggestion from AI
type CorrectionSuggestion struct {
	Timecode        string  `json:"timecode"`
	OriginalSTT     string  `json:"original_stt"`
	ContextAnalysis string  `json:"context_analysis"`
	AIReasoning     string  `json:"ai_reasoning"`
	BestGuess       string  `json:"best_guess"`
	NeedsConfirm    bool    `json:"needs_confirmation"`
	QuestionToUser  string  `json:"question_to_user"`
	ConfidenceScore float64 `json:"confidence_score,omitempty"`
}

// Corrector handles interactive correction with user
type Corrector struct {
	gemini     *GeminiClient
	techniques *TechniqueManager
	glossary   *GlossaryManager
}

// NewCorrector creates a new corrector
func NewCorrector(gemini *GeminiClient, techniques *TechniqueManager, glossary *GlossaryManager) *Corrector {
	return &Corrector{
		gemini:     gemini,
		techniques: techniques,
		glossary:   glossary,
	}
}

// ExtractCorrectionSuggestions analyzes SRT content and extracts correction suggestions
func (c *Corrector) ExtractCorrectionSuggestions(content string) ([]CorrectionSuggestion, error) {
	log.Println("🔍 AI가 교정이 필요한 부분을 분석 중... (Gemini 2.5 Pro 사용)")

	// Filter relevant terms
	filteredTerms := c.techniques.GetFilteredTerms(content)

	// Use Gemini 2.5 Pro (Flash not available in v1beta)
	flashModel := c.gemini.client.GenerativeModel("gemini-2.5-pro")
	flashModel.SetTemperature(0.2)
	flashModel.SetTopK(40)
	flashModel.SetTopP(0.95)
	flashModel.SetMaxOutputTokens(16384) // Increased to prevent truncation

	prompt := fmt.Sprintf(`당신은 한국에서 10년간 바차타를 가르친 전문 강사입니다.

**중요한 배경 지식:**
- 이 자막은 한국인 바차타 강사가 강습하는 영상을 유튜브가 자동으로 만든 STT입니다
- 강사는 바차타 동작을 설명할 때:
  1. 영어를 한국어 발음으로 말하는 경우 (예: "프렙" = Prep)
  2. 스페인어를 한국어 발음으로 말하는 경우 (예: "롬뽀 아델란떼" = Rompe adelante)
  3. 둘 다 섞어서 사용하는 경우가 있습니다

**당신의 임무:**
1. 자동자막의 오타/오인식을 찾아내세요
2. 그 단어가 영어인지 스페인어인지 판단하세요
3. 용어사전을 참고하여 정확한 바차타 용어로 교정하세요
4. **확신이 없거나 애매하면 반드시 사용자에게 질문하세요**
5. **중요: 원본과 제안이 동일하면 절대 제안하지 마세요**
6. 타임코드는 절대 변경하지 마세요
7. **숫자 카운트(원, 투, 쓰리, 포, 파이브, 식스, 세븐, 에잇 등)는 제안하지 마세요 - 자동 변환됨**

**교정 예시:**
- "꼼블레도" → 영어 "Complete"를 스페인어 발음 "Completo(꼼쁠레또)"로 말한 것 같음 → 사용자 확인 필요
- "론포 델렌 때" → 스페인어 "Rompe adelante(롬뽀 아델란떼)" 오인식 → 확신함
- "견각골" → 해부학 용어 "견갑골" 오타 → 확신함
- 힙스로우 -> 힙쓰로우
- 사이트 프레이브 -> 사이드웨이브

**중요: 숫자 카운트는 제안하지 마세요**
- "원, 투, 쓰리" 등의 숫자는 자동으로 아라비아 숫자로 변환되므로 제안 목록에 포함하지 마세요
- "파, 식" 등 앞뒤 문맥을 파악해 숫자로 판단되면 제안하지 말고 자동으로 교정하세요

**필터링된 관련 용어사전:**
%s

**중요: 출력 형식**
- 반드시 순수한 JSON 배열만 출력하세요
- 설명 텍스트나 마크다운 코드 블록을 절대 포함하지 마세요
- 바로 대괄호로 시작해서 대괄호로 끝나야 합니다

**출력 형식 (JSON 배열만):**
[
  {
    "timecode": "00:00:13,719",
    "original_stt": "꼼블레도",
    "context_analysis": "동작을 완전히 끝까지 하라는 설명 문맥",
    "ai_reasoning": "영어 'Complete'를 스페인어 발음 'Completo(꼼쁠레또)'로 말한 것으로 추정되나, 확신이 없어 사용자 확인 필요",
    "best_guess": "꼼쁠레또",
    "needs_confirmation": true,
    "question_to_user": "이 부분이 '완전한'이라는 뜻의 스페인어 'Completo(꼼쁠레또)'가 맞나요?"
  },
  {
    "timecode": "00:00:25,100",
    "original_stt": "론포 델렌 때",
    "context_analysis": "앞으로 나가는 브레이크 스텝 설명",
    "ai_reasoning": "스페인어 'Rompe adelante'를 자동자막이 잘못 인식함. 용어사전에 명확히 정의되어 있음",
    "best_guess": "롬뽀 아델란떼",
    "needs_confirmation": false,
    "question_to_user": ""
  }
]

**SRT 원본:**
%s`, filteredTerms, content)

	// Call Gemini 1.5 Flash
	resp, err := flashModel.GenerateContent(c.gemini.ctx, genai.Text(prompt))
	if err != nil {
		return nil, fmt.Errorf("AI 분석 실패: %v", err)
	}

	// Extract text from response
	var response string
	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		for _, part := range resp.Candidates[0].Content.Parts {
			if txt, ok := part.(genai.Text); ok {
				response += string(txt)
			}
		}
	}

	if response == "" {
		return nil, fmt.Errorf("AI 응답이 비어있음")
	}

	// Clean JSON response
	cleanJSON := cleanJSONResponse(response)

	// Additional validation: Check if JSON is complete
	if !strings.HasSuffix(strings.TrimSpace(cleanJSON), "]") {
		log.Printf("⚠️ JSON이 불완전합니다 (닫는 괄호 없음)")
		log.Printf("응답 내용 (마지막 200자): %s", cleanJSON[max(0, len(cleanJSON)-200):])
		
		// Try to fix incomplete JSON by adding closing bracket
		cleanJSON = strings.TrimSpace(cleanJSON)
		if strings.HasPrefix(cleanJSON, "[") && !strings.HasSuffix(cleanJSON, "]") {
			// Find the last complete object
			lastBraceIdx := strings.LastIndex(cleanJSON, "}")
			if lastBraceIdx > 0 {
				cleanJSON = cleanJSON[:lastBraceIdx+1] + "\n]"
				log.Printf("✅ 불완전한 JSON 자동 복구 시도")
			}
		}
	}

	var suggestions []CorrectionSuggestion
	if err := json.Unmarshal([]byte(cleanJSON), &suggestions); err != nil {
		log.Printf("⚠️ JSON 파싱 실패: %v", err)
		log.Printf("응답 내용 (처음 500자): %s", cleanJSON[:min(500, len(cleanJSON))])
		log.Printf("응답 내용 (마지막 200자): %s", cleanJSON[max(0, len(cleanJSON)-200):])
		return nil, fmt.Errorf("AI 응답 파싱 실패: %v", err)
	}

	log.Printf("✅ %d개의 교정 제안 추출 완료", len(suggestions))
	return suggestions, nil
}

// ApplyCorrections applies user-approved corrections to the content
func (c *Corrector) ApplyCorrections(content string, corrections map[string]string) string {
	result := content
	for original, replacement := range corrections {
		if original != replacement {
			result = strings.ReplaceAll(result, original, replacement)
			log.Printf("✅ 교정 적용: [%s] → [%s]", original, replacement)
		}
	}
	return result
}

// cleanJSONResponse removes markdown code blocks and extra text from JSON response
func cleanJSONResponse(text string) string {
	text = strings.TrimSpace(text)

	// Remove markdown code blocks
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
	}
	if strings.HasSuffix(text, "```") {
		text = strings.TrimSuffix(text, "```")
	}

	text = strings.TrimSpace(text)

	// Find JSON array or object start
	arrayStart := strings.Index(text, "[")
	objectStart := strings.Index(text, "{")

	if arrayStart >= 0 && (objectStart < 0 || arrayStart < objectStart) {
		// Array comes first
		if arrayStart > 0 {
			text = text[arrayStart:]
		}
		// Find array end
		if idx := strings.LastIndex(text, "]"); idx > 0 {
			text = text[:idx+1]
		}
	} else if objectStart > 0 {
		// Object comes first
		text = text[objectStart:]
		// Find object end
		if idx := strings.LastIndex(text, "}"); idx > 0 {
			text = text[:idx+1]
		}
	}

	return strings.TrimSpace(text)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
