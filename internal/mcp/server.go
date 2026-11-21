// Package mcp implements the Model Context Protocol server for Zerelio.
// It exposes infrastructure discovery data through MCP tools and resources
// that AI agents consume. Supports both stdio (subprocess) and HTTP+SSE transports.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zerelio/zerelio/internal/graph"
	"github.com/zerelio/zerelio/internal/search"
	"github.com/zerelio/zerelio/internal/storage"
)

// Server is the core MCP server. It holds storage references and routes
// incoming JSON-RPC messages to tool handlers and resource readers.
type Server struct {
	store    storage.Backend
	graph    *graph.Graph    // optional
	searcher *search.Searcher // optional
	logger   *slog.Logger

	initialized bool
	initMu      sync.RWMutex
}

func NewServer(store storage.Backend, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		store:  store,
		logger: logger.With("component", "mcp"),
	}
}

// SetGraph attaches the graph module for query_graph tool support.
func (s *Server) SetGraph(g *graph.Graph) { s.graph = g }

// SetSearcher attaches the search module for search_resources tool support.
func (s *Server) SetSearcher(sr *search.Searcher) { s.searcher = sr }

// ServeStdio starts the server on stdin/stdout. Blocks until ctx is cancelled.
func (s *Server) ServeStdio(ctx context.Context) error {
	return NewStdioTransport(s, s.logger).Serve(ctx)
}

// ServeHTTP starts the server on an HTTP+SSE port. Blocks until ctx is cancelled.
func (s *Server) ServeHTTP(ctx context.Context, port int) error {
	return NewHTTPTransport(s, port, s.logger).Serve(ctx)
}

// HandleMessage implements MessageHandler. Routes JSON-RPC to the right handler.
func (s *Server) HandleMessage(ctx context.Context, raw []byte) ([]byte, error) {
	req, err := ParseRequest(raw)
	if err != nil {
		return MustMarshal(NewErrorResponse(json.RawMessage("null"), ErrParseError, err.Error(), nil)), nil
	}

	s.logger.Debug("mcp request", "method", req.Method)

	if req.IsNotification() {
		s.handleNotification(ctx, req)
		return nil, nil
	}

	var resp *Response
	switch req.Method {
	case "initialize":
		resp = s.handleInitialize(ctx, req)
	case "ping":
		resp = NewSuccessResponse(req.ID, PingResult{})
	case "tools/list":
		resp = NewSuccessResponse(req.ID, ListToolsResult{Tools: builtInTools()})
	case "tools/call":
		resp = s.handleToolsCall(ctx, req)
	case "resources/list":
		resp = NewSuccessResponse(req.ID, ListResourcesResult{Resources: resourceDescriptions()})
	case "resources/read":
		resp = s.handleResourcesRead(ctx, req)
	case "notifications/initialized":
		resp = NewSuccessResponse(req.ID, struct{}{})
	default:
		resp = NewErrorResponse(req.ID, ErrMethodNotFound, fmt.Sprintf("unknown method: %s", req.Method), nil)
	}

	return MustMarshal(resp), nil
}

func (s *Server) handleNotification(_ context.Context, req *Request) {
	if req.Method == "notifications/initialized" {
		s.initMu.Lock()
		s.initialized = true
		s.initMu.Unlock()
	}
}

func (s *Server) handleInitialize(_ context.Context, req *Request) *Response {
	var params InitializeParams
	if len(req.Params) > 0 {
		json.Unmarshal(req.Params, &params)
	}
	s.logger.Info("mcp initialize", "client", params.ClientInfo.Name)
	s.initMu.Lock()
	s.initialized = true
	s.initMu.Unlock()

	return NewSuccessResponse(req.ID, InitializeResult{
		ProtocolVersion: ServerProtocolVersion,
		Capabilities: ServerCapabilities{
			Tools:     &ToolsCapability{},
			Resources: &ResourcesCapability{},
			Logging:   &LoggingCapability{},
		},
		ServerInfo: ImplementationInfo{Name: "zerelio", Version: ServerVersion},
	})
}

func (s *Server) handleToolsCall(ctx context.Context, req *Request) *Response {
	var params ToolCallParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return NewErrorResponse(req.ID, ErrInvalidParams, "invalid params", nil)
		}
	}
	if params.Name == "" {
		return NewErrorResponse(req.ID, ErrInvalidParams, "tool name required", nil)
	}
	result, err := s.callTool(ctx, &params)
	if err != nil {
		return NewErrorResponse(req.ID, ErrToolExecution, err.Error(), nil)
	}
	return NewSuccessResponse(req.ID, result)
}

func (s *Server) handleResourcesRead(ctx context.Context, req *Request) *Response {
	var params ResourceReadParams
	if len(req.Params) > 0 {
		json.Unmarshal(req.Params, &params)
	}
	if params.URI == "" {
		return NewErrorResponse(req.ID, ErrInvalidParams, "uri required", nil)
	}
	result, err := s.readResource(ctx, params.URI)
	if err != nil {
		return NewErrorResponse(req.ID, ErrResourceNotFound, err.Error(), nil)
	}
	return NewSuccessResponse(req.ID, result)
}

// ─── Tools ───────────────────────────────────────────────────────────────────

func builtInTools() []Tool {
	return []Tool{
		{
			Name:        "query_infrastructure",
			Description: "Query discovered infrastructure by type, provider, region, or keyword. Returns matching resources.",
			InputSchema: Schema{
				Type: "object",
				Properties: map[string]Property{
					"query":    {Type: "string", Description: "Search term or resource type (e.g., 'aws.ec2.instance')"},
					"provider": {Type: "string", Description: "Filter by provider (aws, gcp, azure, k8s)"},
					"region":   {Type: "string", Description: "Filter by region"},
					"limit":    {Type: "integer", Description: "Max results (default 50)"},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "list_scopes",
			Description: "List all configured discovery scopes with their plugin, status, and last run time.",
			InputSchema: Schema{Type: "object", Properties: map[string]Property{}},
		},
		{
			Name:        "get_summary",
			Description: "Return an executive summary of all discovered infrastructure: counts by provider and type.",
			InputSchema: Schema{Type: "object", Properties: map[string]Property{}},
		},
		{
			Name:        "get_topology",
			Description: "Return infrastructure topology as a Mermaid diagram for a given scope or provider.",
			InputSchema: Schema{
				Type: "object",
				Properties: map[string]Property{
					"scope": {Type: "string", Description: "Scope name or 'all'"},
				},
			},
		},
		{
			Name:        "get_relationships",
			Description: "Return all relationships for a given resource ID.",
			InputSchema: Schema{
				Type:     "object",
				Required: []string{"resource_id"},
				Properties: map[string]Property{
					"resource_id": {Type: "string", Description: "Resource ID"},
				},
			},
		},
		{
			Name:        "search_resources",
			Description: "Full-text search over all discovered infrastructure. Supports BM25 ranking and optional filters. Returns scored results.",
			InputSchema: Schema{
				Type:     "object",
				Required: []string{"query"},
				Properties: map[string]Property{
					"query":    {Type: "string", Description: "Search terms (e.g., 'prod database', 'eks cluster us-east-1')"},
					"provider": {Type: "string", Description: "Optional: filter by provider (aws, gcp, azure, k8s)"},
					"type":     {Type: "string", Description: "Optional: filter by resource type"},
					"limit":    {Type: "integer", Description: "Max results (default 20)"},
				},
			},
		},
		{
			Name:        "get_resource",
			Description: "Get full details for a single resource by its ID.",
			InputSchema: Schema{
				Type:     "object",
				Required: []string{"resource_id"},
				Properties: map[string]Property{
					"resource_id": {Type: "string", Description: "Resource ID"},
				},
			},
		},
		{
			Name:        "query_graph",
			Description: "Traverse the infrastructure knowledge graph. Returns the neighbourhood (nodes + edges) around a resource up to N hops away.",
			InputSchema: Schema{
				Type:     "object",
				Required: []string{"resource_id"},
				Properties: map[string]Property{
					"resource_id": {Type: "string", Description: "Starting resource ID"},
					"hops":        {Type: "integer", Description: "Max traversal depth (1-5, default 2)"},
				},
			},
		},
	}
}

func (s *Server) callTool(ctx context.Context, params *ToolCallParams) (*ToolResult, error) {
	args := params.Arguments
	switch params.Name {
	case "query_infrastructure":
		return s.toolQueryInfrastructure(ctx, args)
	case "list_scopes":
		return s.toolListScopes(ctx)
	case "get_summary":
		return s.toolGetSummary(ctx)
	case "get_topology":
		return s.toolGetTopology(ctx, args)
	case "get_relationships":
		return s.toolGetRelationships(ctx, args)
	case "search_resources":
		return s.toolSearchResources(ctx, args)
	case "get_resource":
		return s.toolGetResource(ctx, args)
	case "query_graph":
		return s.toolQueryGraph(ctx, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", params.Name)
	}
}

func (s *Server) toolQueryInfrastructure(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	query := stringArg(args, "query")
	provider := stringArg(args, "provider")
	region := stringArg(args, "region")
	limit := intArg(args, "limit", 50)

	filter := storage.ResourceFilter{
		Query:    query,
		Provider: provider,
		Region:   region,
		Limit:    limit,
	}
	resources, total, err := s.store.GetResources(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("storage error: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Infrastructure Query: %q\n\n", query))
	sb.WriteString(fmt.Sprintf("**Found:** %d (showing %d)\n\n", total, len(resources)))
	if len(resources) == 0 {
		sb.WriteString("_No resources matched._\n")
	} else {
		sb.WriteString("| ID | Name | Type | Provider | Region |\n")
		sb.WriteString("|----|------|------|----------|--------|\n")
		for _, r := range resources {
			sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s |\n",
				r.ID, r.Name, r.Type, r.Provider, r.Region))
		}
	}

	return textResult(sb.String()), nil
}

func (s *Server) toolListScopes(ctx context.Context) (*ToolResult, error) {
	scopes, err := s.store.ListScopes(ctx, "")
	if err != nil {
		return nil, err
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Discovery Scopes (%d)\n\n", len(scopes)))
	sb.WriteString("| Name | Plugin | Status | Last Run |\n")
	sb.WriteString("|------|--------|--------|----------|\n")
	for _, sc := range scopes {
		lastRun := "never"
		if sc.LastRun != nil {
			lastRun = sc.LastRun.Format(time.RFC3339)
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", sc.Name, sc.PluginName, sc.Status, lastRun))
	}
	return textResult(sb.String()), nil
}

func (s *Server) toolGetSummary(ctx context.Context) (*ToolResult, error) {
	resources, _, err := s.store.GetResources(ctx, storage.ResourceFilter{Limit: 10000})
	if err != nil {
		return nil, err
	}

	byProvider := make(map[string]int)
	byType := make(map[string]int)
	for _, r := range resources {
		byProvider[r.Provider]++
		byType[r.Type]++
	}

	var sb strings.Builder
	sb.WriteString("## Infrastructure Executive Summary\n\n")
	sb.WriteString(fmt.Sprintf("_Generated: %s_\n\n", time.Now().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("**Total Resources:** %d\n\n", len(resources)))

	sb.WriteString("### By Provider\n\n| Provider | Count |\n|----------|-------|\n")
	providers := sortedKeys(byProvider)
	for _, p := range providers {
		sb.WriteString(fmt.Sprintf("| %s | %d |\n", p, byProvider[p]))
	}

	sb.WriteString("\n### Top Resource Types\n\n| Type | Count |\n|------|-------|\n")
	types_ := sortedKeysByValue(byType, 20)
	for _, t := range types_ {
		sb.WriteString(fmt.Sprintf("| %s | %d |\n", t, byType[t]))
	}

	return textResult(sb.String()), nil
}

func (s *Server) toolGetTopology(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	scope := stringArg(args, "scope")

	filter := storage.ResourceFilter{Limit: 500}
	if scope != "" && scope != "all" {
		filter.Query = scope
	}
	resources, _, err := s.store.GetResources(ctx, filter)
	if err != nil {
		return nil, err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Topology: %s\n\n", scope))
	sb.WriteString("```mermaid\ngraph TD\n")
	seen := make(map[string]bool)
	for _, r := range resources {
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		nodeID := sanitizeID(r.ID)
		label := r.Name
		if label == "" {
			label = r.ID
		}
		sb.WriteString(fmt.Sprintf("    %s[\"%s\\n%s\"]\n", nodeID, label, r.Type))
	}

	rels, err := s.store.GetRelationships(ctx, storage.RelationshipFilter{Limit: 1000})
	if err == nil {
		for _, rel := range rels {
			if seen[rel.SourceID] && seen[rel.TargetID] {
				sb.WriteString(fmt.Sprintf("    %s -->|%s| %s\n",
					sanitizeID(rel.SourceID), rel.Type, sanitizeID(rel.TargetID)))
			}
		}
	}
	sb.WriteString("```\n")

	return textResult(sb.String()), nil
}

func (s *Server) toolGetRelationships(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	id := stringArg(args, "resource_id")
	if id == "" {
		return nil, fmt.Errorf("resource_id is required")
	}

	outgoing, err := s.store.GetRelationships(ctx, storage.RelationshipFilter{SourceID: id, Limit: 200})
	if err != nil {
		return nil, err
	}
	incoming, err := s.store.GetRelationships(ctx, storage.RelationshipFilter{TargetID: id, Limit: 200})
	if err != nil {
		return nil, err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Relationships for `%s`\n\n", id))
	sb.WriteString(fmt.Sprintf("**Outgoing:** %d  **Incoming:** %d\n\n", len(outgoing), len(incoming)))

	if len(outgoing) > 0 {
		sb.WriteString("### Outgoing\n\n| Type | Target |\n|------|--------|\n")
		for _, r := range outgoing {
			sb.WriteString(fmt.Sprintf("| %s | `%s` |\n", r.Type, r.TargetID))
		}
		sb.WriteString("\n")
	}
	if len(incoming) > 0 {
		sb.WriteString("### Incoming\n\n| Type | Source |\n|------|--------|\n")
		for _, r := range incoming {
			sb.WriteString(fmt.Sprintf("| %s | `%s` |\n", r.Type, r.SourceID))
		}
	}

	return textResult(sb.String()), nil
}

func (s *Server) toolSearchResources(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	query := stringArg(args, "query")
	provider := stringArg(args, "provider")
	resType := stringArg(args, "type")
	limit := intArg(args, "limit", 20)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Search Results for %q\n\n", query))

	if s.searcher != nil {
		results, err := s.searcher.Search(ctx, search.Params{
			Query:    query,
			Provider: provider,
			Type:     resType,
			Limit:    limit,
		})
		if err != nil {
			return nil, fmt.Errorf("search: %w", err)
		}
		sb.WriteString(fmt.Sprintf("**Found:** %d results\n\n", len(results)))
		if len(results) == 0 {
			sb.WriteString("_No resources matched._\n")
		} else {
			sb.WriteString("| Score | Name | Type | Provider | Region |\n")
			sb.WriteString("|-------|------|------|----------|--------|\n")
			for _, r := range results {
				sb.WriteString(fmt.Sprintf("| %.3f | %s | %s | %s | %s |\n",
					r.Score, r.Resource.Name, r.Resource.Type, r.Resource.Provider, r.Resource.Region))
			}
		}
	} else {
		// Fallback: keyword filter via storage.
		resources, total, err := s.store.GetResources(ctx, storage.ResourceFilter{
			Query: query, Provider: provider, Type: resType, Limit: limit,
		})
		if err != nil {
			return nil, err
		}
		sb.WriteString(fmt.Sprintf("**Found:** %d (showing %d)\n\n", total, len(resources)))
		if len(resources) == 0 {
			sb.WriteString("_No resources matched._\n")
		} else {
			sb.WriteString("| Name | Type | Provider | Region |\n|------|------|----------|--------|\n")
			for _, r := range resources {
				sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", r.Name, r.Type, r.Provider, r.Region))
			}
		}
	}
	return textResult(sb.String()), nil
}

func (s *Server) toolGetResource(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	id := stringArg(args, "resource_id")
	if id == "" {
		return nil, fmt.Errorf("resource_id is required")
	}
	r, err := s.store.GetResourceByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("resource %s not found", id)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Resource: %s\n\n", r.Name))
	sb.WriteString(fmt.Sprintf("**ID:** `%s`\n", r.ID))
	sb.WriteString(fmt.Sprintf("**Type:** %s\n", r.Type))
	sb.WriteString(fmt.Sprintf("**Provider:** %s\n", r.Provider))
	sb.WriteString(fmt.Sprintf("**Region:** %s\n", r.Region))
	sb.WriteString(fmt.Sprintf("**Account:** %s\n", r.AccountID))
	sb.WriteString(fmt.Sprintf("**Discovered:** %s\n", r.DiscoveredAt.Format(time.RFC3339)))
	if len(r.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("**Tags:** %s\n", strings.Join(r.Tags, ", ")))
	}
	if len(r.Properties) > 0 {
		sb.WriteString("\n### Properties\n\n| Key | Value |\n|-----|-------|\n")
		for k, v := range r.Properties {
			sb.WriteString(fmt.Sprintf("| %s | %s |\n", k, v))
		}
	}
	return textResult(sb.String()), nil
}

func (s *Server) toolQueryGraph(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	id := stringArg(args, "resource_id")
	if id == "" {
		return nil, fmt.Errorf("resource_id is required")
	}
	hops := intArg(args, "hops", 2)

	if s.graph == nil {
		return textResult("_Graph module not available. Start Zerelio with a PostgreSQL backend to enable graph traversal._"), nil
	}

	subgraph, err := s.graph.Neighborhood(ctx, id, hops)
	if err != nil {
		return nil, fmt.Errorf("graph query: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Graph Neighbourhood: `%s` (%d hops)\n\n", id, hops))
	sb.WriteString(fmt.Sprintf("**Nodes:** %d  **Edges:** %d\n\n", len(subgraph.Nodes), len(subgraph.Edges)))

	if len(subgraph.Nodes) > 0 {
		sb.WriteString("### Nodes\n\n| ID | Type | Provider | Region |\n|----|------|----------|--------|\n")
		for _, n := range subgraph.Nodes {
			typ, _ := n.Properties["type"].(string)
			prov, _ := n.Properties["provider"].(string)
			reg, _ := n.Properties["region"].(string)
			sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s |\n", n.ID, typ, prov, reg))
		}
		sb.WriteString("\n")
	}
	if len(subgraph.Edges) > 0 {
		sb.WriteString("### Edges\n\n| Source | Type | Target |\n|--------|------|--------|\n")
		for _, e := range subgraph.Edges {
			sb.WriteString(fmt.Sprintf("| `%s` | %s | `%s` |\n", e.SourceID, e.Type, e.TargetID))
		}
	}
	return textResult(sb.String()), nil
}

// ─── Resources ───────────────────────────────────────────────────────────────
func resourceDescriptions() []ResourceDescription {
	return []ResourceDescription{
		{URI: "zerelio://summary", Name: "Executive Summary", Description: "High-level summary of all discovered infrastructure.", MimeType: "text/markdown"},
		{URI: "zerelio://topology/{scope}", Name: "Topology", Description: "Infrastructure topology diagram for a given scope.", MimeType: "text/markdown"},
		{URI: "zerelio://resources/{id}", Name: "Resource Dossier", Description: "Full details for a specific resource.", MimeType: "text/markdown"},
	}
}

func (s *Server) readResource(ctx context.Context, uri string) (*ResourceReadResult, error) {
	switch {
	case uri == "zerelio://summary":
		result, err := s.toolGetSummary(ctx)
		if err != nil {
			return nil, err
		}
		return &ResourceReadResult{Contents: []ResourceContents{{URI: uri, MimeType: "text/markdown", Text: result.Content[0].Text}}}, nil

	case strings.HasPrefix(uri, "zerelio://topology/"):
		scope := strings.TrimPrefix(uri, "zerelio://topology/")
		result, err := s.toolGetTopology(ctx, map[string]interface{}{"scope": scope})
		if err != nil {
			return nil, err
		}
		return &ResourceReadResult{Contents: []ResourceContents{{URI: uri, MimeType: "text/markdown", Text: result.Content[0].Text}}}, nil

	case strings.HasPrefix(uri, "zerelio://resources/"):
		id := strings.TrimPrefix(uri, "zerelio://resources/")
		r, err := s.store.GetResourceByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("resource %s not found", id)
		}
		text := fmt.Sprintf("# Resource: %s\n\n**Type:** %s\n**Provider:** %s\n**Region:** %s\n**Account:** %s\n",
			r.Name, r.Type, r.Provider, r.Region, r.AccountID)
		return &ResourceReadResult{Contents: []ResourceContents{{URI: uri, MimeType: "text/markdown", Text: text}}}, nil
	}

	return nil, fmt.Errorf("unknown resource URI: %s", uri)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func textResult(text string) *ToolResult {
	return &ToolResult{Content: []ToolContent{{Type: "text", Text: text}}}
}

func stringArg(args map[string]interface{}, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func intArg(args map[string]interface{}, key string, def int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return def
}

func sanitizeID(id string) string {
	var sb strings.Builder
	for _, c := range id {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			sb.WriteRune(c)
		} else {
			sb.WriteRune('_')
		}
	}
	return sb.String()
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeysByValue(m map[string]int, limit int) []string {
	type kv struct{ k string; v int }
	var pairs []kv
	for k, v := range m {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].v > pairs[j].v })
	keys := make([]string, 0, len(pairs))
	for i, p := range pairs {
		if i >= limit {
			break
		}
		keys = append(keys, p.k)
	}
	return keys
}
