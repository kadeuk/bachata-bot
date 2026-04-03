package main

import (
	"fmt"
	"regexp"
	"strings"
)

// FixSRTTimecodeFormat fixes malformed SRT timecode formats
// YouTube requires: HH:MM:SS,mmm --> HH:MM:SS,mmm
// This function fixes: HH:MM:SS mmm --> HH:MM:SS mmm (missing comma)
func FixSRTTimecodeFormat(content string) (string, error) {
	lines := strings.Split(content, "\n")
	var result []string
	
	// Regex to match timecode lines with space instead of comma
	// Matches: 00:00:00 000 --> 00:00:02 970
	timecodeRegex := regexp.MustCompile(`^(\d{2}:\d{2}:\d{2})\s+(\d{3})\s+-->\s+(\d{2}:\d{2}:\d{2})\s+(\d{3})$`)
	
	for _, line := range lines {
		// Check if this is a malformed timecode line
		if matches := timecodeRegex.FindStringSubmatch(line); matches != nil {
			// Reconstruct with proper comma format
			fixedLine := fmt.Sprintf("%s,%s --> %s,%s", matches[1], matches[2], matches[3], matches[4])
			result = append(result, fixedLine)
		} else {
			// Keep line as-is
			result = append(result, line)
		}
	}
	
	return strings.Join(result, "\n"), nil
}

// ValidateSRTFormat checks if SRT file has valid YouTube-compatible format
func ValidateSRTFormat(content string) []string {
	var issues []string
	
	lines := strings.Split(content, "\n")
	
	// Valid timecode format: HH:MM:SS,mmm --> HH:MM:SS,mmm
	validTimecodeRegex := regexp.MustCompile(`^\d{2}:\d{2}:\d{2},\d{3}\s+-->\s+\d{2}:\d{2}:\d{2},\d{3}$`)
	
	// Invalid timecode format (space instead of comma)
	invalidTimecodeRegex := regexp.MustCompile(`^\d{2}:\d{2}:\d{2}\s+\d{3}\s+-->\s+\d{2}:\d{2}:\d{2}\s+\d{3}$`)
	
	for i, line := range lines {
		line = strings.TrimSpace(line)
		
		// Check if line looks like a timecode
		if strings.Contains(line, "-->") {
			if invalidTimecodeRegex.MatchString(line) {
				issues = append(issues, fmt.Sprintf("Line %d: Invalid timecode format (space instead of comma): %s", i+1, line))
			} else if !validTimecodeRegex.MatchString(line) {
				issues = append(issues, fmt.Sprintf("Line %d: Malformed timecode: %s", i+1, line))
			}
		}
	}
	
	return issues
}
