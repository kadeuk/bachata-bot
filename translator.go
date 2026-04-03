package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/google/generative-ai-go/genai"
)

// Translator handles translation operations
type Translator struct {
	gemini            *GeminiClient
	techniques        *TechniqueManager
	glossary          *GlossaryManager
	chunkSize         int
	parallelProcessor *ParallelProcessor
}

// NewTranslator creates a new translator
func NewTranslator(gemini *GeminiClient, techniques *TechniqueManager, glossary *GlossaryManager) *Translator {
	return &Translator{
		gemini:            gemini,
		techniques:        techniques,
		glossary:          glossary,
		chunkSize:         60,                      // Process 60 entries at a time
		parallelProcessor: NewParallelProcessor(3), // 최대 3개 청크 동시 처리
	}
}

// CorrectKoreanSRT corrects Korean STT errors
func (t *Translator) CorrectKoreanSRT(content string, progressCallback func(current, total int)) (string, error) {
	log.Println("📝 한국어 자막 교정 시작... (Gemini 1.5 Flash 사용)")

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

	// Use Gemini 2.5 Pro (Flash not available in v1beta)
	flashModel := t.gemini.client.GenerativeModel("gemini-2.5-pro")
	flashModel.SetTemperature(0.2)
	flashModel.SetTopK(40)
	flashModel.SetTopP(0.95)
	flashModel.SetMaxOutputTokens(8192)
	flashModel.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(systemPrompt)},
	}

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
			// Use Flash model instead of default Pro model
			resp, err := flashModel.GenerateContent(t.gemini.ctx, genai.Text(userPrompt))
			if err != nil {
				lastErr = fmt.Errorf("API call failed: %v", err)
				log.Printf("⚠️ 청크 %d 처리 실패 (시도 %d/%d): %v", index+1, attempt, maxRetries, err)
				continue
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

// TranslateToLanguage translates SRT to target language using gemini-2.5-flash
func (t *Translator) TranslateToLanguage(koreanSRT, targetLang, langName string, progressCallback func(current, total int)) (string, error) {
	log.Printf("🌍 %s 번역 시작... (Gemini 2.5 Flash 사용)", langName)

	// Parse SRT
	entries, err := ParseSRT(koreanSRT)
	if err != nil {
		return "", fmt.Errorf("failed to parse SRT: %v", err)
	}

	totalEntries := len(entries)
	log.Printf("   총 %d개 자막 항목", totalEntries)

	// Use Gemini 2.5 Flash for translation (high quality, optimized)
	flashModel := t.gemini.client.GenerativeModel("gemini-2.5-flash")
	flashModel.SetTemperature(0.2)
	flashModel.SetTopK(40)
	flashModel.SetTopP(0.95)
	flashModel.SetMaxOutputTokens(8192)

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
		
		flashModel.SystemInstruction = &genai.Content{
			Parts: []genai.Part{genai.Text(systemPrompt)},
		}
		
		userPrompt := fmt.Sprintf("아래 한국어 SRT 자막을 번역하세요. **중요: 모든 자막 항목을 빠짐없이 출력하세요**\n\n%s", chunkText)

		// Retry logic with validation
		maxRetries := 3
		var translatedEntries []SRTEntry
		var lastErr error

		for attempt := 1; attempt <= maxRetries; attempt++ {
			resp, err := flashModel.GenerateContent(t.gemini.ctx, genai.Text(userPrompt))
			if err != nil {
				lastErr = fmt.Errorf("API call failed: %v", err)
				log.Printf("⚠️ 청크 %d 번역 실패 (시도 %d/%d): %v", index+1, attempt, maxRetries, err)
				continue
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
				lastErr = fmt.Errorf("parse failed: %v", err)
				log.Printf("⚠️ 청크 %d 파싱 실패 (시도 %d/%d): %v", index+1, attempt, maxRetries, err)
				continue
			}

			// Validate timecodes and entry count
			if err := ValidateTimecodes(chunk, translatedEntries); err != nil {
				lastErr = err
				log.Printf("⚠️ 청크 %d 검증 실패 (시도 %d/%d): %v", index+1, attempt, maxRetries, err)

				// If this is the last attempt, use original chunk
				if attempt == maxRetries {
					log.Printf("⚠️ 청크 %d: 최대 재시도 횟수 초과, 원본 사용", index+1)
					translatedEntries = chunk
					break
				}
				continue
			}

			// Success!
			break
		}

		if lastErr != nil && len(translatedEntries) == 0 {
			log.Printf("❌ 모든 재시도 실패, 원본 사용")
			translatedEntries = chunk
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

	response, err := t.gemini.GenerateContentWithSystemPrompt(systemPrompt, userPrompt)
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
		response, err := t.gemini.GenerateContent(prompt)
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
