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

// ParseSRT parses an SRT file content into entries
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
		if len(lines) < 3 {
			continue // Invalid entry
		}
		
		// Parse index
		index, err := strconv.Atoi(strings.TrimSpace(lines[0]))
		if err != nil {
			continue // Skip invalid index
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
