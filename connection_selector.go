package main

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

const (
	ordinaryRoutingWeight         = 1
	cyberAccessRoutingWeight      = 2
	competitiveRoutingScoreWindow = 0.30
)

type weightedConnectionCandidate struct {
	connection   *ProviderConnection
	score        float64
	cyberAccess  bool
	telemetrySet bool
}

// selectQuotaCompetitiveConnection applies ordinary Codex routing fairness
// after the pool has enforced eligibility, tier, quota, and health policy.
// Known telemetry is preferred; unknown-only sets retain deterministic bounded
// exploration. Explicit cyber-policy retries never call this function.
func selectQuotaCompetitiveConnection(candidates []weightedConnectionCandidate, sequence uint64) *ProviderConnection {
	if len(candidates) == 0 {
		return nil
	}
	valid := make([]weightedConnectionCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.connection != nil {
			valid = append(valid, candidate)
		}
	}
	if len(valid) == 0 {
		return nil
	}
	candidates = valid
	knownTelemetry := make([]weightedConnectionCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.telemetrySet {
			knownTelemetry = append(knownTelemetry, candidate)
		}
	}
	// Unknown telemetry is not free capacity. Prefer valid observations when
	// available; an all-unknown pool still receives bounded deterministic use.
	if len(knownTelemetry) > 0 {
		candidates = knownTelemetry
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
		if candidate.score >= minimumScore {
			competitive = append(competitive, candidate)
		}
	}
	if len(competitive) == 0 {
		return nil
	}
	sort.Slice(competitive, func(i, j int) bool { return competitive[i].connection.ID < competitive[j].connection.ID })
	totalWeight := 0
	for _, candidate := range competitive {
		totalWeight += routingWeightFor(candidate)
	}
	slot := int(sequence % uint64(totalWeight))
	for _, candidate := range competitive {
		weight := routingWeightFor(candidate)
		if slot < weight {
			return candidate.connection
		}
		slot -= weight
	}
	return competitive[len(competitive)-1].connection
}

// routingWeightFor returns the weighted routing share for a candidate. Cyber
// access carries more weight; ordinary Codex capacity is unweighted so every
// account is consumed evenly (smaller-capacity plans naturally hit their
// limit first, then the larger capacity is burned).
func routingWeightFor(candidate weightedConnectionCandidate) int {
	return routingWeightForConnection(candidate.connection)
}

// routingWeightForConnection returns the routing weight for a connection,
// independent of candidate selection.
func routingWeightForConnection(a *ProviderConnection) int {
	if a == nil {
		return ordinaryRoutingWeight
	}
	if a.CyberAccess {
		return cyberAccessRoutingWeight
	}
	return ordinaryRoutingWeight
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
	RoutingContext RequestRoutingContext
	ConversationID string // compatibility affinity key for explicit internal/Antigravity callers
	RequiredPlan   string
	ClientIP       string
	Model          string
	FanoutIndex    int
	RequireImages  bool
	PreferIdle     bool
	Exclude        map[string]bool
}

// ConnectionSelector owns the boundary between request routing and pool
// scoring/affinity. Protocol-owned callers pass RequestRoutingContext; the
// compatibility ConversationID field remains only for explicit internal model
// executors until their typed profiles land.
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
		routingContext := request.RoutingContext
		// Ordinary protocol-owned affinity must be the private versioned form and
		// must agree with the selected provider/protocol. Compatibility identity is
		// considered only when no typed context was supplied.
		affinityKey := routingContext.AffinityKey
		// Any context field means the caller chose the typed boundary. Inactive or
		// invalid typed contexts must never fall back to an untyped identity.
		typedContextSupplied := routingContext.AffinityKey != "" || routingContext.Provider != "" || routingContext.Protocol != "" || routingContext.CanonicalModel != "" || routingContext.SoftAffinity.Value != ""
		if affinityKey != "" && !routingContextMatchesSelection(routingContext, request.ProviderID) {
			affinityKey = ""
		}
		if affinityKey == "" && !typedContextSupplied {
			affinityKey = request.ConversationID
		}
		return selector.pool.candidate(affinityKey, exclude, request.ProviderID, request.RequiredPlan, request.ClientIP)
	}
}

func isPrivateAffinityKey(value string) bool {
	const prefix = "aff:v1:"
	if len(value) != len(prefix)+sha256.Size*2 || !strings.HasPrefix(value, prefix) {
		return false
	}
	_, err := hex.DecodeString(value[len(prefix):])
	return err == nil
}

func routingContextMatchesSelection(context RequestRoutingContext, providerID ProviderID) bool {
	if !isPrivateAffinityKey(context.AffinityKey) {
		return false
	}
	provider := ProviderID(strings.ToLower(strings.TrimSpace(string(context.Provider))))
	protocol := strings.ToLower(strings.TrimSpace(context.Protocol))
	model := strings.TrimSpace(context.CanonicalModel)
	kind := AffinityKind(strings.ToLower(strings.TrimSpace(string(context.SoftAffinity.Kind))))
	providerID = ProviderID(strings.ToLower(strings.TrimSpace(string(providerID))))
	return provider == providerID && model != "" && recognizedAffinityProtocol(protocol) && recognizedAffinityKind(kind) && affinityKindAllowedForProtocolProvider(protocol, provider, kind)
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
