# How-to guides

## Rotate the PIN

1. Open Home Assistant and navigate to **Settings → Helpers**.
2. Find the `input_text` entity configured as `ENTITY_DOOR_CODE`.
3. Change its value to the new PIN.

All existing sessions are invalidated immediately — anyone logged in will be redirected to the login screen on their next request.

---

## Add a new door

home-keys discovers door entities automatically at startup. To add a new door:

1. Create or add a `lock` entity in Home Assistant.
2. Set a descriptive `friendly_name` for the entity — this is used as the button label.
3. Restart home-keys (`docker compose restart`).

The new entity will appear as a button on the dashboard.

---

## Hide a door from the dashboard

Set `IGNORED_ENTITIES` in `.env` to a comma-separated list of entity IDs you want to exclude:

```dotenv
IGNORED_ENTITIES=lock.internal_door,lock.test_entity
```

Restart the service. The listed entities will not be discovered at startup and will not appear on the dashboard.

---

## Enable and disable unlocking

Toggle the `input_boolean` configured as `ENTITY_UNLOCK_ALLOWANCE` in Home Assistant.

- **on** — users can open doors from the dashboard.
- **off** — the dashboard shows a "not yet enabled" notice; door-open requests return 403.

![Dashboard locked state](screenshots/dashboard-locked.png)

No restart is required — the state is read on every request.

---

## Restrict door controls to a specific network

Set `ALLOWED_NETWORKS` in `.env` to a comma-separated list of IPv4 or IPv6 CIDR ranges:

```dotenv
ALLOWED_NETWORKS=192.168.1.0/24,fd00::/8
```

Restart the service. Authenticated visitors whose IP falls outside these ranges will see a "Please join the WiFi" message on the dashboard and cannot open doors. Visitors inside the ranges are unaffected.

Leave `ALLOWED_NETWORKS` unset (the default) to allow all authenticated visitors regardless of network.

> **Tip:** If home-keys runs behind a reverse proxy, make sure the proxy forwards the real client IP via `X-Forwarded-For` — the allowlist check reads that header first.

---

## Deploy an update

```bash
git pull
docker compose up --build -d
```

The new image is built and the container is replaced with zero config changes required.
