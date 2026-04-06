package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
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
	aiClient   AIClient
	techniques *TechniqueManager
	glossary   *GlossaryManager
}

// NewCorrector creates a new corrector
func NewCorrector(aiClient AIClient, techniques *TechniqueManager, glossary *GlossaryManager) *Corrector {
	return &Corrector{
		aiClient:   aiClient,
		techniques: techniques,
		glossary:   glossary,
	}
}

// ExtractCorrectionSuggestions analyzes SRT content and extracts correction suggestions
func (c *Corrector) ExtractCorrectionSuggestions(content string) ([]CorrectionSuggestion, error) {
	log.Println("🔍 AI가 교정이 필요한 부분을 분석 중... (DeepSeek 사용)")

	// Filter relevant terms
	filteredTerms := c.techniques.GetFilteredTerms(content)

	prompt := fmt.Sprintf(`당신은 한국에서 10년간 바차타를 가르친 전문 강사이자 다국어 번역 전문가입니다. DeepSeek AI로서 다음 지시사항을 정확히 따라주세요.

## 🎯 최우선 목표: 다국어 번역을 위한 바차타 용어 표준화
**이 작업의 궁극적인 목적은 한국어 자막을 영어, 스페인어, 일본어 등 다양한 언어로 정확히 번역하기 위한 것입니다.**

## 📋 중요한 배경 지식
- 이 자막은 한국인 바차타 강사가 강습하는 영상을 유튜브가 자동으로 만든 STT(음성인식) 결과입니다
- 강사는 바차타 동작을 설명할 때 다음을 혼용합니다:
  1. 영어를 한국어 발음으로 말하는 경우 (예: "프렙" = Prep)
  2. 스페인어를 한국어 발음으로 말하는 경우 (예: "롬뽀 아델란떼" = Rompe adelante)
  3. 영어와 스페인어를 섞어서 사용하는 경우

## 🎯 당신의 임무 (단계별 수행)
1. **번역 방해 요소 탐지**: 다국어 번역에 명확히 방해되는 부분만 찾으세요
2. **번역 적합성 판단**: 
   - 원본이 번역에 이상 없으면 → **제안하지 마세요**
   - 번역에 방해되면 → "번역에는 이 용어가 더 맞습니다"라고 제안하세요
3. **한국어 문맥 존중**: 한국어 시청자를 위해 문맥을 크게 해치지 않는 선에서만 교정하세요
4. **의도 존중**: 사용자가 의도적으로 괄호 안에 영어를 추가한 경우 (예: "골빼(아이솔레이션)") 존중하세요
5. **확신 부족 시 질문**: 확신이 없으면 반드시 사용자에게 질문하세요
6. **중복/사소한 제안 금지**:
   - 원본과 제안이 거의 동일하면 절대 제안하지 마세요 (예: "롬뽀 아델란떼 해볼게요" → "롬뽀 아델란떼")
   - 의미 없는 사소한 변경은 제안하지 마세요
7. **타임코드 보존**: 타임코드는 절대 변경하지 마세요
8. **숫자 카운트 제외**: "원, 투, 쓰리, 포, 파이브, 식스, 세븐, 에잇" 등 숫자 카운트는 제안하지 마세요 (자동 변환됨)

## 📝 교정 원칙 (중요!)
### ✅ 제안하지 말아야 할 경우 (번역에 이상 없음):
- "골빼(아이솔레이션)" → **원본 유지** (사용자가 의도적으로 영어 번역을 추가함, 번역에 이상 없음)
- "프렙턴(Prep Turn)" → **원본 유지** (이미 영어가 포함되어 있음, 번역에 이상 없음)
- "롬뽀 아델란떼 해볼게요" → **원본 유지** (사소한 변경, 의미 변화 없음, 번역에 이상 없음)
- 원본이 이미 정확한 바차타 용어인 경우 → **원본 유지**

### 🔧 제안해야 할 경우 (번역에 명확히 방해됨):
- "꼼블레도" → "꼼쁠레또(Completo)" (명확한 오인식, 번역에 방해됨)
- "론포 델렌 때" → "롬뽀 아델란떼(Rompe adelante)" (명확한 오타, 번역에 방해됨)
- "힙스로우" → "힙쓰로우(Hip Throw)" (발음 표기 오류, 번역에 방해됨)
- 원본이 바차타 용어사전과 일치하지 않아 번역에 혼란을 줄 경우 → **표준 용어로 제안**

## ⚠️ 판단 기준:
**"번역에 방해된다"의 기준:**
1. 용어가 바차타 용어사전과 명확히 불일치할 때
2. 오타/오인식으로 인해 다른 언어로 번역 시 의미가 완전히 달라질 때
3. 발음 표기 오류로 인해 원어민이 이해하지 못할 때

**"번역에 이상 없다"의 기준:**
1. 원본이 이미 정확한 바차타 용어일 때
2. 사용자가 의도적으로 추가 정보(괄호 안 영어)를 포함했을 때
3. 사소한 변경으로 의미 변화가 없을 때

## ⚠️ 중요: 숫자 카운트 제외 규칙
- "원, 투, 쓰리" 등의 숫자는 자동으로 아라비아 숫자로 변환되므로 제안 목록에 포함하지 마세요
- "파, 식" 등 앞뒤 문맥을 파악해 숫자로 판단되면 제안하지 말고 자동으로 교정하세요

## 📚 필터링된 관련 용어사전
%s

## 🖥️ 출력 형식 (엄격히 준수)
**반드시 다음 규칙을 지키세요:**
1. **순수 JSON 배열만 출력**: 설명 텍스트, 마크다운 코드 블록, 추가 설명 절대 포함 금지
2. **JSON 형식 엄수**: 바로 대괄호 [ 로 시작해서 대괄호 ] 로 끝나야 함
3. **DeepSeek 최적화**: 마크다운 없이 clean JSON만 출력

## 📄 출력 형식 예시 (JSON 배열)
[
  {
    "timecode": "00:00:13,719",
    "original_stt": "꼼블레도",
    "context_analysis": "동작을 완전히 끝까지 하라는 설명 문맥",
    "ai_reasoning": "영어 'Complete'를 스페인어 발음 'Completo(꼼쁠레또)'로 말한 것으로 추정되나, 확신이 없어 사용자 확인 필요",
    "best_guess": "꼼쁠레또(Completo)",
    "needs_confirmation": true,
    "question_to_user": "이 부분이 '완전한'이라는 뜻의 스페인어 'Completo(꼼쁠레또)'가 맞나요?"
  },
  {
    "timecode": "00:00:25,100",
    "original_stt": "론포 델렌 때",
    "context_analysis": "앞으로 나가는 브레이크 스텝 설명",
    "ai_reasoning": "스페인어 'Rompe adelante'를 자동자막이 잘못 인식함. 용어사전에 명확히 정의되어 있음",
    "best_guess": "롬뽀 아델란떼(Rompe adelante)",
    "needs_confirmation": false,
    "question_to_user": ""
  }
]

## 📜 SRT 원본 내용
%s`, filteredTerms, content)

	// Call AI client
	response, err := c.aiClient.GenerateContent(prompt)
	if err != nil {
		return nil, fmt.Errorf("AI 분석 실패: %v", err)
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
