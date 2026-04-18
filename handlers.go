package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

type DoorConfig struct {
	Key      string // form value, e.g. "haustur"
	Name     string // human-readable, e.g. "Haustür"
	EntityID string // HA entity ID, e.g. "lock.haustur"
}

type App struct {
	Config      *Config
	HAClient    *HAClient
	RateLimiter *RateLimiter
	Templates   *template.Template
	Doors       []DoorConfig
}

func requestCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 10*time.Second)
}

func (a *App) findDoor(key string) *DoorConfig {
	for i := range a.Doors {
		if a.Doors[i].Key == key {
			return &a.Doors[i]
		}
	}
	return nil
}

func (a *App) LoginHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ctx, cancel := requestCtx(r)
		defer cancel()
		code, err := a.HAClient.GetState(ctx, a.Config.EntityDoorCode)
		if err != nil {
			log.Printf("[ERROR] fetch door code: %v", err)
			a.renderLogin(w, "Service unavailable.")
			return
		}
		if strings.TrimSpace(code) == "" {
			a.issueSession(w, r, "")
			return
		}
		a.renderLogin(w, "")
	case http.MethodPost:
		a.handleLoginPost(w, r)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)

	if !a.RateLimiter.Allow(ip) {
		log.Printf("[LOGIN_FAIL] ip=%s reason=rate_limit", ip)
		a.renderLogin(w, "Invalid code.")
		return
	}

	ctx, cancel := requestCtx(r)
	defer cancel()
	code, err := a.HAClient.GetState(ctx, a.Config.EntityDoorCode)
	if err != nil {
		log.Printf("[ERROR] fetch door code: %v", err)
		a.renderLogin(w, "Service unavailable.")
		return
	}
	code = strings.TrimSpace(code)

	if code == "" || r.FormValue("code") == code {
		log.Printf("[LOGIN_OK] ip=%s", ip)
		a.issueSession(w, r, code)
		return
	}

	log.Printf("[LOGIN_FAIL] ip=%s reason=wrong_code", ip)
	a.renderLogin(w, "Invalid code.")
}

func (a *App) issueSession(w http.ResponseWriter, r *http.Request, code string) {
	token, err := GenerateSessionToken(a.Config.SessionSecret, code)
	if err != nil {
		log.Printf("[ERROR] generate session token: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	SetSessionCookie(w, token)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) renderLogin(w http.ResponseWriter, errMsg string) {
	data := struct{ Error string }{Error: errMsg}
	if err := a.Templates.ExecuteTemplate(w, "login.html", data); err != nil {
		log.Printf("[ERROR] render login: %v", err)
	}
}

func (a *App) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

type dashboardData struct {
	Success         string
	Doors           []DoorConfig
	HomeKeysEnabled bool
}

func (a *App) DashboardHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	ctx, cancel := requestCtx(r)
	defer cancel()

	homeKeysEnabled, err := a.HAClient.GetState(ctx, a.Config.EntityUnlockAllowance)
	if err != nil {
		log.Printf("[ERROR] fetch home keys enabled state: %v", err)
		http.Error(w, "Service unavailable.", http.StatusBadGateway)
		return
	}

	data := dashboardData{
		Doors:           a.Doors,
		HomeKeysEnabled: homeKeysEnabled == "on",
	}
	if key := r.URL.Query().Get("success"); key != "" {
		if d := a.findDoor(key); d != nil {
			data.Success = d.Name
		}
	}

	if err := a.Templates.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		log.Printf("[ERROR] render dashboard: %v", err)
	}
}

func (a *App) OpenDoorHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	door := a.findDoor(r.FormValue("door"))
	if door == nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	ip := clientIP(r)

	ctx, cancel := requestCtx(r)
	defer cancel()

	homeKeysEnabled, err := a.HAClient.GetState(ctx, a.Config.EntityUnlockAllowance)
	if err != nil {
		log.Printf("[ERROR] fetch home keys enabled state: %v", err)
		http.Error(w, "Service unavailable.", http.StatusBadGateway)
		return
	}

	if homeKeysEnabled != "on" {
		log.Printf("[DOOR_BLOCKED] ip=%s reason=home_keys_disabled", ip)
		http.Error(w, "Not enabled.", http.StatusForbidden)
		return
	}

	domain, service := DomainService(door.EntityID)
	if err := a.HAClient.CallService(ctx, domain, service, door.EntityID); err != nil {
		log.Printf("[DOOR_ERROR] ip=%s action=%s error=%v", ip, door.Name, err)
		http.Error(w, "Door could not be opened.", http.StatusBadGateway)
		return
	}

	log.Printf("[DOOR] ip=%s action=%s", ip, door.Name)
	http.Redirect(w, r, "/?success="+door.Key, http.StatusSeeOther)
}

func (a *App) entityIDs() []string {
	ids := make([]string, 0, 2+len(a.Doors))
	ids = append(ids, a.Config.EntityUnlockAllowance, a.Config.EntityDoorCode)
	for _, d := range a.Doors {
		ids = append(ids, d.EntityID)
	}
	return ids
}

func (a *App) checkEntities(ctx context.Context) []string {
	type result struct {
		id  string
		err error
	}
	ids := a.entityIDs()
	ch := make(chan result, len(ids))
	for _, id := range ids {
		id := id
		go func() {
			_, err := a.HAClient.GetState(ctx, id)
			ch <- result{id, err}
		}()
	}
	var errs []string
	for range ids {
		if r := <-ch; r.err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.id, r.err))
		}
	}
	return errs
}

func (a *App) HealthHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := requestCtx(r)
	defer cancel()

	errs := a.checkEntities(ctx)

	w.Header().Set("Content-Type", "application/json")
	if len(errs) > 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{"status": "error", "errors": errs})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
