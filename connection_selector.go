package main

import (
	"sort"
	"time"
)

const (
	ordinaryRoutingWeight         = 1
	cyberAccessRoutingWeight      = 2
	competitiveRoutingScoreWindow = 0.15
)

type weightedConnectionCandidate struct {
	connection  *ProviderConnection
	score       float64
	cyberAccess bool
}

// selectQuotaCompetitiveConnection applies ordinary Codex routing fairness
// after the pool has enforced eligibility, tier, quota, and health policy.
// Explicit cyber-policy retries never call this function.
func selectQuotaCompetitiveConnection(candidates []weightedConnectionCandidate, sequence uint64) *ProviderConnection {
	if len(candidates) == 0 {
		return nil
	}
	bestScore := candidates[0].score
	for _, candidate := range candidates[1:] {
		if candidate.score > bestScore {
			bestScore = candidate.score
		}
	}
	minimumScore := bestScore - competitiveRoutingScoreWindow
	competitive := make([]weightedConnectionCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.connection != nil && candidate.score >= minimumScore {
			competitive = append(competitive, candidate)
		}
	}
	if len(competitive) == 0 {
		return nil
	}
	sort.Slice(competitive, func(i, j int) bool { return competitive[i].connection.ID < competitive[j].connection.ID })
	totalWeight := 0
	for _, candidate := range competitive {
		totalWeight += ordinaryRoutingWeight
		if candidate.cyberAccess {
			totalWeight += cyberAccessRoutingWeight - ordinaryRoutingWeight
		}
	}
	slot := int(sequence % uint64(totalWeight))
	for _, candidate := range competitive {
		weight := ordinaryRoutingWeight
		if candidate.cyberAccess {
			weight = cyberAccessRoutingWeight
		}
		if slot < weight {
			return candidate.connection
		}
		slot -= weight
	}
	return competitive[len(competitive)-1].connection
}

// ConnectionSelectionMode declares exceptional capacity constraints without
// duplicating provider-pool scoring in request paths.
type ConnectionSelectionMode int

const (
	SelectGeneral ConnectionSelectionMode = iota
	SelectExactID
	SelectCyberAccess
	SelectModelCapability
	SelectImageFanout
)

// ConnectionSelection is the provider-neutral input to connection selection.
type ConnectionSelection struct {
	Mode           ConnectionSelectionMode
	ProviderID     ProviderID
	ConnectionID   string
	ConversationID string
	RequiredPlan   string
	ClientIP       string
	Model          string
	FanoutIndex    int
	RequireImages  bool
	PreferIdle     bool
	Exclude        map[string]bool
}

// ConnectionSelector owns the boundary between request routing and pool
// scoring/pinning. The existing score engine remains unchanged behind it.
type ConnectionSelector struct {
	pool *ProviderPool
}

func NewConnectionSelector(pool *ProviderPool) *ConnectionSelector {
	return &ConnectionSelector{pool: pool}
}

func (selector *ConnectionSelector) Select(request ConnectionSelection) *ProviderConnection {
	if selector == nil || selector.pool == nil {
		return nil
	}
	switch request.Mode {
	case SelectExactID:
		return selector.pool.candidateByID(request.ConnectionID, request.ProviderID, request.RequiredPlan, request.ClientIP)
	case SelectCyberAccess:
		return selector.pool.candidateWithCyberAccess(request.Exclude, request.ProviderID, request.RequiredPlan, request.ClientIP)
	case SelectModelCapability:
		if request.ProviderID == AccountTypeAntigravity {
			return selector.pool.candidateForAntigravityModel(request.ConversationID, request.Exclude, request.Model, request.ClientIP)
		}
		return nil
	case SelectImageFanout:
		exclude := copyConnectionExclusions(request.Exclude)
		if request.RequireImages {
			selector.pool.excludeImageIncapable(exclude)
		}
		if request.PreferIdle {
			selector.pool.excludeInflightWhenIdleAvailable(request.ProviderID, exclude)
		}
		return selector.pool.imageFanoutCandidate(request.FanoutIndex, exclude, request.RequiredPlan, request.ClientIP)
	default:
		exclude := request.Exclude
		if request.RequireImages || request.PreferIdle {
			exclude = copyConnectionExclusions(request.Exclude)
			if request.RequireImages {
				selector.pool.excludeImageIncapable(exclude)
			}
			if request.PreferIdle {
				selector.pool.excludeInflightWhenIdleAvailable(request.ProviderID, exclude)
			}
		}
		return selector.pool.candidate(request.ConversationID, exclude, request.ProviderID, request.RequiredPlan, request.ClientIP)
	}
}

func copyConnectionExclusions(source map[string]bool) map[string]bool {
	copy := make(map[string]bool, len(source))
	for id, excluded := range source {
		copy[id] = excluded
	}
	return copy
}

func (selector *ConnectionSelector) NearestCooldown(providerID ProviderID, exclude map[string]bool) time.Duration {
	if selector == nil || selector.pool == nil {
		return 0
	}
	return selector.pool.nearestCooldown(providerID, exclude)
}
