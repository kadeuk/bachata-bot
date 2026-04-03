package main

import (
	"fmt"
	"log"
	"sync"
)

// ParallelProcessor handles parallel processing of SRT chunks
type ParallelProcessor struct {
	maxWorkers int
	semaphore  chan struct{}
}

// NewParallelProcessor creates a new parallel processor
func NewParallelProcessor(maxWorkers int) *ParallelProcessor {
	return &ParallelProcessor{
		maxWorkers: maxWorkers,
		semaphore:  make(chan struct{}, maxWorkers),
	}
}

// ChunkResult stores the result of processing a single chunk
type ChunkResult struct {
	Index   int
	Entries []SRTEntry
	Error   error
}

// ProcessChunksParallel processes multiple chunks in parallel and returns them in order
func (pp *ParallelProcessor) ProcessChunksParallel(
	chunks [][]SRTEntry,
	processFn func(chunk []SRTEntry, index int) ([]SRTEntry, error),
) ([]SRTEntry, error) {
	
	results := make([]ChunkResult, len(chunks))
	var wg sync.WaitGroup
	
	log.Printf("🚀 병렬 처리 시작: %d개 청크, 최대 %d개 동시 실행", len(chunks), pp.maxWorkers)
	
	for i, chunk := range chunks {
		wg.Add(1)
		go func(index int, c []SRTEntry) {
			defer wg.Done()
			
			// Semaphore: 동시 실행 제어
			pp.semaphore <- struct{}{}
			defer func() { <-pp.semaphore }()
			
			log.Printf("   청크 %d/%d 처리 시작...", index+1, len(chunks))
			
			// 처리
			processed, err := processFn(c, index)
			
			// 결과 저장 (인덱스 위치에 저장하여 순서 보장)
			results[index] = ChunkResult{
				Index:   index,
				Entries: processed,
				Error:   err,
			}
			
			if err != nil {
				log.Printf("   ⚠️ 청크 %d/%d 처리 실패: %v", index+1, len(chunks), err)
			} else {
				log.Printf("   ✅ 청크 %d/%d 처리 완료 (%d개 항목)", index+1, len(chunks), len(processed))
			}
		}(i, chunk)
	}
	
	// 모든 고루틴 완료 대기
	wg.Wait()
	
	log.Println("✅ 모든 청크 병렬 처리 완료, 순서대로 병합 중...")
	
	// 순서대로 병합 (중요: 인덱스 순서대로!)
	var merged []SRTEntry
	for i := 0; i < len(results); i++ {
		if results[i].Error != nil {
			return nil, fmt.Errorf("청크 %d 처리 실패: %v", i+1, results[i].Error)
		}
		
		// ID 기반 정렬 확인 (추가 안전장치)
		if len(merged) > 0 && len(results[i].Entries) > 0 {
			lastID := merged[len(merged)-1].Index
			firstID := results[i].Entries[0].Index
			if firstID <= lastID {
				log.Printf("⚠️ 경고: 청크 %d의 첫 ID(%d)가 이전 청크의 마지막 ID(%d)보다 작거나 같음", 
					i+1, firstID, lastID)
			}
		}
		
		merged = append(merged, results[i].Entries...)
	}
	
	log.Printf("✅ 병합 완료: 총 %d개 항목", len(merged))
	
	// 최종 ID 순서 검증
	if err := validateIDOrder(merged); err != nil {
		return nil, fmt.Errorf("ID 순서 검증 실패: %v", err)
	}
	
	return merged, nil
}

// validateIDOrder checks if entries are in correct ID order
func validateIDOrder(entries []SRTEntry) error {
	for i := 1; i < len(entries); i++ {
		if entries[i].Index <= entries[i-1].Index {
			return fmt.Errorf("ID 순서 오류: 위치 %d에서 ID %d 다음에 ID %d가 옴", 
				i, entries[i-1].Index, entries[i].Index)
		}
	}
	return nil
}
