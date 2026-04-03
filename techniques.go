package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// BachataTechnique represents a bachata terminology entry
type BachataTechnique struct {
	ID                         int      `json:"id"`
	SpanishPronunciation       string   `json:"spanish_pronunciation"`
	SpanishName                string   `json:"spanish_name"`
	EnglishKoreanPronunciation string   `json:"english_korean_pronunciation"`
	EnglishEquivalent          string   `json:"english_equivalent"`
	Meaning                    string   `json:"meaning"`
	Priority                   int      `json:"priority"`
	Variants                   []string `json:"variants,omitempty"`
}

// TechniqueManager manages bachata terminology
type TechniqueManager struct {
	techniques []BachataTechnique
}

// NewTechniqueManager creates a new technique manager
func NewTechniqueManager(jsonPath string) (*TechniqueManager, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read techniques file: %v", err)
	}

	var techniques []BachataTechnique
	if err := json.Unmarshal(data, &techniques); err != nil {
		return nil, fmt.Errorf("failed to parse techniques JSON: %v", err)
	}

	return &TechniqueManager{
		techniques: techniques,
	}, nil
}

// GetFilteredTerms returns terms relevant to the given content
func (tm *TechniqueManager) GetFilteredTerms(content string) string {
	var relevantTerms []string
	contentLower := strings.ToLower(content)

	for _, tech := range tm.techniques {
		// Check if any pronunciation appears in content
		if strings.Contains(contentLower, strings.ToLower(tech.SpanishPronunciation)) ||
			strings.Contains(contentLower, strings.ToLower(tech.EnglishKoreanPronunciation)) ||
			strings.Contains(contentLower, strings.ToLower(tech.SpanishName)) ||
			strings.Contains(contentLower, strings.ToLower(tech.EnglishEquivalent)) {
			
			termInfo := fmt.Sprintf("%s(%s) = %s(%s): %s",
				tech.SpanishPronunciation,
				tech.SpanishName,
				tech.EnglishKoreanPronunciation,
				tech.EnglishEquivalent,
				tech.Meaning,
			)
			relevantTerms = append(relevantTerms, termInfo)
		}
	}

	if len(relevantTerms) == 0 {
		return "관련 용어 없음"
	}

	return strings.Join(relevantTerms, "\n")
}

// BuildCorrectionPrompt builds the system prompt for Korean correction
func (tm *TechniqueManager) BuildCorrectionPrompt(filteredTerms string) string {
	prompt := "당신은 한국에서 10년간 바차타를 가르친 전문 강사입니다.\n\n" +
		"중요한 배경 지식:\n" +
		"- 이 자막은 한국인 바차타 강사가 강습하는 영상을 유튜브가 자동으로 만든 STT입니다\n" +
		"- 강사는 바차타 동작을 설명할 때 영어를 한국어 발음으로 말하거나 스페인어를 한국어 발음으로 말합니다\n\n" +
		"당신의 임무:\n" +
		"1. 자동자막의 오타/오인식을 찾아내고 교정하세요\n" +
		"2. 타임코드는 절대 변경하지 마세요\n" +
		"3. 자막 번호도 그대로 유지하세요\n\n" +
		"중요: 영어 중복 방지 규칙\n" +
		"- 이미 영어가 괄호 안에 있으면 절대 다시 추가하지 마세요\n" +
		"- 예: 프렙턴(Prep Turn) 그대로 유지, 프랩턴은 프렙턴(Prep Turn)으로 교정\n\n" +
		"**절대 규칙 (반드시 준수):**\n" +
		"1. **숫자 표기 규칙 (최우선 규칙):**\n" +
		"   - 한글 숫자는 무조건 아라비아 숫자로 변환\n" +
		"   - 원→1, 투→2, 쓰리→3, 포→4, 파/파이브→5, 식/식스→6, 세븐→7, 에잇→8\n" +
		"   - 예: \"원 투 쓰리 포\" → \"1 2 3 4\"\n" +
		"   - 예: \"원투쓰리포\" → \"1 2 3 4\"\n" +
		"   - 예: \"식스에\" → \"6에\"\n" +
		"2. **띄어쓰기 규칙:**\n" +
		"   - 숫자 사이는 띄어쓰기만 사용 (콤마 절대 금지)\n" +
		"   - 올바른 예: \"1 2 3 4\", \"1 2 3 4 5 6\"\n" +
		"   - 잘못된 예: \"1, 2, 3, 4\" (콤마 사용 금지)\n" +
		"3. **문맥 파악:**\n" +
		"   - 앞뒤 문맥을 고려하여 가독성 있게 교정\n" +
		"   - 예: \"1 2 3 4 5 6에 왼쪽을 늘려주세요\" (자연스러운 표현)\n\n" +
		"필터링된 관련 용어사전:\n" +
		filteredTerms + "\n\n" +
		"출력 형식:\n" +
		"- 전체 SRT 파일을 그대로 출력하세요\n" +
		"- 타임코드와 번호는 절대 변경하지 마세요\n" +
		"- 텍스트만 교정하세요\n" +
		"- 마크다운 코드 블록을 사용하지 마세요"
	return prompt
}

// BuildTranslationPrompt builds the prompt for translation
func (tm *TechniqueManager) BuildTranslationPrompt(targetLang, langName string) string {
	return fmt.Sprintf("아래 한국어 바차타 강습 자막을 %s로 번역하세요.\n\n"+
		"**중요 배경:**\n"+
		"- 이 자막은 한국인 바차타 강사가 강습하는 영상입니다\n"+
		"- 자막에 나오는 바차타 용어는 영어 또는 스페인어 원어를 한국어 발음으로 표기한 것입니다\n"+
		"- 번역 시 해당 용어의 원어(영어/스페인어)를 정확히 파악하여 번역하세요\n\n"+
		"**예시:**\n"+
		"- \"롬뽀 아델란떼(Rompe adelante)\" 스페인어 그대로 \"Rompe adelante\"\n"+
		"- \"프렙(Prep)\" 영어 \"Prep (Preparation)\"\n"+
		"- \"웨이브(Wave)\" 영어 \"Wave\"\n\n"+
		"**중요 규칙:**\n"+
		"1. 타임코드는 절대 변경하지 마세요\n"+
		"2. 자막 번호는 그대로 유지하세요\n"+
		"3. 바차타 용어는 원어(영어/스페인어)로 번역하세요\n"+
		"4. HTML 태그를 절대 포함하지 마세요\n"+
		"5. 마크다운 코드 블록을 사용하지 마세요\n\n"+
		"**출력 형식:**\n"+
		"- 전체 SRT 파일을 그대로 출력하세요\n"+
		"- 순수한 텍스트만 출력하세요", targetLang)
}

// BuildMetadataPrompt builds the prompt for YouTube metadata generation
func (tm *TechniqueManager) BuildMetadataPrompt() string {
	return "아래 바차타 강습 자막을 바탕으로 유튜브 제목과 설명을 작성하세요.\n\n" +
		"**제목 요구사항:**\n" +
		"- **중요: 한국어 기준 공백 포함 100자 미만 (다른 언어 번역 시에도 각 언어 기준 100자 미만 엄수)**\n" +
		"- **자막 내용과 직접적으로 관련된 구체적인 제목 작성 (예: 자막에서 '메디아 웨이브'를 가르치면 제목에 '메디아 웨이브' 포함)**\n" +
		"- **'바차타' 또는 '패턴' 키워드 중 하나 이상 자연스럽게 포함 (둘 다 필수 아님)**\n" +
		"- SEO 최적화 키워드 추가 포함 (센슈얼바차타, 바차타강습, 바차타레슨 등)\n" +
		"- 클릭을 유도하는 매력적인 문구\n" +
		"- 중요: 댄스강습이라는 표현 사용 금지, 바차타 강습 또는 바차타 레슨 사용\n\n" +
		"**설명 요구사항:**\n" +
		"- **중요: 한국어 기준 정확히 600~800자 분량 (공백 포함, 절대 초과 금지)**\n" +
		"- 다른 언어 번역 시에도 각 언어 기준 600~800자 분량 엄수\n" +
		"- **'바차타' 또는 '패턴' 키워드 중 하나 이상 자연스럽게 포함 (둘 다 필수 아님)**\n" +
		"- SEO 키워드 다량 포함 (바차타, 센슈얼바차타, 바차타강습, 바차타레슨, 바차타패턴, 파트너워크, 리딩, 팔로잉 등)\n" +
		"- 중요: 댄스강습이라는 표현 절대 사용 금지, 바차타 강습 또는 바차타 레슨으로 표현\n" +
		"- **가독성을 위해 반드시 줄바꿈(\\n\\n) 사용하여 문단 구분**\n" +
		"- 구체적인 학습 내용을 번호(1., 2., 3.)로 간결하게 구조화하고 각 항목 사이에 줄바꿈\n" +
		"- 초보자도 이해하기 쉬운 친절한 설명\n" +
		"- 중요: 섹션 제목 사용 금지\n" +
		"- 간결하고 핵심적인 내용만 포함\n" +
		"- 800자를 절대 초과하지 말 것\n\n" +
		"**출력 형식 (JSON):**\n" +
		"{\n" +
		"  \"title\": \"유튜브 제목 (100자 미만, 내용 관련성 높게)\",\n" +
		"  \"description\": \"유튜브 설명 (600~800자, 줄바꿈 포함)\"\n" +
		"}\n"
}
