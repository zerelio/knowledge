// Package credentials resolves plugin credentials from various backends.
// The registry pattern allows adding new credential sources (env, file,
// Vault, AWS Secrets Manager) without changing the orchestration layer.
package credentials

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// Provider resolves credentials for a given source type and path.
type Provider interface {
	// Name is the unique identifier (e.g., "env", "file", "vault").
	Name() string
	// Resolve returns key-value credentials. The meaning of path and params
	// depends on the provider.
	Resolve(ctx context.Context, path string, params map[string]string) (map[string]string, error)
}

// Registry holds all registered credential providers.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry creates a Registry pre-loaded with built-in providers.
func NewRegistry() *Registry {
	r := &Registry{providers: make(map[string]Provider)}
	r.Register(&EnvProvider{})
	r.Register(&FileProvider{})
	r.Register(&StaticProvider{})
	return r
}

// Register adds a provider. Replaces any existing provider with the same name.
func (r *Registry) Register(p Provider) {
	r.providers[p.Name()] = p
}

// Resolve delegates to the named provider.
func (r *Registry) Resolve(ctx context.Context, providerName, path string, params map[string]string) (map[string]string, error) {
	p, ok := r.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("unknown credential provider %q (registered: %s)", providerName, r.registeredNames())
	}
	return p.Resolve(ctx, path, params)
}

func (r *Registry) registeredNames() string {
	names := make([]string, 0, len(r.providers))
	for n := range r.providers {
		names = append(names, n)
	}
	return strings.Join(names, ", ")
}

// ─── Built-in providers ──────────────────────────────────────────────────────

// EnvProvider resolves credentials from environment variables.
// path is a comma-separated list of VAR_NAME=cred_key mappings, e.g.:
//   "AWS_ACCESS_KEY_ID=access_key_id,AWS_SECRET_ACCESS_KEY=secret_access_key"
type EnvProvider struct{}

func (EnvProvider) Name() string { return "env" }

func (EnvProvider) Resolve(_ context.Context, path string, params map[string]string) (map[string]string, error) {
	result := make(map[string]string)
	// params may contain direct key-value overrides.
	for k, v := range params {
		result[k] = v
	}
	// path contains ENV_VAR=cred_key mappings.
	if path != "" {
		for _, mapping := range strings.Split(path, ",") {
			parts := strings.SplitN(strings.TrimSpace(mapping), "=", 2)
			if len(parts) != 2 {
				continue
			}
			envVar, credKey := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			if val := os.Getenv(envVar); val != "" {
				result[credKey] = val
			}
		}
	}
	return result, nil
}

// FileProvider resolves credentials from a file. path is the file path.
// The file must contain lines of key=value.
type FileProvider struct{}

func (FileProvider) Name() string { return "file" }

func (FileProvider) Resolve(_ context.Context, path string, params map[string]string) (map[string]string, error) {
	result := make(map[string]string)
	for k, v := range params {
		result[k] = v
	}
	if path == "" {
		return result, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("credential file %q: %w", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return result, nil
}

// StaticProvider passes through the params map directly.
// Useful when credentials are already embedded in scope config.
type StaticProvider struct{}

func (StaticProvider) Name() string { return "static" }

func (StaticProvider) Resolve(_ context.Context, _ string, params map[string]string) (map[string]string, error) {
	result := make(map[string]string, len(params))
	for k, v := range params {
		result[k] = v
	}
	return result, nil
}
