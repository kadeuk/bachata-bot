package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

const (
	DirInput = "번역전"
)

var (
	geminiClient      *GeminiClient
	techniquesMgr     *TechniqueManager
	translator        *Translator
	sessionManager    *SessionManager
	textCleaner       *TextCleaner
	corrector         *Corrector
	parallelProcessor *ParallelProcessor
	glossaryMgr       *GlossaryManager
	numberConverter   *NumberConverter
	spellingCorrector *SpellingCorrector
	
	// Build-time variables (injected via -ldflags)
	DiscordToken string
	GeminiAPIKey string
)

func main() {
	// Check for CLI flags
	if len(os.Args) > 1 && os.Args[1] == "--fix-srt" {
		if len(os.Args) < 3 {
			log.Fatal("❌ 사용법: bachata-bot --fix-srt <디렉토리 경로>")
		}
		targetDir := os.Args[2]
		log.Printf("🔧 SRT 파일 수정 모드: %s", targetDir)
		if err := FixExistingSRTFiles(targetDir); err != nil {
			log.Fatalf("❌ 수정 실패: %v", err)
		}
		return
	}

	// Get API keys from build-time variables or environment variables
	discordToken := DiscordToken
	geminiAPIKey := GeminiAPIKey
	
	// Fallback to environment variables if build-time variables are empty
	if discordToken == "" {
		if err := godotenv.Load(); err != nil {
			log.Println("⚠️ .env 파일을 찾을 수 없습니다.")
		}
		discordToken = os.Getenv("DISCORD_TOKEN")
	}
	
	if geminiAPIKey == "" {
		if err := godotenv.Load(); err != nil {
			log.Println("⚠️ .env 파일을 찾을 수 없습니다.")
		}
		geminiAPIKey = os.Getenv("GEMINI_API_KEY")
	}

	if discordToken == "" {
		log.Fatal("❌ DISCORD_TOKEN이 설정되지 않았습니다. GitHub Secrets 또는 .env 파일을 확인하세요.")
	}

	if geminiAPIKey == "" {
		log.Fatal("❌ GEMINI_API_KEY가 설정되지 않았습니다. GitHub Secrets 또는 .env 파일을 확인하세요.")
	}

	// Initialize components
	var err error
	geminiClient, err = NewGeminiClient(geminiAPIKey)
	if err != nil {
		log.Fatalf("❌ Gemini 클라이언트 초기화 실패: %v", err)
	}
	defer geminiClient.Close()
	log.Println("✅ Gemini API 연결 완료")

	techniquesMgr, err = NewTechniqueManager("bachata_techniques.json")
	if err != nil {
		log.Printf("⚠️ 바차타 용어 사전 로드 실패: %v", err)
	} else {
		log.Println("✅ 바차타 용어 사전 로드 완료")
	}

	// Initialize glossary manager
	glossaryMgr, err = NewGlossaryManager("correction_glossary.json", "translation_glossary.json")
	if err != nil {
		log.Printf("⚠️ 용어집 관리자 초기화 실패: %v", err)
	}

	translator = NewTranslator(geminiClient, techniquesMgr, glossaryMgr)
	sessionManager = NewSessionManager()
	textCleaner = NewTextCleaner()
	corrector = NewCorrector(geminiClient, techniquesMgr, glossaryMgr)
	parallelProcessor = NewParallelProcessor(3) // 최대 3개 청크 동시 처리
	numberConverter = NewNumberConverter()
	spellingCorrector = NewSpellingCorrector()
	log.Println("✅ 번역기 초기화 완료")
	log.Println("✅ 숫자 변환기 및 맞춤법 교정기 초기화 완료")

	// Create Discord session
	dg, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		log.Fatalf("❌ Discord 세션 생성 실패: %v", err)
	}

	dg.AddHandler(messageCreate)
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	err = dg.Open()
	if err != nil {
		log.Fatalf("❌ Discord 연결 실패: %v", err)
	}
	defer dg.Close()

	log.Println("✅ Discord Bot 시작 완료! SRT 파일을 업로드하세요...")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	log.Println("🛑 Bot 종료 중...")
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore bot messages
	if m.Author.ID == s.State.User.ID {
		return
	}

	userID := m.Author.ID
	session, exists := sessionManager.GetSession(userID)

	// Handle file uploads (SRT files)
	if len(m.Attachments) > 0 {
		for _, attachment := range m.Attachments {
			if strings.HasSuffix(strings.ToLower(attachment.Filename), ".srt") {
				// Check if user already has a session
				if exists && session.State != StateIdle {
					s.ChannelMessageSend(m.ChannelID, "⚠️ 이미 진행 중인 작업이 있습니다. 완료 후 다시 시도하세요.")
					return
				}

				log.Printf("📢 SRT 파일 업로드 감지: %s (사용자: %s)", attachment.Filename, m.Author.Username)
				
				msg, err := s.ChannelMessageSend(m.ChannelID, "📥 파일을 다운로드하는 중...")
				if err != nil {
					log.Printf("❌ 메시지 전송 실패: %v", err)
					return
				}

				// Create session - Use original filename from Discord
				session = sessionManager.CreateSession(userID, m.ChannelID, msg.ID)
				session.CurrentFile = attachment.Filename
				// Extract base filename without extension
				baseFileName := strings.TrimSuffix(attachment.Filename, filepath.Ext(attachment.Filename))
				session.BaseFileName = baseFileName
				session.Glossary = make(map[string]string)
				session.CorrectionApproved = make(map[string]string)
				
				log.Printf("✅ 세션 생성: 파일명=%s, 기본명=%s", attachment.Filename, baseFileName)
				
				// Download file
				go downloadAndProcessFile(s, session, attachment.URL)
				return
			}
		}
	}

	// Handle responses based on session state
	if !exists {
		return
	}

	switch session.State {
	case StateWaitingFilename:
		handleFilenameResponse(s, m, session)
	case StateWaitingTermCheck:
		handleTermCheckResponse(s, m, session)
	case StateWaitingCorrection:
		handleCorrectionResponse(s, m, session)
	case StateWaitingApproval:
		handleApprovalResponse(s, m, session)
	case StateWaitingMetadataApproval:
		handleMetadataApprovalResponse(s, m, session)
	}
}

func downloadAndProcessFile(s *discordgo.Session, session *Session, fileURL string) {
	updateMessage := func(content string) {
		_, err := s.ChannelMessageEdit(session.ChannelID, session.MessageID, content)
		if err != nil {
			log.Printf("⚠️ 메시지 업데이트 실패: %v", err)
		}
	}

	// Download file from Discord
	updateMessage(fmt.Sprintf("📥 파일 다운로드 중: %s", session.CurrentFile))
	
	resp, err := http.Get(fileURL)
	if err != nil {
		updateMessage(fmt.Sprintf("❌ 파일 다운로드 실패: %v", err))
		sessionManager.DeleteSession(session.UserID)
		return
	}
	defer resp.Body.Close()

	contentBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		updateMessage(fmt.Sprintf("❌ 파일 읽기 실패: %v", err))
		sessionManager.DeleteSession(session.UserID)
		return
	}

	session.OriginalContent = string(contentBytes)
	log.Printf("✅ 파일 다운로드 완료: %s (%d bytes)", session.CurrentFile, len(contentBytes))

	// Ask user for proper filename
	session.SetState(StateWaitingFilename)
	sessionManager.UpdateSession(session.UserID, session)
	
	updateMessage(fmt.Sprintf("✅ 파일 다운로드 완료!\n\n"+
		"📝 이 영상의 제목을 입력해주세요.\n"+
		"예: 힙쓰로우배, 프론트웨이브, 바차타기초 등\n\n"+
		"입력한 제목으로 폴더와 파일이 생성됩니다."))
}

func handleFilenameResponse(s *discordgo.Session, m *discordgo.MessageCreate, session *Session) {
	filename := strings.TrimSpace(m.Content)
	
	if filename == "" {
		s.ChannelMessageSend(m.ChannelID, "⚠️ 제목을 입력해주세요.")
		return
	}
	
	// Update session with user-provided filename
	session.BaseFileName = filename
	sessionManager.UpdateSession(session.UserID, session)
	
	log.Printf("✅ 사용자 제공 파일명: %s", filename)
	
	// Continue processing
	continueProcessingAfterFilename(s, session)
}

func continueProcessingAfterFilename(s *discordgo.Session, session *Session) {
	updateMessage := func(content string) {
		_, err := s.ChannelMessageEdit(session.ChannelID, session.MessageID, content)
		if err != nil {
			log.Printf("⚠️ 메시지 업데이트 실패: %v", err)
		}
	}

	// Start processing
	updateMessage(fmt.Sprintf("📁 파일: %s\n🧹 텍스트 정리 중...", session.BaseFileName))
	
	// STEP 1: Clean text with regex (with error handling)
	var cleanedContent string
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ 텍스트 정리 중 패닉 발생: %v", r)
				updateMessage(fmt.Sprintf("❌ 텍스트 정리 실패: %v\n\n원본을 사용합니다.", r))
				cleanedContent = session.OriginalContent
			}
		}()
		cleanedContent = textCleaner.CleanSRTContent(session.OriginalContent)
		
		// Fix SRT timecode format (YouTube requires comma, not space)
		fixedContent, err := FixSRTTimecodeFormat(cleanedContent)
		if err != nil {
			log.Printf("⚠️ 타임코드 형식 수정 실패: %v", err)
		} else {
			cleanedContent = fixedContent
			log.Printf("✅ 타임코드 형식 수정 완료 (YouTube 호환)")
		}
		
		// CRITICAL: Convert Korean numbers to Arabic numbers BEFORE AI processing
		cleanedContent = numberConverter.ConvertNumbersInContent(cleanedContent)
		log.Printf("✅ 숫자 변환 완료 (한글 → 아라비아 숫자)")
		
		log.Printf("✅ 텍스트 정리 완료")
	}()

	// STEP 2: Extract correction suggestions from AI
	updateMessage(fmt.Sprintf("📁 파일: %s\n🤔 AI가 교정이 필요한 부분을 분석 중...", session.CurrentFile))
	
	session.SetState(StateProcessing)
	sessionManager.UpdateSession(session.UserID, session)

	// Extract correction suggestions
	suggestions, err := corrector.ExtractCorrectionSuggestions(cleanedContent)
	if err != nil {
		log.Printf("⚠️ 교정 제안 추출 실패: %v", err)
		// Fallback to direct correction
		updateMessage(fmt.Sprintf("📁 파일: %s\n📝 기본 교정 진행 중...", session.CurrentFile))
		correctedKorean, err := translator.CorrectKoreanSRT(cleanedContent, nil)
		if err != nil {
			updateMessage(fmt.Sprintf("❌ 한국어 교정 실패: %v", err))
			sessionManager.DeleteSession(session.UserID)
			return
		}
		session.CorrectedKorean = correctedKorean
	} else {
		// Store original content for correction
		session.CorrectedKorean = cleanedContent
		
		// Convert suggestions to TermSuggestion format
		for _, sugg := range suggestions {
			term := TermSuggestion{
				Timecode:       sugg.Timecode,
				OriginalTerm:   sugg.OriginalSTT,
				SuggestedTerm:  sugg.BestGuess,
				Context:        sugg.ContextAnalysis,
				Reasoning:      sugg.AIReasoning,
				NeedsConfirm:   sugg.NeedsConfirm,
				QuestionToUser: sugg.QuestionToUser,
			}
			session.PendingTerms = append(session.PendingTerms, term)
		}
		
		log.Printf("✅ %d개의 교정 제안 추출 완료", len(session.PendingTerms))
	}

	// Create output folders using BaseFileName
	dateFolder := time.Now().Format("2006-01-02")
	subtitlePath := filepath.Join(dateFolder, "자막번역완성", session.BaseFileName)
	metadataPath := filepath.Join(dateFolder, "제목설명완성", session.BaseFileName)

	os.MkdirAll(subtitlePath, 0755)
	os.MkdirAll(metadataPath, 0755)

	session.SubtitlePath = subtitlePath
	session.MetadataPath = metadataPath

	log.Printf("✅ 출력 폴더 생성: 자막=%s, 메타=%s", subtitlePath, metadataPath)

	// If we have suggestions, start interactive correction
	if len(session.PendingTerms) > 0 {
		session.SetState(StateWaitingTermCheck)
		session.CurrentTermIdx = 0
		sessionManager.UpdateSession(session.UserID, session)
		
		updateMessage(fmt.Sprintf("✅ 분석 완료! %d개의 교정 제안이 있습니다.\n\n"+
			"지금부터 각 용어를 하나씩 확인하겠습니다.", len(session.PendingTerms)))
		
		// Start asking about first term
		askNextTerm(s, session)
	} else {
		// No suggestions, save and proceed to review
		koreanPath := filepath.Join(subtitlePath, session.BaseFileName+"_한국어.srt")
		os.WriteFile(koreanPath, []byte(session.CorrectedKorean), 0644)
		absKoreanPath, _ := filepath.Abs(koreanPath)

		session.SetState(StateWaitingCorrection)
		sessionManager.UpdateSession(session.UserID, session)

		reviewMessage := fmt.Sprintf("✅ 교정 완료! (교정 제안 없음)\n\n"+
			"📂 저장: `%s`\n\n"+
			"📝 수정이 필요한 부분이 있으면 자연어로 입력하세요.\n"+
			"수정이 없으면 `!승인`을 입력하세요.",
			absKoreanPath)

		updateMessage(reviewMessage)
		log.Printf("✅ 교정 완료 (제안 없음). 사용자 검토 대기 중... (사용자: %s)", session.UserID)
	}
}

func handleTermCheckResponse(s *discordgo.Session, m *discordgo.MessageCreate, session *Session) {
	input := strings.TrimSpace(m.Content)
	term := session.GetCurrentTerm()
	
	if term == nil {
		return
	}

	var replacement string
	
	// Handle user input
	if input == "" || strings.ToLower(input) == "y" || strings.ToLower(input) == "yes" {
		// Apply AI suggestion
		replacement = term.SuggestedTerm
		log.Printf("✅ 용어 승인: [%s] → [%s]", term.OriginalTerm, replacement)
	} else if strings.ToLower(input) == "n" || input == "유지" {
		// Keep original
		replacement = term.OriginalTerm
		log.Printf("➡️ 원본 유지: [%s]", term.OriginalTerm)
	} else {
		// User provided custom term
		replacement = input
		log.Printf("✏️ 사용자 입력: [%s] → [%s]", term.OriginalTerm, replacement)
	}

	// Apply replacement
	session.CorrectedKorean = strings.ReplaceAll(session.CorrectedKorean, term.OriginalTerm, replacement)
	
	// Add to glossary if changed
	if replacement != term.OriginalTerm {
		session.Glossary[term.OriginalTerm] = replacement
		
		// Auto-update correction glossary
		if glossaryMgr != nil {
			if err := glossaryMgr.AddCorrectionTerm(term.OriginalTerm, replacement); err != nil {
				log.Printf("⚠️ 교정 용어집 업데이트 실패: %v", err)
			}
		}
	}

	// Move to next term
	if session.NextTerm() {
		askNextTerm(s, session)
	} else {
		// All terms checked, proceed to final review
		proceedToFinalReview(s, session)
	}
}

func handleCorrectionResponse(s *discordgo.Session, m *discordgo.MessageCreate, session *Session) {
	input := strings.TrimSpace(m.Content)
	
	if input == "!승인" || input == "승인" {
		// User approved, proceed to translation
		proceedToApproval(s, session)
	} else {
		// User wants to make edits
		log.Printf("📝 사용자 수정 요청: %s", input)
		go processUserEdit(s, session, input)
	}
}

func handleApprovalResponse(s *discordgo.Session, m *discordgo.MessageCreate, session *Session) {
	input := strings.ToLower(strings.TrimSpace(m.Content))
	
	if input == "!승인" || input == "승인" || input == "y" || input == "yes" {
		log.Printf("📢 최종 승인 수신: %s", session.UserID)
		go continueTranslation(s, session)
	} else {
		// User wants more edits
		log.Printf("📝 추가 수정 요청: %s", m.Content)
		session.SetState(StateWaitingCorrection)
		sessionManager.UpdateSession(session.UserID, session)
		go processUserEdit(s, session, m.Content)
	}
}

func handleMetadataApprovalResponse(s *discordgo.Session, m *discordgo.MessageCreate, session *Session) {
	input := strings.TrimSpace(m.Content)
	
	if input == "!승인" || input == "승인" {
		log.Printf("📢 메타데이터 승인 수신: %s", session.UserID)
		go continueMetadataTranslation(s, session)
	} else {
		// User wants to edit metadata
		log.Printf("📝 메타데이터 수정 요청: %s", input)
		go processMetadataEdit(s, session, input)
	}
}

func askNextTerm(s *discordgo.Session, session *Session) {
	term := session.GetCurrentTerm()
	if term == nil {
		return
	}

	message := fmt.Sprintf("❓ 용어 확인 필요 (%d/%d)\n\n"+
		"⏱️ 타임코드: %s\n"+
		"❌ 원본: [%s]\n"+
		"✅ 제안: [%s]\n"+
		"📖 문맥: %s\n"+
		"💡 이유: %s\n\n"+
		"👉 선택:\n"+
		"- 엔터 또는 'Y': 제안 적용\n"+
		"- 'N' 또는 '유지': 원본 유지\n"+
		"- 직접 입력: 원하는 용어 입력",
		session.CurrentTermIdx+1, len(session.PendingTerms),
		term.Timecode, term.OriginalTerm, term.SuggestedTerm,
		term.Context, term.Reasoning)

	s.ChannelMessageSend(session.ChannelID, message)
}

func proceedToFinalReview(s *discordgo.Session, session *Session) {
	// Save corrected Korean SRT
	koreanPath := filepath.Join(session.SubtitlePath, session.BaseFileName+"_한국어.srt")
	os.WriteFile(koreanPath, []byte(session.CorrectedKorean), 0644)

	absKoreanPath, _ := filepath.Abs(koreanPath)

	session.SetState(StateWaitingCorrection)
	sessionManager.UpdateSession(session.UserID, session)

	message := fmt.Sprintf("✅ 용어 확인 완료!\n\n"+
		"📂 저장: `%s`\n\n"+
		"📝 추가 수정이 필요하시면 자연어로 입력하세요.\n"+
		"예: '00:00:05,000 부분의 사이드웨이브를 프론트 웨이브로 바꿔줘'\n\n"+
		"수정이 없으면 `!승인`을 입력하세요.",
		absKoreanPath)

	s.ChannelMessageSend(session.ChannelID, message)
}

func processUserEdit(s *discordgo.Session, session *Session, editInstruction string) {
	msg, _ := s.ChannelMessageSend(session.ChannelID, "⏳ AI가 수정 지시를 분석하고 자막을 수정 중...")

	// Parse original SRT to preserve structure
	originalEntries, err := ParseSRT(session.CorrectedKorean)
	if err != nil {
		s.ChannelMessageEdit(session.ChannelID, msg.ID, fmt.Sprintf("❌ 원본 SRT 파싱 실패: %v", err))
		return
	}

	prompt := fmt.Sprintf(`사용자가 바차타 강습 자막을 수정하려고 자연어로 지시했습니다.
사용자의 지시를 이해하고, 수정된 SRT 파일 전체를 출력하세요.

**원본 SRT:**
%s

**사용자 수정 지시:**
%s

**절대적으로 지켜야 할 규칙 (위반 시 실패):**
1. **타임코드는 1글자도 변경하지 마세요** - 원본과 100%% 동일하게 유지
2. **자막 번호는 그대로 유지하세요** - 1부터 %d까지 순차적으로
3. **자막 항목 개수는 정확히 %d개를 유지하세요** - 절대 추가/삭제 금지
4. **모든 자막 항목을 빠짐없이 출력하세요** - 1번부터 %d번까지 전부
5. 사용자가 지시한 텍스트 내용만 수정하세요
6. SRT 형식을 정확히 지켜주세요:
   - 번호
   - 타임코드 (HH:MM:SS,mmm --> HH:MM:SS,mmm)
   - 텍스트
   - 빈 줄

**중요: 항목 개수 확인**
- 원본: %d개 항목
- 출력: %d개 항목 (반드시 동일)
- 1번부터 %d번까지 모두 포함

**예시:**
1
00:00:00,000 --> 00:00:02,970
수정된 텍스트

2
00:00:02,970 --> 00:00:04,880
수정된 텍스트

... (중간 생략하지 말고 모두 출력)

%d
[마지막 타임코드]
[마지막 텍스트]

**출력:** 수정된 전체 SRT 파일 (마크다운 코드 블록 없이, %d개 항목 전부)`, 
		session.CorrectedKorean, editInstruction, 
		len(originalEntries), len(originalEntries), len(originalEntries),
		len(originalEntries), len(originalEntries), len(originalEntries),
		len(originalEntries), len(originalEntries))

	editedSRT, err := geminiClient.GenerateContent(prompt)
	if err != nil {
		s.ChannelMessageEdit(session.ChannelID, msg.ID, fmt.Sprintf("❌ 자막 수정 실패: %v", err))
		return
	}

	// Clean response
	editedSRT = strings.TrimSpace(editedSRT)
	editedSRT = strings.TrimPrefix(editedSRT, "```srt")
	editedSRT = strings.TrimPrefix(editedSRT, "```")
	editedSRT = strings.TrimSuffix(editedSRT, "```")
	editedSRT = strings.TrimSpace(editedSRT)

	// Validate edited SRT
	editedEntries, err := ParseSRT(editedSRT)
	if err != nil {
		s.ChannelMessageEdit(session.ChannelID, msg.ID, fmt.Sprintf("❌ 수정된 SRT 파싱 실패: %v\n\n원본을 유지합니다.", err))
		return
	}

	// Strict validation: Check timecodes match exactly
	if err := ValidateTimecodes(originalEntries, editedEntries); err != nil {
		log.Printf("⚠️ 타임코드 검증 실패: %v", err)
		s.ChannelMessageEdit(session.ChannelID, msg.ID, fmt.Sprintf("❌ AI가 타임코드를 변경했습니다: %v\n\n원본 타임코드를 복원합니다...", err))
		
		// Restore original timecodes but keep edited text
		for i := range editedEntries {
			if i < len(originalEntries) {
				editedEntries[i].Index = originalEntries[i].Index
				editedEntries[i].StartTime = originalEntries[i].StartTime
				editedEntries[i].EndTime = originalEntries[i].EndTime
			}
		}
		
		editedSRT = FormatSRT(editedEntries)
	}

	session.CorrectedKorean = editedSRT

	// Save updated version
	koreanPath := filepath.Join(session.SubtitlePath, session.BaseFileName+"_한국어.srt")
	os.WriteFile(koreanPath, []byte(editedSRT), 0644)

	absKoreanPath, _ := filepath.Abs(koreanPath)

	s.ChannelMessageEdit(session.ChannelID, msg.ID, fmt.Sprintf("✅ 자막이 수정되었습니다!\n\n"+
		"📂 저장 위치: `%s`\n\n"+
		"추가 수정이 필요하면 다시 입력하세요.\n"+
		"수정이 완료되었으면 `!승인`을 입력하세요.",
		absKoreanPath))

	sessionManager.UpdateSession(session.UserID, session)
}

func proceedToApproval(s *discordgo.Session, session *Session) {
	koreanPath := filepath.Join(session.SubtitlePath, session.BaseFileName+"_한국어.srt")
	absKoreanPath, _ := filepath.Abs(koreanPath)

	session.SetState(StateWaitingApproval)
	sessionManager.UpdateSession(session.UserID, session)

	message := fmt.Sprintf("✅ 한국어 자막 최종 확인!\n\n"+
		"📂 저장 위치: `%s`\n\n"+
		"⏸️ 최종 확인 후 `!승인`을 입력하면 10개 언어 번역을 시작합니다.\n"+
		"추가 수정이 필요하면 자연어로 입력하세요.",
		absKoreanPath)

	s.ChannelMessageSend(session.ChannelID, message)
}

func continueTranslation(s *discordgo.Session, session *Session) {
	updateMessage := func(content string) {
		_, err := s.ChannelMessageEdit(session.ChannelID, session.MessageID, content)
		if err != nil {
			log.Printf("⚠️ 메시지 업데이트 실패: %v", err)
		}
	}

	session.SetState(StateTranslating)
	sessionManager.UpdateSession(session.UserID, session)

	updateMessage(fmt.Sprintf("📁 파일: %s\n🌍 10개 언어 번역 시작...", session.CurrentFile))

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

	for i, lang := range languages {
		updateMessage(fmt.Sprintf("📁 파일: %s\n🌍 %s 번역 중... (%d/10)", session.CurrentFile, lang.name, i+1))

		translated, err := translator.TranslateToLanguage(session.CorrectedKorean, lang.code, lang.name, nil)
		if err != nil {
			log.Printf("⚠️ %s 번역 실패: %v", lang.name, err)
			continue
		}

		savePath := filepath.Join(session.SubtitlePath, session.BaseFileName+lang.file)
		os.WriteFile(savePath, []byte(translated), 0644)
		log.Printf("✅ %s 번역 완료: %s", lang.name, savePath)
	}

	updateMessage(fmt.Sprintf("📁 파일: %s\n✅ 자막 번역 완료\n📺 한국어 제목/설명 생성 중...", session.CurrentFile))

	// Generate Korean metadata
	koreanTitle, koreanDesc, err := translator.GenerateMetadata(session.CorrectedKorean)
	if err != nil {
		log.Printf("⚠️ 메타데이터 생성 실패: %v", err)
		updateMessage(fmt.Sprintf("❌ 제목/설명 생성 실패: %v", err))
		sessionManager.DeleteSession(session.UserID)
		return
	}

	// Store metadata in session
	session.KoreanTitle = koreanTitle
	session.KoreanDescription = koreanDesc
	sessionManager.UpdateSession(session.UserID, session)

	// Save Korean metadata
	koreanMetaPath := filepath.Join(session.MetadataPath, session.BaseFileName+"_한국어.txt")
	metaContent := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", koreanTitle, koreanDesc)
	os.WriteFile(koreanMetaPath, []byte(metaContent), 0644)
	absKoreanMetaPath, _ := filepath.Abs(koreanMetaPath)

	log.Printf("✅ 한국어 제목/설명 생성 완료: %s", koreanMetaPath)

	// Ask user for approval
	session.SetState(StateWaitingMetadataApproval)
	sessionManager.UpdateSession(session.UserID, session)

	approvalMessage := fmt.Sprintf("✅ 한국어 제목/설명이 생성되었습니다!\n\n"+
		"📂 저장 위치: `%s`\n\n"+
		"**제목:**\n%s\n\n"+
		"**설명:**\n%s\n\n"+
		"📝 수정이 필요하면 자연어로 입력하세요.\n"+
		"예: '제목을 더 짧게 만들어줘' 또는 '설명에 초보자 강조 추가해줘'\n\n"+
		"수정이 없으면 `!승인`을 입력하면 10개 언어로 번역을 시작합니다.",
		absKoreanMetaPath, koreanTitle, koreanDesc)

	s.ChannelMessageSend(session.ChannelID, approvalMessage)
}

func processMetadataEdit(s *discordgo.Session, session *Session, editInstruction string) {
	msg, _ := s.ChannelMessageSend(session.ChannelID, "⏳ AI가 제목/설명을 수정 중...")

	prompt := fmt.Sprintf(`사용자가 유튜브 제목과 설명을 수정하려고 자연어로 지시했습니다.
사용자의 지시를 이해하고, 수정된 제목과 설명을 JSON 형식으로 출력하세요.

**현재 제목:**
%s

**현재 설명:**
%s

**사용자 수정 지시:**
%s

**출력 형식 (JSON):**
{
  "title": "수정된 제목",
  "description": "수정된 설명"
}`, session.KoreanTitle, session.KoreanDescription, editInstruction)

	response, err := geminiClient.GenerateContent(prompt)
	if err != nil {
		s.ChannelMessageEdit(session.ChannelID, msg.ID, fmt.Sprintf("❌ 제목/설명 수정 실패: %v", err))
		return
	}

	// Parse JSON response
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var metadata struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}

	if err := json.Unmarshal([]byte(response), &metadata); err != nil {
		s.ChannelMessageEdit(session.ChannelID, msg.ID, fmt.Sprintf("❌ 응답 파싱 실패: %v", err))
		return
	}

	// Update session
	session.KoreanTitle = metadata.Title
	session.KoreanDescription = metadata.Description
	sessionManager.UpdateSession(session.UserID, session)

	// Save updated metadata
	koreanMetaPath := filepath.Join(session.MetadataPath, session.BaseFileName+"_한국어.txt")
	metaContent := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", metadata.Title, metadata.Description)
	os.WriteFile(koreanMetaPath, []byte(metaContent), 0644)
	absKoreanMetaPath, _ := filepath.Abs(koreanMetaPath)

	s.ChannelMessageEdit(session.ChannelID, msg.ID, fmt.Sprintf("✅ 제목/설명이 수정되었습니다!\n\n"+
		"📂 저장 위치: `%s`\n\n"+
		"**제목:**\n%s\n\n"+
		"**설명:**\n%s\n\n"+
		"추가 수정이 필요하면 다시 입력하세요.\n"+
		"수정이 완료되었으면 `!승인`을 입력하세요.",
		absKoreanMetaPath, metadata.Title, metadata.Description))
}

func continueMetadataTranslation(s *discordgo.Session, session *Session) {
	msg, _ := s.ChannelMessageSend(session.ChannelID, "🌍 10개 언어로 제목/설명 번역 시작...")

	languages := []struct {
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
	}

	for i, lang := range languages {
		s.ChannelMessageEdit(session.ChannelID, msg.ID, 
			fmt.Sprintf("📺 %s 제목/설명 번역 중... (%d/10)", lang.name, i+1))

		title, desc, err := translator.TranslateMetadata(session.KoreanTitle, session.KoreanDescription, lang.code, lang.name)
		if err != nil {
			log.Printf("⚠️ %s 메타데이터 번역 실패: %v", lang.name, err)
			continue
		}

		metaPath := filepath.Join(session.MetadataPath, session.BaseFileName+lang.file)
		content := fmt.Sprintf("제목:\n%s\n\n설명:\n%s", title, desc)
		os.WriteFile(metaPath, []byte(content), 0644)
		log.Printf("✅ %s 제목/설명 번역 완료: %s", lang.name, metaPath)
	}

	absSubtitlePath, _ := filepath.Abs(session.SubtitlePath)
	absMetadataPath, _ := filepath.Abs(session.MetadataPath)

	finalMessage := fmt.Sprintf("🎉 모든 작업 완료!\n\n"+
		"📁 원본 파일: %s\n\n"+
		"✅ 완료된 작업:\n"+
		"- 한국어 자막 교정\n"+
		"- 10개 언어 자막 번역\n"+
		"- 11개 언어 제목/설명 생성\n\n"+
		"📂 PC 저장 위치:\n"+
		"- 자막: `%s`\n"+
		"- 제목/설명: `%s`",
		session.CurrentFile, absSubtitlePath, absMetadataPath)

	s.ChannelMessageEdit(session.ChannelID, msg.ID, finalMessage)
	log.Println("✅ 모든 번역 작업 완료!")

	sessionManager.DeleteSession(session.UserID)
}
