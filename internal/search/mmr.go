package search

import (
	"strings"

	"github.com/zerelio/zerelio/pkg/types"
)

// mmrRerank applies Maximal Marginal Relevance to a result list.
//
// MMR iteratively selects items that maximize:
//
//	MMR(d) = λ · score(d, query) − (1−λ) · max{ sim(d, dᵢ) : dᵢ ∈ Selected }
//
// where score(d, query) is the RRF relevance score and sim(d, dᵢ) is a
// type-based structural similarity between two resources.
//
// λ=0.6 (default): moderate diversity — avoids returning 20 EC2 instances
// for "compute resources" while keeping the most relevant items first.
// λ=1.0 disables MMR (pure relevance order, identical to RRF output).
//
// Runs in O(candidates × selected) which is negligible at typical limits.
func mmrRerank(candidates []Result, limit int, lambda float64) []Result {
	if len(candidates) <= limit || lambda >= 1.0 {
		if len(candidates) > limit {
			return candidates[:limit]
		}
		return candidates
	}

	// Normalize RRF scores to [0,1] relative to the top score.
	topScore := candidates[0].Score
	if topScore == 0 {
		topScore = 1
	}

	selected := make([]Result, 0, limit)
	remaining := make([]Result, len(candidates))
	copy(remaining, candidates)

	// Always select the top-ranked item first.
	selected = append(selected, remaining[0])
	remaining = remaining[1:]

	for len(selected) < limit && len(remaining) > 0 {
		bestIdx := -1
		bestMMR := -1e18

		for i, candidate := range remaining {
			// Relevance: normalized RRF score.
			relevance := candidate.Score / topScore

			// Max structural similarity to any already-selected resource.
			maxSim := 0.0
			for _, sel := range selected {
				s := resourceTypeSim(candidate.Resource, sel.Resource)
				if s > maxSim {
					maxSim = s
				}
			}

			mmrScore := lambda*relevance - (1-lambda)*maxSim
			if bestIdx == -1 || mmrScore > bestMMR {
				bestIdx = i
				bestMMR = mmrScore
			}
		}

		selected = append(selected, remaining[bestIdx])
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}

	return selected
}

// resourceTypeSim returns a [0,1] structural similarity score between two
// resources based on their type hierarchy and provider.
//
//	1.0  — identical type (e.g., both "aws:ec2:instance")
//	0.7  — same type category (e.g., both "aws:ec2:*")
//	0.3  — same provider (e.g., both "aws:*")
//	0.0  — unrelated
func resourceTypeSim(a, b *types.Resource) float64 {
	if a == nil || b == nil {
		return 0
	}
	if a.Type == b.Type {
		return 1.0
	}
	pa, pb := typeCategory(a.Type), typeCategory(b.Type)
	if pa != "" && pa == pb {
		return 0.7
	}
	if a.Provider != "" && a.Provider == b.Provider {
		return 0.3
	}
	// Fall back to comparing the first segment of the type string.
	sa := firstSegment(a.Type)
	sb := firstSegment(b.Type)
	if sa != "" && sa == sb {
		return 0.3
	}
	return 0.0
}

// typeCategory returns the two-segment prefix of a colon-delimited type,
// e.g. "aws:ec2:instance" → "aws:ec2".
func typeCategory(t string) string {
	parts := strings.SplitN(t, ":", 3)
	if len(parts) >= 2 {
		return parts[0] + ":" + parts[1]
	}
	return ""
}

func firstSegment(t string) string {
	idx := strings.IndexByte(t, ':')
	if idx < 0 {
		return strings.ToLower(t)
	}
	return strings.ToLower(t[:idx])
}
