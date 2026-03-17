package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	DirInput = "번역전"
)

// =====================================================================
// 바차타 테크닉 우선순위 사전 구조체
// =====================================================================
type BachataTechnique struct {
	ID                         int      `json:"id"`
	SpanishPronunciation       string   `json:"spanish_pronunciation"`
	SpanishName                string   `json:"spanish_name"`
	EnglishKoreanPronunciation string   `json:"english_korean_pronunciation"`
	EnglishEquivalent          string   `json:"english_equivalent"`
	Meaning                    string   `json:"meaning"`
	Priority                   int      `json:"priority"`
	Variants                   []string `json:"variants,omitempty"` // 발음 변형들
}

// 범용 영어 용어 예외 리스트
var universalEnglishTerms = map[string]bool{
	"턴":      true,
	"turn":   true,
	"프레임":   true,
	"frame":  true,
	"텐션":    true,
	"tension": true,
	"프렙":    true,
	"prep":   true,
	"preparation": true,
	"리드":    true,
	"lead":   true,
	"팔로우":   true,
	"follow": true,
	"커넥션":   true,
	"connection": true,
}

type ClaudeRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	Messages  []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ClaudeResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

type Suggestion struct {
	Timecode          string `json:"timecode"`
	OriginalSTT       string `json:"original_stt"`
	ContextAnalysis   string `json:"context_analysis"`
	AIReasoning       string `json:"ai_reasoning"`
	BestGuess         string `json:"best_guess"`
	NeedsConfirmation bool   `json:"needs_confirmation"`
	QuestionToUser    string `json:"question_to_user"`
	ConfidenceScore   float64 `json:"confidence_score,omitempty"` // 0.0 ~ 1.0
	IsDanceTerm       bool   `json:"is_dance_term,omitempty"`
}

type MetadataResult struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type TokenUsage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	EstimatedCost float64
}

var globalTokenUsage TokenUsage
var bachataTechniques []BachataTechnique
var techniquesMutex sync.RWMutex

// =====================================================================
// 바차타 테크닉 사전 로드
// =====================================================================
func loadBachataTechniques() error {
	data, err := os.ReadFile("bachata_techniques.json")
	if err != nil {
		return fmt.Errorf("바차타 테크닉 JSON 파일 로드 실패: %v", err)
	}

	err = json.Unmarshal(data, &bachataTechniques)
	if err != nil {
		return fmt.Errorf("바차타 테크닉 JSON 파싱 실패: %v", err)
	}

	// 발음 변형 자동 생성 (예: "삔싸" -> ["빈사", "핀사", "삔사"])
	for i := range bachataTechniques {
		bachataTechniques[i].Variants = generatePronunciationVariants(bachataTechniques[i].SpanishPronunciation)
	}

	fmt.Printf("✅ 바차타 테크닉 사전 로드 완료: %d개 항목\n", len(bachataTechniques))
	return nil
}

// =====================================================================
// 발음 유사도 매칭 (Levenshtein Distance 기반)
// =====================================================================
func levenshteinDistance(s1, s2 string) int {
	s1Runes := []rune(s1)
	s2Runes := []rune(s2)
	
	if len(s1Runes) == 0 {
		return len(s2Runes)
	}
	if len(s2Runes) == 0 {
		return len(s1Runes)
	}

	matrix := make([][]int, len(s1Runes)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2Runes)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	for i := 1; i <= len(s1Runes); i++ {
		for j := 1; j <= len(s2Runes); j++ {
			cost := 0
			if s1Runes[i-1] != s2Runes[j-1] {
				cost = 1
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				min(matrix[i][j-1]+1,  // insertion
					matrix[i-1][j-1]+cost)) // substitution
		}
	}

	return matrix[len(s1Runes)][len(s2Runes)]
}

func similarityScore(s1, s2 string) float64 {
	distance := levenshteinDistance(s1, s2)
	maxLen := max(len([]rune(s1)), len([]rune(s2)))
	if maxLen == 0 {
		return 1.0
	}
	return 1.0 - float64(distance)/float64(maxLen)
}

// =====================================================================
// 발음 변형 생성 (한글 자음 변형)
// =====================================================================
func generatePronunciationVariants(original string) []string {
	result := []string{original}
	
	// 한글 자음 변형 맵 (ㅃ->ㅂ, ㅃ->ㅍ 등)
	consonantMap := map[rune][]rune{
		'ㅃ': {'ㅂ', 'ㅍ', 'ㅃ'},
		'ㅉ': {'ㅈ', 'ㅊ', 'ㅉ'},
		'ㄸ': {'ㄷ', 'ㅌ', 'ㄸ'},
		'ㄲ': {'ㄱ', 'ㅋ', 'ㄲ'},
		'ㅆ': {'ㅅ', 'ㅆ'},
	}

	runes := []rune(original)
	for i, r := range runes {
		if variantRunes, ok := consonantMap[r]; ok {
			for _, variant := range variantRunes {
				newRunes := make([]rune, len(runes))
				copy(newRunes, runes)
				newRunes[i] = variant
				result = append(result, string(newRunes))
			}
		}
	}

	return result
}

// =====================================================================
// 유사 발음 매칭 함수 (Fuzzy Matching)
// =====================================================================
func findBestMatchingTechnique(word string, threshold float64) (*BachataTechnique, float64) {
	techniquesMutex.RLock()
	defer techniquesMutex.RUnlock()

	var bestMatch *BachataTechnique
	var bestScore float64 = 0.0

	for i := range bachataTechniques {
		tech := &bachataTechniques[i]
		
		// 정확한 매칭 우선
		if word == tech.SpanishPronunciation {
			return tech, 1.0
		}

		// 발음 변형 체크
		for _, variant := range tech.Variants {
			if word == variant {
				return tech, 0.95
			}
		}

		// 유사도 계산
		score := similarityScore(word, tech.SpanishPronunciation)
		if score > bestScore && score >= threshold {
			bestScore = score
			bestMatch = tech
		}

		// 영어 발음도 체크
		for _, engPron := range strings.Split(tech.EnglishKoreanPronunciation, " / ") {
			engScore := similarityScore(word, strings.TrimSpace(engPron))
			if engScore > bestScore && engScore >= threshold {
				bestScore = engScore
				bestMatch = tech
			}
		}
	}

	return bestMatch, bestScore
}

// =====================================================================
// 포맷팅 함수: "한국어발음(Spanish_name)" 형식으로 변환
// =====================================================================
func formatTechniqueOutput(tech *BachataTechnique, useEnglish bool) string {
	if useEnglish {
		// 범용 영어 용어는 영어로 유지
		return fmt.Sprintf("%s(%s)", tech.EnglishKoreanPronunciation, tech.EnglishEquivalent)
	}
	return fmt.Sprintf("%s(%s)", tech.SpanishPronunciation, tech.SpanishName)
}

// =====================================================================
// LLM을 통한 문맥 파악 (Context Disambiguation)
// =====================================================================
func checkIfDanceTermWithContext(apiKey, word, context string) (bool, float64) {
	prompt := fmt.Sprintf(`당신은 바차타 댄스 전문가입니다.

아래 자막 문맥에서 "%s"라는 단어가 바차타 댄스 용어로 쓰였는지 판단하세요.

**문맥:**
%s

**판단 기준:**
1. 댄스 동작이나 테크닉을 설명하는 맥락이면 → 댄스 용어
2. 일상적인 의미로 쓰였으면 → 일반 단어

**출력 형식 (JSON만):**
{
  "is_dance_term": true/false,
  "confidence": 0.0~1.0,
  "reasoning": "판단 근거"
}`, word, context)

	result := callClaudeAPI(apiKey, prompt, 300)
	cleanJSON := cleanJSONResponse(result)

	var response struct {
		IsDanceTerm bool    `json:"is_dance_term"`
		Confidence  float64 `json:"confidence"`
		Reasoning   string  `json:"reasoning"`
	}

	err := json.Unmarshal([]byte(cleanJSON), &response)
	if err != nil {
		fmt.Printf("⚠️ LLM 문맥 분석 파싱 실패: %v\n", err)
		return false, 0.0
	}

	return response.IsDanceTerm, response.Confidence
}

// =====================================================================
// 고루틴 기반 비동기 LLM 호출 (병렬 처리)
// =====================================================================
type ContextCheckResult struct {
	Word        string
	IsDanceTerm bool
	Confidence  float64
	Index       int
}

func checkContextAsync(apiKey string, words []string, contexts []string) []ContextCheckResult {
	results := make([]ContextCheckResult, len(words))
	var wg sync.WaitGroup
	resultChan := make(chan ContextCheckResult, len(words))

	for i, word := range words {
		wg.Add(1)
		go func(idx int, w string, ctx string) {
			defer wg.Done()
			isDance, conf := checkIfDanceTermWithContext(apiKey, w, ctx)
			resultChan <- ContextCheckResult{
				Word:        w,
				IsDanceTerm: isDance,
				Confidence:  conf,
				Index:       idx,
			}
		}(i, word, contexts[i])
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for result := range resultChan {
		results[result.Index] = result
	}

	return results
}

// =====================================================================
// 기존 함수들 (유지)
// =====================================================================
func callClaudeAPI(apiKey, prompt string, maxTokens int) string {
	reqBody := ClaudeRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: maxTokens,
		Messages:  []Message{{Role: "user", Content: prompt}},
	}
	jsonData, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonData))
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("   ⚠️ API 호출 실패: %v\n", err)
		return ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	
	var fullResp map[string]interface{}
	json.Unmarshal(body, &fullResp)
	
	if usage, ok := fullResp["usage"].(map[string]interface{}); ok {
		if inputTokens, ok := usage["input_tokens"].(float64); ok {
			globalTokenUsage.InputTokens += int(inputTokens)
		}
		if outputTokens, ok := usage["output_tokens"].(float64); ok {
			globalTokenUsage.OutputTokens += int(outputTokens)
		}
	}
	
	var claudeResp ClaudeResponse
	json.Unmarshal(body, &claudeResp)

	if len(claudeResp.Content) > 0 {
		return claudeResp.Content[0].Text
	}
	return ""
}

func calculateCost() {
	inputCostPerMillion := 3.00
	outputCostPerMillion := 15.00
	
	inputCost := (float64(globalTokenUsage.InputTokens) / 1000000.0) * inputCostPerMillion
	outputCost := (float64(globalTokenUsage.OutputTokens) / 1000000.0) * outputCostPerMillion
	
	totalCostUSD := inputCost + outputCost
	exchangeRate := 1350.0
	totalCostKRW := totalCostUSD * exchangeRate
	
	globalTokenUsage.TotalTokens = globalTokenUsage.InputTokens + globalTokenUsage.OutputTokens
	globalTokenUsage.EstimatedCost = totalCostKRW
}

func cleanJSONResponse(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
	}
	if strings.HasSuffix(text, "```") {
		text = strings.TrimSuffix(text, "```")
	}
	return strings.TrimSpace(text)
}

func filterDictionary(srtContent, dictionaryPath string) string {
	dictBytes, err := os.ReadFile(dictionaryPath)
	if err != nil {
		return ""
	}

	dictLines := strings.Split(string(dictBytes), "\n")
	var relevantTerms []string

	srtLower := strings.ToLower(srtContent)

	for _, line := range dictLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		keywords := extractKeywords(line)
		for _, keyword := range keywords {
			if strings.Contains(srtLower, strings.ToLower(keyword)) {
				relevantTerms = append(relevantTerms, line)
				break
			}
		}
	}

	if len(relevantTerms) == 0 {
		return "관련 용어 없음"
	}

	return strings.Join(relevantTerms, "\n")
}

func extractKeywords(line string) []string {
	var keywords []string

	re := regexp.MustCompile(`([가-힣]+)\s*\(([A-Za-z\s]+)\)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 2 {
		keywords = append(keywords, matches[1], matches[2])
	}

	reOriginal := regexp.MustCompile(`원어:\s*([A-Za-zÀ-ÿ\s]+)`)
	matchesOriginal := reOriginal.FindStringSubmatch(line)
	if len(matchesOriginal) > 1 {
		keywords = append(keywords, matchesOriginal[1])
	}

	return keywords
}

func removeHTMLTags(text string) string {
	re := regexp.MustCompile(`<[^>]+>`)
	return re.ReplaceAllString(text, "")
}

func extractTermFromNaturalLanguage(apiKey, userInput, originalTerm, context string) string {
	fmt.Println("   🤖 AI가 사용자 설명을 분석 중...")
	
	prompt := fmt.Sprintf(`사용자가 바차타 용어 교정을 위해 자연어로 설명했습니다.
사용자의 설명을 이해하고, 정확한 바차타 용어만 추출하세요.

**원본 용어:** %s
**문맥:** %s
**사용자 설명:** %s

**규칙:**
1. 사용자가 설명한 내용에서 정확한 바차타 용어만 추출
2. 예: "이 동작은 꼼쁠레또를 말하는거야 완전이라는 스페인어야" → "꼼쁠레또"
3. 예: "롬뽀 아델란떼가 맞아" → "롬뽀 아델란떼"
4. 예: "프론트 웨이브" → "프론트 웨이브"
5. 만약 사용자가 단순히 용어만 입력했다면 그대로 반환
6. 순수한 용어만 출력 (설명이나 추가 텍스트 없이)

**출력:** 추출된 용어만 출력`, originalTerm, context, userInput)

	result := callClaudeAPI(apiKey, prompt, 100)
	result = strings.TrimSpace(result)
	
	if result == "" {
		result = userInput
	}
	
	fmt.Printf("   ✓ 추출된 용어: [%s]\n", result)
	return result
}

func processMetadataEdit(apiKey, originalText, userInstruction, fieldType string) string {
	fmt.Printf("   🤖 AI가 %s 수정 지시를 분석 중...\n", fieldType)
	
	prompt := fmt.Sprintf(`사용자가 유튜브 %s를 수정하려고 자연어로 지시했습니다.
사용자의 지시를 이해하고, 수정된 %s를 출력하세요.

**원본 %s:**
%s

**사용자 지시:**
%s

**규칙:**
1. 사용자가 "제목은: XXX 이렇게 해줘" → 전체를 XXX로 교체
2. 사용자가 "4박자를 3박자로 바꿔줘" → 해당 부분만 수정
3. 사용자가 "Lv.3-1을 추가해줘" → 적절한 위치에 추가
4. 사용자가 단순히 새로운 텍스트만 입력 → 전체 교체
5. 수정된 전체 %s만 출력 (설명이나 추가 텍스트 없이)

**출력:** 수정된 %s 전체`, fieldType, fieldType, fieldType, originalText, userInstruction, fieldType, fieldType)

	result := callClaudeAPI(apiKey, prompt, 1000)
	result = strings.TrimSpace(result)
	
	if result == "" {
		result = userInstruction
	}
	
	fmt.Printf("   ✓ 수정된 %s 생성 완료\n", fieldType)
	return result
}

func translateToLanguage(apiKey, koreanSRT, targetLang, langName string) string {
	fmt.Printf("   📡 API 호출 시작: %s 번역 요청 중...\n", langName)

	prompt := fmt.Sprintf(`아래 한국어 바차타 강습 자막을 %s로 번역하세요.

**중요 배경:**
- 이 자막은 한국인 바차타 강사가 강습하는 영상입니다
- 자막에 나오는 바차타 용어는 영어 또는 스페인어 원어를 한국어 발음으로 표기한 것입니다
- 번역 시 해당 용어의 원어(영어/스페인어)를 정확히 파악하여 번역하세요

**예시:**
- "롬뽀 아델란떼" → 스페인어 "Rompe adelante" → 영어로는 "Forward break"
- "프렙" → 영어 "Prep (Preparation)"
- "웨이브" → 영어 "Wave"

**중요: HTML 태그 제거**
- SRT 파일은 동영상 자막이므로 <b>, </b>, <br> 등의 HTML 태그를 절대 포함하지 마세요
- 순수한 텍스트만 출력하세요

타임코드와 번호는 절대 변경하지 마세요.
번역된 전체 SRT 파일 내용만 출력하세요. (JSON이나 다른 형식 사용 금지)

**한국어 원본:**
%s`, targetLang, koreanSRT)

	maxRetries := 3
	var result string
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			fmt.Printf("   ⏳ 재시도 %d/%d...\n", attempt, maxRetries)
			time.Sleep(2 * time.Second)
		}
		
		result = callClaudeAPI(apiKey, prompt, 8000)
		
		if result != "" {
			break
		}
		
		if attempt < maxRetries {
			fmt.Printf("   ⚠️ %s 번역 실패, 재시도 중...\n", langName)
		}
	}

	if result == "" {
		fmt.Printf("   ❌ %s 번역 실패: %d번 시도 후에도 API 응답 없음\n", langName, maxRetries)
		return ""
	}

	result = removeHTMLTags(result)

	fmt.Printf("   ✓ %s 번역 응답 수신 완료 (길이: %d자)\n", langName, len(result))
	return result
}

func translateMetadata(apiKey, title, description, targetLang, langName string) MetadataResult {
	fmt.Printf("   📡 API 호출 시작: %s 제목/설명 번역 요청 중...\n", langName)

	prompt := fmt.Sprintf(`아래 유튜브 제목과 설명을 %s로 번역하세요.

**출력 형식 (JSON만):**
{
  "title": "번역된 제목",
  "description": "번역된 설명"
}

**한국어 원본:**
제목: %s
설명: %s`, targetLang, title, description)

	maxRetries := 3
	var result string
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			fmt.Printf("   ⏳ 재시도 %d/%d...\n", attempt, maxRetries)
			time.Sleep(2 * time.Second)
		}
		
		result = callClaudeAPI(apiKey, prompt, 2000)
		
		if result != "" {
			break
		}
		
		if attempt < maxRetries {
			fmt.Printf("   ⚠️ %s 메타데이터 번역 실패, 재시도 중...\n", langName)
		}
	}

	if result == "" {
		fmt.Printf("   ❌ %s 메타데이터 번역 실패: %d번 시도 후에도 API 응답 없음\n", langName, maxRetries)
		return MetadataResult{}
	}

	cleanJSON := cleanJSONResponse(result)

	var meta MetadataResult
	err := json.Unmarshal([]byte(cleanJSON), &meta)

	if err != nil {
		fmt.Printf("   ⚠️ %s 메타데이터 파싱 실패: %v\n", langName, err)
		fmt.Printf("   응답 내용: %s\n", cleanJSON[:min(200, len(cleanJSON))])
		return MetadataResult{}
	}

	fmt.Printf("   ✓ %s 제목/설명 번역 완료\n", langName)
	return meta
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

func main() {
	// 바차타 테크닉 사전 로드
	err := loadBachataTechniques()
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		fmt.Println("⚠️ 바차타 테크닉 사전 없이 진행합니다.")
	}

	os.MkdirAll(DirInput, 0755)

	files, _ := os.ReadDir(DirInput)
	var srtFiles []string
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".srt" {
			srtFiles = append(srtFiles, file.Name())
		}
	}

	if len(srtFiles) == 0 {
		fmt.Printf("⚠️ '%s' 폴더에 SRT 파일이 없습니다.\n", DirInput)
		return
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Println("❌ 에러: ANTHROPIC_API_KEY 환경 변수가 설정되지 않았습니다.")
		return
	}

	targetFile := srtFiles[0]
	filePath := filepath.Join(DirInput, targetFile)
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("❌ 파일 읽기 실패: %v\n", err)
		return
	}

	modifiedContent := string(contentBytes)

	fmt.Printf("\n🎯 [%s] AI 교정 파이프라인을 시작합니다...\n", targetFile)

	// =====================================================================
	// [STEP 0] 바차타 테크닉 우선순위 사전 기반 자동 치환
	// =====================================================================
	if len(bachataTechniques) > 0 {
		fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("🎯 STEP 0: 바차타 테크닉 우선순위 사전 기반 자동 치환")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		scanner := bufio.NewScanner(os.Stdin)
		words := strings.Fields(modifiedContent)
		replacements := make(map[string]string)

		for _, word := range words {
			cleanWord := strings.Trim(word, ".,!?\"'()[]{}:;")
			
			// 이미 처리한 단어는 스킵
			if _, exists := replacements[cleanWord]; exists {
				continue
			}

			// 범용 영어 용어는 스킵
			if universalEnglishTerms[strings.ToLower(cleanWord)] {
				continue
			}

			// 유사도 매칭 (threshold: 0.75)
			tech, score := findBestMatchingTechnique(cleanWord, 0.75)
			if tech == nil {
				continue
			}

			// 영어 용어로 사용된 경우 감지
			// 영어 발음과 정확히 매칭되는 경우만 영어로 표기
			isEnglishTerm := false
			for _, engPron := range strings.Split(tech.EnglishKoreanPronunciation, " / ") {
				trimmedEngPron := strings.TrimSpace(engPron)
				engScore := similarityScore(cleanWord, trimmedEngPron)
				if engScore >= 0.75 {
					isEnglishTerm = true
					break
				}
			}

			// 높은 유사도 (0.9 이상): 자동 치환
			// 단, 영어 발음이 여러 개인 경우 (/ 포함) 스킵 (사용자 확인 필요)
			if score >= 0.9 {
				// 영어 발음이 여러 개 있는 경우 (예: "프레퍼레이션 / 업")
				if strings.Contains(tech.EnglishKoreanPronunciation, " / ") {
					// 여러 개 있으면 자동 치환하지 않고 사용자 확인 필요 (아래 중간 유사도 로직으로)
					// 스페인어와 매칭된 경우만 자동 치환
					if !isEnglishTerm {
						formatted := formatTechniqueOutput(tech, false)
						replacements[cleanWord] = formatted
						fmt.Printf("✅ 자동 치환: [%s] → [%s] (유사도: %.2f)\n", cleanWord, formatted, score)
						continue
					}
					// 영어 용어이면서 여러 개인 경우 스킵
				} else {
					// 영어 발음이 하나만 있는 경우 기존대로 자동 치환
					formatted := formatTechniqueOutput(tech, isEnglishTerm)
					replacements[cleanWord] = formatted
					fmt.Printf("✅ 자동 치환: [%s] → [%s] (유사도: %.2f)\n", cleanWord, formatted, score)
					continue
				}
			}

			// 중간 유사도 (0.75~0.9): 사용자 확인 필요
			fmt.Printf("\n❓ 애매한 매칭 발견:\n")
			fmt.Printf("   원본: [%s]\n", cleanWord)
			
			// 영어 용어인 경우와 스페인어 용어인 경우를 구분하여 제안
			if isEnglishTerm {
				fmt.Printf("   제안: [%s(%s)] (유사도: %.2f)\n", tech.EnglishKoreanPronunciation, tech.EnglishEquivalent, score)
				fmt.Printf("   의미: %s\n", tech.Meaning)
				fmt.Print("   👉 치환할까요? (Y/n): ")
			} else {
				fmt.Printf("   제안: [%s(%s)] (유사도: %.2f)\n", tech.SpanishPronunciation, tech.SpanishName, score)
				fmt.Printf("   의미: %s\n", tech.Meaning)
				fmt.Print("   👉 치환할까요? (Y/n): ")
			}
			
			scanner.Scan()
			userInput := strings.TrimSpace(strings.ToLower(scanner.Text()))
			
			if userInput == "" || userInput == "y" || userInput == "yes" {
				formatted := formatTechniqueOutput(tech, isEnglishTerm)
				replacements[cleanWord] = formatted
				fmt.Printf("✅ [%s] → [%s] 적용됨\n", cleanWord, formatted)
			} else {
				fmt.Println("➡️ 원본 유지")
			}
		}

		// 치환 적용
		for original, replacement := range replacements {
			modifiedContent = strings.ReplaceAll(modifiedContent, original, replacement)
		}

		fmt.Printf("\n✅ STEP 0 완료: %d개 용어 치환됨\n", len(replacements))
	}

	// =====================================================================
	// [STEP 1] 한국어 STT 완벽 교정 (기존 로직 유지)
	// =====================================================================
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("📝 STEP 1: 한국어 STT 완벽 교정")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	fmt.Println("⏳ [1-0] 용어사전 필터링 중...")
	filteredDict := filterDictionary(modifiedContent, "용어사전.txt")
	fmt.Printf("   ✓ 필터링 완료: %d개 관련 용어 추출됨\n", len(strings.Split(filteredDict, "\n")))

	fmt.Println("⏳ [1-1] AI가 10년차 바차타 강사 페르소나로 문맥을 분석 중...")

	prompt1 := fmt.Sprintf(`당신은 한국에서 10년간 바차타를 가르친 전문 강사입니다.

**중요한 배경 지식:**
- 이 자막은 한국인 바차타 강사가 강습하는 영상을 유튜브가 자동으로 만든 STT입니다
- 강사는 바차타 동작을 설명할 때:
  1. 영어를 한국어 발음으로 말하는 경우 (예: "프렙" = Prep)
  2. 스페인어를 한국어 발음으로 말하는 경우 (예: "롬뽀 아델란떼" = Rompe adelante)
  3. 둘 다 섞어서 사용하는 경우가 있습니다
  4. 타임코드 절대 보존: 내가 제공한 원본 타임코드는 1초도, 0.001초도 절대 건드리지 마. 그대로 복사해서 써.
  5. 한국어 포함 번역한 모든 자막도 타임코드에 맞게 해야합니다

**당신의 임무:**
1. 자동자막의 오타/오인식을 찾아내세요
2. 그 단어가 영어인지 스페인어인지 판단하세요
3. 용어사전을 참고하여 정확한 바차타 용어로 교정하세요
4. **확신이 없거나 애매하면 반드시 사용자에게 질문하세요**
5. **중요: 원본과 제안이 동일하면 절대 제안하지 마세요 (API 비용 낭비 방지)**
6. 타임코드 절대 보존: 내가 제공한 원본 타임코드는 1초도, 0.001초도 절대 건드리지 마. 그대로 복사해서 써.
7. 한국어 포함 번역한 모든 자막도 타임코드에 맞게 해야합니다

**교정 예시:**
- "꼼블레도" → 영어 "Complete"를 스페인어 발음 "Completo(꼼쁠레또)"로 말한 것 같음 → 사용자 확인 필요
- "론포 델렌 때" → 스페인어 "Rompe adelante(롬뽀 아델란떼)" 오인식 → 확신함
- "견각골" → 해부학 용어 "견갑골" 오타 → 확신함
- 카운트 숫자 변환: 모든 카운트(원, 투, 쓰리, 포, 파이, 파이브, 식스, 세븐, 에잇 등)는 반드시 아라비아 숫자(1 2 3 4 5 6 7 8)로 표기할 것 타임코드에 맞게 해야함 1 2 등 띄어쓰기가독성에 좋게 띄어쓰기를 해야합니다 1 2 3와 1 2 3 4 5 6 7 8 은 띄어쓰기가 달라야 합니다 그렇다고 해서 1 2 일때 너무 멀면안됩니다 예쁘게 가독성에 좋게 띄어쓰기를 하세요
- 볼레로 핀사 카운트 (예: 십 구 인싸 파이 십 세) -> 문맥에 맞춰 '볼레로 핀사(Pinza) 5 6 7' 등으로 수정
- 힙스로우 -> 힙쓰로우
- 사이트 프레이브 -> 사이드웨이브
- 맞춤법 및 문맥 수정: '거에요' -> '거예요', '맞고' -> '막고', '골만도' -> '골반도' 등 자동 자막 특유의 명백한 오타를 문맥에 맞게 자연스럽게 고칠 것.

**필터링된 관련 용어사전:**
%s

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
%s`, filteredDict, modifiedContent)

	fmt.Println("   📡 Claude API 호출 중...")
	extractionResult := callClaudeAPI(apiKey, prompt1, 4000)

	if extractionResult != "" {
		fmt.Println("   ✓ AI 응답 수신 완료")
		cleanJSON := cleanJSONResponse(extractionResult)

		var suggestions []Suggestion
		err := json.Unmarshal([]byte(cleanJSON), &suggestions)

		if err != nil {
			fmt.Printf("❌ AI 응답 파싱 에러: %v\n", err)
			fmt.Printf("응답 원본 (처음 500자):\n%s\n", extractionResult[:min(500, len(extractionResult))])
			fmt.Println("\n⚠️ JSON 파싱 실패. 교정 단계를 건너뜁니다.")
		} else if len(suggestions) > 0 {
			scanner := bufio.NewScanner(os.Stdin)
			fmt.Printf("\n🧠 AI가 %d개의 교정 제안을 생성했습니다. 각 항목을 검토해주세요.\n", len(suggestions))
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			for i, s := range suggestions {
				if s.OriginalSTT == s.BestGuess {
					continue
				}
				
				if s.OriginalSTT != "" && strings.Contains(modifiedContent, s.OriginalSTT) {
					fmt.Printf("\n[%d/%d] ⏱️ 타임코드: %s\n", i+1, len(suggestions), s.Timecode)
					fmt.Printf("❌ 원본: [%s]\n", s.OriginalSTT)
					fmt.Printf("📖 문맥: %s\n", s.ContextAnalysis)
					fmt.Printf("💡 분석: %s\n", s.AIReasoning)

					if s.NeedsConfirmation && s.QuestionToUser != "" {
						fmt.Printf("\n❓ AI 질문: %s\n", s.QuestionToUser)
						fmt.Printf("✅ AI 제안: [%s]\n", s.BestGuess)
						fmt.Printf("👉 선택 (엔터=AI제안 적용 / '유지'=원본유지 / 직접입력): ")
					} else {
						fmt.Printf("✅ 제안: [%s]\n", s.BestGuess)
						fmt.Printf("👉 선택 (엔터=적용 / '유지'=원본유지 / 직접입력): ")
					}

					scanner.Scan()
					userInput := strings.TrimSpace(scanner.Text())

					if userInput == "유지" {
						fmt.Println("➡️ 원본 유지")
					} else if userInput == "" {
						modifiedContent = strings.ReplaceAll(modifiedContent, s.OriginalSTT, s.BestGuess)
						fmt.Printf("✅ [%s] → [%s] 적용됨\n", s.OriginalSTT, s.BestGuess)
					} else {
						finalTerm := extractTermFromNaturalLanguage(apiKey, userInput, s.OriginalSTT, s.ContextAnalysis)
						modifiedContent = strings.ReplaceAll(modifiedContent, s.OriginalSTT, finalTerm)
						fmt.Printf("✅ [%s] → [%s] 적용됨\n", s.OriginalSTT, finalTerm)
					}
				}
			}
		} else {
			fmt.Println("⚠️ 교정 제안 없음")
		}
	}

	fmt.Println("\n⏳ [1-2] 전체 문장 흐름 다듬기 중...")

	prompt2 := fmt.Sprintf(`아래는 바차타 강습 자막입니다. 용어는 이미 확정되었으므로 절대 변경하지 마세요.
타임코드와 번호는 그대로 유지하고, 문장만 자연스럽게 다듬어주세요.

**규칙:**
- 타임코드 형식 유지
- 자막 번호 유지
- 확정된 바차타 용어 변경 금지
- 문장만 자연스럽게 교정

완성된 SRT 파일 전체를 출력하세요.

%s`, modifiedContent)

	fmt.Println("   📡 Claude API 호출 중...")
	finalKoreanSRT := callClaudeAPI(apiKey, prompt2, 8000)

	if finalKoreanSRT == "" {
		fmt.Println("❌ 문장 다듬기 실패")
		return
	}
	
	finalKoreanSRT = removeHTMLTags(finalKoreanSRT)
	
	fmt.Println("   ✓ 문장 다듬기 완료")

	dateFolder := time.Now().Format("2006-01-02")
	baseFileName := strings.TrimSuffix(targetFile, filepath.Ext(targetFile))
	
	subtitlePath := filepath.Join(dateFolder, "자막번역완성", baseFileName)
	metadataPath := filepath.Join(dateFolder, "제목설명완성", baseFileName)
	
	os.MkdirAll(subtitlePath, 0755)
	os.MkdirAll(metadataPath, 0755)

	koreanPath := filepath.Join(subtitlePath, baseFileName+"_한국어.srt")
	os.WriteFile(koreanPath, []byte(finalKoreanSRT), 0644)

	fmt.Printf("\n✅ STEP 1 완료: %s\n", koreanPath)

	// =====================================================================
	// 사용자 확인 및 수정 루프: 한국어 자막 검토 후 번역 승인
	// =====================================================================
	reviewScanner := bufio.NewScanner(os.Stdin)
	
	for {
		fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("✅ 한국어 자막을 완성했습니다!")
		fmt.Printf("📁 저장 위치: %s\n", koreanPath)
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Print("\n👉 검토 후 선택해주세요:\n")
		fmt.Print("   1. 번역 진행 (Y)\n")
		fmt.Print("   2. 자막 수정 (N)\n")
		fmt.Print("선택: ")
		
		reviewScanner.Scan()
		reviewChoice := strings.TrimSpace(strings.ToLower(reviewScanner.Text()))
		
		if reviewChoice == "y" || reviewChoice == "yes" || reviewChoice == "" {
			fmt.Println("\n✅ 번역을 시작합니다!")
			break
		}
		
		// 수정 루프
		for {
			fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			fmt.Println("📝 자막 수정 모드")
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			fmt.Println("수정할 내용을 자연어로 입력하세요.")
			fmt.Println("예: '00:00:05,000 부분의 사이드웨이브를 프론트 웨이브로 바꿔줘'")
			fmt.Print("\n수정 내용: ")
			
			reviewScanner.Scan()
			editInstruction := strings.TrimSpace(reviewScanner.Text())
			
			if editInstruction == "" {
				fmt.Println("⚠️ 수정 내용이 입력되지 않았습니다.")
				continue
			}
			
			fmt.Println("\n⏳ AI가 수정 지시를 분석하고 자막을 수정 중...")
			
			editPrompt := fmt.Sprintf(`사용자가 바차타 강습 자막을 수정하려고 자연어로 지시했습니다.
사용자의 지시를 이해하고, 수정된 SRT 파일 전체를 출력하세요.

**원본 SRT:**
%s

**사용자 수정 지시:**
%s

**규칙:**
1. 타임코드는 절대 변경하지 마세요
2. 자막 번호는 그대로 유지하세요
3. 사용자가 지시한 부분만 정확히 수정하세요
4. 수정된 전체 SRT 파일을 출력하세요 (JSON이나 다른 형식 사용 금지)

**출력:** 수정된 전체 SRT 파일`, finalKoreanSRT, editInstruction)

			editedSRT := callClaudeAPI(apiKey, editPrompt, 8000)
			
			if editedSRT == "" {
				fmt.Println("❌ 자막 수정 실패")
				continue
			}
			
			editedSRT = removeHTMLTags(editedSRT)
			finalKoreanSRT = editedSRT
			
			// 수정된 자막 저장
			os.WriteFile(koreanPath, []byte(finalKoreanSRT), 0644)
			fmt.Printf("✅ 자막이 수정되어 저장되었습니다: %s\n", koreanPath)
			
			// 추가 수정 여부 확인
			fmt.Print("\n👉 또 수정하시겠습니까? (Y/n): ")
			reviewScanner.Scan()
			continueEdit := strings.TrimSpace(strings.ToLower(reviewScanner.Text()))
			
			if continueEdit != "y" && continueEdit != "yes" && continueEdit != "" {
				break
			}
		}
		
		// 수정 완료 후 다시 검토
		fmt.Println("\n⏳ 수정 후 다시 한국어 자막을 만들었습니다.")
	}

	// =====================================================================
	// [STEP 2] 11개국 다국어 번역 (기존 로직 유지)
	// =====================================================================
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("🌍 STEP 2: 11개국 다국어 번역 (개별 API 호출)")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	fmt.Println("\n⏳ [2-1] 영어 번역 시작...")
	englishSRT := translateToLanguage(apiKey, finalKoreanSRT, "English", "영어")
	if englishSRT != "" {
		savePath := filepath.Join(subtitlePath, baseFileName+"_영어.srt")
		os.WriteFile(savePath, []byte(englishSRT), 0644)
		fmt.Println("✅ 영어 번역 완료 및 저장")
	}

	fmt.Println("\n⏳ [2-2] 스페인어 번역 시작...")
	spanishSRT := translateToLanguage(apiKey, finalKoreanSRT, "Español", "스페인어")
	if spanishSRT != "" {
		savePath := filepath.Join(subtitlePath, baseFileName+"_스페인어.srt")
		os.WriteFile(savePath, []byte(spanishSRT), 0644)
		fmt.Println("✅ 스페인어 번역 완료 및 저장")
	}

	fmt.Println("\n⏳ [2-3] 폴란드어 번역 시작...")
	polishSRT := translateToLanguage(apiKey, finalKoreanSRT, "Polski", "폴란드어")
	if polishSRT != "" {
    	savePath := filepath.Join(subtitlePath, baseFileName+"_폴란드어.srt")
    	os.WriteFile(savePath, []byte(polishSRT), 0644)
    	fmt.Println("✅ 폴란드어 번역 완료 및 저장")
	}

	fmt.Println("\n⏳ [2-4] 일본어 번역 시작...")
	japaneseSRT := translateToLanguage(apiKey, finalKoreanSRT, "日本語", "일본어")
	if japaneseSRT != "" {
		savePath := filepath.Join(subtitlePath, baseFileName+"_일본어.srt")
		os.WriteFile(savePath, []byte(japaneseSRT), 0644)
		fmt.Println("✅ 일본어 번역 완료 및 저장")
	}

	fmt.Println("\n⏳ [2-5] 중국어 번역 시작...")
	chineseSRT := translateToLanguage(apiKey, finalKoreanSRT, "中文 (简体)", "중국어")
	if chineseSRT != "" {
		savePath := filepath.Join(subtitlePath, baseFileName+"_중국어.srt")
		os.WriteFile(savePath, []byte(chineseSRT), 0644)
		fmt.Println("✅ 중국어 번역 완료 및 저장")
	}

	fmt.Println("\n⏳ [2-6] 프랑스어 번역 시작...")
	frenchSRT := translateToLanguage(apiKey, finalKoreanSRT, "Français", "프랑스어")
	if frenchSRT != "" {
		savePath := filepath.Join(subtitlePath, baseFileName+"_프랑스어.srt")
		os.WriteFile(savePath, []byte(frenchSRT), 0644)
		fmt.Println("✅ 프랑스어 번역 완료 및 저장")
	}

	fmt.Println("\n⏳ [2-7] 독일어 번역 시작...")
	germanSRT := translateToLanguage(apiKey, finalKoreanSRT, "Deutsch", "독일어")
	if germanSRT != "" {
		savePath := filepath.Join(subtitlePath, baseFileName+"_독일어.srt")
		os.WriteFile(savePath, []byte(germanSRT), 0644)
		fmt.Println("✅ 독일어 번역 완료 및 저장")
	}

	fmt.Println("\n⏳ [2-8] 이탈리아어 번역 시작...")
	italianSRT := translateToLanguage(apiKey, finalKoreanSRT, "Italiano", "이탈리아어")
	if italianSRT != "" {
		savePath := filepath.Join(subtitlePath, baseFileName+"_이탈리아어.srt")
		os.WriteFile(savePath, []byte(italianSRT), 0644)
		fmt.Println("✅ 이탈리아어 번역 완료 및 저장")
	}

	fmt.Println("\n⏳ [2-9] 베트남어 번역 시작...")
	vietnameseSRT := translateToLanguage(apiKey, finalKoreanSRT, "Tiếng Việt", "베트남어")
	if vietnameseSRT != "" {
		savePath := filepath.Join(subtitlePath, baseFileName+"_베트남어.srt")
		os.WriteFile(savePath, []byte(vietnameseSRT), 0644)
		fmt.Println("✅ 베트남어 번역 완료 및 저장")
	}

	fmt.Println("\n⏳ [2-10] 말레이어 번역 시작...")
	malaySRT := translateToLanguage(apiKey, finalKoreanSRT, "Bahasa Melayu", "말레이어")
	if malaySRT != "" {
    	savePath := filepath.Join(subtitlePath, baseFileName+"_말레이어.srt")
    	os.WriteFile(savePath, []byte(malaySRT), 0644)
    	fmt.Println("✅ 말레이어 번역 완료 및 저장")
	}

	fmt.Println("\n✅ STEP 2 완료: 10개 언어 번역 완료")

	// =====================================================================
	// [STEP 3] 유튜브 제목/설명 생성 (기존 로직 유지)
	// =====================================================================
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("📺 STEP 3: 유튜브 제목 및 설명 생성")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	fmt.Println("\n⏳ [3-0] 한국어 제목/설명 생성 중...")

	promptMeta := fmt.Sprintf(`아래 바차타 강습 자막을 바탕으로 유튜브 제목과 설명을 작성하세요.

**제목 요구사항:**
- 60자 이내
- SEO 최적화 키워드 포함
- 클릭을 유도하는 매력적인 문구

**설명 요구사항:**
- **800~900자** 분량
- SEO 키워드 다량 포함 (바차타, 센슈얼바차타, 댄스강습, 파트너워크, 리딩, 팔로잉 등)
- 구체적인 학습 내용을 **절 번호(1., 2., 3.)**로 간결하게 구조화
- 초보자도 이해하기 쉬운 친절한 설명
- **중요**: 섹션 제목(예: "이 댄스강습을 통해 얻을 수 있는 것:", "핵심 내용:" 등) 사용 금지
- 구조:
  [도입부] 이 영상에서 배울 내용 소개 (100자)
  
  [핵심 내용 - 절 번호로 간결하게 구조화]
  1. 첫 번째 학습 포인트 (간결하게)
  2. 두 번째 학습 포인트 (간결하게)
  3. 세 번째 학습 포인트 (간결하게)
  ... (총 300자 - 각 항목당 2줄 이내)
  
  [학습 효과 - 섹션 제목 없이 바로 내용]
  센슈얼바차타의 우아하고 세련된 사이드웨이브를 완벽히 마스터하여... (200자)
  
  [해시태그]
  #바차타 #센슈얼바차타 #댄스강습 #파트너워크 등 (100자)

**출력 형식 (JSON):**
{
  "title": "유튜브 제목",
  "description": "유튜브 설명 (800~900자)"
}

**자막 내용:**
%s`, finalKoreanSRT)

	fmt.Println("   📡 Claude API 호출 중...")
	metaJSON := callClaudeAPI(apiKey, promptMeta, 2000)
	cleanMetaJSON := cleanJSONResponse(metaJSON)

	var koreanMeta MetadataResult
	err = json.Unmarshal([]byte(cleanMetaJSON), &koreanMeta)

	if err != nil {
		fmt.Printf("❌ 메타데이터 파싱 실패: %v\n", err)
		return
	}

	fmt.Printf("\n📌 생성된 한국어 제목:\n%s\n", koreanMeta.Title)
	fmt.Printf("\n📌 생성된 한국어 설명:\n%s\n", koreanMeta.Description)
	
	scanner := bufio.NewScanner(os.Stdin)
	
	fmt.Print("\n제목 수정 (엔터=그대로 사용 / 자연어로 수정 지시): ")
	scanner.Scan()
	titleInput := scanner.Text()
	if titleInput != "" {
		koreanMeta.Title = processMetadataEdit(apiKey, koreanMeta.Title, titleInput, "제목")
		fmt.Println("✅ 제목 수정됨")
	} else {
		fmt.Println("➡️ AI 생성 제목 사용")
	}
	
	fmt.Print("\n설명 수정 (엔터=그대로 사용 / 자연어로 수정 지시): ")
	scanner.Scan()
	descInput := scanner.Text()
	if descInput != "" {
		koreanMeta.Description = processMetadataEdit(apiKey, koreanMeta.Description, descInput, "설명")
		fmt.Println("✅ 설명 수정됨")
	} else {
		fmt.Println("➡️ AI 생성 설명 사용")
	}

	koreanMetaPath := filepath.Join(metadataPath, baseFileName+"_한국어.txt")
	metaContent := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", koreanMeta.Title, koreanMeta.Description)
	os.WriteFile(koreanMetaPath, []byte(metaContent), 0644)
	fmt.Println("✅ 한국어 제목/설명 저장 완료")

	fmt.Println("\n⏳ 나머지 10개 언어 제목/설명 번역 시작...")

	fmt.Println("\n[3-1] 영어 제목/설명 번역 중...")
	engMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "English", "영어")
	if engMeta.Title != "" {
		metaPath := filepath.Join(metadataPath, baseFileName+"_영어.txt")
		content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", engMeta.Title, engMeta.Description)
		os.WriteFile(metaPath, []byte(content), 0644)
		fmt.Println("✅ 영어 제목/설명 저장 완료")
	}

	fmt.Println("\n[3-2] 스페인어 제목/설명 번역 중...")
	spaMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "Español", "스페인어")
	if spaMeta.Title != "" {
		metaPath := filepath.Join(metadataPath, baseFileName+"_스페인어.txt")
		content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", spaMeta.Title, spaMeta.Description)
		os.WriteFile(metaPath, []byte(content), 0644)
		fmt.Println("✅ 스페인어 제목/설명 저장 완료")
	}

	fmt.Println("\n[3-3] 폴란드어 제목/설명 번역 중...")
	plMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "Polski", "폴란드어")
	if plMeta.Title != "" {
    	metaPath := filepath.Join(metadataPath, baseFileName+"_폴란드어.txt")
    	content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", plMeta.Title, plMeta.Description)
    	os.WriteFile(metaPath, []byte(content), 0644)
    	fmt.Println("✅ 폴란드어 제목/설명 저장 완료")
	}

	fmt.Println("\n[3-4] 일본어 제목/설명 번역 중...")
	jpnMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "日本語", "일본어")
	if jpnMeta.Title != "" {
		metaPath := filepath.Join(metadataPath, baseFileName+"_일본어.txt")
		content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", jpnMeta.Title, jpnMeta.Description)
		os.WriteFile(metaPath, []byte(content), 0644)
		fmt.Println("✅ 일본어 제목/설명 저장 완료")
	}

	fmt.Println("\n[3-5] 중국어 제목/설명 번역 중...")
	chnMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "中文 (简体)", "중국어")
	if chnMeta.Title != "" {
		metaPath := filepath.Join(metadataPath, baseFileName+"_중국어.txt")
		content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", chnMeta.Title, chnMeta.Description)
		os.WriteFile(metaPath, []byte(content), 0644)
		fmt.Println("✅ 중국어 제목/설명 저장 완료")
	}

	fmt.Println("\n[3-6] 프랑스어 제목/설명 번역 중...")
	frMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "Français", "프랑스어")
	if frMeta.Title != "" {
		metaPath := filepath.Join(metadataPath, baseFileName+"_프랑스어.txt")
		content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", frMeta.Title, frMeta.Description)
		os.WriteFile(metaPath, []byte(content), 0644)
		fmt.Println("✅ 프랑스어 제목/설명 저장 완료")
	}

	fmt.Println("\n[3-7] 독일어 제목/설명 번역 중...")
	deMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "Deutsch", "독일어")
	if deMeta.Title != "" {
		metaPath := filepath.Join(metadataPath, baseFileName+"_독일어.txt")
		content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", deMeta.Title, deMeta.Description)
		os.WriteFile(metaPath, []byte(content), 0644)
		fmt.Println("✅ 독일어 제목/설명 저장 완료")
	}

	fmt.Println("\n[3-8] 이탈리아어 제목/설명 번역 중...")
	itMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "Italiano", "이탈리아어")
	if itMeta.Title != "" {
		metaPath := filepath.Join(metadataPath, baseFileName+"_이탈리아어.txt")
		content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", itMeta.Title, itMeta.Description)
		os.WriteFile(metaPath, []byte(content), 0644)
		fmt.Println("✅ 이탈리아어 제목/설명 저장 완료")
	}

	fmt.Println("\n[3-9] 베트남어 제목/설명 번역 중...")
	viMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "Tiếng Việt", "베트남어")
	if viMeta.Title != "" {
		metaPath := filepath.Join(metadataPath, baseFileName+"_베트남어.txt")
		content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", viMeta.Title, viMeta.Description)
		os.WriteFile(metaPath, []byte(content), 0644)
		fmt.Println("✅ 베트남어 제목/설명 저장 완료")
	}

	fmt.Println("\n[3-10] 말레이어 제목/설명 번역 중...")
	malayMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "Bahasa Melayu", "말레이어")
	if malayMeta.Title != "" {
    	metaPath := filepath.Join(metadataPath, baseFileName+"_말레이어.txt")
    	content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", malayMeta.Title, malayMeta.Description)
    	os.WriteFile(metaPath, []byte(content), 0644)
    	fmt.Println("✅ 말레이어 제목/설명 저장 완료")
	}

	calculateCost()

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("🎉 모든 작업 완료!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("📁 결과 폴더: %s\n", dateFolder)
	fmt.Printf("  ├─ 자막번역완성/%s/ (11개 언어 SRT)\n", baseFileName)
	fmt.Printf("  └─ 제목설명완성/%s/ (11개 언어 제목/설명)\n", baseFileName)
	
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("💰 토큰 사용량 및 예상 비용")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("📊 총 토큰: %s개\n", formatNumber(globalTokenUsage.TotalTokens))
	fmt.Printf("   ├─ 입력 토큰: %s개 ($%.4f)\n", formatNumber(globalTokenUsage.InputTokens), float64(globalTokenUsage.InputTokens)/1000000.0*3.0)
	fmt.Printf("   └─ 출력 토큰: %s개 ($%.4f)\n", formatNumber(globalTokenUsage.OutputTokens), float64(globalTokenUsage.OutputTokens)/1000000.0*15.0)
	fmt.Printf("\n💵 예상 비용: ₩%s원 (환율 1,350원 기준)\n", formatNumber(int(globalTokenUsage.EstimatedCost)))
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

func formatNumber(n int) string {
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}
	
	var result []rune
	for i, r := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, r)
	}
	return string(result)
}
