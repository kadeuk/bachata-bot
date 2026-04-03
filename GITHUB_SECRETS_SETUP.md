# GitHub Secrets 설정 가이드

## 개요
이 프로젝트는 API 키를 안전하게 관리하기 위해 GitHub Secrets를 사용합니다.
빌드 시 `-ldflags`를 통해 API 키가 실행 파일에 주입되므로, 별도의 `.env` 파일 없이 실행 파일 하나만으로 동작합니다.

## GitHub Secrets 등록 방법

### 1. GitHub 저장소 설정 페이지로 이동
1. GitHub에서 `https://github.com/kadeuk/bachata-bot` 저장소로 이동
2. 상단 메뉴에서 **Settings** 클릭
3. 왼쪽 사이드바에서 **Secrets and variables** → **Actions** 클릭

### 2. 필요한 Secrets 등록

다음 2개의 Secret을 등록해야 합니다:

#### Secret 1: DISCORD_TOKEN
- **Name**: `DISCORD_TOKEN`
- **Value**: Discord Bot 토큰
  - Discord Developer Portal에서 발급: https://discord.com/developers/applications
  - 형식 예시: `MTxxxxxxxxxxxxxxxxxx.xxxxxx.xxxxxxxxxxxxxxxxxxxxxxxxxxx`

#### Secret 2: GEMINI_API_KEY
- **Name**: `GEMINI_API_KEY`
- **Value**: Google Gemini API 키
  - Google AI Studio에서 발급: https://aistudio.google.com/app/apikey
  - 형식 예시: `AIzaSyxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx`

### 3. Secret 등록 단계
1. **New repository secret** 버튼 클릭
2. **Name** 필드에 Secret 이름 입력 (예: `DISCORD_TOKEN`)
3. **Secret** 필드에 실제 API 키 값 입력
4. **Add secret** 버튼 클릭
5. 위 과정을 `GEMINI_API_KEY`에 대해서도 반복

## 빌드 프로세스

### GitHub Actions 자동 빌드
- 태그를 푸시하면 자동으로 빌드가 시작됩니다
- 예: `git tag v1.0.0 && git push origin v1.0.0`
- 또는 GitHub Actions 페이지에서 수동으로 실행 가능 (workflow_dispatch)

### 빌드 시 API 키 주입
GitHub Actions는 다음과 같이 빌드합니다:
```bash
go build -ldflags "-X main.DiscordToken=${{ secrets.DISCORD_TOKEN }} -X main.GeminiAPIKey=${{ secrets.GEMINI_API_KEY }}" -o bachata-bot
```

이렇게 하면:
- API 키가 실행 파일 내부에 포함됩니다
- `.env` 파일 없이도 실행 파일 하나만으로 동작합니다
- 소스 코드에는 API 키가 노출되지 않습니다

## 로컬 개발 환경

로컬에서 개발할 때는 여전히 `.env` 파일을 사용할 수 있습니다:

1. `.env` 파일 생성 (`.env.example` 참고)
2. API 키 입력:
   ```
   DISCORD_TOKEN=your_discord_token_here
   GEMINI_API_KEY=your_gemini_api_key_here
   ```
3. 일반적으로 빌드: `go build`
4. 실행: `./bachata-bot` (또는 Windows: `bachata-bot.exe`)

## 보안 주의사항

✅ **안전한 방법:**
- GitHub Secrets에 API 키 저장
- `.env` 파일은 `.gitignore`에 포함되어 Git에 추적되지 않음
- 빌드된 실행 파일만 배포

❌ **위험한 방법 (절대 금지):**
- 소스 코드에 API 키 하드코딩
- `.env` 파일을 Git에 커밋
- API 키를 공개 저장소에 업로드

## 문제 해결

### 빌드 실패 시
1. GitHub Secrets가 올바르게 등록되었는지 확인
2. Secret 이름이 정확한지 확인 (`DISCORD_TOKEN`, `GEMINI_API_KEY`)
3. API 키가 유효한지 확인

### 실행 파일이 API 키를 찾지 못할 때
- GitHub Actions로 빌드된 실행 파일인지 확인
- 로컬 빌드의 경우 `.env` 파일이 있는지 확인

## 참고 자료
- [GitHub Secrets 공식 문서](https://docs.github.com/en/actions/security-guides/encrypted-secrets)
- [Go -ldflags 사용법](https://pkg.go.dev/cmd/link)
