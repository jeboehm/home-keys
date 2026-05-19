# Explanation

## Why stateless sessions?

home-keys has no database. Sessions are encoded entirely in a signed cookie — the server stores nothing between requests. This keeps the deployment minimal (a single binary in a scratch container) and makes horizontal scaling trivial.

The signing key is derived from `HMAC(SESSION_SECRET, current_PIN)`. This means that changing the PIN in Home Assistant immediately invalidates every active session: the next request by any logged-in user will fail HMAC validation and redirect them to the login screen. No explicit session store or revocation list is needed.

---

## How entity discovery works

At startup, home-keys calls the Home Assistant `/api/states` endpoint, which returns the state of every entity in the system. The application filters this list to entities with the `lock` domain prefix and calls `lock.unlock` to open them. Entities listed in `IGNORED_ENTITIES` are removed from this set.

The button label on the dashboard comes from the entity's `friendly_name` attribute in HA. If no `friendly_name` is set, the entity ID is used as a fallback. This means renaming an entity in HA and restarting home-keys is all that is needed to update the label.

The set of doors is fixed at startup. Changes in HA (new entities, renames) take effect after a restart.

---

## The unlock allowance gate

The `ENTITY_UNLOCK_ALLOWANCE` `input_boolean` acts as a real-time gate. Its state is checked on every dashboard render and every door-open request — not cached. Turning it off in HA takes effect for the next user interaction with no restart needed.

When the allowance is off, logged-in users see the "not yet enabled" notice:

![Dashboard — unlocking disabled](screenshots/dashboard-locked.png)

Any direct `POST /open` request is rejected with `403 Forbidden` regardless of how it is sent.

---

## The network allowlist

`ALLOWED_NETWORKS` adds a second layer of protection on top of PIN authentication. When configured, the dashboard and `POST /open` check whether the visitor's IP belongs to any of the listed CIDRs **after** validating the session but **before** consulting the unlock allowance `input_boolean`.

This ordering means:

1. Unauthenticated visitors → redirected to login (unchanged)
2. Authenticated visitor, IP not in allowlist → "Please join the WiFi" message; `POST /open` returns 403
3. Authenticated visitor, IP in allowlist, allowance off → "Not activated" message; `POST /open` returns 403
4. Authenticated visitor, IP in allowlist, allowance on → door buttons active

Because the check happens per-request, moving a device off the trusted network takes effect immediately with no session invalidation needed.

Both IPv4 (`192.168.1.0/24`) and IPv6 (`fd00::/8`) CIDRs are supported and can be mixed in a single `ALLOWED_NETWORKS` value. When `ALLOWED_NETWORKS` is unset, the feature is disabled and all authenticated visitors can operate doors (the pre-existing behaviour).

The client IP is resolved from `X-Forwarded-For` (first entry) when present, falling back to the TCP remote address — the same logic used by the login rate limiter.

---

## Rate limiting design

The in-memory rate limiter uses a per-IP sliding window (5 attempts / 15 minutes). It is intentionally simple:

- No persistence — a restart resets all buckets. This is acceptable because the limiter's purpose is to slow down online guessing, not to enforce a hard policy over restarts.
- Memory is bounded — expired buckets are swept every 5 minutes by a background goroutine.
- The limit applies to login POST attempts only, not to authenticated requests.

For deployments behind a reverse proxy, the limiter reads the `X-Forwarded-For` header to get the real client IP.
