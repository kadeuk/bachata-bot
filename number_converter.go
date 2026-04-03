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
	// STRATEGY: Only convert numbers when they appear in SEQUENCE (2+ consecutive numbers)
	// This prevents "어떤 식으로" → "어떤 6으로" while allowing "파 식 세븐 에잇" → "5 6 7 8"
	
	// Define allowed Korean particles that can follow numbers
	allowedParticles := []string{
		"에", "에서", "랑", "와", "과", "로", "으로", "까지", "부터", "만",
		"도", "은", "는", "이", "가", "을", "를", "의", "한테", "께",
		",", ".", "!", "?", // punctuation
	}
	
	// Sort by length (longest first) to avoid partial replacements
	orderedKeys := []string{
		"파이브", "식스", "쓰리", "세븐", "에잇", "나인",
		"원", "투", "포", "파", "퐈", "식", "텐",
	}
	
	// Split text into words (tokens)
	words := strings.Fields(text)
	
	// STEP 1: Mark which words are numbers (or number+particle)
	isNumber := make([]bool, len(words))
	for i, word := range words {
		for _, korean := range orderedKeys {
			if word == korean {
				isNumber[i] = true
				break
			}
			
			// Check if word is number + particle
			if strings.HasPrefix(word, korean) && len(word) > len(korean) {
				remainder := word[len(korean):]
				
				// Strip punctuation
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
				
				// Check if it's a valid number+particle combination
				if cleanRemainder == "" && punctuation != "" {
					isNumber[i] = true
					break
				}
				for _, particle := range allowedParticles {
					if cleanRemainder == particle {
						isNumber[i] = true
						break
					}
				}
				if isNumber[i] {
					break
				}
			}
		}
	}
	
	// STEP 2: Find sequences of 2+ consecutive numbers
	sequences := []struct{ start, end int }{}
	i := 0
	for i < len(words) {
		if isNumber[i] {
			start := i
			for i < len(words) && isNumber[i] {
				i++
			}
			end := i
			// Only mark as sequence if 2+ numbers
			if end-start >= 2 {
				sequences = append(sequences, struct{ start, end int }{start, end})
			}
		} else {
			i++
		}
	}
	
	// STEP 3: Convert only numbers in sequences
	inSequence := make([]bool, len(words))
	for _, seq := range sequences {
		for i := seq.start; i < seq.end; i++ {
			inSequence[i] = true
		}
	}
	
	// STEP 4: Apply conversion only to numbers in sequences
	for i, word := range words {
		if !inSequence[i] {
			continue
		}
		
		for _, korean := range orderedKeys {
			arabic, exists := nc.replacements[korean]
			if !exists {
				continue
			}
			
			if word == korean {
				words[i] = arabic
				break
			}
			
			if strings.HasPrefix(word, korean) && len(word) > len(korean) {
				remainder := word[len(korean):]
				
				// Strip punctuation
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
				
				isAllowedParticle := false
				if cleanRemainder == "" && punctuation != "" {
					isAllowedParticle = true
				} else {
					for _, particle := range allowedParticles {
						if cleanRemainder == particle {
							isAllowedParticle = true
							break
						}
					}
				}
				
				if isAllowedParticle {
					words[i] = arabic + remainder
					break
				}
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
