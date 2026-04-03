# Bachata Bot 수정 이력 (AI가 읽을 수 있는 형식)

## 2026-03-18 14:30 - JSON 파싱 오류 해결 및 디버깅 시스템 추가

### 문제 상황
1. **JSON 파싱 오류 반복 발생**
   - 에러 메시지: `invalid character ',' after top-level value`
   - AI가 JSON 응답 앞뒤에 설명 텍스트를 포함하여 파싱 실패
   - 예시: `"교정이 필요한 부분들을 찾았습니다:\n\n[...]\n\n추가 설명"`

2. **원인 분석**
   - `cleanJSONResponse()` 함수가 JSON 시작 전 텍스트만 제거
   - JSON 끝 이후의 텍스트는 제거하지 않음
   - AI가 JSON 배열 끝에 추가 설명을 붙여서 응답

### 해결 방법

#### 1. cleanJSONResponse() 함수 개선
```go
// JSON 배열 끝 이후의 텍스트 제거 추가
if idx := strings.LastIndex(text, "]"); idx > 0 && idx < len(text)-1 {
    text = text[:idx+1]  // ] 까지만 자르기
} else if idx := strings.LastIndex(text, "}"); idx > 0 && idx < len(text)-1 {
    text = text[:idx+1]  // } 까지만 자르기
}
```

**작동 원리**:
- `LastIndex`로 마지막 `]` 또는 `}` 위치 찾기
- 그 위치 이후의 모든 텍스트 제거
- 순수한 JSON만 남김

#### 2. 디버깅 시스템 추가
**목적**: JSON 파싱 오류 발생 시 원인 파악을 위한 상세 로그

**생성되는 파일들** (`debug_logs/` 폴더):
1. `ai_response_YYYY-MM-DD_HH-MM-SS.txt` - AI 원본 응답
2. `cleaned_json_YYYY-MM-DD_HH-MM-SS.txt` - 정제된 JSON
3. `error_YYYY-MM-DD_HH-MM-SS.txt` - 에러 발생 시 상세 로그

**에러 로그 포맷**:
```
=== JSON 파싱 에러 ===
시간: 2026-03-18 14:30:00
에러: invalid character ',' after top-level value

원본 응답:
[전체 AI 응답]

정제된 JSON:
[cleanJSONResponse 처리 후]
```

#### 3. 콘솔 출력 개선
```
✓ AI 응답 수신 완료
🐛 디버그: 원본 AI 응답 저장됨 → debug_logs/ai_response_2026-03-18_14-30-00.txt
🐛 디버그: 정제된 JSON 저장됨 → debug_logs/cleaned_json_2026-03-18_14-30-00.txt
❌ AI 응답 파싱 에러: invalid character ',' after top-level value
응답 원본 (처음 500자):
[...]
정제된 JSON (처음 500자):
[...]
🐛 디버그: 에러 로그 저장됨 → debug_logs/error_2026-03-18_14-30-00.txt
```

### 추가 수정 사항

#### 1. 제목 글자 수 제한
- **변경**: 60자 → 100자 (공백 포함)
- **적용**: 한국어 및 모든 언어 번역

#### 2. 용어 변경
- "댄스강습" → "바차타 강습" 또는 "바차타 레슨"
- AI 프롬프트에 명시적 금지 규칙 추가

#### 3. 바차타 용어 사전 업데이트
**추가된 용어**:
- `하우(Rau)` - 브라질 슬랭, 토하듯이 하는 동작
- `프렙턴(Prep Turn)` - 준비턴 (Preparation Turn)

**수정된 용어**:
- `볼레로(Bolero)` - "프레퍼레이션 / 업" → 개별 항목으로 분리
  - ID 4: 프레퍼레이션(Preparation)
  - ID 17: 업(Up)

### 다음 AI가 참고할 사항

#### JSON 파싱 오류 발생 시 체크리스트
1. `debug_logs/` 폴더의 최신 파일 확인
2. `ai_response_*.txt` - AI가 실제로 뭘 응답했는지 확인
3. `cleaned_json_*.txt` - cleanJSONResponse가 제대로 작동했는지 확인
4. `error_*.txt` - 전체 에러 컨텍스트 확인

#### cleanJSONResponse 함수 로직
```
1. 마크다운 코드 블록 제거 (```json, ```)
2. JSON 시작 전 텍스트 제거 ([ 또는 { 이전)
3. JSON 끝 후 텍스트 제거 (] 또는 } 이후) ← 신규 추가!
4. 공백 정리
```

#### 알려진 AI 응답 패턴
- ✅ 정상: `[{...}, {...}]`
- ❌ 오류: `"설명...\n\n[{...}]"`
- ❌ 오류: `[{...}]\n\n추가 설명`
- ❌ 오류: `"설명...\n\n```json\n[{...}]\n```\n\n추가 설명"`

모든 패턴이 현재 `cleanJSONResponse`로 처리 가능!

### 파일 위치
- 소스 코드: `main.go`
- 용어 사전: `bachata_techniques.json`
- 디버그 로그: `debug_logs/` (자동 생성)
- 이 문서: `CHANGELOG_AI.md`

### 컴파일 및 실행
```bash
go run main.go
```

**필수 환경 변수**: `ANTHROPIC_API_KEY`
