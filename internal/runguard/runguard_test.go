package runguard

import (
	"sync"
	"testing"
)

func TestTryAcquireAndRelease(t *testing.T) {
	g := New()

	if !g.TryAcquire("task:foo") {
		t.Fatal("first acquire should succeed")
	}
	if g.TryAcquire("task:foo") {
		t.Fatal("second acquire of same key should fail")
	}

	// Different key should succeed
	if !g.TryAcquire("task:bar") {
		t.Fatal("acquire of different key should succeed")
	}

	g.Release("task:foo")

	// After release, should be able to acquire again
	if !g.TryAcquire("task:foo") {
		t.Fatal("acquire after release should succeed")
	}
}

func TestReleaseNonExistent(t *testing.T) {
	g := New()
	// Should not panic
	g.Release("nonexistent")
}

func TestNamespaceIsolation(t *testing.T) {
	g := New()

	if !g.TryAcquire("task:deploy") {
		t.Fatal("task namespace acquire should succeed")
	}
	if !g.TryAcquire("pipeline:deploy") {
		t.Fatal("pipeline namespace with same name should succeed independently")
	}
}

func TestConcurrentAccess(t *testing.T) {
	g := New()
	const n = 100

	acquired := make(chan bool, n)
	var wg sync.WaitGroup
	wg.Add(n)

	for range n {
		go func() {
			defer wg.Done()
			acquired <- g.TryAcquire("task:concurrent")
		}()
	}

	wg.Wait()
	close(acquired)

	successCount := 0
	for ok := range acquired {
		if ok {
			successCount++
		}
	}

	if successCount != 1 {
		t.Fatalf("expected exactly 1 successful acquire, got %d", successCount)
	}
}
