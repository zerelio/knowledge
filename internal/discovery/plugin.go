package discovery

import (
	"context"

	"github.com/zerelio/zerelio/pkg/types"
)

// Plugin is the interface that all discovery plugins must implement.
// Plugins are loaded via hashicorp/go-plugin over gRPC.
type Plugin interface {
	// Name returns the unique plugin identifier (e.g., "aws", "gcp", "k8s").
	Name() string

	// Discover runs discovery against the given scope and streams results
	// back via the provided channel. The plugin must close the channel when done.
	Discover(ctx context.Context, scope *types.DiscoveryScope) (<-chan DiscoveryResult, error)
}

// DiscoveryResult is a batch of resources and relationships from a plugin.
type DiscoveryResult struct {
	Resources     []*types.Resource
	Relationships []*types.Relationship
	Err           error
}

// PluginConfig holds configuration for launching a plugin subprocess.
type PluginConfig struct {
	// Path is the absolute path to the plugin executable.
	Path string

	// Name is the plugin identifier (must match Plugin.Name()).
	Name string

	// Env contains additional environment variables for the subprocess.
	Env []string
}
