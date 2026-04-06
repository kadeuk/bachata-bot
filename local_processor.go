package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ProcessLocalFiles processes SRT files from the "번역전" folder
func ProcessLocalFiles() error {
	log.Println("🔍 번역전 폴더에서 SRT 파일 검색 중...")

	// Check if 번역전 folder exists
	translationDir := "번역전"
	if _, err := os.Stat(translationDir); os.IsNotExist(err) {
		// Try to create the directory
		if err := os.MkdirAll(translationDir, 0755); err != nil {
			return fmt.Errorf("번역전 폴더 생성 실패: %v", err)
		}
		log.Printf("📁 번역전 폴더 생성 완료: %s", translationDir)
	}

	// Find all SRT files in 번역전 folder
	var srtFiles []string
	err := filepath.Walk(translationDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".srt") {
			srtFiles = append(srtFiles, path)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("파일 검색 실패: %v", err)
	}

	if len(srtFiles) == 0 {
		log.Println("⚠️ 번역전 폴더에 SRT 파일이 없습니다.")
		log.Println("📁 SRT 파일을 번역전 폴더에 넣어주세요.")
		return nil
	}

	log.Printf("✅ %d개의 SRT 파일 발견", len(srtFiles))

	// Process each file
	for i, filePath := range srtFiles {
		log.Printf("📄 파일 처리 중 (%d/%d): %s", i+1, len(srtFiles), filePath)
		
		if err := processSingleFile(filePath); err != nil {
			log.Printf("⚠️ 파일 처리 실패 (%s): %v", filePath, err)
			continue
		}
	}

	log.Println("✅ 모든 파일 처리 완료!")
	return nil
}

// processSingleFile processes a single SRT file
func processSingleFile(filePath string) error {
	// Read file content
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("파일 읽기 실패: %v", err)
	}

	originalContent := string(contentBytes)
	fileName := filepath.Base(filePath)
	baseFileName := strings.TrimSuffix(fileName, filepath.Ext(fileName))

	log.Printf("📝 파일 처리 시작: %s (%d bytes)", fileName, len(contentBytes))

	// STEP 1: Clean text
	cleanedContent := textCleaner.CleanSRTContent(originalContent)
	
	// Fix SRT timecode format
	fixedContent, err := FixSRTTimecodeFormat(cleanedContent)
	if err != nil {
		log.Printf("⚠️ 타임코드 형식 수정 실패: %v", err)
	} else {
		cleanedContent = fixedContent
	}
	
	// Convert Korean numbers to Arabic numbers
	cleanedContent = numberConverter.ConvertNumbersInContent(cleanedContent)
	
	// AI post-processing for remaining numbers
	cleanedContent, err = deepseekClient.PostProcessNumbers(cleanedContent)
	if err != nil {
		log.Printf("⚠️ AI 숫자 후처리 실패: %v", err)
	}

	// STEP 2: Correct Korean SRT
	log.Println("📝 한국어 자막 교정 중...")
	correctedKorean, err := translator.CorrectKoreanSRT(cleanedContent, nil)
	if err != nil {
		return fmt.Errorf("한국어 교정 실패: %v", err)
	}

	// Create output folders
	dateFolder := time.Now().Format("2006-01-02")
	subtitlePath := filepath.Join(dateFolder, "자막번역완성", baseFileName)
	metadataPath := filepath.Join(dateFolder, "제목설명완성", baseFileName)

	os.MkdirAll(subtitlePath, 0755)
	os.MkdirAll(metadataPath, 0755)

	// Validate Korean SRT before saving
	validatedKorean, err := ValidateAndFixSRT(correctedKorean, originalContent)
	if err != nil {
		log.Printf("⚠️ 한국어 자막 검증 실패: %v", err)
		validatedKorean = correctedKorean // Use original if validation fails
	}

	// Save corrected Korean SRT
	koreanPath := filepath.Join(subtitlePath, baseFileName+"_한국어.srt")
	if err := os.WriteFile(koreanPath, []byte(validatedKorean), 0644); err != nil {
		return fmt.Errorf("한국어 파일 저장 실패: %v", err)
	}

	log.Printf("✅ 한국어 자막 저장 완료: %s", koreanPath)

	// STEP 3: Translate to 10 languages
	languages := []struct {
		code string
		name string
		file string
	}{
		{"English", "영어", "_영어.srt"},
		{"Español", "스페인어", "_스페인어.srt"},
		{"Polski", "폴란드어", "_폴란드어.srt"},
		{"日本語", "일본어", "_일본어.srt"},
		{"中文 (简体)", "중국어", "_중국어.srt"},
		{"Français", "프랑스어", "_프랑스어.srt"},
		{"Deutsch", "독일어", "_독일어.srt"},
		{"Italiano", "이탈리아어", "_이탈리아어.srt"},
		{"Tiếng Việt", "베트남어", "_베트남어.srt"},
		{"Bahasa Melayu", "말레이어", "_말레이어.srt"},
	}

	for _, lang := range languages {
		log.Printf("🌍 %s 번역 중...", lang.name)
		
		translated, err := translator.TranslateToLanguage(correctedKorean, lang.code, lang.name, nil)
		if err != nil {
			log.Printf("⚠️ %s 번역 실패: %v", lang.name, err)
			continue
		}

		savePath := filepath.Join(subtitlePath, baseFileName+lang.file)
		if err := os.WriteFile(savePath, []byte(translated), 0644); err != nil {
			log.Printf("⚠️ %s 파일 저장 실패: %v", lang.name, err)
			continue
		}
		
		log.Printf("✅ %s 번역 완료: %s", lang.name, savePath)
	}

	// STEP 4: Generate metadata
	log.Println("📺 한국어 제목/설명 생성 중...")
	koreanTitle, koreanDesc, err := translator.GenerateMetadata(correctedKorean)
	if err != nil {
		log.Printf("⚠️ 메타데이터 생성 실패: %v", err)
	} else {
		// Save Korean metadata
		koreanMetaPath := filepath.Join(metadataPath, baseFileName+"_한국어.txt")
		metaContent := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", koreanTitle, koreanDesc)
		if err := os.WriteFile(koreanMetaPath, []byte(metaContent), 0644); err != nil {
			log.Printf("⚠️ 한국어 메타데이터 저장 실패: %v", err)
		} else {
			log.Printf("✅ 한국어 제목/설명 저장 완료: %s", koreanMetaPath)
		}

		// Translate metadata to 10 languages
		for _, lang := range []struct {
			code string
			name string
			file string
		}{
			{"English", "영어", "_영어.txt"},
			{"Español", "스페인어", "_스페인어.txt"},
			{"Polski", "폴란드어", "_폴란드어.txt"},
			{"日本語", "일본어", "_일본어.txt"},
			{"中文 (简体)", "중국어", "_중국어.txt"},
			{"Français", "프랑스어", "_프랑스어.txt"},
			{"Deutsch", "독일어", "_독일어.txt"},
			{"Italiano", "이탈리아어", "_이탈리아어.txt"},
			{"Tiếng Việt", "베트남어", "_베트남어.txt"},
			{"Bahasa Melayu", "말레이어", "_말레이어.txt"},
		} {
			log.Printf("📺 %s 제목/설명 번역 중...", lang.name)
			
			title, desc, err := translator.TranslateMetadata(koreanTitle, koreanDesc, lang.code, lang.name)
			if err != nil {
				log.Printf("⚠️ %s 메타데이터 번역 실패: %v", lang.name, err)
				continue
			}

			metaPath := filepath.Join(metadataPath, baseFileName+lang.file)
			content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", title, desc)
			if err := os.WriteFile(metaPath, []byte(content), 0644); err != nil {
				log.Printf("⚠️ %s 메타데이터 저장 실패: %v", lang.name, err)
				continue
			}
			
			log.Printf("✅ %s 제목/설명 번역 완료: %s", lang.name, metaPath)
		}
	}

	log.Printf("✅ 파일 처리 완료: %s", fileName)
	return nil
}

// printHelp prints CLI help message
func printHelp() {
	fmt.Println(`🎵 Bachata Bot - 자막 번역 및 교정 도구

사용법:
  bachata-bot [옵션]

옵션:
  --process-local    번역전 폴더의 SRT 파일을 자동 처리
  --fix-srt <경로>   지정된 폴더의 SRT 파일 형식 수정
  --help, -h         도움말 출력

예시:
  bachata-bot --process-local
  bachata-bot --fix-srt ./자막폴더

설명:
  --process-local: 번역전 폴더에 있는 모든 SRT 파일을 자동으로 처리합니다.
                   한국어 교정 → 10개 언어 번역 → 제목/설명 생성

  --fix-srt: 지정된 폴더의 SRT 파일 형식을 YouTube 호환 형식으로 수정합니다.
             타임코드 형식(공백 → 쉼표)을 자동으로 수정합니다.

폴더 구조:
  번역전/           - 원본 SRT 파일을 넣는 폴더
  2026-01-01/      - 날짜별 출력 폴더
    ├── 자막번역완성/ - 번역된 SRT 파일
    └── 제목설명완성/ - 번역된 제목/설명 파일
`)
}