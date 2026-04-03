# 바차타 자막 번역 봇 - 작업 인수인계 보고서

## 📋 프로젝트 개요
- **프로젝트명**: 바차타 자막 번역 Discord Bot
- **언어**: Go (Golang)
- **주요 라이브러리**: 
  - `github.com/bwmarrin/discordgo` (Discord Bot)
  - `github.com/google/generative-ai-go/genai` (Gemini API)
  - `github.com/joho/godotenv` (환경변수)
- **AI 모델**: 
  - Gemini 2.5 Pro (현재 사용 중)
  - Gemini 1.5 Flash (향후 1차 교정용)

---

## 🎯 프로젝트 목표

### 최종 목표
**Discord 대화형 봇**으로 바차타 강습 영상 자막을 자동 교정 및 11개 언어로 번역

### 핵심 요구사항
1. **Discord 파일 업로드 감지** → SRT 파일 자동 다운로드
2. **상태 머신 기반 대화형 워크플로우**
3. **2단계 파이프라인**: 1차 교정 → 사용자 승인 → 2차 번역
4. **병렬 처리(Goroutine)**: 속도 향상 + 순서 보장
5. **단어장 자동 수집**: `glossary_draft.json` 생성
6. **자막 누락 0%**: 재시도 로직 + 검증

---

## 📁 현재 파일 구조

```
bachata-bot/
├── main.go                    # Discord Bot 메인 로직 (상태 머신)
├── session.go                 # 세션 상태 관리
├── gemini.go                  # Gemini API 클라이언트
├── translator.go              # 번역 로직 (재시도 + 검증)
├── srt_processor.go           # SRT 파싱/포맷팅
├── techniques.go              # 바차타 용어 사전 관리
├── text_cleaner.go            # 정규표현식 텍스트 정리
├── bachata_techniques.json    # 바차타 용어 사전 (19개 항목)
├── .env                       # API 키 (DISCORD_TOKEN, GEMINI_API_KEY)
└── 번역전/                    # 입력 SRT 파일 폴더
```

---

## ✅ 완료된 작업

### 1. Discord Bot 기본 구조 ✅
- `main.go`: Discord 연결 및 메시지 핸들러
- `session.go`: 사용자별 세션 상태 관리
- 명령어: `!번역실행`, `!승인`

### 2. Gemini API 통합 ✅
- `gemini.go`: Gemini 2.5 Pro 연결
- 모델명: `models/gemini-2.5-pro`
- 재시도 로직 (3회)

### 3. SRT 처리 시스템 ✅
- `srt_processor.go`: 파싱, 청크 분할, 병합
- 청크 크기: 60개 항목
- 타임코드 검증 로직

### 4. 번역 파이프라인 ✅
- `translator.go`: 한국어 교정 + 10개 언어 번역
- 재시도 로직 (3회) + 검증
- 누락 방지: 원본 사용 fallback

### 5. 텍스트 정리 ✅
- `text_cleaner.go`: 정규표현식 기반
- 불필요한 숫자, 쉼표, 공백 제거

### 6. 바차타 용어 사전 ✅
- `bachata_techniques.json`: 19개 용어
- `techniques.go`: 용어 필터링 및 프롬프트 생성

---

## ⚠️ 발견된 문제 및 해결

### 문제 1: 자막 누락 (Entry Count Mismatch)
**증상**: AI가 첫 번째 자막 항목을 누락
**원인**: Gemini API 응답 불완전
**해결**: 
- 재시도 로직 (3회)
- 검증 실패 시 원본 사용
- 프롬프트에 "모든 자막 항목을 빠짐없이 출력하세요" 추가

### 문제 2: Gemini 모델명 오류
**증상**: `models/gemini-1.5-pro` 404 에러
**원인**: v1beta API에서 지원 안 함
**해결**: `models/gemini-2.5-pro`로 변경

### 문제 3: 크로스 플랫폼 경로
**증상**: Windows/Mac 경로 차이
**해결**: `path/filepath` 패키지 사용

---

## 🚧 미완성 작업 (다음 AI가 해야 할 일)

### 🔴 최우선 작업: Discord 대화형 봇 완성

#### 1. 파일 업로드 감지 및 다운로드
```go
// main.go에 추가 필요
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
    // 파일 첨부 확인
    if len(m.Attachments) > 0 {
        for _, attachment := range m.Attachments) {
            if strings.HasSuffix(attachment.Filename, ".srt") {
                // 파일 다운로드
                // 세션 생성
                // STATE_WAITING_INFO로 전환
            }
        }
    }
}
```

#### 2. 상태 머신 확장
**현재 상태**:
- `StateIdle`
- `StateProcessing`
- `StateWaitingApproval`

**추가 필요**:
```go
const (
    StateIdle              SessionState = "IDLE"
    StateWaitingInfo       SessionState = "WAITING_INFO"        // 새로 추가
    StateProcessing1       SessionState = "PROCESSING_1"        // 새로 추가
    StateWaitingApproval   SessionState = "WAITING_APPROVAL"
    StateFeedback          SessionState = "FEEDBACK"            // 새로 추가
    StateTranslating       SessionState = "TRANSLATING"         // 새로 추가
)
```

#### 3. 대화 시나리오 구현

**시나리오 1: 파일 업로드**
```
사용자: [SRT 파일 업로드]
봇: "파일을 받았습니다! 영상 제목과 핵심 내용을 알려주세요."
→ STATE_WAITING_INFO
```

**시나리오 2: 컨텍스트 입력**
```
사용자: "힙쓰로우 배우기 영상입니다"
봇: "🤔 AI가 자막을 분석하고 있습니다..."
→ STATE_PROCESSING_1
```

**시나리오 3: 1차 교정 완료**
```
봇: [corrected_ko.srt 업로드]
봇: [glossary_draft.json 업로드]
봇: "교정이 완료되었습니다. 승인(Y)하시겠습니까? 수정할 부분이 있으면 자연어로 말씀해주세요."
→ STATE_WAITING_APPROVAL
```

**시나리오 4: 피드백 처리**
```
사용자: "'하우'는 전부 'Rau'로 바꿔줘"
봇: "수정 중입니다..."
봇: [수정된 파일 재업로드]
→ STATE_WAITING_APPROVAL (다시)
```

**시나리오 5: 승인 및 번역**
```
사용자: "Y" 또는 "승인"
봇: "🌍 10개 언어 번역을 시작합니다..."
→ STATE_TRANSLATING
```

---

### 🔴 2단계 파이프라인 구현

#### Phase 1: Gemini 1.5 Flash (교정 + 단어장)

**새 파일 생성 필요**: `corrector.go`

```go
package main

type Corrector struct {
    gemini *GeminiClient
}

// CorrectionResult represents the JSON response
type CorrectionResult struct {
    ID            int               `json:"id"`
    CorrectedText string            `json:"corrected_text"`
    Changes       map[string]string `json:"changes"`
}

func (c *Corrector) CorrectWithGlossary(entries []SRTEntry, context string) ([]CorrectionResult, error) {
    // 프롬프트 생성
    prompt := fmt.Sprintf(`You are a Korean subtitle corrector for Bachata dance videos.

**Context**: %s

**Task**: Correct the following subtitles. Return strictly as a JSON array.

**Important Rules**:
1. Return EXACTLY the same number of entries as input
2. Each entry must have: "id", "corrected_text", "changes"
3. "changes" object contains ONLY the words you modified (original: corrected)
4. Do not drop any lines
5. Preserve all timecodes

**Input Subtitles**:
%s

**Output Format**:
[
  {
    "id": 1,
    "corrected_text": "교정된 문장",
    "changes": {"원본단어": "수정단어"}
  }
]`, context, FormatSRT(entries))

    // Gemini 1.5 Flash 호출
    model := c.gemini.client.GenerativeModel("models/gemini-1.5-flash")
    response, err := model.GenerateContent(c.gemini.ctx, genai.Text(prompt))
    
    // JSON 파싱
    var results []CorrectionResult
    json.Unmarshal([]byte(cleanJSON), &results)
    
    return results, nil
}
```

#### Phase 2: 단어장 수집

**새 파일 생성 필요**: `glossary.go`

```go
package main

type Glossary struct {
    Entries map[string]string `json:"entries"`
}

func (g *Glossary) AddChanges(changes map[string]string) {
    for original, corrected := range changes {
        g.Entries[original] = corrected
    }
}

func (g *Glossary) SaveToFile(path string) error {
    data, _ := json.MarshalIndent(g, "", "  ")
    return os.WriteFile(path, data, 0644)
}
```

---

### 🔴 병렬 처리 (Goroutine) 구현

**새 파일 생성 필요**: `parallel_processor.go`

```go
package main

import (
    "sync"
)

type ParallelProcessor struct {
    maxWorkers int
    semaphore  chan struct{}
}

func NewParallelProcessor(maxWorkers int) *ParallelProcessor {
    return &ParallelProcessor{
        maxWorkers: maxWorkers,
        semaphore:  make(chan struct{}, maxWorkers),
    }
}

type ChunkResult struct {
    Index   int
    Entries []SRTEntry
    Error   error
}

func (pp *ParallelProcessor) ProcessChunks(chunks [][]SRTEntry, processFn func([]SRTEntry) ([]SRTEntry, error)) ([]SRTEntry, error) {
    results := make([]ChunkResult, len(chunks))
    var wg sync.WaitGroup
    
    for i, chunk := range chunks {
        wg.Add(1)
        go func(index int, c []SRTEntry) {
            defer wg.Done()
            
            // Semaphore: 동시 실행 제어
            pp.semaphore <- struct{}{}
            defer func() { <-pp.semaphore }()
            
            // 처리
            processed, err := processFn(c)
            results[index] = ChunkResult{
                Index:   index,
                Entries: processed,
                Error:   err,
            }
        }(i, chunk)
    }
    
    wg.Wait()
    
    // 순서대로 정렬 (중요!)
    var merged []SRTEntry
    for i := 0; i < len(results); i++ {
        if results[i].Error != nil {
            return nil, results[i].Error
        }
        merged = append(merged, results[i].Entries...)
    }
    
    return merged, nil
}
```

---

### 🔴 메타데이터 생성 규칙 업데이트

**수정 필요**: `techniques.go` - `BuildMetadataPrompt()`

```go
func (tm *TechniqueManager) BuildMetadataPrompt() string {
    return "아래 바차타 강습 자막을 바탕으로 유튜브 제목과 설명을 작성하세요.\n\n" +
        "**제목 요구사항:**\n" +
        "- 공백 포함 99자 이내 (엄수!)\n" +
        "- 반드시 '패턴'과 '바차타' 단어 포함\n" +
        "- SEO 최적화 키워드 포함\n" +
        "- 클릭을 유도하는 매력적인 문구\n\n" +
        "**설명 요구사항:**\n" +
        "- 600~900자 분량 (엄수!)\n" +
        "- 반드시 '패턴'과 '바차타' 단어 포함\n" +
        "- SEO 키워드 다량 포함 (바차타, 센슈얼바차타, 바차타강습, 파트너워크, 리딩, 팔로잉 등)\n" +
        "- 구체적인 학습 내용을 절 번호(1., 2., 3.)로 간결하게 구조화\n" +
        "- 초보자도 이해하기 쉬운 친절한 설명\n\n" +
        "**출력 형식 (JSON):**\n" +
        "{\n" +
        "  \"title\": \"유튜브 제목 (99자 이내)\",\n" +
        "  \"description\": \"유튜브 설명 (600~900자)\"\n" +
        "}\n"
}
```

---

## 🔧 환경 설정

### .env 파일
```env
DISCORD_TOKEN=your_discord_bot_token_here
GEMINI_API_KEY=your_gemini_api_key_here
```

### 필수 Go 패키지
```bash
go get github.com/bwmarrin/discordgo
go get github.com/google/generative-ai-go/genai
go get github.com/joho/godotenv
go get google.golang.org/api/option
```

---

## 📝 중요 지시사항 (절대 지켜야 함)

### 1. 자막 누락 방지
- **모든 청크 처리 후 항목 수 검증**
- 원본 354개 → 교정본 354개 (정확히 일치)
- 불일치 시 재시도 (최대 3회)
- 3회 실패 시 원본 사용

### 2. 순서 보장
- 병렬 처리 시 **반드시 ID 기반 정렬**
- `results[index]` 방식으로 순서 보존
- 최종 병합 시 인덱스 순서대로

### 3. Discord 파일 처리
- 파일 업로드 감지: `m.Attachments`
- 파일 다운로드: `http.Get(attachment.URL)`
- 파일 업로드: `s.ChannelFileSend(channelID, filename, file)`

### 4. 상태 머신 엄수
- 모든 사용자 입력은 현재 상태에 따라 처리
- 상태 전환 시 반드시 `session.SetState()` 호출
- 세션 저장: `sessionManager.UpdateSession()`

### 5. 에러 처리
- 모든 API 호출에 재시도 로직
- 사용자에게 친절한 에러 메시지
- 로그에 상세한 디버그 정보

### 6. 메타데이터 규칙
- 제목: 99자 이내, "패턴" + "바차타" 필수
- 설명: 600~900자, "패턴" + "바차타" 필수
- SEO 최적화 키워드 포함

---

## 🎯 다음 AI가 해야 할 작업 순서

### Step 1: 파일 업로드 처리 구현
1. `main.go`에 파일 첨부 감지 로직 추가
2. SRT 파일 다운로드 함수 작성
3. 세션 생성 및 STATE_WAITING_INFO 전환

### Step 2: 컨텍스트 입력 처리
1. STATE_WAITING_INFO 핸들러 작성
2. 사용자 입력을 세션에 저장
3. STATE_PROCESSING_1로 전환

### Step 3: 1차 교정 시스템 구현
1. `corrector.go` 파일 생성
2. Gemini 1.5 Flash 연동
3. JSON 응답 파싱
4. `corrected_ko.srt` 생성

### Step 4: 단어장 수집 구현
1. `glossary.go` 파일 생성
2. `changes` 객체 누적
3. `glossary_draft.json` 저장

### Step 5: 병렬 처리 구현
1. `parallel_processor.go` 파일 생성
2. Goroutine + Semaphore 패턴
3. ID 기반 순서 정렬

### Step 6: 피드백 처리 구현
1. STATE_FEEDBACK 핸들러 작성
2. 자연어 피드백 → Gemini 전달
3. 단어장 업데이트 및 재교정

### Step 7: 2차 번역 구현
1. Gemini 2.5 Pro 사용
2. 병렬 처리 적용
3. 11개 언어 번역

### Step 8: 메타데이터 규칙 업데이트
1. `techniques.go` 수정
2. 99자 제한 검증
3. "패턴", "바차타" 포함 검증

### Step 9: 통합 테스트
1. 전체 워크플로우 테스트
2. 에러 케이스 처리
3. 성능 최적화

---

## 📊 현재 코드 통계

- **총 파일 수**: 8개
- **총 코드 라인**: ~1,500줄
- **완성도**: 약 40%
- **남은 작업**: 약 60%

---

## 🚨 주의사항

1. **절대 CLI로 도망치지 마세요** - Discord 대화형 봇이 최종 목표
2. **상태 머신 패턴 필수** - map으로 세션 관리
3. **자막 누락 0%** - 검증 로직 철저히
4. **순서 보장** - 병렬 처리 시 ID 기반 정렬
5. **메타데이터 규칙 엄수** - 99자, 600~900자, "패턴"+"바차타" 필수

---

## 📞 문의사항

이 보고서를 읽고 불명확한 부분이 있으면 코드 주석과 기존 구현을 참고하세요.

**화이팅! 🚀**
