package main

import (
	"fmt"
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

// ParseChunkResponse parses API response back to SRT entries
func ParseChunkResponse(response string) ([]SRTEntry, error) {
	// Clean up response (remove markdown code blocks if present)
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```srt")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)
	
	return ParseSRT(response)
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
