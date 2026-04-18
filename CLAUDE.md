# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
go build ./...

# Run locally (requires .env populated from .env.example)
go run .

# Run with Docker Compose
docker compose up --build

# Vet / lint
go vet ./...
```

There are no automated tests in this project.

## Architecture

`home-keys` is a small, self-contained Go HTTP server (no external dependencies beyond the standard library) that provides a PIN-protected web UI to open doors via the Home Assistant REST API.

**Request flow:**

1. `main.go` — loads config from env, wires up `App`, runs a startup health check against all HA entities, then starts `net/http` with five routes.
2. `middleware.go` — `RequireAuth` wraps protected handlers; on each request it fetches the current door code from HA and validates the session cookie HMAC against it (so rotating the code in HA instantly invalidates all sessions). Also hosts the in-memory IP rate limiter (5 attempts / 15 min window).
3. `auth.go` — stateless session tokens: `random_32_bytes_hex.HMAC-SHA256`. The signing key is derived as `HMAC(SESSION_SECRET, current_code)`, binding token validity to the code value.
4. `ha.go` — thin HA REST API client. `DomainService()` maps entity ID prefixes (`lock`, `cover`, `button`, `script`) to the correct HA service call.
5. `handlers.go` — `LoginHandler`, `DashboardHandler`, `OpenDoorHandler`, `HealthHandler`. Door opening is gated on HA `input_boolean.home_keys_enabled` being `on`.
6. `templates/` — two embedded HTML templates (`login.html`, `dashboard.html`) compiled into the binary via `//go:embed`.

**Key entities read from HA at runtime:**

- `input_boolean.home_keys_enabled` — gate for door-open actions
- `input_text.home_keys_code` — current PIN (empty = no PIN required)
- All lock entities

**Deployment:** Docker multi-stage build produces a `scratch`-based image.
