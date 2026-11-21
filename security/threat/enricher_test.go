package threat_test

import (
	"context"
	"testing"
	"time"

	"github.com/zerelio/zerelio/pkg/types"
	"github.com/zerelio/zerelio/security/threat"
)

func newResource(id, resType string, props map[string]string) *types.Resource {
	return &types.Resource{
		ID:           id,
		Name:         id,
		Type:         resType,
		Provider:     "aws",
		Region:       "us-east-1",
		Properties:   props,
		DiscoveredAt: time.Now(),
		ValidFrom:    time.Now(),
	}
}

func TestNoop(t *testing.T) {
	e := &threat.NoopEnricher{}
	p, err := e.Enrich(context.Background(), newResource("r1", "aws:s3:bucket", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ThreatLevel != threat.ThreatNone {
		t.Errorf("noop threat level = %s, want none", p.ThreatLevel)
	}
}

func TestMisconfig_PublicS3(t *testing.T) {
	e := &threat.MisconfigEnricher{}
	r := newResource("r1", "aws:s3:bucket", map[string]string{"acl": "public-read"})
	p, err := e.Enrich(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if p.ThreatLevel != threat.ThreatMedium {
		t.Errorf("public bucket threat = %s, want medium", p.ThreatLevel)
	}
	if len(p.Indicators) == 0 {
		t.Error("expected indicators for public bucket")
	}
}

func TestMisconfig_PrivateS3(t *testing.T) {
	e := &threat.MisconfigEnricher{}
	r := newResource("r1", "aws:s3:bucket", map[string]string{"acl": "private"})
	p, _ := e.Enrich(context.Background(), r)
	if p.ThreatLevel != threat.ThreatNone {
		t.Errorf("private bucket should have no threat, got %s", p.ThreatLevel)
	}
}

func TestMisconfig_PublicRDS(t *testing.T) {
	e := &threat.MisconfigEnricher{}
	r := newResource("r1", "aws:rds:instance", map[string]string{"publicly_accessible": "true"})
	p, _ := e.Enrich(context.Background(), r)
	if p.ThreatLevel != threat.ThreatMedium {
		t.Errorf("public RDS threat = %s, want medium", p.ThreatLevel)
	}
}

func TestComposite_TakesHighestLevel(t *testing.T) {
	noop := &threat.NoopEnricher{}
	misconfig := &threat.MisconfigEnricher{}
	composite := threat.NewComposite(noop, misconfig)

	r := newResource("r1", "aws:s3:bucket", map[string]string{"acl": "public-read"})
	p, err := composite.Enrich(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if p.ThreatLevel != threat.ThreatMedium {
		t.Errorf("composite should escalate to medium, got %s", p.ThreatLevel)
	}
}
