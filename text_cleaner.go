package main

import (
	"regexp"
	"strings"
)

// TextCleaner handles subtitle text cleaning and formatting
type TextCleaner struct{}

// NewTextCleaner creates a new text cleaner
func NewTextCleaner() *TextCleaner {
	return &TextCleaner{}
}

// CleanSubtitleText removes unnecessary characters and improves readability
func (tc *TextCleaner) CleanSubtitleText(text string) string {
	// Remove leading numbers with dots (e.g., "1. ", "2. ")
	re1 := regexp.MustCompile(`(?m)^\d+\.\s*`)
	text = re1.ReplaceAllString(text, "")
	
	// Remove duplicate commas
	re2 := regexp.MustCompile(`,\s*,+`)
	text = re2.ReplaceAllString(text, ",")
	
	// Remove trailing commas before newlines
	re3 := regexp.MustCompile(`,\s*$`)
	text = re3.ReplaceAllString(text, "")
	
	// Remove excessive spaces (2 or more spaces)
	re4 := regexp.MustCompile(`\s{2,}`)
	text = re4.ReplaceAllString(text, " ")
	
	// Remove spaces before punctuation
	re5 := regexp.MustCompile(`\s+([,.!?])`)
	text = re5.ReplaceAllString(text, "$1")
	
	// Clean up lines
	lines := strings.Split(text, "\n")
	var cleanedLines []string
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanedLines = append(cleanedLines, line)
		}
	}
	
	return strings.Join(cleanedLines, "\n")
}

// CleanSRTContent cleans the entire SRT file while preserving structure
func (tc *TextCleaner) CleanSRTContent(srtContent string) string {
	lines := strings.Split(srtContent, "\n")
	var result []string
	
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		
		// Check if this is a subtitle text line (not number, not timecode, not empty)
		if line != "" && !isTimecode(line) && !isNumber(line) {
			// Clean the subtitle text
			cleaned := tc.CleanSubtitleText(line)
			// Convert all spoken numbers to Arabic numerals
			cleaned = tc.ConvertSpokenNumbersToArabic(cleaned)
			result = append(result, cleaned)
		} else {
			// Keep structure lines as-is
			result = append(result, line)
		}
	}
	
	return strings.Join(result, "\n")
}

// ConvertSpokenNumbersToArabic converts all spoken Korean/English numbers to Arabic numerals
func (tc *TextCleaner) ConvertSpokenNumbersToArabic(text string) string {
	// Korean number words (원, 투, 쓰리, etc.)
	koreanNumbers := map[string]string{
		"원":   "1",
		"투":   "2",
		"쓰리":  "3",
		"포":   "4",
		"파이브": "5",
		"식스":  "6",
		"세븐":  "7",
		"에잇":  "8",
	}
	
	// English number words
	englishNumbers := map[string]string{
		"원":    "1",
		"투":    "2",
		"쓰리":   "3",
		"포":    "4",
		"파이브":  "5",
		"파":    "5", // Alternative pronunciation
		"식스":   "6",
		"식":    "6", // Alternative pronunciation
		"세븐":   "7",
		"에잇":   "8",
		"나인":   "9",
		"텐":    "10",
		"one":   "1",
		"two":   "2",
		"three": "3",
		"four":  "4",
		"five":  "5",
		"six":   "6",
		"seven": "7",
		"eight": "8",
		"nine":  "9",
		"ten":   "10",
	}
	
	// Combine all number mappings
	allNumbers := make(map[string]string)
	for k, v := range koreanNumbers {
		allNumbers[k] = v
	}
	for k, v := range englishNumbers {
		allNumbers[k] = v
	}
	
	// Replace each number word with its Arabic numeral
	// Use word boundaries to avoid partial matches
	for word, num := range allNumbers {
		// Case-insensitive replacement with word boundaries
		re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(word) + `\b`)
		text = re.ReplaceAllString(text, num)
	}
	
	// Remove commas between numbers (e.g., "1, 2, 3" -> "1 2 3")
	re := regexp.MustCompile(`(\d)\s*,\s*(\d)`)
	text = re.ReplaceAllString(text, "$1 $2")
	
	// Remove trailing commas after numbers (e.g., "1," -> "1")
	// Go regex doesn't support lookahead, so use a simpler approach
	re2 := regexp.MustCompile(`(\d)\s*,\s*`)
	text = re2.ReplaceAllStringFunc(text, func(match string) string {
		// Check if followed by a digit
		if regexp.MustCompile(`\d\s*,\s*\d`).MatchString(match) {
			return match // Keep if between digits (already handled above)
		}
		// Remove comma if not between digits
		return regexp.MustCompile(`\d`).FindString(match) + " "
	})
	
	// Clean up multiple spaces
	re3 := regexp.MustCompile(`\s{2,}`)
	text = re3.ReplaceAllString(text, " ")
	
	return text
}

// isTimecode checks if a line is a timecode
func isTimecode(line string) bool {
	// Match format: 00:00:00,000 --> 00:00:00,000
	matched, _ := regexp.MatchString(`^\d{2}:\d{2}:\d{2},\d{3}\s*-->\s*\d{2}:\d{2}:\d{2},\d{3}$`, line)
	return matched
}

// isNumber checks if a line is just a number (subtitle index)
func isNumber(line string) bool {
	matched, _ := regexp.MatchString(`^\d+$`, strings.TrimSpace(line))
	return matched
}

// NormalizeSpacing ensures consistent spacing in subtitle text
func (tc *TextCleaner) NormalizeSpacing(text string) string {
	// Ensure single space after punctuation
	re := regexp.MustCompile(`([,.!?])([^\s])`)
	text = re.ReplaceAllString(text, "$1 $2")
	
	// Remove space before punctuation
	re2 := regexp.MustCompile(`\s+([,.!?])`)
	text = re2.ReplaceAllString(text, "$1")
	
	return text
}
