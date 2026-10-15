package state

import "sync"

// ReadConcurrency bounds how many independent reads ReadEach runs at once.
// On a shared file system each open, stat, and directory listing is a round
// trip, so reading independent files concurrently overlaps their latency.
const ReadConcurrency = 16

// ReadEach calls read for every index in [0, count) from up to
// ReadConcurrency goroutines, and returns once all calls have returned. read
// must be safe to call concurrently; callers keep results in a slice indexed
// like their input, so output order does not depend on scheduling.
func ReadEach(count int, read func(index int)) {
	workers := min(ReadConcurrency, count)
	if workers <= 1 {
		for index := range count {
			read(index)
		}
		return
	}
	indexes := make(chan int)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := range indexes {
				read(index)
			}
		}()
	}
	for index := range count {
		indexes <- index
	}
	close(indexes)
	wait.Wait()
}
