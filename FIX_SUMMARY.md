# JSON 파싱 오류 수정 완료

## 문제 분석

### 발생한 오류
```
2026/03/24 13:40:05 ⚠️ JSON 파싱 실패: unexpected end of JSON input
2026/03/24 13:40:05 응답 내용 (처음 500자): [
  {
    "timecode": "00:00:22,370",
    "original_stt": "원, 투, 쓰리",
    ...
```

### 근본 원인
1. **AI 응답이 중간에 잘림**: Gemini API의 `MaxOutputTokens`가 8192로 설정되어 있어, 긴 JSON 배열이 생성될 때 응답이 중간에 잘려서 닫는 괄호 `]`가 누락됨
2. **불완전한 JSON**: 마지막 객체가 완성되지 않거나 배열이 닫히지 않아 `unexpected end of JSON input` 오류 발생

### 디버그 로그 분석
`debug_logs/error_2026-03-18_14-37-06.txt` 파일을 확인한 결과:
- AI 응답이 중간에 끊김: `"best_guess": "여기서 시작 1 휙 3` (문장이 완성되지 않음)
- 닫는 괄호 누락: JSON 배열이 `]`로 끝나지 않음

## 적용된 수정사항

### 1. 자동 복구 로직 추가 (`corrector.go`)
```go
// Additional validation: Check if JSON is complete
if !strings.HasSuffix(strings.TrimSpace(cleanJSON), "]") {
    log.Printf("⚠️ JSON이 불완전합니다 (닫는 괄호 없음)")
    log.Printf("응답 내용 (마지막 200자): %s", cleanJSON[max(0, len(cleanJSON)-200):])
    
    // Try to fix incomplete JSON by adding closing bracket
    cleanJSON = strings.TrimSpace(cleanJSON)
    if strings.HasPrefix(cleanJSON, "[") && !strings.HasSuffix(cleanJSON, "]") {
        // Find the last complete object
        lastBraceIdx := strings.LastIndex(cleanJSON, "}")
        if lastBraceIdx > 0 {
            cleanJSON = cleanJSON[:lastBraceIdx+1] + "\n]"
            log.Printf("✅ 불완전한 JSON 자동 복구 시도")
        }
    }
}
```

**동작 방식:**
- JSON 배열이 `]`로 끝나지 않으면 자동으로 감지
- 마지막으로 완성된 객체(`}`)를 찾아서 그 뒤에 닫는 괄호 `]` 추가
- 불완전한 마지막 객체는 제거하고 완성된 객체들만 파싱

### 2. 토큰 제한 증가
```go
flashModel.SetMaxOutputTokens(16384) // 8192 → 16384로 증가
```

**효과:**
- AI 응답이 더 길어질 수 있어 중간에 잘릴 가능성 감소
- 더 많은 교정 제안을 한 번에 처리 가능

### 3. 향상된 로깅
```go
log.Printf("응답 내용 (처음 500자): %s", cleanJSON[:min(500, len(cleanJSON))])
log.Printf("응답 내용 (마지막 200자): %s", cleanJSON[max(0, len(cleanJSON)-200):])
```

**효과:**
- 문제 발생 시 응답의 시작과 끝을 모두 확인 가능
- 디버깅이 더 쉬워짐

## 테스트 방법

1. Discord 봇 실행:
```bash
bachata-bot.exe
```

2. Discord에서 SRT 파일 업로드

3. 로그 확인:
   - `✅ 불완전한 JSON 자동 복구 시도` 메시지가 나타나면 자동 복구 성공
   - `✅ X개의 교정 제안 추출 완료` 메시지 확인

## 예상 결과

### 이전 (오류 발생)
```
⚠️ JSON 파싱 실패: unexpected end of JSON input
⚠️ 교정 제안 추출 실패: AI 응답 파싱 실패: unexpected end of JSON input
```

### 수정 후 (자동 복구)
```
⚠️ JSON이 불완전합니다 (닫는 괄호 없음)
응답 내용 (마지막 200자): ...
✅ 불완전한 JSON 자동 복구 시도
✅ 58개의 교정 제안 추출 완료
```

## 추가 개선 사항

향후 더 안정적인 처리를 위해 고려할 사항:
1. **청크 분할**: 매우 긴 SRT 파일의 경우 교정 제안도 청크로 나누어 처리
2. **재시도 로직**: 자동 복구 실패 시 더 작은 단위로 재시도
3. **스트리밍 응답**: Gemini API의 스트리밍 기능 활용하여 실시간으로 JSON 검증

## 빌드 완료

```bash
✅ go build -o bachata-bot.exe 성공
```

새로운 실행 파일이 생성되었습니다. 이제 Discord 봇을 실행하여 테스트할 수 있습니다.
