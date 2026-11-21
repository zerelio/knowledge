// Package rbac implements Zerelio's role-based access control engine.
//
// The policy is modelled as:
//
//	Subject (user with roles) → Action → Resource type
//
// Three built-in roles mirror the core types: viewer, editor, admin.
// A DefaultPolicy() is always present; callers can extend it or swap it.
//
// Companion file policy.rego contains the OPA Rego equivalent of the
// default policy, usable when deploying Zerelio behind an OPA sidecar.
package rbac

import "slices"

// Action is an operation a user performs.
type Action string

const (
	ActionRead   Action = "read"
	ActionWrite  Action = "write"
	ActionDelete Action = "delete"
	ActionRun    Action = "run"   // trigger discovery runs
	ActionAdmin  Action = "admin" // user management, audit
)

// ResourceType is the class of object being acted upon.
type ResourceType string

const (
	ResourceResources   ResourceType = "resources"
	ResourceScopes      ResourceType = "scopes"
	ResourcePlugins     ResourceType = "plugins"
	ResourceGraph       ResourceType = "graph"
	ResourceSearch      ResourceType = "search"
	ResourceAudit       ResourceType = "audit"
	ResourceUsers       ResourceType = "users"
	ResourceCredentials ResourceType = "credentials"
	ResourceAny         ResourceType = "*" // wildcard
)

// Rule maps a resource type to the actions permitted for it.
type Rule struct {
	Resource ResourceType
	Actions  []Action
}

// Policy maps role names to their allowed rules.
type Policy struct {
	roles map[string][]Rule
}

// NewPolicy creates an empty policy.
func NewPolicy() *Policy {
	return &Policy{roles: make(map[string][]Rule)}
}

// AddRole registers a role with its access rules.
func (p *Policy) AddRole(role string, rules ...Rule) *Policy {
	p.roles[role] = append(p.roles[role], rules...)
	return p
}

// DefaultPolicy returns the standard three-tier policy (viewer / editor / admin).
// It mirrors the permissions in pkg/types/user.go.
func DefaultPolicy() *Policy {
	return NewPolicy().
		AddRole("viewer",
			Rule{ResourceResources, []Action{ActionRead}},
			Rule{ResourceGraph, []Action{ActionRead}},
			Rule{ResourceSearch, []Action{ActionRead}},
			Rule{ResourceScopes, []Action{ActionRead}},
			Rule{ResourcePlugins, []Action{ActionRead}},
		).
		AddRole("editor",
			Rule{ResourceResources, []Action{ActionRead, ActionWrite}},
			Rule{ResourceGraph, []Action{ActionRead}},
			Rule{ResourceSearch, []Action{ActionRead}},
			Rule{ResourceScopes, []Action{ActionRead, ActionWrite, ActionRun}},
			Rule{ResourcePlugins, []Action{ActionRead}},
			Rule{ResourceCredentials, []Action{ActionRead, ActionWrite}},
		).
		AddRole("admin",
			Rule{ResourceAny, []Action{ActionRead, ActionWrite, ActionDelete, ActionRun, ActionAdmin}},
		)
}

// Enforcer evaluates access decisions against a Policy.
type Enforcer struct {
	policy *Policy
}

// New creates an Enforcer backed by the given policy.
// Use DefaultPolicy() for standard Zerelio entitlements.
func New(policy *Policy) *Enforcer {
	return &Enforcer{policy: policy}
}

// Allow returns true if any of the subject's roles permit action on resource.
func (e *Enforcer) Allow(roles []string, action Action, resource ResourceType) bool {
	for _, role := range roles {
		rules, ok := e.policy.roles[role]
		if !ok {
			continue
		}
		for _, rule := range rules {
			if rule.Resource != resource && rule.Resource != ResourceAny {
				continue
			}
			if slices.Contains(rule.Actions, action) {
				return true
			}
		}
	}
	return false
}

// MustAllow is Allow but panics on denial — useful in tests.
func (e *Enforcer) MustAllow(roles []string, action Action, resource ResourceType) {
	if !e.Allow(roles, action, resource) {
		panic("rbac: access denied: " + string(action) + " on " + string(resource) +
			" for roles " + join(roles))
	}
}

func join(ss []string) string {
	out := "["
	for i, s := range ss {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out + "]"
}
