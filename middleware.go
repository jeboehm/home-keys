package main

import (
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

func (a *App) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx, cancel := requestCtx(r)
		defer cancel()
		code, err := a.HAClient.GetState(ctx, a.Config.EntityDoorCode)
		if err != nil {
			log.Printf("[ERROR] auth fetch code: %v", err)
			http.Error(w, "Service unavailable.", http.StatusBadGateway)
			return
		}

		if !ValidateSessionToken(a.Config.SessionSecret, cookie.Value, strings.TrimSpace(code)) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

const (
	rateLimitMax    = 5
	rateLimitWindow = 15 * time.Minute
	cleanupInterval = 5 * time.Minute
)

type bucket struct {
	attempts  int
	windowEnd time.Time
}

type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

func newRateLimiter() *RateLimiter {
	return &RateLimiter{buckets: make(map[string]*bucket)}
}

func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[ip]
	if !ok || now.After(b.windowEnd) {
		rl.buckets[ip] = &bucket{attempts: 1, windowEnd: now.Add(rateLimitWindow)}
		return true
	}
	if b.attempts >= rateLimitMax {
		return false
	}
	b.attempts++
	return true
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, b := range rl.buckets {
			if now.After(b.windowEnd) {
				delete(rl.buckets, ip)
			}
		}
		rl.mu.Unlock()
	}
}
