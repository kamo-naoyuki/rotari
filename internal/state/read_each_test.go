package state

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestReadEachCallsEveryIndexOnceWithBoundedConcurrency(t *testing.T) {
	for _, count := range []int{0, 1, 3, ReadConcurrency * 3} {
		var mu sync.Mutex
		calls := make(map[int]int)
		var running, peak atomic.Int64
		ReadEach(count, func(index int) {
			now := running.Add(1)
			for {
				old := peak.Load()
				if now <= old || peak.CompareAndSwap(old, now) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			running.Add(-1)
			mu.Lock()
			calls[index]++
			mu.Unlock()
		})
		if len(calls) != count {
			t.Fatalf("count %d: %d indexes called", count, len(calls))
		}
		for index, n := range calls {
			if n != 1 || index < 0 || index >= count {
				t.Fatalf("count %d: index %d called %d times", count, index, n)
			}
		}
		if peak.Load() > ReadConcurrency {
			t.Fatalf("count %d: %d reads at once, limit %d", count, peak.Load(), ReadConcurrency)
		}
	}
}
