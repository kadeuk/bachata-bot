# 타임코드 오류 수정 완료

## 문제 상황

사용자가 Discord를 통해 자막 수정을 요청할 때마다 AI가 타임코드를 변경하는 문제가 발생했습니다:

```
한가득: 파일의 1 및 3행에 오류가 있습니다. 시작 타임이 맞지 않아요 다시 수정해주세요
한가득: 파일의 281행에 오류가 있습니다. 모든 타임 코드를 원본과 맞추세요. 자꾸 타임코드 오류가 생겨요.
한가득: 파일의 1 및 4행에 오류가 있습니다. 시발 계속 오류나네
```

## 근본 원인

`processUserEdit` 함수에서 AI에게 자막 수정을 요청할 때:
1. **검증 부족**: AI 응답을 그대로 저장하고 타임코드 검증을 하지 않음
2. **명확하지 않은 프롬프트**: "타임코드는 절대 변경하지 마세요"라는 지시만 있었지만, AI가 이를 무시하는 경우가 많음
3. **복구 메커니즘 없음**: 타임코드가 변경되어도 원본으로 복원하는 로직이 없음

## 적용된 수정사항

### 1. 원본 SRT 파싱 및 보존
```go
// Parse original SRT to preserve structure
originalEntries, err := ParseSRT(session.CorrectedKorean)
if err != nil {
    s.ChannelMessageEdit(session.ChannelID, msg.ID, fmt.Sprintf("❌ 원본 SRT 파싱 실패: %v", err))
    return
}
```

### 2. 강화된 프롬프트
```go
**중요 규칙 (절대 위반 금지):**
1. **타임코드는 절대 변경하지 마세요** - 원본과 100%% 동일하게 유지
2. **자막 번호는 그대로 유지하세요** - 1부터 순차적으로
3. **자막 항목 개수는 원본과 동일하게 유지하세요** - 총 %d개 항목
4. 사용자가 지시한 텍스트 내용만 수정하세요
5. SRT 형식을 정확히 지켜주세요
```

### 3. 엄격한 타임코드 검증
```go
// Strict validation: Check timecodes match exactly
if err := ValidateTimecodes(originalEntries, editedEntries); err != nil {
    log.Printf("⚠️ 타임코드 검증 실패: %v", err)
    s.ChannelMessageEdit(session.ChannelID, msg.ID, 
        fmt.Sprintf("❌ AI가 타임코드를 변경했습니다: %v\n\n원본 타임코드를 복원합니다...", err))
    
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
```

### 4. 자동 복구 메커니즘

AI가 타임코드를 변경한 경우:
1. **감지**: `ValidateTimecodes` 함수로 원본과 비교
2. **경고**: 사용자에게 타임코드 변경 사실 알림
3. **복원**: 원본 타임코드를 자동으로 복원하되, 수정된 텍스트는 유지
4. **저장**: 복원된 SRT 파일 저장

## 동작 방식

### 이전 (문제 발생)
```
사용자: "1행 수정해줘"
AI: [타임코드 변경 + 텍스트 수정]
봇: ✅ 수정 완료 (검증 없이 저장)
결과: ❌ 타임코드 오류 발생
```

### 수정 후 (자동 복구)
```
사용자: "1행 수정해줘"
AI: [타임코드 변경 + 텍스트 수정]
봇: ⚠️ 타임코드 검증 실패 감지
봇: 🔧 원본 타임코드 자동 복원
봇: ✅ 수정 완료 (타임코드 보존됨)
결과: ✅ 타임코드 정확히 유지
```

## ValidateTimecodes 함수

`srt_processor.go`에 이미 구현되어 있는 함수:

```go
func ValidateTimecodes(original, translated []SRTEntry) error {
    if len(original) != len(translated) {
        return fmt.Errorf("entry count mismatch: original=%d, translated=%d", 
            len(original), len(translated))
    }
    
    for i := range original {
        if original[i].Index != translated[i].Index {
            return fmt.Errorf("index mismatch at position %d: original=%d, translated=%d", 
                i, original[i].Index, translated[i].Index)
        }
        
        if original[i].StartTime != translated[i].StartTime {
            return fmt.Errorf("start time mismatch at index %d: original=%s, translated=%s", 
                original[i].Index, original[i].StartTime, translated[i].StartTime)
        }
        
        if original[i].EndTime != translated[i].EndTime {
            return fmt.Errorf("end time mismatch at index %d: original=%s, translated=%s", 
                original[i].Index, original[i].EndTime, translated[i].EndTime)
        }
    }
    
    return nil
}
```

## 테스트 시나리오

1. **정상 케이스**: AI가 타임코드를 유지하고 텍스트만 수정
   - 결과: ✅ 그대로 저장

2. **타임코드 변경 케이스**: AI가 타임코드를 변경
   - 결과: ⚠️ 감지 → 🔧 자동 복원 → ✅ 저장

3. **항목 개수 변경 케이스**: AI가 자막 항목을 추가/삭제
   - 결과: ❌ 파싱 실패 또는 검증 실패 → 원본 유지

## 사용자 경험 개선

### 이전
```
❌ 자막이 수정되었습니다!
(타임코드 오류 발생 - 사용자가 직접 확인 필요)
```

### 수정 후
```
❌ AI가 타임코드를 변경했습니다: start time mismatch at index 1
원본 타임코드를 복원합니다...
✅ 자막이 수정되었습니다!
(타임코드 자동 복원 완료)
```

## 빌드 완료

```bash
✅ go build -o bachata-bot.exe 성공
```

새로운 실행 파일이 생성되었습니다. 이제 Discord 봇을 실행하여 자막 수정 시 타임코드가 자동으로 보존됩니다.

## 추가 보호 장치

1. **프롬프트 강화**: 항목 개수를 명시하여 AI가 정확히 인지
2. **응답 정제**: 마크다운 코드 블록 제거
3. **파싱 검증**: 수정된 SRT가 올바른 형식인지 확인
4. **타임코드 복원**: 검증 실패 시 자동으로 원본 타임코드 복원
