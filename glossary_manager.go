package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

// GlossaryManager handles both correction and translation glossaries
type GlossaryManager struct {
	correctionPath   string
	translationPath  string
	correctionGloss  map[string]string
	translationGloss map[string]string
	mu               sync.RWMutex
}

// NewGlossaryManager creates a new glossary manager
func NewGlossaryManager(correctionPath, translationPath string) (*GlossaryManager, error) {
	gm := &GlossaryManager{
		correctionPath:   correctionPath,
		translationPath:  translationPath,
		correctionGloss:  make(map[string]string),
		translationGloss: make(map[string]string),
	}

	// Load correction glossary
	if err := gm.loadCorrectionGlossary(); err != nil {
		log.Printf("⚠️ 교정 용어집 로드 실패: %v (새로 생성됩니다)", err)
	}

	// Load translation glossary
	if err := gm.loadTranslationGlossary(); err != nil {
		log.Printf("⚠️ 번역 용어집 로드 실패: %v (새로 생성됩니다)", err)
	}

	return gm, nil
}

// loadCorrectionGlossary loads correction glossary from file
func (gm *GlossaryManager) loadCorrectionGlossary() error {
	data, err := os.ReadFile(gm.correctionPath)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, &gm.correctionGloss); err != nil {
		return err
	}

	log.Printf("✅ 교정 용어집 로드 완료: %d개 항목", len(gm.correctionGloss))
	return nil
}

// loadTranslationGlossary loads translation glossary from file
func (gm *GlossaryManager) loadTranslationGlossary() error {
	data, err := os.ReadFile(gm.translationPath)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, &gm.translationGloss); err != nil {
		return err
	}

	log.Printf("✅ 번역 용어집 로드 완료: %d개 항목", len(gm.translationGloss))
	return nil
}

// AddCorrectionTerm adds a term to correction glossary and saves to file
func (gm *GlossaryManager) AddCorrectionTerm(original, corrected string) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	// Check if already exists
	if existing, exists := gm.correctionGloss[original]; exists {
		if existing == corrected {
			// Same mapping, no need to update
			return nil
		}
	}

	// Add new term
	gm.correctionGloss[original] = corrected
	log.Printf("📝 교정 용어집 추가: [%s] → [%s]", original, corrected)

	// Save to file
	return gm.saveCorrectionGlossary()
}

// AddTranslationTerm adds a term to translation glossary and saves to file
func (gm *GlossaryManager) AddTranslationTerm(korean, translated string) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	// Check if already exists
	if existing, exists := gm.translationGloss[korean]; exists {
		if existing == translated {
			// Same mapping, no need to update
			return nil
		}
	}

	// Add new term
	gm.translationGloss[korean] = translated
	log.Printf("📝 번역 용어집 추가: [%s] → [%s]", korean, translated)

	// Save to file
	return gm.saveTranslationGlossary()
}

// saveCorrectionGlossary saves correction glossary to file
func (gm *GlossaryManager) saveCorrectionGlossary() error {
	data, err := json.MarshalIndent(gm.correctionGloss, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON 마샬링 실패: %v", err)
	}

	if err := os.WriteFile(gm.correctionPath, data, 0644); err != nil {
		return fmt.Errorf("파일 저장 실패: %v", err)
	}

	return nil
}

// saveTranslationGlossary saves translation glossary to file
func (gm *GlossaryManager) saveTranslationGlossary() error {
	data, err := json.MarshalIndent(gm.translationGloss, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON 마샬링 실패: %v", err)
	}

	if err := os.WriteFile(gm.translationPath, data, 0644); err != nil {
		return fmt.Errorf("파일 저장 실패: %v", err)
	}

	return nil
}

// GetMiniCorrectionGlossary extracts only relevant terms from text
func (gm *GlossaryManager) GetMiniCorrectionGlossary(text string) map[string]string {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	mini := make(map[string]string)
	textLower := strings.ToLower(text)

	for original, corrected := range gm.correctionGloss {
		// Check if the original term appears in the text
		if strings.Contains(textLower, strings.ToLower(original)) {
			mini[original] = corrected
		}
	}

	if len(mini) > 0 {
		log.Printf("🔍 교정 용어집 필터링: %d개 → %d개 (토큰 절약)", len(gm.correctionGloss), len(mini))
	}

	return mini
}

// GetMiniTranslationGlossary extracts only relevant terms from text
func (gm *GlossaryManager) GetMiniTranslationGlossary(text string) map[string]string {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	mini := make(map[string]string)
	textLower := strings.ToLower(text)

	for korean, translated := range gm.translationGloss {
		// Check if the Korean term appears in the text
		if strings.Contains(textLower, strings.ToLower(korean)) {
			mini[korean] = translated
		}
	}

	if len(mini) > 0 {
		log.Printf("🔍 번역 용어집 필터링: %d개 → %d개 (토큰 절약)", len(gm.translationGloss), len(mini))
	}

	return mini
}

// BuildCorrectionPromptWithGlossary builds system prompt with mini glossary
func (gm *GlossaryManager) BuildCorrectionPromptWithGlossary(text string) string {
	miniGloss := gm.GetMiniCorrectionGlossary(text)

	prompt := `너는 SRT 자막 교정 전문가야. 다음 규칙을 절대적으로 지켜:

**절대 규칙 (위반 시 실패):**
1. 타임코드(00:00:01,000 --> 00:00:03,500)는 절대 1글자도 변경하지 마
2. 자막 번호와 항목 개수를 정확히 유지해
3. SRT 형식을 정확히 지켜 (번호, 타임코드, 텍스트, 빈 줄)

**교정 우선순위:**
1. 아래 용어집에 있는 단어는 무조건 용어집대로 교정
2. 맞춤법과 띄어쓰기 교정
3. 자연스러운 한국어로 다듬기`

	if len(miniGloss) > 0 {
		prompt += "\n\n**용어집 (최우선 적용):**\n"
		for original, corrected := range miniGloss {
			prompt += fmt.Sprintf("- \"%s\" → \"%s\"\n", original, corrected)
		}
	}

	return prompt
}

// BuildTranslationPromptWithGlossary builds system prompt with mini glossary
func (gm *GlossaryManager) BuildTranslationPromptWithGlossary(text, targetLang, langName string) string {
	miniGloss := gm.GetMiniTranslationGlossary(text)

	prompt := fmt.Sprintf(`너는 SRT 자막 번역 전문가야. 한국어를 %s(%s)로 번역해.

**절대 규칙 (위반 시 실패):**
1. 타임코드(00:00:01,000 --> 00:00:03,500)는 절대 1글자도 변경하지 마
2. 자막 번호와 항목 개수를 정확히 유지해
3. SRT 형식을 정확히 지켜 (번호, 타임코드, 텍스트, 빈 줄)

**번역 우선순위:**
1. 아래 용어집에 있는 단어는 무조건 용어집대로 번역
2. 춤 전문 용어는 원어 그대로 유지 (예: Bachata, Sensual)
3. 자연스러운 %s로 번역`, targetLang, langName, targetLang)

	if len(miniGloss) > 0 {
		prompt += "\n\n**용어집 (최우선 적용):**\n"
		for korean, translated := range miniGloss {
			prompt += fmt.Sprintf("- \"%s\" → \"%s\"\n", korean, translated)
		}
	}

	prompt += fmt.Sprintf(`

**중요: 새로운 춤 전문 용어 발견 시**
번역하면서 용어집에 없는 새로운 춤 전문 용어를 발견하면, 번역 결과 마지막에 다음 형식으로 추가해:

<<<NEW_TERMS>>>
{"한국어1": "%s1", "한국어2": "%s2"}
<<<END_NEW_TERMS>>>

예시:
<<<NEW_TERMS>>>
{"힙 무브먼트": "Hip Movement", "아이솔레이션": "Isolation"}
<<<END_NEW_TERMS>>>`, targetLang, targetLang)

	return prompt
}

// ExtractNewTermsFromResponse extracts new terms from translation response
func (gm *GlossaryManager) ExtractNewTermsFromResponse(response string) map[string]string {
	newTerms := make(map[string]string)

	// Find the new terms section
	startMarker := "<<<NEW_TERMS>>>"
	endMarker := "<<<END_NEW_TERMS>>>"

	startIdx := strings.Index(response, startMarker)
	if startIdx == -1 {
		return newTerms
	}

	endIdx := strings.Index(response, endMarker)
	if endIdx == -1 || endIdx <= startIdx {
		return newTerms
	}

	// Extract JSON
	jsonStr := strings.TrimSpace(response[startIdx+len(startMarker) : endIdx])

	// Parse JSON
	if err := json.Unmarshal([]byte(jsonStr), &newTerms); err != nil {
		log.Printf("⚠️ 새 용어 파싱 실패: %v", err)
		return make(map[string]string)
	}

	log.Printf("🆕 새로운 용어 발견: %d개", len(newTerms))
	return newTerms
}

// RemoveNewTermsMarkers removes the new terms section from response
func (gm *GlossaryManager) RemoveNewTermsMarkers(response string) string {
	startMarker := "<<<NEW_TERMS>>>"
	endMarker := "<<<END_NEW_TERMS>>>"

	startIdx := strings.Index(response, startMarker)
	if startIdx == -1 {
		return response
	}

	endIdx := strings.Index(response, endMarker)
	if endIdx == -1 {
		return response
	}

	// Remove the entire section
	return strings.TrimSpace(response[:startIdx] + response[endIdx+len(endMarker):])
}
