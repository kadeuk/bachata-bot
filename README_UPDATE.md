# 바차타 자막 번역 프로그램 업데이트 가이드

## 🎯 업데이트 개요

기존 `main.go`에 바차타 테크닉 우선순위 사전 기반 자동 치환 기능이 추가되었습니다.

## 📋 주요 변경사항

### 1. **바차타 테크닉 우선순위 사전 (JSON)**
- 파일: `bachata_techniques.json`
- 16가지 핵심 바차타 테크닉 정의
- 스페인어 발음, 영어 발음, 의미 포함

### 2. **새로운 기능**

#### ✅ 우선순위 기반 사전 시스템
- JSON 데이터가 기존 TXT 사전보다 우선 적용
- 충돌 시 JSON 데이터의 매핑 룰이 최우선

#### ✅ 유사 발음 매칭 (Fuzzy Matching)
- Levenshtein Distance 알고리즘 사용
- 오인식된 오타 자동 감지 (예: "빈사", "핀사" → "삔싸")
- 유사도 점수 기반 자동/수동 치환

#### ✅ 자동 포맷팅
- 출력 형식: `한국어발음(Spanish_name)`
- 예시: `삔싸(Pinza)`, `온다(Onda)`, `꿀리또(Culito)`

#### ✅ LLM 기반 문맥 파악
- 애매한 단어의 문맥 분석 (예: "온다" - 일반 동사 vs 댄스 용어)
- OpenAI/Claude API를 통한 자동 판별
- 고루틴(Goroutine) 기반 비동기 병렬 처리로 속도 최적화

#### ✅ 사용자 확인 시스템
- 유사도 0.75~0.9: 사용자 확인 필요
- 유사도 0.9 이상: 자동 치환
- 유사도 0.75 미만: 무시

#### ✅ 범용 영어 용어 예외 처리
- Turn, Frame, Tension, Preparation 등은 영어 유지
- 억지로 스페인어로 번역하지 않음

## 🚀 사용 방법

### 기존 프로그램 백업
```bash
# 기존 main.go 백업
cp main.go main_backup.go
```

### 새 버전 적용
```bash
# main_updated.go를 main.go로 교체
# 주의: 현재 main_updated.go에 컴파일 에러가 있으므로 수정 필요
```

### 실행
```bash
go run main.go
```

## 📊 처리 흐름

```
STEP 0: 바차타 테크닉 우선순위 사전 기반 자동 치환
  ↓
  - JSON 사전 로드
  - 유사도 매칭 (Fuzzy Matching)
  - 자동 치환 (유사도 ≥ 0.9)
  - 사용자 확인 (유사도 0.75~0.9)
  ↓
STEP 1: 한국어 STT 완벽 교정 (기존 로직)
  ↓
STEP 2: 11개국 다국어 번역 (기존 로직)
  ↓
STEP 3: 유튜브 제목/설명 생성 (기존 로직)
```

## ⚠️ 현재 이슈

`main_updated.go` 파일에 다음 컴파일 에러가 있습니다:

1. **중복 선언 에러**: `main.go`와 `main_updated.go`가 같은 패키지에 있어서 발생
   - **해결 방법**: `main.go`를 삭제하거나 다른 폴더로 이동

2. **발음 변형 생성 버그** (Line 194):
   ```go
   // 잘못된 코드
   variants = append(variants, string(newRunes))
   
   // 수정 필요
   // variants 변수명 충돌 - 다른 이름 사용 필요
   ```

3. **미사용 import** (Line 9):
   ```go
   "math" // 사용되지 않음 - 제거 필요
   ```

## 🔧 수정 방법

### 1단계: 기존 main.go 제거
```bash
# main.go를 백업 폴더로 이동
mkdir backup
mv main.go backup/
```

### 2단계: main_updated.go 수정

#### 수정 1: math import 제거
```go
// 삭제
import (
    ...
    "math"  // ← 이 줄 삭제
    ...
)
```

#### 수정 2: 발음 변형 생성 함수 수정
```go
func generatePronunciationVariants(original string) []string {
	result := []string{original}  // variants → result로 변경
	
	consonantMap := map[rune][]rune{
		'ㅃ': {'ㅂ', 'ㅍ', 'ㅃ'},
		'ㅉ': {'ㅈ', 'ㅊ', 'ㅉ'},
		'ㄸ': {'ㄷ', 'ㅌ', 'ㄸ'},
		'ㄲ': {'ㄱ', 'ㅋ', 'ㄲ'},
		'ㅆ': {'ㅅ', 'ㅆ'},
	}

	runes := []rune(original)
	for i, r := range runes {
		if variantRunes, ok := consonantMap[r]; ok {  // variants → variantRunes
			for _, variant := range variantRunes {
				newRunes := make([]rune, len(runes))
				copy(newRunes, runes)
				newRunes[i] = variant
				result = append(result, string(newRunes))  // 수정됨
			}
		}
	}

	return result  // 수정됨
}
```

### 3단계: 파일명 변경
```bash
mv main_updated.go main.go
```

### 4단계: 컴파일 및 실행
```bash
go run main.go
```

## 📝 JSON 데이터 구조

```json
{
  "id": 1,
  "spanish_pronunciation": "온다",
  "spanish_name": "Onda",
  "english_korean_pronunciation": "프론트 웨이브 / 바디롤",
  "english_equivalent": "Front Wave / Body Roll",
  "meaning": "'파도'라는 뜻. 가장 기본적인 앞뒤 웨이브.",
  "priority": 1
}
```

## 🎓 알고리즘 설명

### Levenshtein Distance (편집 거리)
- 두 문자열 간의 유사도를 0.0~1.0으로 계산
- 예: "삔싸" vs "빈사" → 유사도 0.75
- 예: "삔싸" vs "삔싸" → 유사도 1.0

### 유사도 임계값
- **0.9 이상**: 자동 치환 (높은 확신)
- **0.75~0.9**: 사용자 확인 필요 (중간 확신)
- **0.75 미만**: 무시 (낮은 확신)

## 💡 사용 예시

### 입력 (STT 오인식)
```
"빈사로 들어갈게요"
```

### 처리 과정
1. "빈사" 감지
2. JSON 사전에서 유사도 매칭
3. "삔싸(Pinza)" 발견 (유사도 0.75)
4. 사용자 확인 요청
5. 사용자 승인 시 치환

### 출력
```
"삔싸(Pinza)로 들어갈게요"
```

## 📞 문의

이슈 발생 시 GitHub Issues에 등록해주세요.
