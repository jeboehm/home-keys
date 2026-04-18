package main

import (
	"testing"
	"time"
)

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := newRateLimiter()
	for i := 0; i < rateLimitMax; i++ {
		if !rl.Allow("1.2.3.4") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
}

func TestRateLimiter_BlocksAtLimit(t *testing.T) {
	rl := newRateLimiter()
	for i := 0; i < rateLimitMax; i++ {
		rl.Allow("1.2.3.4")
	}
	if rl.Allow("1.2.3.4") {
		t.Error("attempt beyond limit should be blocked")
	}
}

func TestRateLimiter_ResetsAfterWindow(t *testing.T) {
	rl := newRateLimiter()
	for i := 0; i < rateLimitMax; i++ {
		rl.Allow("1.2.3.4")
	}
	// Expire the window manually.
	rl.mu.Lock()
	rl.buckets["1.2.3.4"].windowEnd = time.Now().Add(-time.Second)
	rl.mu.Unlock()

	if !rl.Allow("1.2.3.4") {
		t.Error("attempt after window expiry should be allowed")
	}
}
