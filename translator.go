package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// Translator handles translation operations
type Translator struct {
	aiClient          AIClient
	techniques        *TechniqueManager
	glossary          *GlossaryManager
	chunkSize         int
	parallelProcessor *ParallelProcessor
	absoluteRules     *AbsoluteTranslationRules
}

// AbsoluteTranslationRules represents the dance translation rules
type AbsoluteTranslationRules struct {
	SystemRules                 []string          `json:"system_rules"`
	Glossary                    map[string]string `json:"glossary"`
	ExcellentTranslationExamples []struct {
		Context string `json:"context"`
		Korean  string `json:"korean"`
		English string `json:"english"`
	} `json:"excellent_translation_examples"`
}

// NewTranslator creates a new translator
func NewTranslator(aiClient AIClient, techniques *TechniqueManager, glossary *GlossaryManager) *Translator {
	translator := &Translator{
		aiClient:          aiClient,
		techniques:        techniques,
		glossary:          glossary,
		chunkSize:         40,                      // Process 40 entries at a time (142번 문제 해결)
		parallelProcessor: NewParallelProcessor(3), // 최대 3개 청크 동시 처리
	}

	// Load absolute translation rules from JSON file
	if err := translator.loadAbsoluteTranslationRules(); err != nil {
		log.Printf("⚠️ 절대 번역 규칙 파일 로드 실패: %v", err)
	} else {
		log.Println("✅ 절대 번역 규칙 로드 완료")
	}

	return translator
}

// loadAbsoluteTranslationRules loads dance translation rules from JSON file
func (t *Translator) loadAbsoluteTranslationRules() error {
	data, err := os.ReadFile("절대 번역 규칙.json")
	if err != nil {
		return fmt.Errorf("failed to read rules file: %v", err)
	}

	var rules AbsoluteTranslationRules
	if err := json.Unmarshal(data, &rules); err != nil {
		return fmt.Errorf("failed to parse rules JSON: %v", err)
	}

	t.absoluteRules = &rules
	return nil
}

// CorrectKoreanSRT corrects Korean STT errors
func (t *Translator) CorrectKoreanSRT(content string, progressCallback func(current, total int)) (string, error) {
	log.Println("📝 한국어 자막 교정 시작... (DeepSeek 사용)")

	// Parse SRT
	entries, err := ParseSRT(content)
	if err != nil {
		return "", fmt.Errorf("failed to parse SRT: %v", err)
	}

	totalEntries := len(entries)
	log.Printf("   총 %d개 자막 항목 발견", totalEntries)

	// Filter relevant terms
	filteredTerms := t.techniques.GetFilteredTerms(content)
	systemPrompt := t.techniques.BuildCorrectionPrompt(filteredTerms)

	// Split into chunks
	chunks := ChunkSRT(entries, t.chunkSize)
	log.Printf("   %d개 청크로 분할 (청크당 %d개 항목)", len(chunks), t.chunkSize)

	// Process chunks in parallel
	mergedEntries, err := t.parallelProcessor.ProcessChunksParallel(chunks, func(chunk []SRTEntry, index int) ([]SRTEntry, error) {
		chunkText := GetChunkText(chunk)
		userPrompt := fmt.Sprintf("아래 SRT 자막을 교정하세요. **중요: 모든 자막 항목을 빠짐없이 출력하세요**\n\n%s", chunkText)

		// Retry logic with validation
		maxRetries := 3
		var correctedEntries []SRTEntry
		var lastErr error

		for attempt := 1; attempt <= maxRetries; attempt++ {
			// Use AIClient interface instead of Gemini SDK
			response, err := t.aiClient.GenerateContentWithSystemPrompt(systemPrompt, userPrompt)
			if err != nil {
				lastErr = fmt.Errorf("API call failed: %v", err)
				log.Printf("⚠️ 청크 %d 처리 실패 (시도 %d/%d): %v", index+1, attempt, maxRetries, err)
				continue
			}

			correctedEntries, err = ParseChunkResponse(response)
			if err != nil {
				lastErr = fmt.Errorf("parse failed: %v", err)
				log.Printf("⚠️ 청크 %d 파싱 실패 (시도 %d/%d): %v", index+1, attempt, maxRetries, err)
				continue
			}

			// Validate timecodes and entry count
			if err := ValidateTimecodes(chunk, correctedEntries); err != nil {
				lastErr = err
				log.Printf("⚠️ 청크 %d 검증 실패 (시도 %d/%d): %v", index+1, attempt, maxRetries, err)

				// If this is the last attempt, use original chunk
				if attempt == maxRetries {
					log.Printf("⚠️ 청크 %d: 최대 재시도 횟수 초과, 원본 사용", index+1)
					correctedEntries = chunk
					break
				}
				continue
			}

			// Success!
			break
		}

		if lastErr != nil && len(correctedEntries) == 0 {
			log.Printf("❌ 모든 재시도 실패, 원본 사용")
			correctedEntries = chunk
		}

		return correctedEntries, nil
	})

	if err != nil {
		return "", fmt.Errorf("parallel processing failed: %v", err)
	}

	result := FormatSRT(mergedEntries)

	// Fix SRT timecode format for YouTube compatibility
	result, err = FixSRTTimecodeFormat(result)
	if err != nil {
		log.Printf("⚠️ 타임코드 형식 수정 실패 (한국어): %v", err)
	}

	log.Printf("✅ 한국어 교정 완료: %d개 항목", len(mergedEntries))
	return result, nil
}

// TranslateToLanguage translates SRT to target language using DeepSeek
func (t *Translator) TranslateToLanguage(koreanSRT, targetLang, langName string, progressCallback func(current, total int)) (string, error) {
	log.Printf("🌍 %s 번역 시작... (DeepSeek 사용)", langName)

	// Parse SRT
	entries, err := ParseSRT(koreanSRT)
	if err != nil {
		return "", fmt.Errorf("failed to parse SRT: %v", err)
	}

	totalEntries := len(entries)
	log.Printf("   총 %d개 자막 항목", totalEntries)

	// Split into chunks
	chunks := ChunkSRT(entries, t.chunkSize)
	log.Printf("   %d개 청크로 분할", len(chunks))

	// Process chunks in parallel
	mergedEntries, err := t.parallelProcessor.ProcessChunksParallel(chunks, func(chunk []SRTEntry, index int) ([]SRTEntry, error) {
		chunkText := GetChunkText(chunk)
		
		// Build system prompt with mini glossary (token optimization)
		var systemPrompt string
		if t.glossary != nil {
			systemPrompt = t.glossary.BuildTranslationPromptWithGlossary(chunkText, targetLang, langName)
		} else {
			systemPrompt = t.techniques.BuildTranslationPrompt(targetLang, langName)
		}
		
		// ENHANCEMENT: Add expert dance translation rules
		systemPrompt += fmt.Sprintf(`
## 🎵 전문 라틴 댄스 (바차타/살사) 번역가 규칙 (100%% 준수 필수):

You are an expert translator specializing in Latin dance (Bachata/Salsa) instructional videos. 
Translate the provided Korean subtitles into %s while strictly following these rules:

### 1. Zero Omission (누락 절대 금지):
- **절대적으로 어떤 구체적인 동작, 신체 부위, 타이밍/카운트 마커도 누락하지 마세요.**
- 예: "7 사이드 웨이브" → 반드시 "7"과 "side wave" 모두 포함하여 번역
- 예: "오른쪽 어깨" → "right shoulder" (어깨 누락 금지)
- 예: "카운트 5에서" → "on count 5" (카운트 누락 금지)

### 2. Preserve Dance Terminology (댄스 용어 보존):
- **모든 스페인어/영어 댄스 전문 용어를 정확히 그대로 유지하세요. 일반적인 설명으로 과도하게 의역하지 마세요.**
- 예: "골빼" → "Golpe" (NOT "hip isolation")
- 예: "롬뽀 아델란떼" → "Rombo Adelante" (NOT "forward rhombus")
- 예: "볼레로" → "Voleo" (NOT "throw")
- **팁**: 필요시 명확성을 위해 괄호를 사용할 수 있습니다. 예: "Golpe (isolation)"

### 3. Natural Instructor Tone (자연스러운 강사 톤):
- **번역이 자연스럽고 대화식이며 교육적으로 들리도록 하세요. 마치 전문 댄스 강사가 학생들에게 직접 말하는 것처럼.**
- 딱딱하고 로봇 같은 단어 대 단어 번역을 피하면서 정확한 핵심 의미를 보존하세요.
- 예: "이렇게 해보세요" → "Try it like this" (NOT "Do like this")
- 예: "잘했어요!" → "Great job!" (NOT "Did well")

### 4. Maintain SRT Format (포맷 유지):
- **정확한 타임스탬프와 자막 번호를 엄격히 유지하세요. SRT 구조를 변경하지 마세요.**
- 번호, 타임코드, 텍스트, 빈 줄 형식 정확히 유지
- 타임코드 1글자도 변경 금지
- 자막 항목 수 정확히 일치

### 5. Quality Verification (품질 검증):
1. 원본의 모든 댄스 용어가 번역에 포함되었는지 확인
2. 고유명사가 의역되지 않았는지 확인  
3. 동작 이름이 누락되지 않았는지 확인
4. 기술 용어가 정확하게 번역되었는지 확인
5. 자연스러운 강사 톤이 유지되었는지 확인

**위반 시 실패: 용어 누락, 과도한 의역, 부자연스러운 톤은 허용되지 않습니다.**`, targetLang)
		
		userPrompt := fmt.Sprintf(`아래 한국어 SRT 자막을 번역하세요.

## 🚨 절대 규칙 (100%% 준수 필수, 위반 시 실패):
1. **항목 수 정확히 일치**: 입력된 항목 수(%d개)와 출력된 항목 수가 정확히 같아야 합니다
2. **모든 항목 빠짐없이 출력**: 1번부터 %d번까지 전부 출력하세요
3. **불완전한 항목 금지**: "143\n00" 같은 불완전한 항목 생성 금지
4. **SRT 형식 엄수**: 번호 → 타임코드 → 텍스트 → 빈 줄
5. **타임코드 절대 변경 금지**: 원본 타임코드를 1글자도 변경하지 마세요
6. **항목 순서 유지**: 1, 2, 3, ... 순서대로 출력

## ⚠️ 경고:
- 항목이 하나라도 빠지면 실패입니다
- 불완전한 항목("번호\n불완전한타임코드") 생성 금지
- 마지막 항목(%d번)까지 반드시 포함

## 📋 출력 형식:
- 순수 SRT 텍스트만 출력
- 마크다운 코드 블록 사용 금지
- 추가 설명 없이 SRT만 출력
- 항목 수: 정확히 %d개

## 📜 번역할 자막 내용 (총 %d개 항목):
%s`, len(chunk), len(chunk), len(chunk), len(chunk), len(chunk), chunkText)

		// 강제 완성 재시도 로직 (사용자 요구: "두번세번 100번을 해서라도")
		maxRetries := 5  // 3회에서 5회로 증가
		var translatedEntries []SRTEntry
		var lastErr error
		var lastResponse string

		for attempt := 1; attempt <= maxRetries; attempt++ {
			response, err := t.aiClient.GenerateContentWithSystemPrompt(systemPrompt, userPrompt)
			if err != nil {
				lastErr = fmt.Errorf("API 호출 실패: %v", err)
				log.Printf("⚠️ 청크 %d 번역 실패 (시도 %d/%d): %v", index+1, attempt, maxRetries, err)
				
				// 짧은 대기 후 재시도
				if attempt < maxRetries {
					time.Sleep(2 * time.Second)
				}
				continue
			}

			lastResponse = response

			// Extract new terms if glossary is available
			if t.glossary != nil {
				newTerms := t.glossary.ExtractNewTermsFromResponse(response)
				for korean, translated := range newTerms {
					if err := t.glossary.AddTranslationTerm(korean, translated); err != nil {
						log.Printf("⚠️ 번역 용어집 업데이트 실패: %v", err)
					}
				}
				// Remove markers from response
				response = t.glossary.RemoveNewTermsMarkers(response)
			}

			translatedEntries, err = ParseChunkResponse(response)
			if err != nil {
				lastErr = fmt.Errorf("파싱 실패: %v", err)
				log.Printf("⚠️ 청크 %d 파싱 실패 (시도 %d/%d): %v", index+1, attempt, maxRetries, err)
				
				// 응답이 너무 짧은 경우 (불완전한 응답)
				if len(response) < 100 && attempt < maxRetries {
					log.Printf("⚠️ 응답 너무 짧음 (%d자), 재시도", len(response))
					time.Sleep(2 * time.Second)
					continue
				}
				continue
			}

			// Validate timecodes and entry count - 엄격한 검증
			if err := ValidateTimecodes(chunk, translatedEntries); err != nil {
				lastErr = err
				log.Printf("⚠️ 청크 %d 검증 실패 (시도 %d/%d): %v", index+1, attempt, maxRetries, err)
				
				// 항목 수가 부족한 경우
				if len(translatedEntries) < len(chunk) {
					log.Printf("⚠️ 항목 수 부족: 예상=%d, 실제=%d", len(chunk), len(translatedEntries))
					
					// 마지막 시도가 아니면 재시도
					if attempt < maxRetries {
						log.Printf("⚠️ 항목 수 불일치, 재시도")
						time.Sleep(3 * time.Second)
						continue
					}
				}

				// If this is the last attempt, use original chunk
				if attempt == maxRetries {
					log.Printf("⚠️ 청크 %d: 최대 재시도 횟수 초과", index+1)
					
					// 원본 청크 사용하지만, 가능한 한 많은 항목 유지
					if len(translatedEntries) > 0 {
						log.Printf("⚠️ 부분적 결과 사용: %d/%d 항목", len(translatedEntries), len(chunk))
						// 부분적 결과라도 사용
					} else {
						log.Printf("⚠️ 원본 청크 사용")
						translatedEntries = chunk
					}
					break
				}
				continue
			}

			// 추가 검증: 모든 항목이 있는지 확인
			if len(translatedEntries) != len(chunk) {
				lastErr = fmt.Errorf("항목 수 불일치: 예상=%d, 실제=%d", len(chunk), len(translatedEntries))
				log.Printf("⚠️ %s", lastErr)
				
				if attempt < maxRetries {
					log.Printf("⚠️ 항목 수 불일치, 재시도")
					time.Sleep(3 * time.Second)
					continue
				}
			}

			// 성공!
			log.Printf("✅ 청크 %d 번역 성공 (시도 %d/%d): %d개 항목", 
				index+1, attempt, maxRetries, len(translatedEntries))
			break
		}

		if lastErr != nil && len(translatedEntries) == 0 {
			log.Printf("❌ 청크 %d: 모든 재시도 실패, 원본 사용", index+1)
			
			// 응답이 있으면 로깅
			if lastResponse != "" {
				log.Printf("⚠️ 마지막 응답 길이: %d자", len(lastResponse))
				if len(lastResponse) < 500 {
					log.Printf("⚠️ 마지막 응답 (처음 500자): %s", lastResponse[:min(500, len(lastResponse))])
				}
			}
			
			translatedEntries = chunk
		} else if len(translatedEntries) < len(chunk) {
			log.Printf("⚠️ 청크 %d: 부분적 결과 (%d/%d 항목)", 
				index+1, len(translatedEntries), len(chunk))
		}

		return translatedEntries, nil
	})

	if err != nil {
		return "", fmt.Errorf("parallel processing failed: %v", err)
	}

	result := FormatSRT(mergedEntries)

	// Fix SRT timecode format for YouTube compatibility
	result, err = FixSRTTimecodeFormat(result)
	if err != nil {
		log.Printf("⚠️ 타임코드 형식 수정 실패 (%s): %v", langName, err)
	}

	log.Printf("✅ %s 번역 완료: %d개 항목", langName, len(mergedEntries))
	return result, nil
}

// GenerateMetadata generates YouTube title and description
func (t *Translator) GenerateMetadata(koreanSRT string) (string, string, error) {
	log.Println("📺 유튜브 제목/설명 생성 중...")

	systemPrompt := t.techniques.BuildMetadataPrompt()
	userPrompt := fmt.Sprintf("아래 자막을 바탕으로 유튜브 제목과 설명을 생성하세요:\n\n%s", koreanSRT)

	response, err := t.aiClient.GenerateContentWithSystemPrompt(systemPrompt, userPrompt)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate metadata: %v", err)
	}

	// Clean JSON response
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	// Parse JSON
	var metadata struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}

	if err := json.Unmarshal([]byte(response), &metadata); err != nil {
		return "", "", fmt.Errorf("failed to parse metadata JSON: %v", err)
	}

	log.Println("✅ 제목/설명 생성 완료")
	return metadata.Title, metadata.Description, nil
}

// TranslateMetadata translates metadata to target language with retry logic
func (t *Translator) TranslateMetadata(title, description, targetLang, langName string) (string, string, error) {
	log.Printf("📺 %s 제목/설명 번역 중...", langName)

	prompt := fmt.Sprintf(`아래 유튜브 제목과 설명을 %s로 번역하세요.

**중요 요구사항:**
1. 반드시 순수한 JSON만 출력하세요. 설명 텍스트나 마크다운 코드 블록을 포함하지 마세요.
2. **제목(title)은 반드시 %s 기준 공백 포함 100자 미만으로 작성하세요**
3. **제목은 원본 내용과 관련성 높게 번역하세요**
4. **'바차타(Bachata)' 또는 '패턴(Pattern)' 키워드 중 하나 이상 자연스럽게 포함 (둘 다 필수 아님)**
5. **설명(description)은 반드시 %s 기준 정확히 600~800자 분량으로 작성하세요 (공백 포함, 절대 초과 금지)**
6. **설명에 가독성을 위해 줄바꿈(\\n\\n) 사용하여 문단 구분**
7. **번호 목록(1., 2., 3.)이 있으면 각 항목 사이에 줄바꿈 추가**
8. SEO 최적화를 위해 관련 키워드를 자연스럽게 포함하세요
9. 800자를 절대 초과하지 말고, 필요하면 핵심 내용만 간결하게 요약하세요

**출력 형식 (JSON만):**
{
  "title": "번역된 제목 (100자 미만, 내용 관련성 높게)",
  "description": "번역된 설명 (600~800자, 줄바꿈 포함)"
}

**한국어 원본:**
제목: %s
설명: %s`, targetLang, targetLang, targetLang, title, description)

	maxRetries := 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		response, err := t.aiClient.GenerateContent(prompt)
		if err != nil {
			lastErr = fmt.Errorf("API call failed: %v", err)
			log.Printf("⚠️ %s 메타데이터 번역 실패 (시도 %d/%d): %v", langName, attempt, maxRetries, err)
			continue
		}

		// Clean JSON response more aggressively
		response = strings.TrimSpace(response)
		
		// Remove markdown code blocks
		if strings.HasPrefix(response, "```json") {
			response = strings.TrimPrefix(response, "```json")
		} else if strings.HasPrefix(response, "```") {
			response = strings.TrimPrefix(response, "```")
		}
		if strings.HasSuffix(response, "```") {
			response = strings.TrimSuffix(response, "```")
		}
		
		response = strings.TrimSpace(response)
		
		// Find JSON object boundaries
		startIdx := strings.Index(response, "{")
		endIdx := strings.LastIndex(response, "}")
		
		if startIdx >= 0 && endIdx > startIdx {
			response = response[startIdx : endIdx+1]
		}

		// Parse JSON
		var metadata struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		}

		if err := json.Unmarshal([]byte(response), &metadata); err != nil {
			lastErr = fmt.Errorf("JSON parse failed: %v", err)
			log.Printf("⚠️ %s 메타데이터 JSON 파싱 실패 (시도 %d/%d): %v", langName, attempt, maxRetries, err)
			log.Printf("   응답 내용 (처음 200자): %s", response[:min(200, len(response))])
			
			if attempt == maxRetries {
				return "", "", fmt.Errorf("failed to parse translated metadata JSON after %d attempts: %v", maxRetries, lastErr)
			}
			continue
		}

		// Validate that we got non-empty results
		if metadata.Title == "" || metadata.Description == "" {
			lastErr = fmt.Errorf("empty title or description in response")
			log.Printf("⚠️ %s 메타데이터 빈 응답 (시도 %d/%d)", langName, attempt, maxRetries)
			continue
		}

		log.Printf("✅ %s 제목/설명 번역 완료", langName)
		return metadata.Title, metadata.Description, nil
	}

	return "", "", fmt.Errorf("failed to translate metadata after %d attempts: %v", maxRetries, lastErr)
}
