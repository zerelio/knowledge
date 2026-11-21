package compliance_test

import (
	"testing"
	"time"

	"github.com/zerelio/zerelio/internal/compliance"
	"github.com/zerelio/zerelio/pkg/types"
)

func res(id, typ, name string, props map[string]string) *types.Resource {
	r := &types.Resource{
		ID:           id,
		Type:         typ,
		Name:         name,
		TenantID:     "t1",
		DiscoveredAt: time.Now(),
		Properties:   props,
	}
	if r.Properties == nil {
		r.Properties = map[string]string{}
	}
	return r
}

func TestRequiredTags_Missing(t *testing.T) {
	e := compliance.NewEvaluator("env", "owner")
	r := res("r1", "aws:ec2:instance", "web-prod", map[string]string{"env": "prod"})
	findings := e.Evaluate(r)
	found := false
	for _, f := range findings {
		if f.RuleID == "required-tags" {
			found = true
		}
	}
	if !found {
		t.Error("expected required-tags finding for missing 'owner' tag")
	}
}

func TestRequiredTags_Present(t *testing.T) {
	e := compliance.NewEvaluator("env", "owner")
	r := res("r2", "aws:ec2:instance", "web-prod", map[string]string{"env": "prod", "owner": "platform"})
	for _, f := range e.Evaluate(r) {
		if f.RuleID == "required-tags" {
			t.Errorf("unexpected required-tags finding: %s", f.Message)
		}
	}
}

func TestPublicS3Bucket_Flagged(t *testing.T) {
	e := compliance.NewEvaluator()
	r := res("r3", "aws:s3:bucket", "data-bucket", map[string]string{"public_acl": "true"})
	findings := e.Evaluate(r)
	found := false
	for _, f := range findings {
		if f.RuleID == "aws-s3-public-bucket" {
			found = true
			if f.Severity != types.SeverityCritical {
				t.Errorf("expected critical, got %s", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected aws-s3-public-bucket finding")
	}
}

func TestPublicS3Bucket_Private(t *testing.T) {
	e := compliance.NewEvaluator()
	r := res("r4", "aws:s3:bucket", "private-bucket", map[string]string{"block_public_acls": "true"})
	for _, f := range e.Evaluate(r) {
		if f.RuleID == "aws-s3-public-bucket" {
			t.Errorf("unexpected public-bucket finding on private bucket")
		}
	}
}

func TestWideOpenSG_Flagged(t *testing.T) {
	e := compliance.NewEvaluator()
	r := res("r5", "aws:ec2:security_group", "allow-all", map[string]string{
		"inbound_rules": `[{"cidr":"0.0.0.0/0","port":22}]`,
	})
	findings := e.Evaluate(r)
	found := false
	for _, f := range findings {
		if f.RuleID == "aws-sg-wide-open" {
			found = true
		}
	}
	if !found {
		t.Error("expected aws-sg-wide-open finding")
	}
}

func TestStaleResource(t *testing.T) {
	e := compliance.NewEvaluator()
	r := res("r6", "aws:ec2:instance", "old-vm", nil)
	r.DiscoveredAt = time.Now().Add(-45 * 24 * time.Hour) // 45 days ago
	findings := e.Evaluate(r)
	found := false
	for _, f := range findings {
		if f.RuleID == "stale-resource" {
			found = true
		}
	}
	if !found {
		t.Error("expected stale-resource finding for 45-day-old resource")
	}
}

func TestRecentResource_NotStale(t *testing.T) {
	e := compliance.NewEvaluator()
	r := res("r7", "aws:ec2:instance", "new-vm", nil)
	r.DiscoveredAt = time.Now().Add(-2 * 24 * time.Hour) // 2 days ago
	for _, f := range e.Evaluate(r) {
		if f.RuleID == "stale-resource" {
			t.Error("unexpected stale-resource finding for 2-day-old resource")
		}
	}
}

func TestEvaluateAll(t *testing.T) {
	e := compliance.NewEvaluator("env")
	resources := []*types.Resource{
		res("r8", "aws:s3:bucket", "pub", map[string]string{"public_acl": "true"}),
		res("r9", "aws:ec2:instance", "vm", map[string]string{"env": "prod", "owner": "ops", "team": "platform", "description": "prod vm"}),
	}
	findings := e.EvaluateAll(resources)
	// r8 should have aws-s3-public-bucket + required-tags + missing-description
	// r9 should have no findings
	if len(findings) == 0 {
		t.Error("expected at least one finding")
	}
}
