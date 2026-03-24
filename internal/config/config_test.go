package config

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRLockUnlockBasic(t *testing.T) {
	cfg := &Config{}
	cfg.RLock()
	cfg.RUnlock()
	// No panic means success.
}

func TestLockUnlockBasic(t *testing.T) {
	cfg := &Config{}
	cfg.Lock()
	cfg.Unlock()
}

func TestConcurrentReadsDoNotBlock(t *testing.T) {
	cfg := &Config{
		Tasks: []Task{{Name: "t1", Prompt: "p1"}},
	}

	const readers = 10
	var ready sync.WaitGroup
	ready.Add(readers)

	var done sync.WaitGroup
	done.Add(readers)

	start := make(chan struct{})

	for i := 0; i < readers; i++ {
		go func() {
			defer done.Done()
			ready.Done()
			<-start

			cfg.RLock()
			// Simulate a read that holds the lock briefly.
			_ = len(cfg.Tasks)
			time.Sleep(10 * time.Millisecond)
			cfg.RUnlock()
		}()
	}

	// Wait for all goroutines to be ready, then release them simultaneously.
	ready.Wait()
	t0 := time.Now()
	close(start)
	done.Wait()
	elapsed := time.Since(t0)

	// If reads blocked each other, total time would be >= readers * 10ms (100ms).
	// With concurrent reads, it should complete in roughly 10-20ms.
	if elapsed > 50*time.Millisecond {
		t.Errorf("concurrent reads took %v, expected < 50ms (reads may be blocking each other)", elapsed)
	}
}

func TestWriteBlocksReads(t *testing.T) {
	cfg := &Config{
		Tasks: []Task{{Name: "t1", Prompt: "p1"}},
	}

	// Acquire write lock.
	cfg.Lock()

	var readStarted atomic.Int32
	var readCompleted atomic.Int32

	// Launch a reader that should block.
	go func() {
		readStarted.Store(1)
		cfg.RLock()
		readCompleted.Store(1)
		cfg.RUnlock()
	}()

	// Give the goroutine time to reach the RLock call.
	time.Sleep(30 * time.Millisecond)

	if readStarted.Load() != 1 {
		t.Fatal("reader goroutine did not start")
	}
	if readCompleted.Load() != 0 {
		t.Error("reader should be blocked while write lock is held")
	}

	// Release write lock; reader should proceed.
	cfg.Unlock()
	time.Sleep(30 * time.Millisecond)

	if readCompleted.Load() != 1 {
		t.Error("reader should have completed after write lock was released")
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	// This test is mainly for the -race detector: multiple goroutines
	// reading and writing Config.Tasks concurrently with proper locking.
	cfg := &Config{
		Tasks: []Task{{Name: "t1", Prompt: "p1"}},
	}

	var wg sync.WaitGroup
	const iterations = 100

	// Readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				cfg.RLock()
				_ = len(cfg.Tasks)
				if len(cfg.Tasks) > 0 {
					_ = cfg.Tasks[0].Name
				}
				cfg.RUnlock()
			}
		}()
	}

	// Writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < iterations; j++ {
			cfg.Lock()
			cfg.Tasks = append(cfg.Tasks, Task{Name: "new", Prompt: "p"})
			cfg.Tasks = cfg.Tasks[:1] // trim back
			cfg.Unlock()
		}
	}()

	wg.Wait()
}
