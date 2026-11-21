# ADR-006: Authentication and Authorization Architecture

**Status:** Accepted
**Date:** 2026-11-21
**Decision Makers:** Principal Architect

## Context

Knowledge Tree's auth service has critical security issues: SHA-256 password hashing with a static salt, data races on the user map, no persistent storage, hardcoded JWT secrets, and no token rotation. The auth service is a separate microservice (500 lines) that adds operational complexity without benefit.

## Decision

### Merge auth into the main server module

Auth becomes `internal/auth/` within zerelio-server. No separate process, no HTTP proxying.

### Ed25519 for JWT signing (not HS256)

- **HS256 (HMAC-SHA256):** Symmetric — the same secret signs and verifies. If any service that verifies tokens is compromised, the attacker can forge tokens.
- **Ed25519:** Asymmetric — private key signs, public key verifies. Only the auth module holds the private key. All other components (and external services) can verify with the public key without being able to forge tokens.

Ed25519 is compact (64-byte signatures vs 256-byte RSA), fast (faster than RS256), and has no known side-channel vulnerabilities with constant-time implementations.

### Argon2id for password hashing

- **SHA-256 (current):** Fast hash, unsuitable for passwords. Can be brute-forced at billions of hashes per second on GPUs.
- **bcrypt:** Good, but fixed 72-byte password limit and aging resistance to GPU attacks.
- **Argon2id (chosen):** Winner of the Password Hashing Competition. Memory-hard, resistant to GPU and ASIC attacks. Parameters: time=1, memory=64MB, threads=4, salt=16 bytes (random per user).

### Refresh token rotation

When a refresh token is used, the old token is invalidated and a new one is issued. This limits the window of exploitation if a refresh token is stolen.

### Persistent storage for all auth state

All auth state (users, sessions, refresh tokens, API tokens) is stored in PostgreSQL. No in-memory-only state except ephemeral caches.

## Token Lifecycle

```
Login:
  1. Validate credentials (email + password against argon2id hash)
  2. Issue access token (Ed25519 signed, 15-minute expiry)
  3. Issue refresh token (Ed25519 signed, 7-day expiry, stored in DB)
  4. Return both tokens

API Request:
  1. Extract Bearer token from Authorization header
  2. Verify Ed25519 signature
  3. Check expiry
  4. Extract claims (user_id, email, roles)
  5. Inject claims into request context

Token Refresh:
  1. Validate refresh token (signature, expiry)
  2. Check refresh token exists in DB (not revoked)
  3. Revoke old refresh token
  4. Issue new access token + new refresh token
  5. Return both

Logout:
  1. Revoke refresh token in DB
  2. (Access token naturally expires in ≤15 minutes)
```

## RBAC Model

```
Permissions (granular):
  resources:read, resources:write, resources:delete
  graph:read, graph:write
  discovery:read, discovery:run
  docs:read, docs:write
  admin:read, admin:write
  audit:read
  data:export, data:import

Default Roles (inheriting):
  viewer: resources:read, graph:read, discovery:read, docs:read
  editor: viewer + resources:write, discovery:run, docs:write, data:export
  admin: editor + resources:delete, graph:write, admin:*, audit:read, data:import

Custom Roles:
  Defined by combining permissions
  Stored in database
  Assignable to users
```

## Consequences

**Positive:**
- Eliminates all critical security issues from Knowledge Tree auth
- Asymmetric JWT signing prevents token forgery even if verifiers are compromised
- Memory-hard password hashing resists GPU/ASIC brute force
- Persistent storage eliminates state loss on restart
- Token rotation limits stolen credential exposure window
- Fine-grained permissions enable least-privilege access

**Negative:**
- Ed25519 key management adds complexity (key generation, rotation, distribution)
- Argon2id hashing is intentionally slow (~100ms per hash) — acceptable for login but not for hot paths
- Refresh token rotation means clients must handle token updates on every refresh
