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
			"원": "1",
			"투": "2",
			"쓰리": "3",
			"포": "4",
			"파이브": "5",
			"파": "5",
			"퐈": "5",
			"식스": "6",
			"식": "6",
			"세븐": "7",
			"에잇": "8",
			"나인": "9",
			"텐": "10",
			"앤": "&",
		},
	}
}

// ConvertNumbers converts Korean numbers to Arabic numbers in the text
func (nc *NumberConverter) ConvertNumbers(text string) string {
	result := text
	
	// Sort by length (longest first) to avoid partial replacements
	// e.g., "파이브" should be replaced before "파"
	orderedKeys := []string{
		"파이브", "식스", "쓰리", "세븐", "에잇", "나인",
		"원", "투", "포", "파", "퐈", "식", "텐", "앤",
	}
	
	for _, korean := range orderedKeys {
		if arabic, exists := nc.replacements[korean]; exists {
			// Use word boundary to avoid replacing parts of other words
			// But allow it to work with particles like "에", "에서", etc.
			result = strings.ReplaceAll(result, korean, arabic)
		}
	}
	
	return result
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
			"되요": "돼요",
			"안되": "안 돼",
			"안돼": "안 돼",
			"되는": "되는",
			"되지": "되지",
			"되고": "되고",
			"되면": "되면",
			"되서": "돼서",
			"되게": "되게",
			"되니": "되니",
			"되어": "돼",
			"되어요": "돼요",
			"되었": "됐",
			"되었어요": "됐어요",
			"되었습니다": "됐습니다",
			
			// 기타 흔한 오류
			"웬지": "왠지",
			"왠만하면": "웬만하면",
			"틀리다": "다르다", // 문맥에 따라 다를 수 있음
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
