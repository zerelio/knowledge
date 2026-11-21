// Package compliance implements infrastructure compliance rule evaluation.
//
// Architecture:
//
//   Rule interface — each rule inspects one Resource and returns zero or
//   more Findings. Rules are pure functions; they never call external services.
//
//   Evaluator.Evaluate — runs all registered rules against a resource and
//   returns the aggregated findings.
//
//   Evaluator.EvaluateAll — convenience wrapper that evaluates an entire
//   slice of resources and stores findings via the provided storage backend.
//
// Built-in rules:
//   - RequiredTags: resources missing mandatory tags (configurable, default:
//     env, owner, team).
//   - PublicS3Bucket: AWS S3 buckets with public_acl=true or
//     block_public_acls=false in their properties.
//   - WideOpenSecurityGroup: AWS EC2 security groups that allow 0.0.0.0/0
//     (or ::/0) on SSH (22) or RDP (3389).
//   - StaleResource: resources not seen in >30 days.
//   - MissingDescription: any resource with an empty 'description' property
//     (severity Info — teams often forget to annotate things).
package compliance

import (
	"fmt"
	"strings"
	"time"

	"github.com/zerelio/zerelio/pkg/types"
)

// Rule is the single interface every compliance check must implement.
type Rule interface {
	// ID is a stable, lowercase-dashed unique identifier, e.g. "aws-s3-public-bucket".
	ID() string
	// Name is a human-readable display name.
	Name() string
	// Check evaluates one resource and returns any findings.
	// A nil or empty slice means the resource is compliant with this rule.
	Check(r *types.Resource) []*types.ComplianceFinding
}

// Evaluator runs a set of rules against resources.
type Evaluator struct {
	rules []Rule
}

// NewEvaluator creates an Evaluator pre-loaded with all built-in rules.
func NewEvaluator(requiredTags ...string) *Evaluator {
	if len(requiredTags) == 0 {
		requiredTags = []string{"env", "owner", "team"}
	}
	return &Evaluator{rules: []Rule{
		&RequiredTagsRule{Tags: requiredTags},
		&PublicS3BucketRule{},
		&WideOpenSecurityGroupRule{},
		&StaleResourceRule{Threshold: 30 * 24 * time.Hour},
		&MissingDescriptionRule{},
	}}
}

// Register adds a custom rule to the evaluator.
func (e *Evaluator) Register(r Rule) { e.rules = append(e.rules, r) }

// Evaluate runs all rules against a single resource.
func (e *Evaluator) Evaluate(r *types.Resource) []*types.ComplianceFinding {
	var findings []*types.ComplianceFinding
	for _, rule := range e.rules {
		for _, f := range rule.Check(r) {
			f.ResourceID = r.ID
			f.TenantID = r.TenantID
			f.EvaluatedAt = time.Now()
			if f.Status == "" {
				f.Status = types.FindingStatusOpen
			}
			findings = append(findings, f)
		}
	}
	return findings
}

// EvaluateAll runs all rules against every resource in the slice, returning
// the full set of findings.
func (e *Evaluator) EvaluateAll(resources []*types.Resource) []*types.ComplianceFinding {
	var all []*types.ComplianceFinding
	for _, r := range resources {
		all = append(all, e.Evaluate(r)...)
	}
	return all
}

// ─── Built-in rules ──────────────────────────────────────────────────────────

// RequiredTagsRule flags resources that are missing mandatory tags.
type RequiredTagsRule struct {
	Tags []string // tag keys that must be present in resource.Properties
}

func (r *RequiredTagsRule) ID() string   { return "required-tags" }
func (r *RequiredTagsRule) Name() string { return "Required Tags" }

func (r *RequiredTagsRule) Check(res *types.Resource) []*types.ComplianceFinding {
	var missing []string
	for _, tag := range r.Tags {
		if v, ok := res.Properties[tag]; !ok || v == "" {
			missing = append(missing, tag)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return []*types.ComplianceFinding{{
		RuleID:      r.ID(),
		RuleName:    r.Name(),
		Severity:    types.SeverityLow,
		Message:     fmt.Sprintf("resource is missing required tags: %s", strings.Join(missing, ", ")),
		Remediation: fmt.Sprintf("add the following tags to %s %s: %s", res.Type, res.Name, strings.Join(missing, ", ")),
	}}
}

// PublicS3BucketRule flags AWS S3 buckets exposed to the internet.
type PublicS3BucketRule struct{}

func (r *PublicS3BucketRule) ID() string   { return "aws-s3-public-bucket" }
func (r *PublicS3BucketRule) Name() string { return "Public S3 Bucket" }

func (r *PublicS3BucketRule) Check(res *types.Resource) []*types.ComplianceFinding {
	if res.Type != "aws:s3:bucket" && res.Type != "s3_bucket" {
		return nil
	}
	if isTrueStr(res.Properties, "public_acl", "public_access", "acl_public") ||
		isFalseStr(res.Properties, "block_public_acls", "block_public_access") {
		return []*types.ComplianceFinding{{
			RuleID:      r.ID(),
			RuleName:    r.Name(),
			Severity:    types.SeverityCritical,
			Message:     fmt.Sprintf("S3 bucket %q is publicly accessible", res.Name),
			Remediation: "Enable S3 Block Public Access settings at the bucket and account level. Set ACL to 'private'.",
		}}
	}
	return nil
}

// WideOpenSecurityGroupRule flags security groups that expose SSH/RDP to 0.0.0.0/0.
type WideOpenSecurityGroupRule struct{}

func (r *WideOpenSecurityGroupRule) ID() string   { return "aws-sg-wide-open" }
func (r *WideOpenSecurityGroupRule) Name() string { return "Wide-Open Security Group" }

func (r *WideOpenSecurityGroupRule) Check(res *types.Resource) []*types.ComplianceFinding {
	if !strings.Contains(res.Type, "security_group") && !strings.Contains(res.Type, "securitygroup") {
		return nil
	}
	for _, key := range []string{"inbound_rules", "ingress_rules", "ingress", "rules"} {
		if str, ok := res.Properties[key]; ok {
			if (strings.Contains(str, "0.0.0.0/0") || strings.Contains(str, "::/0")) &&
				(strings.Contains(str, ":22") || strings.Contains(str, `"22"`) || strings.Contains(str, `"3389"`) || strings.Contains(str, ":3389")) {
				return []*types.ComplianceFinding{{
					RuleID:      r.ID(),
					RuleName:    r.Name(),
					Severity:    types.SeverityHigh,
					Message:     fmt.Sprintf("security group %q allows SSH or RDP from 0.0.0.0/0", res.Name),
					Remediation: "Restrict inbound rules for ports 22 and 3389 to known IP ranges. Use a bastion host or VPN instead.",
				}}
			}
		}
	}
	return nil
}

// StaleResourceRule flags resources that haven't been seen recently.
type StaleResourceRule struct {
	Threshold time.Duration // default 30 days
}

func (r *StaleResourceRule) ID() string   { return "stale-resource" }
func (r *StaleResourceRule) Name() string { return "Stale Resource" }

func (r *StaleResourceRule) Check(res *types.Resource) []*types.ComplianceFinding {
	threshold := r.Threshold
	if threshold == 0 {
		threshold = 30 * 24 * time.Hour
	}
	if res.DiscoveredAt.IsZero() {
		return nil
	}
	age := time.Since(res.DiscoveredAt)
	if age < threshold {
		return nil
	}
	days := int(age.Hours() / 24)
	return []*types.ComplianceFinding{{
		RuleID:      r.ID(),
		RuleName:    r.Name(),
		Severity:    types.SeverityMedium,
		Message:     fmt.Sprintf("resource %q has not been seen in %d days (last discovered: %s)", res.Name, days, res.DiscoveredAt.Format("2006-01-02")),
		Remediation: "Verify the resource still exists. If decommissioned, remove it from the discovery scope.",
	}}
}

// MissingDescriptionRule flags resources without a description.
type MissingDescriptionRule struct{}

func (r *MissingDescriptionRule) ID() string   { return "missing-description" }
func (r *MissingDescriptionRule) Name() string { return "Missing Description" }

func (r *MissingDescriptionRule) Check(res *types.Resource) []*types.ComplianceFinding {
	if v, ok := res.Properties["description"]; ok && v != "" {
		return nil
	}
	return []*types.ComplianceFinding{{
		RuleID:      r.ID(),
		RuleName:    r.Name(),
		Severity:    types.SeverityInfo,
		Message:     fmt.Sprintf("resource %q has no description", res.Name),
		Remediation: "Add a 'description' tag or annotation describing the purpose and owner of this resource.",
	}}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func isTrueStr(props map[string]string, keys ...string) bool {
	for _, k := range keys {
		if v, ok := props[k]; ok {
			s := strings.ToLower(v)
			if s == "true" || s == "1" || s == "yes" || s == "public" {
				return true
			}
		}
	}
	return false
}

func isFalseStr(props map[string]string, keys ...string) bool {
	for _, k := range keys {
		if v, ok := props[k]; ok {
			s := strings.ToLower(v)
			if s == "false" || s == "0" || s == "no" {
				return true
			}
		}
	}
	return false
}
