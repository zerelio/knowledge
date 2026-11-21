package rbac_test

import (
	"testing"

	"github.com/zerelio/zerelio/security/rbac"
)

func TestDefaultPolicy_Viewer(t *testing.T) {
	e := rbac.New(rbac.DefaultPolicy())
	roles := []string{"viewer"}

	// Viewer can read resources, graph, search, scopes, plugins.
	for _, res := range []rbac.ResourceType{
		rbac.ResourceResources, rbac.ResourceGraph,
		rbac.ResourceSearch, rbac.ResourceScopes, rbac.ResourcePlugins,
	} {
		if !e.Allow(roles, rbac.ActionRead, res) {
			t.Errorf("viewer should be able to read %s", res)
		}
	}

	// Viewer cannot write, delete, run, or admin anything.
	for _, action := range []rbac.Action{rbac.ActionWrite, rbac.ActionDelete, rbac.ActionRun, rbac.ActionAdmin} {
		if e.Allow(roles, action, rbac.ResourceResources) {
			t.Errorf("viewer should not be able to %s resources", action)
		}
	}

	// Viewer cannot access users or credentials.
	for _, res := range []rbac.ResourceType{rbac.ResourceUsers, rbac.ResourceCredentials, rbac.ResourceAudit} {
		if e.Allow(roles, rbac.ActionRead, res) {
			t.Errorf("viewer should not access %s", res)
		}
	}
}

func TestDefaultPolicy_Editor(t *testing.T) {
	e := rbac.New(rbac.DefaultPolicy())
	roles := []string{"editor"}

	if !e.Allow(roles, rbac.ActionWrite, rbac.ResourceResources) {
		t.Error("editor should write resources")
	}
	if !e.Allow(roles, rbac.ActionRun, rbac.ResourceScopes) {
		t.Error("editor should run scopes")
	}
	if !e.Allow(roles, rbac.ActionWrite, rbac.ResourceCredentials) {
		t.Error("editor should write credentials")
	}
	if e.Allow(roles, rbac.ActionAdmin, rbac.ResourceUsers) {
		t.Error("editor should not admin users")
	}
	if e.Allow(roles, rbac.ActionDelete, rbac.ResourceResources) {
		t.Error("editor should not delete resources")
	}
}

func TestDefaultPolicy_Admin(t *testing.T) {
	e := rbac.New(rbac.DefaultPolicy())
	roles := []string{"admin"}

	// Admin can do everything on every resource type.
	for _, action := range []rbac.Action{
		rbac.ActionRead, rbac.ActionWrite, rbac.ActionDelete, rbac.ActionRun, rbac.ActionAdmin,
	} {
		for _, res := range []rbac.ResourceType{
			rbac.ResourceResources, rbac.ResourceScopes, rbac.ResourceUsers,
			rbac.ResourceAudit, rbac.ResourceCredentials, rbac.ResourceGraph,
		} {
			if !e.Allow(roles, action, res) {
				t.Errorf("admin should be able to %s %s", action, res)
			}
		}
	}
}

func TestAllow_MultipleRoles(t *testing.T) {
	e := rbac.New(rbac.DefaultPolicy())
	// User with both viewer and editor roles: union of permissions.
	roles := []string{"viewer", "editor"}

	if !e.Allow(roles, rbac.ActionWrite, rbac.ResourceResources) {
		t.Error("viewer+editor should be able to write resources (via editor)")
	}
}

func TestAllow_UnknownRole(t *testing.T) {
	e := rbac.New(rbac.DefaultPolicy())
	if e.Allow([]string{"unknown"}, rbac.ActionRead, rbac.ResourceResources) {
		t.Error("unknown role should be denied")
	}
}

func TestCustomPolicy(t *testing.T) {
	p := rbac.NewPolicy().AddRole("auditor",
		rbac.Rule{Resource: rbac.ResourceAudit, Actions: []rbac.Action{rbac.ActionRead}},
	)
	e := rbac.New(p)

	if !e.Allow([]string{"auditor"}, rbac.ActionRead, rbac.ResourceAudit) {
		t.Error("auditor should read audit events")
	}
	if e.Allow([]string{"auditor"}, rbac.ActionRead, rbac.ResourceResources) {
		t.Error("auditor should not read resources")
	}
}
