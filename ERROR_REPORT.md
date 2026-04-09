# DeepSeek v3 마이그레이션 오류 분석 보고서

## 📋 개요
Bachata Bot의 AI 모델을 Gemini에서 DeepSeek v3로 마이그레이션한 후 발생하는 모든 오류를 분석한 보고서입니다.

## ✅ 완료된 작업
1. 코드베이스의 Gemini 참조를 DeepSeek으로 변경 완료
2. `deepseek.go` 파일 생성 및 `AIClient` 인터페이스 구현
3. `translator.go` 및 `corrector.go`에서 `t.aiClient`, `c.aiClient` 사용으로 변경
4. 프로그램 컴파일 및 기본 실행 테스트 완료

## ⚠️ 발견된 주요 오류

### 1. GitHub Actions 워크플로우 오류 (가장 심각)
**파일**: `.github/workflows/build.yml`
**문제**: 
- 라인 28, 34, 40, 46에서 여전히 `GeminiAPIKey`를 사용
- `-X main.GeminiAPIKey=${{ secrets.GEMINI_API_KEY }}` → `-X main.DeepSeekAPIKey=${{ secrets.DEEPSEEK_API_KEY }}`로 변경 필요
- Go 버전이 1.21으로 설정되어 있지만 `go.mod`에는 1.25.6이 지정됨

**영향**: 
- GitHub Actions로 빌드된 바이너리는 DeepSeek API 키를 받지 못해 실행 실패
- macOS와 Windows 모두에서 동일한 문제 발생

### 2. 환경 변수 설정 문제
**파일**: `.env.example`
**문제**: 
- `.env` 파일이 존재하지 않음 (샘플만 있음)
- 실제 실행 시 환경 변수 설정 필요

### 3. 코드 내 잠재적 문제점
**파일**: `translator.go`, `corrector.go`
**문제**:
- `DEEPSEEK_MIGRATION_REPORT.md`에 따르면 내부 구현이 완전히 마이그레이션되지 않았을 가능성 있음
- 하지만 실제 코드 검사에서는 `t.gemini`나 `c.gemini` 참조가 발견되지 않음

### 4. 모델 설정 문제 가능성
**파일**: `deepseek.go`
**문제**:
- 라인 65: `model := "deepseek-chat"`로 설정됨
- DeepSeek v3의 정확한 모델 이름 확인 필요 (`deepseek-chat` vs `deepseek-v3`)

## 🔧 테스트 결과

### 성공한 테스트:
1. `go build` - 컴파일 성공
2. 기본 실행 (환경 변수 없이) - 예상대로 Discord 토큰 오류
3. `--process-local` 모드 실행 - DeepSeek API 연결 시도 (인증 실패는 예상됨)

### 실패한 테스트:
1. GitHub Actions 빌드 - `GeminiAPIKey` 참조로 인해 실패 예상
2. 실제 DeepSeek API 키 없이 완전한 기능 테스트 불가

## 🎯 수정 필요 작업 목록

### 우선순위 1: GitHub Actions 워크플로우 수정
1. `build.yml` 파일에서 `GeminiAPIKey` → `DeepSeekAPIKey`로 변경
2. Go 버전을 1.21 → 1.25.6으로 업데이트
3. GitHub Secrets에 `DEEPSEEK_API_KEY` 추가 필요

### 우선순위 2: 환경 설정
1. 실제 `.env` 파일 생성 또는 환경 변수 설정
2. DeepSeek API 키 획득 및 설정

### 우선순위 3: 코드 검증
1. `translator.go`의 모든 AI 호출이 `t.aiClient`를 통해 이루어지는지 확인
2. `corrector.go`의 `ExtractCorrectionSuggestions` 메서드가 올바르게 JSON 응답을 처리하는지 확인
3. DeepSeek 모델 이름 확인 (`deepseek-chat`이 맞는지)

### 우선순위 4: 통합 테스트
1. 실제 DeepSeek API 키로 전체 기능 테스트
2. SRT 파일 처리 파이프라인 테스트
3. 다국어 번역 기능 테스트

## 📝 상세 수정 내용

### 1. GitHub Actions 워크플로우 수정 필요:
```yaml
# 변경 전:
-ldflags "-X main.DiscordToken=${{ secrets.DISCORD_TOKEN }} -X main.GeminiAPIKey=${{ secrets.GEMINI_API_KEY }}"

# 변경 후:
-ldflags "-X main.DiscordToken=${{ secrets.DISCORD_TOKEN }} -X main.DeepSeekAPIKey=${{ secrets.DEEPSEEK_API_KEY }}"
```

### 2. Go 버전 업데이트:
```yaml
# 변경 전:
go-version: '1.21'

# 변경 후:
go-version: '1.25'
```

### 3. GitHub Secrets 업데이트 필요:
- `GEMINI_API_KEY` → `DEEPSEEK_API_KEY`로 변경
- 실제 DeepSeek API 키 설정

## 🚀 다음 단계

1. **수정 승인 요청**: 이 보고서를 검토한 후 수정을 진행할지 결정
2. **워크플로우 수정**: `build.yml` 파일 수정
3. **테스트**: 수정 후 GitHub Actions 실행 테스트
4. **배포**: 새로운 릴리스 생성 및 배포

## 💡 참고사항

- 현재 코드베이스는 기본적으로 정상적으로 컴파일되고 실행됨
- 주요 문제는 빌드 시스템(GitHub Actions)과의 불일치
- DeepSeek API 키가 없어 완전한 기능 테스트는 불가능했음
- macOS와 Windows 모두 동일한 문제가 발생할 것으로 예상됨

## 📞 문제 해결을 위한 추가 정보 필요

1. 실제 DeepSeek API 키가 있는지?
2. GitHub Secrets에 `DEEPSEEK_API_KEY`가 설정되어 있는지?
3. 이전에 Gemini API로 정상 작동했는지?

이 보고서를 바탕으로 수정 작업을 진행하시겠습니까?