// Package threat provides threat intelligence enrichment hooks for discovered
// infrastructure resources.
//
// Architecture:
//
//	Enricher (interface)
//	├── NoopEnricher   — default; returns no threats (zero overhead)
//	├── CompositeEnricher — fans out to multiple enrichers, merges results
//	└── (future) IntegrationEnricher — calls external TI APIs
//
// The Profile produced by an enricher is advisory — callers decide what to do
// with threat findings (alert, quarantine, annotate).
package threat

import (
	"context"
	"time"

	"github.com/zerelio/zerelio/pkg/types"
)

// ThreatLevel is the severity of a threat finding.
type ThreatLevel string

const (
	ThreatNone     ThreatLevel = "none"
	ThreatLow      ThreatLevel = "low"
	ThreatMedium   ThreatLevel = "medium"
	ThreatHigh     ThreatLevel = "high"
	ThreatCritical ThreatLevel = "critical"
)

// CVE records a Common Vulnerabilities and Exposures entry.
type CVE struct {
	ID          string      `json:"id"`           // e.g. "CVE-2023-12345"
	Score       float64     `json:"score"`        // CVSS v3 score (0–10)
	Description string      `json:"description,omitempty"`
	Level       ThreatLevel `json:"level"`
}

// Indicator is an Indicator of Compromise (IoC) or misconfiguration.
type Indicator struct {
	Type        string `json:"type"`        // "misconfiguration" | "ioc" | "exposure"
	Description string `json:"description"`
	Remediation string `json:"remediation,omitempty"`
}

// Profile is the threat assessment for a single resource.
type Profile struct {
	ResourceID  string      `json:"resource_id"`
	ThreatLevel ThreatLevel `json:"threat_level"`
	CVEs        []CVE       `json:"cves,omitempty"`
	Indicators  []Indicator `json:"indicators,omitempty"`
	AssessedAt  time.Time   `json:"assessed_at"`
}

// Enricher enriches a resource with threat intelligence.
type Enricher interface {
	Enrich(ctx context.Context, r *types.Resource) (*Profile, error)
}

// ---- NoopEnricher ----

// NoopEnricher returns a clean profile with no threats. It is the default
// when no threat intelligence integration is configured.
type NoopEnricher struct{}

func (n *NoopEnricher) Enrich(_ context.Context, r *types.Resource) (*Profile, error) {
	return &Profile{
		ResourceID:  r.ID,
		ThreatLevel: ThreatNone,
		AssessedAt:  time.Now(),
	}, nil
}

// ---- CompositeEnricher ----

// CompositeEnricher fans a resource out to multiple enrichers and merges the
// results, taking the highest threat level seen.
type CompositeEnricher struct {
	enrichers []Enricher
}

// NewComposite creates an enricher that runs all provided enrichers in sequence.
func NewComposite(enrichers ...Enricher) *CompositeEnricher {
	return &CompositeEnricher{enrichers: enrichers}
}

func (c *CompositeEnricher) Enrich(ctx context.Context, r *types.Resource) (*Profile, error) {
	merged := &Profile{
		ResourceID:  r.ID,
		ThreatLevel: ThreatNone,
		AssessedAt:  time.Now(),
	}

	for _, e := range c.enrichers {
		p, err := e.Enrich(ctx, r)
		if err != nil {
			// Best-effort — a failing enricher does not block others.
			continue
		}
		if severityRank(p.ThreatLevel) > severityRank(merged.ThreatLevel) {
			merged.ThreatLevel = p.ThreatLevel
		}
		merged.CVEs = append(merged.CVEs, p.CVEs...)
		merged.Indicators = append(merged.Indicators, p.Indicators...)
	}
	return merged, nil
}

// ---- MisconfigEnricher ----

// MisconfigEnricher checks for common cloud resource misconfigurations.
// It operates purely on the resource metadata — no network calls.
type MisconfigEnricher struct{}

// Enrich checks for well-known misconfigurations in resource properties.
func (m *MisconfigEnricher) Enrich(_ context.Context, r *types.Resource) (*Profile, error) {
	profile := &Profile{
		ResourceID:  r.ID,
		ThreatLevel: ThreatNone,
		AssessedAt:  time.Now(),
	}

	var indicators []Indicator

	// S3: public bucket
	if r.Type == "aws:s3:bucket" {
		if r.Properties["acl"] == "public-read" || r.Properties["acl"] == "public-read-write" {
			indicators = append(indicators, Indicator{
				Type:        "misconfiguration",
				Description: "S3 bucket has public ACL",
				Remediation: "Set bucket ACL to private; use bucket policy for controlled access",
			})
		}
	}

	// Security groups / firewall rules: open to 0.0.0.0/0
	if r.Type == "aws:ec2:security-group" || r.Type == "gcp:compute:firewall" {
		if r.Properties["cidr"] == "0.0.0.0/0" || r.Properties["source_ranges"] == "0.0.0.0/0" {
			indicators = append(indicators, Indicator{
				Type:        "exposure",
				Description: "Resource exposes port(s) to 0.0.0.0/0 (internet)",
				Remediation: "Restrict CIDR to known IP ranges or use VPN/bastion",
			})
		}
	}

	// RDS: publicly accessible
	if r.Type == "aws:rds:instance" && r.Properties["publicly_accessible"] == "true" {
		indicators = append(indicators, Indicator{
			Type:        "exposure",
			Description: "RDS instance is publicly accessible",
			Remediation: "Disable public accessibility; place RDS in a private subnet",
		})
	}

	if len(indicators) > 0 {
		profile.Indicators = indicators
		profile.ThreatLevel = ThreatMedium
	}
	return profile, nil
}

// ---- helpers ----

func severityRank(l ThreatLevel) int {
	switch l {
	case ThreatCritical:
		return 4
	case ThreatHigh:
		return 3
	case ThreatMedium:
		return 2
	case ThreatLow:
		return 1
	default:
		return 0
	}
}
