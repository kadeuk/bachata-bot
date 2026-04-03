package main

import (
	"sync"
)

// SessionState represents the current state of a user session
type SessionState string

const (
	StateIdle                    SessionState = "IDLE"
	StateWaitingFilename         SessionState = "WAITING_FILENAME"         // 파일명 입력 대기
	StateWaitingFile             SessionState = "WAITING_FILE"             // 파일 업로드 대기
	StateProcessing              SessionState = "PROCESSING"               // 1차 교정 진행 중
	StateWaitingTermCheck        SessionState = "WAITING_TERM_CHECK"       // 용어 확인 대기
	StateWaitingCorrection       SessionState = "WAITING_CORRECTION"       // 추가 수정 대기
	StateWaitingApproval         SessionState = "WAITING_APPROVAL"         // 최종 승인 대기
	StateTranslating             SessionState = "TRANSLATING"              // 번역 진행 중
	StateWaitingMetadataApproval SessionState = "WAITING_METADATA_APPROVAL" // 메타데이터 승인 대기
)

// TermSuggestion represents a term that needs user confirmation
type TermSuggestion struct {
	Timecode        string
	OriginalTerm    string
	SuggestedTerm   string
	Context         string
	Reasoning       string
	ConfidenceScore float64
	NeedsConfirm    bool
	QuestionToUser  string
}

// Session stores the state of a user's translation workflow
type Session struct {
	UserID             string
	ChannelID          string
	MessageID          string
	State              SessionState
	CurrentFile        string
	OriginalContent    string
	CorrectedKorean    string
	PendingTerms       []TermSuggestion
	CurrentTermIdx     int
	SubtitlePath       string
	MetadataPath       string
	BaseFileName       string
	VideoContext       string            // 영상 컨텍스트 (사용자 입력)
	Glossary           map[string]string // 수집된 단어장
	CorrectionApproved map[string]string // 승인된 교정 내역
	KoreanTitle        string            // 한국어 제목
	KoreanDescription  string            // 한국어 설명
}

// SessionManager manages user sessions
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewSessionManager creates a new session manager
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

// GetSession retrieves a session by user ID
func (sm *SessionManager) GetSession(userID string) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	session, exists := sm.sessions[userID]
	return session, exists
}

// CreateSession creates a new session for a user
func (sm *SessionManager) CreateSession(userID, channelID, messageID string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	session := &Session{
		UserID:    userID,
		ChannelID: channelID,
		MessageID: messageID,
		State:     StateIdle,
	}
	
	sm.sessions[userID] = session
	return session
}

// UpdateSession updates an existing session
func (sm *SessionManager) UpdateSession(userID string, session *Session) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.sessions[userID] = session
}

// DeleteSession removes a session
func (sm *SessionManager) DeleteSession(userID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, userID)
}

// SetState updates the session state
func (s *Session) SetState(state SessionState) {
	s.State = state
}

// GetCurrentTerm returns the current term being processed
func (s *Session) GetCurrentTerm() *TermSuggestion {
	if s.CurrentTermIdx < len(s.PendingTerms) {
		return &s.PendingTerms[s.CurrentTermIdx]
	}
	return nil
}

// NextTerm moves to the next term
func (s *Session) NextTerm() bool {
	s.CurrentTermIdx++
	return s.CurrentTermIdx < len(s.PendingTerms)
}

// HasMoreTerms checks if there are more terms to process
func (s *Session) HasMoreTerms() bool {
	return s.CurrentTermIdx < len(s.PendingTerms)
}
