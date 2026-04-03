package main

import (
	"strings"
)

// NumberConverter handles Korean number to Arabic number conversion
type NumberConverter struct {
	replacements map[string]string
}

// NewNumberConverter creates a new number converter
func NewNumberConverter() *NumberConverter {
	return &NumberConverter{
		replacements: map[string]string{
			"원":   "1",
			"투":   "2",
			"쓰리":  "3",
			"포":   "4",
			"파이브": "5",
			"파":   "5",
			"퐈":   "5",
			"식스":  "6",
			"식":   "6",
			"세븐":  "7",
			"에잇":  "8",
			"나인":  "9",
			"텐":   "10",
			"앤":   "&",
		},
	}
}

// ConvertNumbers converts Korean numbers to Arabic numbers in the text
func (nc *NumberConverter) ConvertNumbers(text string) string {
	// WHITELIST APPROACH: Only convert numbers that are standalone or followed by allowed particles
	// This prevents "익스텐션" → "익스10션" by only converting "텐" when it's a standalone word
	
	// Define allowed Korean particles that can follow numbers
	allowedParticles := []string{
		"에", "에서", "랑", "와", "과", "로", "으로", "까지", "부터", "만",
		"도", "은", "는", "이", "가", "을", "를", "의", "한테", "께",
		",", ".", "!", "?", // punctuation
	}
	
	// Sort by length (longest first) to avoid partial replacements
	orderedKeys := []string{
		"파이브", "식스", "쓰리", "세븐", "에잇", "나인",
		"원", "투", "포", "파", "퐈", "식", "텐", "앤",
	}
	
	// Split text into words (tokens)
	words := strings.Fields(text)
	
	for i, word := range words {
		// Try each number pattern (longest first)
		for _, korean := range orderedKeys {
			arabic, exists := nc.replacements[korean]
			if !exists {
				continue
			}
			
			// Case 1: Word is EXACTLY the number (standalone)
			// Example: "포" → "4"
			if word == korean {
				words[i] = arabic
				break
			}
			
			// Case 2: Word is [number] + [particle] + [optional punctuation]
			// Example: "포에서," → "4에서," or "포!" → "4!"
			if strings.HasPrefix(word, korean) && len(word) > len(korean) {
				remainder := word[len(korean):]
				
				// Strip trailing punctuation first to check particle
				punctuation := ""
				cleanRemainder := remainder
				for len(cleanRemainder) > 0 {
					lastChar := cleanRemainder[len(cleanRemainder)-1:]
					if lastChar == "," || lastChar == "." || lastChar == "!" || lastChar == "?" {
						punctuation = lastChar + punctuation
						cleanRemainder = cleanRemainder[:len(cleanRemainder)-1]
					} else {
						break
					}
				}
				
				// Check if cleanRemainder is an allowed particle (or empty for punctuation-only)
				isAllowedParticle := false
				
				// Allow if remainder is ONLY punctuation (e.g., "포!" → "4!")
				if cleanRemainder == "" && punctuation != "" {
					isAllowedParticle = true
				} else {
					// Check if cleanRemainder EXACTLY matches an allowed particle
					// CRITICAL: Must be exact match to prevent "에이스" (starts with "에") from passing
					for _, particle := range allowedParticles {
						if cleanRemainder == particle {
							isAllowedParticle = true
							break
						}
					}
				}
				
				// Only convert if followed by allowed particle (or punctuation only)
				if isAllowedParticle {
					words[i] = arabic + remainder
					break
				}
				// Otherwise SKIP (e.g., "포인트" has "인트" which is not allowed)
			}
		}
	}
	
	return strings.Join(words, " ")
}

// ConvertNumbersInSRT converts numbers in all SRT entries
func (nc *NumberConverter) ConvertNumbersInSRT(entries []SRTEntry) []SRTEntry {
	converted := make([]SRTEntry, len(entries))
	for i, entry := range entries {
		converted[i] = entry
		converted[i].Text = nc.ConvertNumbers(entry.Text)
	}
	return converted
}

// ConvertNumbersInContent converts numbers in raw SRT content
func (nc *NumberConverter) ConvertNumbersInContent(content string) string {
	entries, err := ParseSRT(content)
	if err != nil {
		// If parsing fails, just do simple replacement
		return nc.ConvertNumbers(content)
	}

	converted := nc.ConvertNumbersInSRT(entries)
	return FormatSRT(converted)
}

// SpellingCorrector handles automatic spelling corrections
type SpellingCorrector struct {
	corrections map[string]string
}

// NewSpellingCorrector creates a new spelling corrector
func NewSpellingCorrector() *SpellingCorrector {
	return &SpellingCorrector{
		corrections: map[string]string{
			// 맞춤법 오류
			"되요":    "돼요",
			"안되":    "안 돼",
			"안돼":    "안 돼",
			"되는":    "되는",
			"되지":    "되지",
			"되고":    "되고",
			"되면":    "되면",
			"되서":    "돼서",
			"되게":    "되게",
			"되니":    "되니",
			"되어":    "돼",
			"되어요":   "돼요",
			"되었":    "됐",
			"되었어요":  "됐어요",
			"되었습니다": "됐습니다",

			// 기타 흔한 오류
			"웬지":   "왠지",
			"왠만하면": "웬만하면",
			"틀리다":  "다르다", // 문맥에 따라 다를 수 있음
		},
	}
}

// CorrectSpelling applies automatic spelling corrections
func (sc *SpellingCorrector) CorrectSpelling(text string) (string, []string) {
	result := text
	applied := []string{}

	for wrong, correct := range sc.corrections {
		if strings.Contains(result, wrong) {
			result = strings.ReplaceAll(result, wrong, correct)
			applied = append(applied, wrong+" → "+correct)
		}
	}

	return result, applied
}

// CorrectSpellingInSRT applies spelling corrections to SRT entries
func (sc *SpellingCorrector) CorrectSpellingInSRT(entries []SRTEntry) ([]SRTEntry, map[string]int) {
	corrected := make([]SRTEntry, len(entries))
	stats := make(map[string]int)

	for i, entry := range entries {
		corrected[i] = entry
		newText, applied := sc.CorrectSpelling(entry.Text)
		corrected[i].Text = newText

		for _, correction := range applied {
			stats[correction]++
		}
	}

	return corrected, stats
}
