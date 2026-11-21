# ADR-007: Plugin Architecture — hashicorp/go-plugin with gRPC

**Status:** Accepted
**Date:** 2026-11-21
**Decision Makers:** Principal Architect

## Context

Knowledge Tree already has a working plugin system using hashicorp/go-plugin with gRPC, supporting 22 discovery plugins. The research phase identified WASM (Extism + WIT) as a potential alternative with better sandboxing and multi-language support.

## Decision

**Keep hashicorp/go-plugin with gRPC for Zerelio Knowledge.** WASM migration deferred to a future phase.

## Rationale

1. **22 working plugins exist.** Rewriting the plugin framework means rewriting all 22 plugins. The migration cost is not justified.
2. **gRPC process isolation already provides sandboxing.** Each plugin runs as a separate OS process — a crash in a plugin doesn't crash the host.
3. **Go is the right language for infrastructure plugins.** All major cloud SDKs (AWS, Azure, GCP) have first-class Go support. WASM would require recompiling cloud SDKs to WASM, which is not well-supported.
4. **hashicorp/go-plugin is battle-tested.** Used by Terraform, Vault, Nomad, and Packer in production at massive scale.

## Plugin Interface

```go
type DiscoveryPlugin interface {
    Info() PluginInfo
    Discover(ctx context.Context, req DiscoverRequest) (<-chan DiscoveryEvent, error)
    ValidateConfig(config map[string]string) ValidationResult
}
```

Plugins emit a stream of DiscoveryEvents (resource, relationship, progress, or error) over a buffered gRPC channel.

## Plugin SDK

The SDK (`pkg/sdk/`) provides:
- `Serve()` function for plugin binary entry point
- Helper functions for creating resources and relationships
- AES-GCM encryption service for credential handling
- Resilient HTTP client with circuit breaker and retry
- Configuration parsing utilities

## Future Considerations

- **WASM support** can be added as an alternative plugin runtime alongside gRPC, not replacing it
- **OCI plugin registry** for plugin distribution is a future feature
- **Plugin marketplace** for community plugins is a Zerelio Security commercial feature

## Consequences

**Positive:**
- Zero migration cost for 22 existing plugins
- Proven architecture at scale
- Strong ecosystem (cloud SDKs, monitoring libraries all native Go)

**Negative:**
- Plugins must be written in Go (or any language supporting hashicorp/go-plugin's gRPC protocol)
- Process-per-plugin has higher memory overhead than WASM sandboxing
- No built-in capability-mediated access (mitigated: credential broker controls what credentials plugins receive)
