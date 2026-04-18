package main

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fakeHA builds a test HA server. The handler map is keyed by entity ID path suffix.
// If a key is missing the server returns 500.
type fakeHAConfig struct {
	states   map[string]string // entity_id → state
	failOpen bool              // make CallService return 500
}

func newFakeHA(cfg fakeHAConfig) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/states/") {
			id := strings.TrimPrefix(r.URL.Path, "/api/states/")
			state, ok := cfg.states[id]
			if !ok {
				http.Error(w, "not found", http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"state": state})
			return
		}
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/services/") {
			if cfg.failOpen {
				http.Error(w, "error", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
}

func newTestApp(t *testing.T, ha *httptest.Server, doors []DoorConfig, unlockAllowance, doorCode string) *App {
	t.Helper()
	tmpl := template.Must(template.ParseFS(templateFS, "templates/*.html"))
	return &App{
		Config: &Config{
			SessionSecret:        []byte("test-secret-at-least-16-chars!!"),
			EntityUnlockAllowance: unlockAllowance,
			EntityDoorCode:       doorCode,
		},
		HAClient:    NewHAClient(ha.URL, "token"),
		RateLimiter: newRateLimiter(),
		Templates:   tmpl,
		Doors:       doors,
	}
}

func TestHealthHandler_OK(t *testing.T) {
	ha := newFakeHA(fakeHAConfig{states: map[string]string{
		"input_boolean.allowance": "on",
		"input_text.code":         "1234",
		"lock.front":              "locked",
	}})
	defer ha.Close()

	doors := []DoorConfig{{Key: "lock_front", Name: "Front", EntityID: "lock.front"}}
	app := newTestApp(t, ha, doors, "input_boolean.allowance", "input_text.code")

	w := httptest.NewRecorder()
	app.HealthHandler(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestHealthHandler_Error(t *testing.T) {
	ha := newFakeHA(fakeHAConfig{states: map[string]string{
		"input_boolean.allowance": "on",
		// input_text.code missing → 500
		"lock.front": "locked",
	}})
	defer ha.Close()

	doors := []DoorConfig{{Key: "lock_front", Name: "Front", EntityID: "lock.front"}}
	app := newTestApp(t, ha, doors, "input_boolean.allowance", "input_text.code")

	w := httptest.NewRecorder()
	app.HealthHandler(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("got status %d, want 503", w.Code)
	}
}

func TestLoginHandler_GET_NoCode(t *testing.T) {
	ha := newFakeHA(fakeHAConfig{states: map[string]string{
		"input_text.code": "",
	}})
	defer ha.Close()

	app := newTestApp(t, ha, nil, "input_boolean.allowance", "input_text.code")

	w := httptest.NewRecorder()
	app.LoginHandler(w, httptest.NewRequest(http.MethodGet, "/login", nil))

	if w.Code != http.StatusSeeOther {
		t.Errorf("empty code should redirect, got %d", w.Code)
	}
}

func TestLoginHandler_POST_CorrectCode(t *testing.T) {
	ha := newFakeHA(fakeHAConfig{states: map[string]string{
		"input_text.code": "1234",
	}})
	defer ha.Close()

	app := newTestApp(t, ha, nil, "input_boolean.allowance", "input_text.code")

	form := url.Values{"code": {"1234"}}
	r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.RemoteAddr = "127.0.0.1:1234"

	w := httptest.NewRecorder()
	app.LoginHandler(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("correct code should redirect, got %d", w.Code)
	}
}

func TestLoginHandler_POST_WrongCode(t *testing.T) {
	ha := newFakeHA(fakeHAConfig{states: map[string]string{
		"input_text.code": "1234",
	}})
	defer ha.Close()

	app := newTestApp(t, ha, nil, "input_boolean.allowance", "input_text.code")

	form := url.Values{"code": {"9999"}}
	r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.RemoteAddr = "127.0.0.1:1234"

	w := httptest.NewRecorder()
	app.LoginHandler(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("wrong code should re-render login (200), got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Invalid code") {
		t.Error("response should contain error message")
	}
}

func TestOpenDoorHandler_UnlockAllowanceOff(t *testing.T) {
	ha := newFakeHA(fakeHAConfig{states: map[string]string{
		"input_boolean.allowance": "off",
	}})
	defer ha.Close()

	doors := []DoorConfig{{Key: "lock_front", Name: "Front", EntityID: "lock.front"}}
	app := newTestApp(t, ha, doors, "input_boolean.allowance", "input_text.code")

	form := url.Values{"door": {"lock_front"}}
	r := httptest.NewRequest(http.MethodPost, "/open", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.RemoteAddr = "127.0.0.1:1234"

	w := httptest.NewRecorder()
	app.OpenDoorHandler(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("unlockAllowance=off should return 403, got %d", w.Code)
	}
}

func TestOpenDoorHandler_Success(t *testing.T) {
	ha := newFakeHA(fakeHAConfig{states: map[string]string{
		"input_boolean.allowance": "on",
		"lock.front":              "locked",
	}})
	defer ha.Close()

	doors := []DoorConfig{{Key: "lock_front", Name: "Front", EntityID: "lock.front"}}
	app := newTestApp(t, ha, doors, "input_boolean.allowance", "input_text.code")

	form := url.Values{"door": {"lock_front"}}
	r := httptest.NewRequest(http.MethodPost, "/open", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.RemoteAddr = "127.0.0.1:1234"

	w := httptest.NewRecorder()
	app.OpenDoorHandler(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("success should redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "success=lock_front") {
		t.Errorf("redirect location missing success param: %s", loc)
	}
}
