# 바차타 봇 리팩토링 계획서

## 목표
CLI 기반 → Discord Bot 기반으로 전환하고, Claude API → Gemini 3.1 Pro로 교체

## 아키텍처 변경

### Before (현재)
```
사용자 → PowerShell CLI → main.go → Claude API → 로컬 파일 저장
         ↑ 사용자 입력 대기 (Scanln)
```

### After (목표)
```
사용자 → Discord 명령어 (!번역실행) → Discord Bot → Gemini API → 로컬 파일 저장
                                      ↓
                                Discord 채팅으로 진행상황 알림
```

## 파일 구조

```
bachata-bot/
├── main.go                    # Discord Bot 메인 (새로 작성)
├── gemini.go                  # Gemini API 호출 로직
├── translator.go              # 번역 로직 (청크 처리)
├── techniques.go              # 바차타 용어 사전 관리
├── srt_processor.go           # SRT 파일 파싱/병합
├── bachata_techniques.json    # 용어 사전 (기존 유지)
├── 용어사전.txt                # 필터링용 사전 (기존 유지)
├── go.mod                     # 의존성 관리
└── .env                       # 환경 변수 (GEMINI_API_KEY, DISCORD_TOKEN)
```

## 핵심 변경 사항

### 1. Discord Bot 통합
```go
// main.go
- Discord 봇 초기화 (discordgo 라이브러리)
- 명령어 핸들러: !번역실행
- 진행상황 메시지 전송 함수
```

### 2. Gemini API 통합
```go
// gemini.go
- Claude API 코드 완전 제거
- google/generative-ai-go/genai 사용
- 환경 변수: GEMINI_API_KEY
- 모델: gemini-3.1-pro
```

### 3. 청크 처리 로직
```go
// translator.go
- SRT를 50~80라인 단위로 분할
- 각 청크를 Gemini API로 번역
- 결과를 순서대로 병합
- 타임코드 보존 검증
```

### 4. 자동화된 용어 치환
```go
// techniques.go
- 사용자 입력 없이 자동 치환
- 중복 영어 병기 방지 로직
- "프렙턴(Prep Turn)" → "프렙턴(Prep Turn)" (중복 방지)
```

### 5. 프롬프트 최적화
```
- "이미 영어가 괄호 안에 있으면 절대 다시 추가하지 마세요"
- "원본: 프렙턴(Prep Turn) → 출력: 프렙턴(Prep Turn)" (그대로 유지)
- "원본: 프랩턴 → 출력: 프렙턴(Prep Turn)" (교정만)
```

## 구현 순서

### Phase 1: 기본 구조 (1-2시간)
1. ✅ Discord Bot 기본 설정
2. ✅ Gemini API 연동
3. ✅ 명령어 핸들러 구현

### Phase 2: 핵심 로직 (2-3시간)
4. ✅ SRT 청크 분할/병합 로직
5. ✅ Gemini API 호출 (청크별)
6. ✅ 용어 사전 자동 치환

### Phase 3: 통합 및 테스트 (1-2시간)
7. ✅ 전체 워크플로우 통합
8. ✅ 에러 핸들링
9. ✅ Discord 메시지 포맷팅

## 의존성 패키지

```bash
go get github.com/bwmarrin/discordgo
go get github.com/google/generative-ai-go/genai
go get github.com/joho/godotenv
go get google.golang.org/api/option
```

## 환경 변수 (.env)

```
GEMINI_API_KEY=your_gemini_api_key_here
DISCORD_TOKEN=your_discord_bot_token_here
```

## 제거할 코드

1. ❌ `fmt.Scanln`, `bufio.Scanner` 등 모든 사용자 입력 대기
2. ❌ `debug_logs` 폴더 생성 로직
3. ❌ Claude API 관련 코드 전체
4. ❌ `cleanJSONResponse` 함수 (Gemini는 더 안정적)
5. ❌ 대화형 교정 루프 (자동화)

## 유지할 코드

1. ✅ 바차타 용어 사전 (`bachata_techniques.json`)
2. ✅ 용어 필터링 로직
3. ✅ 로컬 파일 시스템 저장 로직
4. ✅ 날짜별 폴더 생성
5. ✅ 토큰 사용량 계산

## 예상 워크플로우

```
1. 사용자: Discord에서 "!번역실행" 입력
2. 봇: "🎯 번역을 시작합니다..." 메시지 전송
3. 봇: 로컬 "번역전" 폴더에서 SRT 파일 읽기
4. 봇: "📝 STEP 1: 한국어 교정 중... (1/355)" 진행상황 업데이트
5. 봇: 청크별로 Gemini API 호출 (50라인씩)
6. 봇: "🌍 STEP 2: 영어 번역 중... (1/355)" 진행상황 업데이트
7. 봇: 10개 언어 모두 번역 완료
8. 봇: 로컬에 "2026-03-18/자막번역완성/파일명/" 폴더 생성
9. 봇: "✅ 완료! 저장 위치: C:\Users\Admin\bachata-bot\2026-03-18\..." 메시지 전송
```

## 성능 최적화

- 청크 크기: 50~80 라인 (실험적으로 조정)
- 병렬 처리: 언어별 번역은 순차 처리 (API 제한 고려)
- 재시도 로직: API 실패 시 3회 재시도
- 타임아웃: 각 API 호출당 60초

## 테스트 계획

1. 단위 테스트: SRT 청크 분할/병합
2. 통합 테스트: 전체 워크플로우 (355라인 자막)
3. 엣지 케이스: 
   - 빈 파일
   - 잘못된 SRT 형식
   - API 오류
   - 네트워크 끊김

## 예상 소요 시간

- 코드 작성: 4-6시간
- 테스트 및 디버깅: 2-3시간
- 총 예상: 6-9시간

## 다음 단계

1. Discord Bot 토큰 발급
2. Gemini API 키 발급
3. 코드 작성 시작
