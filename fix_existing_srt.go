package main

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// FixExistingSRTFiles scans a directory and fixes all SRT files with malformed timecodes
func FixExistingSRTFiles(rootDir string) error {
	log.Printf("🔍 자막 파일 검사 시작: %s", rootDir)
	
	fixedCount := 0
	errorCount := 0
	
	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		
		// Skip directories
		if d.IsDir() {
			return nil
		}
		
		// Only process .srt files
		if !strings.HasSuffix(strings.ToLower(path), ".srt") {
			return nil
		}
		
		// Read file
		content, err := os.ReadFile(path)
		if err != nil {
			log.Printf("⚠️ 파일 읽기 실패: %s - %v", path, err)
			errorCount++
			return nil
		}
		
		// Check if file has issues
		issues := ValidateSRTFormat(string(content))
		if len(issues) == 0 {
			// File is already valid
			return nil
		}
		
		log.Printf("🔧 수정 필요: %s (%d개 문제)", path, len(issues))
		
		// Fix the file
		fixed, err := FixSRTTimecodeFormat(string(content))
		if err != nil {
			log.Printf("❌ 수정 실패: %s - %v", path, err)
			errorCount++
			return nil
		}
		
		// Verify the fix worked
		remainingIssues := ValidateSRTFormat(fixed)
		if len(remainingIssues) > 0 {
			log.Printf("⚠️ 수정 후에도 문제 남음: %s (%d개)", path, len(remainingIssues))
			errorCount++
			return nil
		}
		
		// Write back to file
		err = os.WriteFile(path, []byte(fixed), 0644)
		if err != nil {
			log.Printf("❌ 파일 쓰기 실패: %s - %v", path, err)
			errorCount++
			return nil
		}
		
		log.Printf("✅ 수정 완료: %s", path)
		fixedCount++
		
		return nil
	})
	
	if err != nil {
		return fmt.Errorf("디렉토리 스캔 실패: %v", err)
	}
	
	log.Printf("\n📊 수정 완료:")
	log.Printf("   ✅ 수정된 파일: %d개", fixedCount)
	log.Printf("   ❌ 오류 발생: %d개", errorCount)
	
	return nil
}
