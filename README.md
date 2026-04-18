# home-keys

A minimal web app that lets guests open doors via Home Assistant — protected by a PIN code managed directly in HA.

![Dashboard](docs/screenshots/dashboard.png)

## How it works

- Guests visit the URL and enter the current PIN (stored in an `input_text` helper in Home Assistant)
- After login, they see a button for every discovered lock entity — but only if the `input_boolean` unlock allowance is switched on in HA
- Pressing a button calls the appropriate HA service (`lock.unlock`) for that entity
- All `lock` entities and their display names are **auto-discovered** at startup from Home Assistant — no hardcoded entity IDs required

The session token is HMAC-bound to the current PIN, so changing the PIN in HA instantly invalidates all active sessions.

## Requirements

- Home Assistant with a long-lived access token
- Two HA helpers:
  - An `input_boolean` — enables/disables door access (`ENTITY_UNLOCK_ALLOWANCE`)
  - An `input_text` — holds the current PIN, leave empty to disable PIN requirement (`ENTITY_DOOR_CODE`)
- One or more `lock` entities in Home Assistant

## Quick start

```bash
cp .env.example .env
# edit .env with your HA URL, token, and helper entity IDs
docker compose up --build
```

See the [tutorial](docs/tutorial.md) for a full step-by-step walkthrough.

## Configuration

Copy `.env.example` to `.env` and fill in the values:

| Variable                  | Required | Description                                               |
| ------------------------- | -------- | --------------------------------------------------------- |
| `SESSION_SECRET`          | Yes      | Random secret for HMAC signing (`openssl rand -hex 32`)   |
| `HA_URL`                  | Yes      | Home Assistant base URL, e.g. `http://homeassistant:8123` |
| `HA_TOKEN`                | Yes      | Long-lived access token from HA                           |
| `ENTITY_UNLOCK_ALLOWANCE` | Yes      | `input_boolean` entity ID that enables door unlocking     |
| `ENTITY_DOOR_CODE`        | Yes      | `input_text` entity ID holding the login PIN              |
| `IGNORED_ENTITIES`        | No       | Comma-separated entity IDs to hide from the dashboard     |
| `LISTEN_ADDR`             | No       | Listen address (default: `:8080`)                         |

## Running

**Docker Compose (recommended):**

```bash
cp .env.example .env
# edit .env
docker compose up --build
```

**Local:**

```bash
go run .
```

On startup the app discovers all door entities from HA, then verifies it can reach all configured entities. If any are unreachable, it exits with an error.

## Documentation

| Document                           | Type        | Contents                                             |
| ---------------------------------- | ----------- | ---------------------------------------------------- |
| [Tutorial](docs/tutorial.md)       | Tutorial    | Step-by-step setup from clone to first door open     |
| [How-to guides](docs/how-to.md)    | How-to      | Rotate PIN, add/ignore doors, deploy updates         |
| [Reference](docs/reference.md)     | Reference   | All env vars, HTTP routes, log prefixes, rate limits |
| [Explanation](docs/explanation.md) | Explanation | Auth model, entity discovery, unlock allowance gate  |

## Security notes

- Sessions are stateless HMAC cookies (no server-side storage)
- Login is rate-limited to 5 attempts per IP per 15 minutes
- Cookies are `HttpOnly`, `Secure`, `SameSite=Strict`
- The session signing key is derived from `SESSION_SECRET + current PIN`, so rotating the PIN invalidates all sessions

## Architecture

**Request flow:**

1. `main.go` — loads config from env, discovers door entities from HA, wires up `App`, runs a startup health check, then starts `net/http` with five routes.
2. `middleware.go` — `RequireAuth` wraps protected handlers; on each request it fetches the current door code from HA and validates the session cookie HMAC against it (so rotating the code in HA instantly invalidates all sessions). Also hosts the in-memory IP rate limiter (5 attempts / 15 min window).
3. `auth.go` — stateless session tokens: `random_32_bytes_hex.HMAC-SHA256`. The signing key is derived as `HMAC(SESSION_SECRET, current_code)`, binding token validity to the code value.
4. `ha.go` — thin HA REST API client. Calls `lock.unlock` for all discovered lock entities. `GetAllStates()` fetches all entities for auto-discovery.
5. `handlers.go` — `LoginHandler`, `DashboardHandler`, `OpenDoorHandler`, `HealthHandler`. Door opening is gated on the unlock allowance `input_boolean` being `on`.
6. `templates/` — two embedded HTML templates (`login.html`, `dashboard.html`) compiled into the binary via `//go:embed`.
