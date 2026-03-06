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
	"time"
)

const (
	DirInput = "번역전"
)

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
	
	// 토큰 사용량 추적을 위한 전체 응답 파싱
	var fullResp map[string]interface{}
	json.Unmarshal(body, &fullResp)
	
	// 토큰 사용량 추출
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

// 토큰 비용 계산 함수 (Claude Sonnet 4 기준)
func calculateCost() {
	// Claude Sonnet 4 요금 (2025년 5월 기준)
	// 입력: $3.00 per million tokens
	// 출력: $15.00 per million tokens
	inputCostPerMillion := 3.00
	outputCostPerMillion := 15.00
	
	inputCost := (float64(globalTokenUsage.InputTokens) / 1000000.0) * inputCostPerMillion
	outputCost := (float64(globalTokenUsage.OutputTokens) / 1000000.0) * outputCostPerMillion
	
	totalCostUSD := inputCost + outputCost
	
	// 환율 (1 USD = 1,350 KRW 가정)
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

// 용어사전에서 관련 용어만 필터링하는 함수 (토큰 절약)
func filterDictionary(srtContent, dictionaryPath string) string {
	dictBytes, err := os.ReadFile(dictionaryPath)
	if err != nil {
		return ""
	}

	dictLines := strings.Split(string(dictBytes), "\n")
	var relevantTerms []string

	// SRT 내용을 소문자로 변환하여 검색
	srtLower := strings.ToLower(srtContent)

	for _, line := range dictLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 용어사전 각 줄에서 키워드 추출 (한글, 영어, 스페인어)
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

// 용어사전 라인에서 키워드 추출
func extractKeywords(line string) []string {
	var keywords []string

	// "베이직(Basic)" 형태에서 "베이직", "Basic" 추출
	re := regexp.MustCompile(`([가-힣]+)\s*\(([A-Za-z\s]+)\)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 2 {
		keywords = append(keywords, matches[1], matches[2])
	}

	// "원어: Básico" 형태에서 "Básico" 추출
	reOriginal := regexp.MustCompile(`원어:\s*([A-Za-zÀ-ÿ\s]+)`)
	matchesOriginal := reOriginal.FindStringSubmatch(line)
	if len(matchesOriginal) > 1 {
		keywords = append(keywords, matchesOriginal[1])
	}

	return keywords
}

// 개별 언어 번역 함수 (디버깅 강화)
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

타임코드와 번호는 절대 변경하지 마세요.
번역된 전체 SRT 파일 내용만 출력하세요. (JSON이나 다른 형식 사용 금지)

**한국어 원본:**
%s`, targetLang, koreanSRT)

	result := callClaudeAPI(apiKey, prompt, 8000)

	if result == "" {
		fmt.Printf("   ❌ %s 번역 실패: API 응답 없음\n", langName)
		return ""
	}

	fmt.Printf("   ✓ %s 번역 응답 수신 완료 (길이: %d자)\n", langName, len(result))
	return result
}

// 개별 언어 메타데이터 번역 함수
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

	result := callClaudeAPI(apiKey, prompt, 2000)

	if result == "" {
		fmt.Printf("   ❌ %s 메타데이터 번역 실패: API 응답 없음\n", langName)
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

func main() {
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
	// [STEP 1] 한국어 STT 완벽 교정
	// =====================================================================
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("📝 STEP 1: 한국어 STT 완벽 교정")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 필터링된 용어사전 생성 (토큰 절약)
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

**당신의 임무:**
1. 자동자막의 오타/오인식을 찾아내세요
2. 그 단어가 영어인지 스페인어인지 판단하세요
3. 용어사전을 참고하여 정확한 바차타 용어로 교정하세요
4. **확신이 없거나 애매하면 반드시 사용자에게 질문하세요**

**교정 예시:**
- "꼼블레도" → 영어 "Complete"를 스페인어 발음 "Completo(꼼쁠레또)"로 말한 것 같음 → 사용자 확인 필요
- "론포 델렌 때" → 스페인어 "Rompe adelante(롬뽀 아델란떼)" 오인식 → 확신함
- "견각골" → 해부학 용어 "견갑골" 오타 → 확신함
- "원 투 쓰리 4" → 카운트 "원 투 쓰리 포"로 통일 → 확신함

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
			fmt.Println("응답 원본:", extractionResult[:min(500, len(extractionResult))])
		} else if len(suggestions) > 0 {
			scanner := bufio.NewScanner(os.Stdin)
			fmt.Printf("\n🧠 AI가 %d개의 교정 제안을 생성했습니다. 각 항목을 검토해주세요.\n", len(suggestions))
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			for i, s := range suggestions {
				if s.OriginalSTT != "" && strings.Contains(modifiedContent, s.OriginalSTT) {
					fmt.Printf("\n[%d/%d] ⏱️ 타임코드: %s\n", i+1, len(suggestions), s.Timecode)
					fmt.Printf("❌ 원본: [%s]\n", s.OriginalSTT)
					fmt.Printf("📖 문맥: %s\n", s.ContextAnalysis)
					fmt.Printf("💡 분석: %s\n", s.AIReasoning)

					// AI가 확신이 없어서 사용자 확인이 필요한 경우
					if s.NeedsConfirmation && s.QuestionToUser != "" {
						fmt.Printf("\n❓ AI 질문: %s\n", s.QuestionToUser)
						fmt.Printf("✅ AI 제안: [%s]\n", s.BestGuess)
						fmt.Printf("👉 선택 (엔터=AI제안 적용 / '유지'=원본유지 / 직접입력): ")
					} else {
						// AI가 확신하는 경우
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
						modifiedContent = strings.ReplaceAll(modifiedContent, s.OriginalSTT, userInput)
						fmt.Printf("✅ [%s] → [%s] 적용됨\n", s.OriginalSTT, userInput)
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
	fmt.Println("   ✓ 문장 다듬기 완료")

	// 날짜 폴더 및 파일별 하위 폴더 생성
	dateFolder := time.Now().Format("2006-01-02")
	baseFileName := strings.TrimSuffix(targetFile, filepath.Ext(targetFile))
	
	// 구조: 2026-03-06/자막번역완성/사이드웨이브기본파트너웍/
	subtitlePath := filepath.Join(dateFolder, "자막번역완성", baseFileName)
	metadataPath := filepath.Join(dateFolder, "제목설명완성", baseFileName)
	
	os.MkdirAll(subtitlePath, 0755)
	os.MkdirAll(metadataPath, 0755)

	// 한국어 완성본 저장
	koreanPath := filepath.Join(subtitlePath, baseFileName+"_한국어.srt")
	os.WriteFile(koreanPath, []byte(finalKoreanSRT), 0644)

	fmt.Printf("\n✅ STEP 1 완료: %s\n", koreanPath)

	// =====================================================================
	// [STEP 2] 11개국 다국어 번역
	// =====================================================================
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("🌍 STEP 2: 11개국 다국어 번역 (개별 API 호출)")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 영어
	fmt.Println("\n⏳ [2-1] 영어 번역 시작...")
	englishSRT := translateToLanguage(apiKey, finalKoreanSRT, "English", "영어")
	if englishSRT != "" {
		savePath := filepath.Join(subtitlePath, baseFileName+"_영어.srt")
		os.WriteFile(savePath, []byte(englishSRT), 0644)
		fmt.Println("✅ 영어 번역 완료 및 저장")
	}

	// 스페인어
	fmt.Println("\n⏳ [2-2] 스페인어 번역 시작...")
	spanishSRT := translateToLanguage(apiKey, finalKoreanSRT, "Español", "스페인어")
	if spanishSRT != "" {
		savePath := filepath.Join(subtitlePath, baseFileName+"_스페인어.srt")
		os.WriteFile(savePath, []byte(spanishSRT), 0644)
		fmt.Println("✅ 스페인어 번역 완료 및 저장")
	}

	// 포르투갈어
	fmt.Println("\n⏳ [2-3] 포르투갈어 번역 시작...")
	portugueseSRT := translateToLanguage(apiKey, finalKoreanSRT, "Português", "포르투갈어")
	if portugueseSRT != "" {
		savePath := filepath.Join(subtitlePath, baseFileName+"_포르투갈어.srt")
		os.WriteFile(savePath, []byte(portugueseSRT), 0644)
		fmt.Println("✅ 포르투갈어 번역 완료 및 저장")
	}

	// 일본어
	fmt.Println("\n⏳ [2-4] 일본어 번역 시작...")
	japaneseSRT := translateToLanguage(apiKey, finalKoreanSRT, "日本語", "일본어")
	if japaneseSRT != "" {
		savePath := filepath.Join(subtitlePath, baseFileName+"_일본어.srt")
		os.WriteFile(savePath, []byte(japaneseSRT), 0644)
		fmt.Println("✅ 일본어 번역 완료 및 저장")
	}

	// 중국어
	fmt.Println("\n⏳ [2-5] 중국어 번역 시작...")
	chineseSRT := translateToLanguage(apiKey, finalKoreanSRT, "中文 (简体)", "중국어")
	if chineseSRT != "" {
		savePath := filepath.Join(subtitlePath, baseFileName+"_중국어.srt")
		os.WriteFile(savePath, []byte(chineseSRT), 0644)
		fmt.Println("✅ 중국어 번역 완료 및 저장")
	}

	// 프랑스어
	fmt.Println("\n⏳ [2-6] 프랑스어 번역 시작...")
	frenchSRT := translateToLanguage(apiKey, finalKoreanSRT, "Français", "프랑스어")
	if frenchSRT != "" {
		savePath := filepath.Join(subtitlePath, baseFileName+"_프랑스어.srt")
		os.WriteFile(savePath, []byte(frenchSRT), 0644)
		fmt.Println("✅ 프랑스어 번역 완료 및 저장")
	}

	// 독일어
	fmt.Println("\n⏳ [2-7] 독일어 번역 시작...")
	germanSRT := translateToLanguage(apiKey, finalKoreanSRT, "Deutsch", "독일어")
	if germanSRT != "" {
		savePath := filepath.Join(subtitlePath, baseFileName+"_독일어.srt")
		os.WriteFile(savePath, []byte(germanSRT), 0644)
		fmt.Println("✅ 독일어 번역 완료 및 저장")
	}

	// 이탈리아어
	fmt.Println("\n⏳ [2-8] 이탈리아어 번역 시작...")
	italianSRT := translateToLanguage(apiKey, finalKoreanSRT, "Italiano", "이탈리아어")
	if italianSRT != "" {
		savePath := filepath.Join(subtitlePath, baseFileName+"_이탈리아어.srt")
		os.WriteFile(savePath, []byte(italianSRT), 0644)
		fmt.Println("✅ 이탈리아어 번역 완료 및 저장")
	}

	// 러시아어
	fmt.Println("\n⏳ [2-9] 러시아어 번역 시작...")
	russianSRT := translateToLanguage(apiKey, finalKoreanSRT, "Русский", "러시아어")
	if russianSRT != "" {
		savePath := filepath.Join(subtitlePath, baseFileName+"_러시아어.srt")
		os.WriteFile(savePath, []byte(russianSRT), 0644)
		fmt.Println("✅ 러시아어 번역 완료 및 저장")
	}

	// 베트남어
	fmt.Println("\n⏳ [2-10] 베트남어 번역 시작...")
	vietnameseSRT := translateToLanguage(apiKey, finalKoreanSRT, "Tiếng Việt", "베트남어")
	if vietnameseSRT != "" {
		savePath := filepath.Join(subtitlePath, baseFileName+"_베트남어.srt")
		os.WriteFile(savePath, []byte(vietnameseSRT), 0644)
		fmt.Println("✅ 베트남어 번역 완료 및 저장")
	}

	fmt.Println("\n✅ STEP 2 완료: 10개 언어 번역 완료")

	// =====================================================================
	// [STEP 3] 유튜브 제목/설명 생성
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
	fmt.Print("\n승인하시겠습니까? (엔터=승인 / '수정'=직접입력): ")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	approval := strings.TrimSpace(scanner.Text())

	if approval == "수정" {
		fmt.Print("새 제목 입력: ")
		scanner.Scan()
		koreanMeta.Title = scanner.Text()

		fmt.Print("새 설명 입력: ")
		scanner.Scan()
		koreanMeta.Description = scanner.Text()
	}

	// 한국어 제목/설명 저장
	koreanMetaPath := filepath.Join(metadataPath, baseFileName+"_한국어.txt")
	metaContent := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", koreanMeta.Title, koreanMeta.Description)
	os.WriteFile(koreanMetaPath, []byte(metaContent), 0644)
	fmt.Println("✅ 한국어 제목/설명 저장 완료")

	fmt.Println("\n⏳ 나머지 10개 언어 제목/설명 번역 시작...")

	// 영어
	fmt.Println("\n[3-1] 영어 제목/설명 번역 중...")
	engMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "English", "영어")
	if engMeta.Title != "" {
		metaPath := filepath.Join(metadataPath, baseFileName+"_영어.txt")
		content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", engMeta.Title, engMeta.Description)
		os.WriteFile(metaPath, []byte(content), 0644)
		fmt.Println("✅ 영어 제목/설명 저장 완료")
	}

	// 스페인어
	fmt.Println("\n[3-2] 스페인어 제목/설명 번역 중...")
	spaMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "Español", "스페인어")
	if spaMeta.Title != "" {
		metaPath := filepath.Join(metadataPath, baseFileName+"_스페인어.txt")
		content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", spaMeta.Title, spaMeta.Description)
		os.WriteFile(metaPath, []byte(content), 0644)
		fmt.Println("✅ 스페인어 제목/설명 저장 완료")
	}

	// 포르투갈어
	fmt.Println("\n[3-3] 포르투갈어 제목/설명 번역 중...")
	porMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "Português", "포르투갈어")
	if porMeta.Title != "" {
		metaPath := filepath.Join(metadataPath, baseFileName+"_포르투갈어.txt")
		content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", porMeta.Title, porMeta.Description)
		os.WriteFile(metaPath, []byte(content), 0644)
		fmt.Println("✅ 포르투갈어 제목/설명 저장 완료")
	}

	// 일본어
	fmt.Println("\n[3-4] 일본어 제목/설명 번역 중...")
	jpnMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "日本語", "일본어")
	if jpnMeta.Title != "" {
		metaPath := filepath.Join(metadataPath, baseFileName+"_일본어.txt")
		content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", jpnMeta.Title, jpnMeta.Description)
		os.WriteFile(metaPath, []byte(content), 0644)
		fmt.Println("✅ 일본어 제목/설명 저장 완료")
	}

	// 중국어
	fmt.Println("\n[3-5] 중국어 제목/설명 번역 중...")
	chnMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "中文 (简体)", "중국어")
	if chnMeta.Title != "" {
		metaPath := filepath.Join(metadataPath, baseFileName+"_중국어.txt")
		content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", chnMeta.Title, chnMeta.Description)
		os.WriteFile(metaPath, []byte(content), 0644)
		fmt.Println("✅ 중국어 제목/설명 저장 완료")
	}

	// 프랑스어
	fmt.Println("\n[3-6] 프랑스어 제목/설명 번역 중...")
	frMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "Français", "프랑스어")
	if frMeta.Title != "" {
		metaPath := filepath.Join(metadataPath, baseFileName+"_프랑스어.txt")
		content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", frMeta.Title, frMeta.Description)
		os.WriteFile(metaPath, []byte(content), 0644)
		fmt.Println("✅ 프랑스어 제목/설명 저장 완료")
	}

	// 독일어
	fmt.Println("\n[3-7] 독일어 제목/설명 번역 중...")
	deMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "Deutsch", "독일어")
	if deMeta.Title != "" {
		metaPath := filepath.Join(metadataPath, baseFileName+"_독일어.txt")
		content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", deMeta.Title, deMeta.Description)
		os.WriteFile(metaPath, []byte(content), 0644)
		fmt.Println("✅ 독일어 제목/설명 저장 완료")
	}

	// 이탈리아어
	fmt.Println("\n[3-8] 이탈리아어 제목/설명 번역 중...")
	itMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "Italiano", "이탈리아어")
	if itMeta.Title != "" {
		metaPath := filepath.Join(metadataPath, baseFileName+"_이탈리아어.txt")
		content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", itMeta.Title, itMeta.Description)
		os.WriteFile(metaPath, []byte(content), 0644)
		fmt.Println("✅ 이탈리아어 제목/설명 저장 완료")
	}

	// 러시아어
	fmt.Println("\n[3-9] 러시아어 제목/설명 번역 중...")
	ruMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "Русский", "러시아어")
	if ruMeta.Title != "" {
		metaPath := filepath.Join(metadataPath, baseFileName+"_러시아어.txt")
		content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", ruMeta.Title, ruMeta.Description)
		os.WriteFile(metaPath, []byte(content), 0644)
		fmt.Println("✅ 러시아어 제목/설명 저장 완료")
	}

	// 베트남어
	fmt.Println("\n[3-10] 베트남어 제목/설명 번역 중...")
	viMeta := translateMetadata(apiKey, koreanMeta.Title, koreanMeta.Description, "Tiếng Việt", "베트남어")
	if viMeta.Title != "" {
		metaPath := filepath.Join(metadataPath, baseFileName+"_베트남어.txt")
		content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", viMeta.Title, viMeta.Description)
		os.WriteFile(metaPath, []byte(content), 0644)
		fmt.Println("✅ 베트남어 제목/설명 저장 완료")
	}

	// 토큰 사용량 및 비용 계산
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

// 숫자 포맷팅 함수 (천 단위 콤마)
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
