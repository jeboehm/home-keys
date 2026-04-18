package main

import (
	"context"
	"embed"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

//go:embed templates/*
var templateFS embed.FS

// Config holds all runtime configuration.
type Config struct {
	SessionSecret         []byte
	HAUrl                 string
	HAToken               string
	EntityUnlockAllowance string
	EntityDoorCode        string
	IgnoredEntities       []string
	ListenAddr            string
}

func loadConfig() *Config {
	ignored := []string{}
	if v := os.Getenv("IGNORED_ENTITIES"); v != "" {
		for _, e := range strings.Split(v, ",") {
			if t := strings.TrimSpace(e); t != "" {
				ignored = append(ignored, t)
			}
		}
	}

	cfg := &Config{
		SessionSecret:         []byte(mustEnv("SESSION_SECRET")),
		HAUrl:                 mustEnv("HA_URL"),
		HAToken:               mustEnv("HA_TOKEN"),
		EntityUnlockAllowance: envOr("ENTITY_UNLOCK_ALLOWANCE", "input_boolean.home_keys_enabled"),
		EntityDoorCode:        envOr("ENTITY_DOOR_CODE", "input_text.home_keys_code"),
		IgnoredEntities:       ignored,
		ListenAddr:            envOr("LISTEN_ADDR", ":8080"),
	}
	if len(cfg.SessionSecret) < 16 {
		log.Fatal("SESSION_SECRET is too short (minimum 16 characters)")
	}
	return cfg
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required environment variable %s is not set", key)
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var doorDomains = map[string]bool{
	"lock": true,
}

func discoverDoors(ctx context.Context, ha *HAClient, ignored map[string]bool) ([]DoorConfig, error) {
	states, err := ha.GetAllStates(ctx)
	if err != nil {
		return nil, err
	}

	var doors []DoorConfig
	for _, s := range states {
		domain, _, _ := strings.Cut(s.EntityID, ".")
		if !doorDomains[domain] || ignored[s.EntityID] {
			continue
		}
		name := s.FriendlyName
		if name == "" {
			name = s.EntityID
		}
		doors = append(doors, DoorConfig{
			Key:      strings.ReplaceAll(s.EntityID, ".", "_"),
			Name:     name,
			EntityID: s.EntityID,
		})
	}
	return doors, nil
}

func main() {
	cfg := loadConfig()

	tmpl := template.Must(template.ParseFS(templateFS, "templates/*.html"))
	haClient := NewHAClient(cfg.HAUrl, cfg.HAToken)
	rl := newRateLimiter()
	go rl.cleanup()

	ignoredSet := make(map[string]bool, len(cfg.IgnoredEntities))
	for _, e := range cfg.IgnoredEntities {
		ignoredSet[e] = true
	}

	discoverCtx, discoverCancel := context.WithTimeout(context.Background(), 10*time.Second)
	doors, err := discoverDoors(discoverCtx, haClient, ignoredSet)
	discoverCancel()
	if err != nil {
		log.Fatalf("entity discovery failed: %v", err)
	}
	if len(doors) == 0 {
		log.Fatal("no door entities discovered — check HA connection and IGNORED_ENTITIES")
	}
	log.Printf("Discovered %d door(s)", len(doors))

	app := &App{
		Config:      cfg,
		HAClient:    haClient,
		RateLimiter: rl,
		Templates:   tmpl,
		Doors:       doors,
	}

	startupCtx, startupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if errs := app.checkEntities(startupCtx); len(errs) > 0 {
		startupCancel()
		log.Fatalf("startup health check failed:\n  %s", strings.Join(errs, "\n  "))
	}
	startupCancel()
	log.Printf("Startup health check passed")

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", app.HealthHandler)
	mux.HandleFunc("/login", app.LoginHandler)
	mux.HandleFunc("/logout", app.LogoutHandler)
	mux.Handle("/open", app.RequireAuth(http.HandlerFunc(app.OpenDoorHandler)))
	mux.Handle("/", app.RequireAuth(http.HandlerFunc(app.DashboardHandler)))

	server := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Starting home-keys on %s", cfg.ListenAddr)
	log.Fatal(server.ListenAndServe())
}
