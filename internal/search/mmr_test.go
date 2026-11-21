package search

import (
	"testing"

	"github.com/zerelio/zerelio/pkg/types"
)

func makeResult(id, resType string, score float64) Result {
	return Result{
		Resource: &types.Resource{ID: id, Type: resType, Name: id},
		Score:    score,
	}
}

func TestMMR_Empty(t *testing.T) {
	got := mmrRerank(nil, 5, 0.6)
	if len(got) != 0 {
		t.Errorf("expected 0 results, got %d", len(got))
	}
}

func TestMMR_LimitNotExceeded(t *testing.T) {
	candidates := []Result{
		makeResult("r1", "aws:ec2:instance", 1.0),
		makeResult("r2", "aws:ec2:instance", 0.9),
	}
	got := mmrRerank(candidates, 5, 0.6)
	if len(got) != 2 {
		t.Errorf("expected 2 results (all candidates), got %d", len(got))
	}
}

func TestMMR_PureLambda1_SameOrderAsInput(t *testing.T) {
	// λ=1 disables MMR — output order must match input (highest score first).
	candidates := []Result{
		makeResult("r1", "aws:ec2:instance", 1.0),
		makeResult("r2", "aws:s3:bucket", 0.9),
		makeResult("r3", "aws:rds:instance", 0.8),
	}
	got := mmrRerank(candidates, 2, 1.0)
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if got[0].Resource.ID != "r1" || got[1].Resource.ID != "r2" {
		t.Errorf("expected r1,r2 order, got %s,%s", got[0].Resource.ID, got[1].Resource.ID)
	}
}

func TestMMR_DiversityOverRedundancy(t *testing.T) {
	// 3 EC2 instances (high relevance but redundant) + 1 S3 bucket (lower relevance).
	// With λ=0.4 (heavy diversity), the S3 bucket should be selected before the
	// 3rd EC2 instance.
	candidates := []Result{
		makeResult("ec2a", "aws:ec2:instance", 1.0),
		makeResult("ec2b", "aws:ec2:instance", 0.95),
		makeResult("s3", "aws:s3:bucket", 0.85),
		makeResult("ec2c", "aws:ec2:instance", 0.80),
	}
	got := mmrRerank(candidates, 3, 0.4)
	if len(got) != 3 {
		t.Fatalf("expected 3 results, got %d", len(got))
	}
	// First must always be highest scorer.
	if got[0].Resource.ID != "ec2a" {
		t.Errorf("first result should be ec2a (highest score), got %s", got[0].Resource.ID)
	}
	// S3 bucket should appear before the 3rd EC2 instance.
	s3Pos, ec2cPos := -1, -1
	for i, r := range got {
		switch r.Resource.ID {
		case "s3":
			s3Pos = i
		case "ec2c":
			ec2cPos = i
		}
	}
	if s3Pos < 0 {
		t.Error("s3 not selected at all")
	}
	if ec2cPos >= 0 && s3Pos > ec2cPos {
		t.Errorf("expected s3 (diverse) before ec2c (redundant), got s3@%d ec2c@%d", s3Pos, ec2cPos)
	}
}

func TestResourceTypeSim(t *testing.T) {
	cases := []struct {
		a, b    *types.Resource
		wantMin float64
		wantMax float64
	}{
		{
			&types.Resource{Type: "aws:ec2:instance"},
			&types.Resource{Type: "aws:ec2:instance"},
			1.0, 1.0,
		},
		{
			&types.Resource{Type: "aws:ec2:instance"},
			&types.Resource{Type: "aws:ec2:security_group"},
			0.6, 0.8,
		},
		{
			&types.Resource{Type: "aws:ec2:instance"},
			&types.Resource{Type: "k8s:pod"},
			0.0, 0.0,
		},
	}
	for _, tc := range cases {
		got := resourceTypeSim(tc.a, tc.b)
		if got < tc.wantMin || got > tc.wantMax {
			t.Errorf("resourceTypeSim(%s, %s) = %v, want [%v, %v]",
				tc.a.Type, tc.b.Type, got, tc.wantMin, tc.wantMax)
		}
	}
}
