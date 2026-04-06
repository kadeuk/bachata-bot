# DeepSeek 모델 마이그레이션 작업 보고서

## 📋 개요
이 보고서는 Bachata Bot의 AI 모델을 Gemini에서 DeepSeek으로 마이그레이션한 작업 내용을 상세히 기록합니다. 다음 AI가 남은 작업을 완료할 수 있도록 모든 변경사항과 필요한 작업을 명시합니다.

## 🎯 목표
- Gemini API에서 DeepSeek API로 전환
- 가성비 좋은 DeepSeek-V3 모델 사용
- 기존 기능 유지하면서 비용 절감
- 한국어 바차타 자막 교정/번역 성능 유지

## ✅ 완료된 작업

### 1. 새로운 파일 생성

#### `deepseek.go` - DeepSeek 클라이언트 구현
```go
// 주요 기능:
// - DeepSeek-V3 (deepseek-chat) 모델 사용
// - AIClient 인터페이스 구현
// - 재시도 로직 (최대 3회)
// - HTTP 타임아웃 120초 설정
// - 토큰 사용량 로깅
// - JSON 응답 파싱 및 정제
```

#### `ai_client.go` - AI 클라이언트 인터페이스 정의
```go
// AIClient 인터페이스:
// - GenerateContent(prompt string) (string, error)
// - GenerateContentWithSystemPrompt(systemPrompt, userPrompt string) (string, error)
// - PostProcessNumbers(content string) (string, error)
// - CountTokens(text string) (int, error)
// - Close() error
//
// 구현체:
// - GeminiClient (기존)
// - DeepSeekClient (신규)
```

### 2. 주요 파일 수정

#### `main.go` - 메인 애플리케이션
```diff
- var geminiClient *GeminiClient
+ var deepseekClient *DeepSeekClient

- GeminiAPIKey string
+ DeepSeekAPIKey string

- geminiClient, err = NewGeminiClient(geminiAPIKey)
+ deepseekClient, err = NewDeepSeekClient(deepseekAPIKey)

- translator = NewTranslator(geminiClient, ...)
+ translator = NewTranslator(deepseekClient, ...)

- corrector = NewCorrector(geminiClient, ...)
+ corrector = NewCorrector(deepseekClient, ...)

// 모든 geminiClient 참조를 deepseekClient로 변경 완료
```

#### `.env` - 환경 변수
```diff
# Gemini API Key
# Get from: https://aistudio.google.com/app/apikey
- GEMINI_API_KEY=AIzaSyD7qEq4tU9M56PiMHJAKcV1vvE1UT2btfE

# DeepSeek API Key
# Get from: https://platform.deepseek.com/api_keys
+ DEEPSEEK_API_KEY=sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

#### `translator.go` - 번역기
```diff
// 구조체 변경:
- gemini *GeminiClient
+ aiClient AIClient

// 생성자 변경:
- func NewTranslator(gemini *GeminiClient, ...)
+ func NewTranslator(aiClient AIClient, ...)
```

#### `corrector.go` - 교정기
```diff
// 구조체 변경:
- gemini *GeminiClient
+ aiClient AIClient

// 생성자 변경:
- func NewCorrector(gemini *GeminiClient, ...)
+ func NewCorrector(aiClient AIClient, ...)
```

### 3. 모델 선택
- **주요 모델**: `deepseek-chat` (DeepSeek-V3)
- **대체 모델**: `deepseek-coder`, `deepseek-math`
- **선택 이유**:
  - 한국어 처리 능력 우수
  - 128K 토큰 컨텍스트
  - Gemini Pro 대비 가성비 좋음
  - API 제한 완화

## ⚠️ 부분적으로 완료된 작업 (추가 작업 필요)

### 1. `translator.go` - 내부 구현 수정 필요
**현재 상태**: 구조체와 생성자는 수정되었지만, 내부 메서드들이 여전히 Gemini SDK를 직접 사용합니다.

**수정해야 할 메서드**:
1. `CorrectKoreanSRT()` - 라인 50-130
   ```go
   // 문제: Gemini SDK 직접 사용 (genai 패키지)
   flashModel := t.gemini.client.GenerativeModel("gemini-2.5-pro")
   resp, err := flashModel.GenerateContent(t.gemini.ctx, genai.Text(userPrompt))
   
   // 해결: AIClient 인터페이스 사용으로 변경
   response, err := t.aiClient.GenerateContentWithSystemPrompt(systemPrompt, userPrompt)
   ```

2. `TranslateToLanguage()` - 라인 155-250
   ```go
   // 문제: Gemini SDK 직접 사용
   flashModel := t.gemini.client.GenerativeModel("gemini-2.5-flash")
   resp, err := flashModel.GenerateContent(t.gemini.ctx, genai.Text(userPrompt))
   
   // 해결: AIClient 인터페이스 사용
   ```

3. `GenerateMetadata()` - 라인 274-310
   ```go
   // 문제: gemini 직접 참조
   response, err := t.gemini.GenerateContentWithSystemPrompt(...)
   
   // 해결: aiClient 사용
   response, err := t.aiClient.GenerateContentWithSystemPrompt(...)
   ```

4. `TranslateMetadata()` - 라인 331-400
   ```go
   // 문제: gemini 직접 참조
   response, err := t.gemini.GenerateContent(prompt)
   
   // 해결: aiClient 사용
   response, err := t.aiClient.GenerateContent(prompt)
   ```

### 2. `corrector.go` - 내부 구현 수정 필요
**현재 상태**: 구조체와 생성자는 수정되었지만, 내부 메서드들이 Gemini SDK를 직접 사용합니다.

**수정해야 할 메서드**:
1. `ExtractCorrectionSuggestions()` - 라인 48-120
   ```go
   // 문제: Gemini SDK 직접 사용
   flashModel := c.gemini.client.GenerativeModel("gemini-2.5-pro")
   resp, err := flashModel.GenerateContent(c.gemini.ctx, genai.Text(prompt))
   
   // 해결: AIClient 인터페이스 사용
   // 주의: 이 메서드는 JSON 배열을 반환하는 특수한 형식이므로
   // DeepSeekClient에 유사한 메서드를 추가하거나 프롬프트를 조정해야 함
   ```

### 3. `gemini.go` - 유지보수
- 현재 상태: 그대로 유지 (AIClient 인터페이스 구현)
- 필요시 Gemini로 폴백 가능
- DeepSeekClient와 동일한 인터페이스 제공

## 🔧 컴파일 에러 해결 필요

### 현재 컴파일 에러 목록:
1. `translator.go`:
   - `t.gemini` 참조 에러 (라인 78, 158, 192, 277, 334)
   - 미정의 변수: `chunkText`, `lastErr`, `index`, `attempt`, `maxRetries`
   - `continue` 문 에러

2. `corrector.go`:
   - `c.gemini` 참조 에러 (라인 48, 117)

### 해결 방안:
1. **간단한 해결책**: `t.gemini` → `t.aiClient`로 일괄 변경
2. **완전한 해결책**: Gemini SDK 코드를 AIClient 인터페이스 호출로 재작성

## 📝 DeepSeekClient 추가 구현 필요

### 현재 구현된 메서드:
1. `GenerateContent(prompt string) (string, error)`
2. `GenerateContentWithSystemPrompt(systemPrompt, userPrompt string) (string, error)`
3. `PostProcessNumbers(content string) (string, error)`
4. `CountTokens(text string) (int, error)`
5. `Close() error`

### 추가 필요할 수 있는 메서드:
1. **JSON 배열 응답 전문 메서드**:
   ```go
   func (dc *DeepSeekClient) GenerateJSONArray(prompt string) ([]map[string]interface{}, error)
   ```
   - `corrector.go`의 `ExtractCorrectionSuggestions()`에서 필요
   - Gemini는 구조화된 JSON 배열을 잘 반환하지만 DeepSeek도 가능

2. **스트리밍 응답 지원** (선택사항):
   ```go
   func (dc *DeepSeekClient) GenerateContentStream(prompt string, callback func(chunk string)) error
   ```

## 🎯 프롬프트 최적화 필요

### Gemini vs DeepSeek 프롬프트 차이:
1. **시스템 프롬프트 형식**:
   - Gemini: `SystemInstruction` 필드 사용
   - DeepSeek: 메시지 배열의 `role: "system"` 사용

2. **JSON 응답 형식**:
   - Gemini: 마크다운 코드 블록에 JSON 포함 경향
   - DeepSeek: 순수 JSON 반환 경향 (테스트 필요)

3. **한국어 이해도**:
   - DeepSeek-V3 한국어 능력 테스트 필요
   - 필요시 프롬프트에 한국어 지시 강화

### 테스트 필요 프롬프트:
1. 자막 교정 프롬프트 (`translator.go`)
2. 교정 제안 추출 프롬프트 (`corrector.go`)
3. 번역 프롬프트 (`translator.go`)
4. 메타데이터 생성 프롬프트 (`translator.go`)

## 🧪 테스트 계획

### 1단계: 컴파일 테스트
```bash
go build -o bachata-bot.exe
```

### 2단계: 기본 기능 테스트
```bash
# .env에 실제 DeepSeek API 키 설정 후
bachata-bot.exe
# Discord Bot 시작 확인
```

### 3단계: 통합 테스트
1. SRT 파일 업로드 테스트
2. 한국어 교정 기능 테스트
3. 번역 기능 테스트
4. 메타데이터 생성 테스트

### 4단계: 성능 비교 테스트
- Gemini vs DeepSeek 응답 품질 비교
- 응답 시간 측정
- 토큰 사용량 비교
- 비용 효율성 분석

## 📊 파일 변경 요약

### 신규 생성 파일:
1. `deepseek.go` - DeepSeek 클라이언트 구현
2. `ai_client.go` - AI 클라이언트 인터페이스
3. `DEEPSEEK_MIGRATION_REPORT.md` - 이 보고서

### 수정된 파일:
1. `main.go` - 주요 참조 변경
2. `.env` - 환경 변수 변경
3. `translator.go` - 구조체/생성자 변경 (내부 구현 미완)
4. `corrector.go` - 구조체/생성자 변경 (내부 구현 미완)

### 변경되지 않은 파일 (Gemini 의존성 유지):
1. `gemini.go` - Gemini 클라이언트 (AIClient 구현)
2. 기타 유틸리티 파일들

## 🚀 다음 AI를 위한 작업 체크리스트

### 우선순위 1: 컴파일 에러 해결
- [ ] `translator.go`의 `t.gemini` 참조를 `t.aiClient`로 변경
- [ ] `corrector.go`의 `c.gemini` 참조를 `c.aiClient`로 변경
- [ ] 미정의 변수 오류 해결
- [ ] `go build` 성공 확인

### 우선순위 2: DeepSeekClient 기능 보완
- [ ] JSON 배열 응답 지원 메서드 추가 (선택사항)
- [ ] 프롬프트 최적화 테스트
- [ ] 에러 처리 강화

### 우선순위 3: 통합 테스트
- [ ] 실제 DeepSeek API 키로 테스트
- [ ] SRT 파일 처리 전流程 테스트
- [ ] 응답 품질 검증

### 우선순위 4: 문서화
- [ ] API 키 획득 방법 문서화
- [ ] 트러블슈팅 가이드 작성
- [ ] 비용 비교 분석

## 💡 중요한 참고사항

### 1. API 키 보안
- `.env` 파일의 예시 키는 더미 값
- 실제 DeepSeek API 키는 [platform.deepseek.com](https://platform.deepseek.com/api_keys)에서 획득
- GitHub에 실제 키 커밋 금지

### 2. 폴백 전략
- 현재 구조는 AIClient 인터페이스를 통해 Gemini/DeepSeek 전환 가능
- 필요시 `main.go`에서 클라이언트 선택 로직 추가 가능
- 예: DeepSeek 실패 시 Gemini로 자동 전환

### 3. 비용 고려사항
- DeepSeek은 토큰 기반 과금
- 대용량 처리 시 토큰 사용량 모니터링 필요
- 무료 티어 제한 확인 필요

### 4. 한국어 특화
- DeepSeek-V3의 한국어 능력 테스트 필수
- 바차타 전문 용어 처리 능력 검증
- 필요시 용어사전 기반 프롬프트 강화

## 📞 문제 발생 시 참고

### 일반적인 문제:
1. **API 연결 실패**: API 키 확인, 네트워크 연결 확인
2. **응답 형식 오류**: 프롬프트 조정, JSON 파싱 로직 검토
3. **성능 저하**: 모델 변경, 프롬프트 최적화

### DeepSeek 특정 문제:
1. **속도 문제**: `deepseek-chat` 대신 `deepseek-coder` 시도
2. **한국어 품질**: 프롬프트에 한국어 지시 강화
3. **토큰 제한**: 청크 크기 조정, 토큰 수 줄이기

## 🎉 마무리
이 마이그레이션 작업은 Bachata Bot의 장기적인 비용 절감과 성능 향상을 목표로 합니다. DeepSeek의 가성비와 한국어 처리 능력을 활용하여 바차타 자막 교정/번역 서비스를 지속적으로 개선할 수 있을 것입니다.

**다음 AI는 이 보고서를 참고하여 남은 작업을 완료해 주시기 바랍니다.**