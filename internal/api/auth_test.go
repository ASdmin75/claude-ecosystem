package api

import (
	"testing"
	"time"
)

func TestLoginRateLimiter_AllowsFirstAttempts(t *testing.T) {
	rl := newLoginRateLimiter(1*time.Minute, 3)

	if !rl.allow("192.168.1.1") {
		t.Fatal("expected first attempt to be allowed")
	}
	if !rl.allow("192.168.1.1") {
		t.Fatal("expected second attempt to be allowed")
	}
	if !rl.allow("192.168.1.1") {
		t.Fatal("expected third attempt to be allowed")
	}
}

func TestLoginRateLimiter_BlocksAfterMax(t *testing.T) {
	rl := newLoginRateLimiter(1*time.Minute, 3)

	for i := range 3 {
		if !rl.allow("10.0.0.1") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}

	// 4th attempt should be blocked.
	if rl.allow("10.0.0.1") {
		t.Fatal("expected 4th attempt to be blocked")
	}
}

func TestLoginRateLimiter_DifferentIPsIndependent(t *testing.T) {
	rl := newLoginRateLimiter(1*time.Minute, 2)

	// Exhaust limit for IP A.
	rl.allow("ip-a")
	rl.allow("ip-a")
	if rl.allow("ip-a") {
		t.Fatal("ip-a should be blocked")
	}

	// IP B should still be allowed.
	if !rl.allow("ip-b") {
		t.Fatal("ip-b should be allowed independently")
	}
}

func TestLoginRateLimiter_ResetsAfterWindow(t *testing.T) {
	// Use a very short window for testing.
	rl := newLoginRateLimiter(50*time.Millisecond, 2)

	rl.allow("10.0.0.2")
	rl.allow("10.0.0.2")
	if rl.allow("10.0.0.2") {
		t.Fatal("should be blocked after max attempts")
	}

	// Wait for window to expire.
	time.Sleep(60 * time.Millisecond)

	if !rl.allow("10.0.0.2") {
		t.Fatal("should be allowed after window expires")
	}
}

func TestLoginRateLimiter_ExactBoundary(t *testing.T) {
	rl := newLoginRateLimiter(1*time.Minute, 1)

	if !rl.allow("boundary") {
		t.Fatal("first attempt with max=1 should be allowed")
	}
	if rl.allow("boundary") {
		t.Fatal("second attempt with max=1 should be blocked")
	}
}
