# Zerelio RBAC — OPA Rego policy
#
# This file mirrors the DefaultPolicy() in enforcer.go.
# Deploy it to an OPA sidecar or bundle server to enforce policy externally
# (e.g., in a service-mesh or Kubernetes admission-controller context).
#
# Input schema:
#   input.roles    — array of role strings for the requesting user
#   input.action   — one of: "read" | "write" | "delete" | "run" | "admin"
#   input.resource — one of: "resources" | "scopes" | "plugins" | "graph" |
#                              "search" | "audit" | "users" | "credentials" | "*"

package zerelio.rbac

import rego.v1

default allow := false

# Admin role: unrestricted.
allow if {
    "admin" in input.roles
}

# Viewer role.
viewer_resources := {"resources", "graph", "search", "scopes", "plugins"}
viewer_actions   := {"read"}

allow if {
    "viewer" in input.roles
    input.resource in viewer_resources
    input.action in viewer_actions
}

# Editor role.
editor_resource_actions := {
    "resources":    {"read", "write"},
    "graph":        {"read"},
    "search":       {"read"},
    "scopes":       {"read", "write", "run"},
    "plugins":      {"read"},
    "credentials":  {"read", "write"},
}

allow if {
    "editor" in input.roles
    action_set := editor_resource_actions[input.resource]
    input.action in action_set
}
