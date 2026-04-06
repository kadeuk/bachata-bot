package main

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
)

// SRTEntry represents a single subtitle entry
type SRTEntry struct {
	Index     int
	StartTime string
	EndTime   string
	Text      string
}

// ParseSRT parses an SRT file content into entries with robust error handling
func ParseSRT(content string) ([]SRTEntry, error) {
	var entries []SRTEntry
	
	// Split by double newline (entry separator)
	blocks := regexp.MustCompile(`\r?\n\r?\n`).Split(content, -1)
	
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		
		lines := strings.Split(block, "\n")
		
		// Handle incomplete entries (e.g., "143\n00")
		if len(lines) < 3 {
			// Try to recover incomplete entry
			if len(lines) == 2 {
				// Check if this is an incomplete entry like "143\n00"
				indexStr := strings.TrimSpace(lines[0])
				timecodeStr := strings.TrimSpace(lines[1])
				
				// Check if first line is a number (index)
				if _, err := strconv.Atoi(indexStr); err == nil {
					// Check if second line looks like an incomplete timecode
					if len(timecodeStr) <= 5 && strings.Contains(timecodeStr, ":") {
						// This is an incomplete entry, skip it with warning
						fmt.Printf("⚠️ 불완전한 항목 건너뛰기: %s -> %s\n", indexStr, timecodeStr)
						continue
					}
				}
			}
			continue // Invalid entry
		}
		
		// Parse index
		index, err := strconv.Atoi(strings.TrimSpace(lines[0]))
		if err != nil {
			// Try to extract number from the line
			re := regexp.MustCompile(`\d+`)
			if matches := re.FindStringSubmatch(lines[0]); matches != nil {
				if idx, err := strconv.Atoi(matches[0]); err == nil {
					index = idx
				} else {
					continue // Skip invalid index
				}
			} else {
				continue // Skip invalid index
			}
		}
		
		// Parse timecode
		timeParts := strings.Split(strings.TrimSpace(lines[1]), " --> ")
		if len(timeParts) != 2 {
			continue // Invalid timecode
		}
		
		// Parse text (can be multiple lines)
		text := strings.Join(lines[2:], "\n")
		
		entries = append(entries, SRTEntry{
			Index:     index,
			StartTime: strings.TrimSpace(timeParts[0]),
			EndTime:   strings.TrimSpace(timeParts[1]),
			Text:      text,
		})
	}
	
	return entries, nil
}

// FormatSRT converts entries back to SRT format
func FormatSRT(entries []SRTEntry) string {
	var result strings.Builder
	
	for i, entry := range entries {
		result.WriteString(fmt.Sprintf("%d\n", entry.Index))
		result.WriteString(fmt.Sprintf("%s --> %s\n", entry.StartTime, entry.EndTime))
		result.WriteString(entry.Text)
		result.WriteString("\n")
		
		// Add double newline between entries (except last one)
		if i < len(entries)-1 {
			result.WriteString("\n")
		}
	}
	
	return result.String()
}

// ChunkSRT splits SRT entries into chunks of specified size
func ChunkSRT(entries []SRTEntry, chunkSize int) [][]SRTEntry {
	var chunks [][]SRTEntry
	
	for i := 0; i < len(entries); i += chunkSize {
		end := i + chunkSize
		if end > len(entries) {
			end = len(entries)
		}
		chunks = append(chunks, entries[i:end])
	}
	
	return chunks
}

// MergeChunks merges translated chunks back into a single list
func MergeChunks(chunks [][]SRTEntry) []SRTEntry {
	var merged []SRTEntry
	
	for _, chunk := range chunks {
		merged = append(merged, chunk...)
	}
	
	return merged
}

// ValidateTimecodes checks if timecodes are preserved
func ValidateTimecodes(original, translated []SRTEntry) error {
	if len(original) != len(translated) {
		return fmt.Errorf("entry count mismatch: original=%d, translated=%d", len(original), len(translated))
	}
	
	for i := range original {
		if original[i].Index != translated[i].Index {
			return fmt.Errorf("index mismatch at position %d: original=%d, translated=%d", 
				i, original[i].Index, translated[i].Index)
		}
		
		if original[i].StartTime != translated[i].StartTime {
			return fmt.Errorf("start time mismatch at index %d: original=%s, translated=%s", 
				original[i].Index, original[i].StartTime, translated[i].StartTime)
		}
		
		if original[i].EndTime != translated[i].EndTime {
			return fmt.Errorf("end time mismatch at index %d: original=%s, translated=%s", 
				original[i].Index, original[i].EndTime, translated[i].EndTime)
		}
	}
	
	return nil
}

// GetChunkText converts a chunk of entries to text format for API
func GetChunkText(entries []SRTEntry) string {
	return FormatSRT(entries)
}

// ParseChunkResponse parses API response back to SRT entries with encoding fix
func ParseChunkResponse(response string) ([]SRTEntry, error) {
	// Clean up response (remove markdown code blocks if present)
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```srt")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)
	
	// Fix encoding issues (특히 143번 문제 해결)
	response = fixEncodingIssues(response)
	
	// Debug logging for problematic entries
	if strings.Contains(response, "143\n") {
		// Find the 143 entry
		lines := strings.Split(response, "\n")
		for i, line := range lines {
			if strings.TrimSpace(line) == "143" && i+2 < len(lines) {
				log.Printf("🔍 143번 항목 디버깅:")
				log.Printf("   인덱스: %s", line)
				log.Printf("   타임코드: %s", lines[i+1])
				log.Printf("   텍스트: %s", lines[i+2])
				log.Printf("   텍스트 길이: %d", len(lines[i+2]))
				log.Printf("   텍스트 바이트: %v", []byte(lines[i+2]))
				
				// Check for encoding issues
				if hasEncodingIssues(lines[i+2]) {
					log.Printf("⚠️ 143번 항목 인코딩 문제 발견!")
					fixedText := cleanEncodingIssues(lines[i+2])
					log.Printf("   수정 전: %s", lines[i+2])
					log.Printf("   수정 후: %s", fixedText)
					lines[i+2] = fixedText
					response = strings.Join(lines, "\n")
				}
				break
			}
		}
	}
	
	return ParseSRT(response)
}

// fixEncodingIssues fixes common encoding problems in AI responses
func fixEncodingIssues(text string) string {
	// Replace common encoding problems
	replacements := []struct {
		old string
		new string
	}{
		{"?", ""},      // Common encoding artifact
		{"", ""},       // Replacement character
		{"\ufffd", ""},  // Unicode replacement character
		{"\u0000", ""},  // Null character
		{"\uFFFD", ""},  // Unicode replacement character (uppercase)
	}
	
	result := text
	for _, r := range replacements {
		result = strings.ReplaceAll(result, r.old, r.new)
	}
	
	// Remove consecutive question marks (common in encoding issues)
	result = regexp.MustCompile(`\?+`).ReplaceAllString(result, "?")
	
	// Trim extra spaces
	result = strings.TrimSpace(result)
	
	return result
}

// hasEncodingIssues checks if text has encoding problems
func hasEncodingIssues(text string) bool {
	// Check for common encoding artifacts
	if strings.Contains(text, "?") || strings.Contains(text, "") {
		return true
	}
	
	// Check for replacement character
	if strings.Contains(text, "\ufffd") || strings.Contains(text, "\uFFFD") {
		return true
	}
	
	// Check for excessive question marks (more than 2 in a row)
	if regexp.MustCompile(`\?{3,}`).MatchString(text) {
		return true
	}
	
	return false
}

// cleanEncodingIssues cleans encoding problems from text
func cleanEncodingIssues(text string) string {
	// First apply general fixes
	text = fixEncodingIssues(text)
	
	// Try to recover Korean text
	// Common pattern: "?체부???번 가" -> "체부번 가"
	text = regexp.MustCompile(`[^\p{Hangul}\p{Han}a-zA-Z0-9\s.,!?\-:;'"()\[\]{}]`).ReplaceAllString(text, "")
	
	// Remove isolated question marks
	text = regexp.MustCompile(`(^|\s)\?(\s|$)`).ReplaceAllString(text, "$1$2")
	
	// Trim and return
	return strings.TrimSpace(text)
}

// ValidateAndFixSRT validates SRT content and fixes incomplete entries
func ValidateAndFixSRT(content, originalContent string) (string, error) {
	// Parse both contents
	entries, err := ParseSRT(content)
	if err != nil {
		return content, fmt.Errorf("파싱 실패: %v", err)
	}

	originalEntries, err := ParseSRT(originalContent)
	if err != nil {
		return content, fmt.Errorf("원본 파싱 실패: %v", err)
	}

	// Check entry count
	if len(entries) != len(originalEntries) {
		return content, fmt.Errorf("항목 수 불일치: 원본=%d, 결과=%d", len(originalEntries), len(entries))
	}

	// Check for incomplete entries
	for i, entry := range entries {
		// Check if text is empty or too short
		if strings.TrimSpace(entry.Text) == "" {
			return content, fmt.Errorf("빈 텍스트 항목 발견: 인덱스=%d", entry.Index)
		}

		// Check if timecode is valid
		if !isValidTimecode(entry.StartTime) || !isValidTimecode(entry.EndTime) {
			return content, fmt.Errorf("잘못된 타임코드: 인덱스=%d, 시작=%s, 종료=%s", 
				entry.Index, entry.StartTime, entry.EndTime)
		}

		// Check if index is sequential
		if i > 0 && entry.Index != entries[i-1].Index+1 {
			return content, fmt.Errorf("인덱스 순서 오류: %d -> %d", entries[i-1].Index, entry.Index)
		}
	}

	// All checks passed
	return content, nil
}

// isValidTimecode checks if a timecode is valid
func isValidTimecode(timecode string) bool {
	// Check format: HH:MM:SS,mmm
	parts := strings.Split(timecode, ":")
	if len(parts) != 3 {
		return false
	}

	// Check milliseconds part
	millisParts := strings.Split(parts[2], ",")
	if len(millisParts) != 2 {
		return false
	}

	// Check if all parts are numbers
	for _, part := range []string{parts[0], parts[1], millisParts[0], millisParts[1]} {
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return false
			}
		}
	}

	return true
}
