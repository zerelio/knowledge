# ADR-008: No License Validation in Self-Hosted OSS Core

**Status:** Accepted
**Date:** 2026-11-21
**Supersedes:** Parts of ADR-005 (licensing strategy)

## Context

ADR-005 described a future Zerelio Security commercial product with per-seat
licensing enforced via Ed25519 JWT tokens from a central license server.

An implementation was started (`security/license/`) based on this model. After
review, the model is wrong for the product as it actually exists.

## Problem

Zerelio is self-hosted open-source software. Any operator who runs it:

1. Already controls the binary and can modify it.
2. Already configures their own authentication (JWT keys, passwords, auth disable flag).
3. Has no dependency on a Zerelio-operated server.

A license token from a central server would require:
- Operators to call home to validate their own instance.
- Zerelio to operate a license server with 100% uptime.
- Friction for every self-hosted deployment.

None of these make sense. Because the operator controls the deployment, there
is nothing a license token could protect that a motivated operator could not
trivially bypass by modifying the config or the binary.

## Decision

**Remove the license validation module entirely from the security/ package.**

The security/ module contains two things worth keeping:

- **RBAC** (`security/rbac/`): Controls what authenticated users are allowed to
  do _within_ an instance. Fully local, no external dependency. Useful.

- **Threat intelligence** (`security/threat/`): Enriches discovered resources
  with local misconfiguration checks and a hook interface for external threat
  feeds. Fully local, no external dependency. Useful.

If a commercial distribution of Zerelio ever exists, licensing would be
handled at the distribution layer (e.g., a hosted SaaS offering, not an
in-process token check in the OSS binary).

## Consequences

- `security/license/` and its JWT dependency are deleted.
- The `replace github.com/zerelio/zerelio => ../` in security/go.mod is kept
  because rbac/ and threat/ still import pkg/types.
- ADR-005's mention of "per-seat or per-node pricing via Ed25519 JWT" is
  superseded by this decision.
- Future commercial value comes from: hosted SaaS, enterprise support, and
  proprietary integrations — not from a license gate on a self-hosted binary.
