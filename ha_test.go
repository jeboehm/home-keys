package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDomainService(t *testing.T) {
	cases := []struct {
		entityID        string
		wantDomain      string
		wantService     string
	}{
		{"lock.front_door", "lock", "unlock"},
		{"cover.garage", "cover", "open_cover"},
		{"button.doorbell", "button", "press"},
		{"script.open_gate", "script", "turn_on"},
		{"switch.light", "switch", "open"},
	}
	for _, c := range cases {
		d, s := DomainService(c.entityID)
		if d != c.wantDomain || s != c.wantService {
			t.Errorf("DomainService(%q) = (%q, %q), want (%q, %q)", c.entityID, d, s, c.wantDomain, c.wantService)
		}
	}
}

func TestGetState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"state": "locked"})
	}))
	defer srv.Close()

	client := NewHAClient(srv.URL, "token")
	state, err := client.GetState(context.Background(), "lock.front_door")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != "locked" {
		t.Errorf("got state %q, want %q", state, "locked")
	}
}

func TestGetState_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewHAClient(srv.URL, "token")
	_, err := client.GetState(context.Background(), "lock.missing")
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

func TestGetAllStates(t *testing.T) {
	payload := []map[string]any{
		{
			"entity_id": "lock.front_door",
			"state":     "locked",
			"attributes": map[string]string{"friendly_name": "Front Door"},
		},
		{
			"entity_id": "switch.light",
			"state":     "on",
			"attributes": map[string]string{"friendly_name": "Light"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	client := NewHAClient(srv.URL, "token")
	states, err := client.GetAllStates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("got %d states, want 2", len(states))
	}
	if states[0].EntityID != "lock.front_door" || states[0].FriendlyName != "Front Door" {
		t.Errorf("unexpected first state: %+v", states[0])
	}
}
