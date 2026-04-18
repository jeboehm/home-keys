# Reference

## Environment variables

| Variable                  | Required | Default                           | Description                                                                       |
| ------------------------- | -------- | --------------------------------- | --------------------------------------------------------------------------------- |
| `SESSION_SECRET`          | Yes      | —                                 | HMAC signing secret. Minimum 16 characters. Generate with `openssl rand -hex 32`. |
| `HA_URL`                  | Yes      | —                                 | Base URL of your Home Assistant instance, e.g. `http://homeassistant:8123`.       |
| `HA_TOKEN`                | Yes      | —                                 | Long-lived access token from HA → Profile → Security.                             |
| `ENTITY_UNLOCK_ALLOWANCE` | Yes      | `input_boolean.home_keys_enabled` | Entity ID of the `input_boolean` that gates door unlocking.                       |
| `ENTITY_DOOR_CODE`        | Yes      | `input_text.home_keys_code`       | Entity ID of the `input_text` holding the login PIN. Empty state = no PIN.        |
| `IGNORED_ENTITIES`        | No       | —                                 | Comma-separated list of `lock` entity IDs to exclude from the dashboard.          |
| `LISTEN_ADDR`             | No       | `:8080`                           | TCP address the HTTP server binds to.                                             |

---

## HTTP routes

| Method | Path       | Auth     | Description                                                                                   |
| ------ | ---------- | -------- | --------------------------------------------------------------------------------------------- |
| `GET`  | `/login`   | —        | Renders the login form. Redirects to `/` if no PIN is configured.                             |
| `POST` | `/login`   | —        | Validates the submitted PIN and issues a session cookie.                                      |
| `POST` | `/logout`  | —        | Clears the session cookie and redirects to `/login`.                                          |
| `GET`  | `/`        | Required | Renders the door dashboard.                                                                   |
| `POST` | `/open`    | Required | Opens the door identified by the `door` form field.                                           |
| `GET`  | `/healthz` | —        | Returns JSON health status. `200 {"status":"ok"}` or `503 {"status":"error","errors":[...]}`. |

---

## Supported entity domains

Only `lock` entities are auto-discovered and displayed on the dashboard. The HA service called is `lock.unlock`.

---

## Log prefixes

| Prefix           | Meaning                                                        |
| ---------------- | -------------------------------------------------------------- |
| `[LOGIN_OK]`     | Successful login                                               |
| `[LOGIN_FAIL]`   | Failed login attempt (wrong code or rate-limited)              |
| `[DOOR]`         | Door successfully opened                                       |
| `[DOOR_BLOCKED]` | Open attempt rejected because `ENTITY_UNLOCK_ALLOWANCE` is off |
| `[DOOR_ERROR]`   | HA service call failed                                         |
| `[ERROR]`        | Internal error (HA unreachable, template error, etc.)          |
| `[HA]`           | Raw HA service call response body                              |

---

## Rate limiting

Login attempts are rate-limited per IP address: **5 attempts per 15-minute window**. After 5 failures, subsequent attempts return the "Falscher Code" error without querying HA until the window resets.
