package main

import (
	"sync/atomic"
	"time"
)

type ProviderConnectionResetCreditsView struct {
	Known                     bool        `json:"known"`
	AvailableCount            int         `json:"available_count"`
	Expirations               []time.Time `json:"expirations"`
	RetrievedAt               *time.Time  `json:"retrieved_at,omitempty"`
	ManagementAvailable       bool        `json:"management_available"`
	InventoryRefreshAvailable bool        `json:"inventory_refresh_available"`
	RedemptionAvailable       bool        `json:"redemption_available"`
	DashboardURL              string      `json:"dashboard_url"`
}

type ProviderConnectionRuntimeView struct {
	Status                 string     `json:"status"`
	StatusDetail           string     `json:"status_detail,omitempty"`
	RateLimitUntil         *time.Time `json:"rate_limit_until,omitempty"`
	PrimaryUsedPercent     *float64   `json:"primary_used_percent,omitempty"`
	PrimaryWindowMinutes   *int       `json:"primary_window_minutes,omitempty"`
	PrimaryResetAt         *time.Time `json:"primary_reset_at,omitempty"`
	SecondaryUsedPercent   *float64   `json:"secondary_used_percent,omitempty"`
	SecondaryWindowMinutes *int       `json:"secondary_window_minutes,omitempty"`
	SecondaryResetAt       *time.Time `json:"secondary_reset_at,omitempty"`
	UsageRetrievedAt       *time.Time `json:"usage_retrieved_at,omitempty"`
	UsageSource            string     `json:"usage_source,omitempty"`
}

// OperatorProviderConnectionView is the canonical operator-facing connection
// DTO. It contains no provider-specific compatibility identity fields.
type OperatorProviderConnectionView struct {
	ID                string                             `json:"id"`
	PublicID          string                             `json:"public_id"`
	ProviderID        ProviderID                         `json:"provider_id"`
	Identity          ConnectionIdentity                 `json:"identity"`
	PlanType          string                             `json:"plan_type,omitempty"`
	Disabled          bool                               `json:"disabled"`
	Dead              bool                               `json:"dead"`
	NeedsVerification bool                               `json:"needs_verification,omitempty"`
	VerificationURL   string                             `json:"verification_url,omitempty"`
	HealthError       string                             `json:"health_error,omitempty"`
	CyberAccess       bool                               `json:"cyber_access,omitempty"`
	Inflight          int64                              `json:"inflight"`
	ExpiresAt         time.Time                          `json:"expires_at,omitempty"`
	LastRefresh       time.Time                          `json:"last_refresh,omitempty"`
	Penalty           float64                            `json:"penalty"`
	Score             float64                            `json:"score"`
	ScoreTooltip      string                             `json:"score_tooltip,omitempty"`
	IsPrimary         bool                               `json:"is_primary"`
	Runtime           ProviderConnectionRuntimeView      `json:"runtime"`
	ResetCredits      ProviderConnectionResetCreditsView `json:"reset_credits"`
	Usage             UsageSnapshot                      `json:"usage"`
	Totals            AccountUsage                       `json:"totals"`
}

// LegacyOperatorConnectionView preserves /admin/accounts while compatibility
// clients migrate to the canonical v2 DTO.
type LegacyOperatorConnectionView struct {
	ID                      string            `json:"id"`
	PublicID                string            `json:"public_id"`
	Type                    ProviderID        `json:"type"`
	DisplayName             string            `json:"display_name"`
	ExternalSubject         string            `json:"external_subject,omitempty"`
	IdentityAttributes      map[string]string `json:"identity_attributes,omitempty"`
	PlanType                string            `json:"plan_type,omitempty"`
	AccountID               string            `json:"account_id,omitempty"`
	IDTokenChatGPTAccountID string            `json:"id_token_chatgpt_account_id,omitempty"`
	Email                   string            `json:"email,omitempty"`
	Disabled                bool              `json:"disabled"`
	Dead                    bool              `json:"dead"`
	NeedsVerification       bool              `json:"needs_verification,omitempty"`
	VerificationURL         string            `json:"verification_url,omitempty"`
	HealthError             string            `json:"health_error,omitempty"`
	CyberAccess             bool              `json:"cyber_access,omitempty"`
	Inflight                int64             `json:"inflight"`
	ExpiresAt               time.Time         `json:"expires_at,omitempty"`
	LastRefresh             time.Time         `json:"last_refresh,omitempty"`
	Penalty                 float64           `json:"penalty"`
	Score                   float64           `json:"score"`
	ScoreTooltip            string            `json:"score_tooltip,omitempty"`
	IsPrimary               bool              `json:"is_primary"`
	Usage                   UsageSnapshot     `json:"usage"`
	Totals                  AccountUsage      `json:"totals"`
}

type connectionViewSnapshot struct {
	Canonical OperatorProviderConnectionView
	Legacy    LegacyOperatorConnectionView
}

type poolStatsConnectionSnapshot struct {
	View           ProviderConnectionStats
	ConnectionID   string
	AddedAt        time.Time
	PrimaryUsage   float64
	SecondaryUsage float64
	CyberEligible  bool
}

// ConnectionViewService is the only read-model component allowed to inspect
// mutable ProviderConnection state. It snapshots under the connection lock and
// returns detached DTO values to HTTP/data consumers.
type ConnectionViewService struct {
	pool                  *ProviderPool
	providerStateWritable bool
}

func NewConnectionViewService(pool *ProviderPool, providerStateWritable ...bool) *ConnectionViewService {
	writable := len(providerStateWritable) > 0 && providerStateWritable[0]
	return &ConnectionViewService{pool: pool, providerStateWritable: writable}
}

func (service *ConnectionViewService) OperatorConnections() []OperatorProviderConnectionView {
	snapshots := service.snapshots(time.Now())
	views := make([]OperatorProviderConnectionView, len(snapshots))
	for index := range snapshots {
		views[index] = snapshots[index].Canonical
	}
	markCanonicalPrimary(views)
	return views
}

func (service *ConnectionViewService) LegacyOperatorConnections() []LegacyOperatorConnectionView {
	snapshots := service.snapshots(time.Now())
	views := make([]LegacyOperatorConnectionView, len(snapshots))
	for index := range snapshots {
		views[index] = snapshots[index].Legacy
	}
	markLegacyPrimary(views)
	return views
}

func (service *ConnectionViewService) PoolStatsConnections(now time.Time, disableRefresh bool) []poolStatsConnectionSnapshot {
	if service == nil || service.pool == nil {
		return []poolStatsConnectionSnapshot{}
	}
	connections := service.pool.allAccounts()
	out := make([]poolStatsConnectionSnapshot, 0, len(connections))
	for _, connection := range connections {
		if connection == nil {
			continue
		}
		connection.mu.Lock()
		status := "healthy"
		if connection.Dead || connection.Disabled {
			status = "dead"
		} else if accountCoolingDownLocked(connection, now) || accountUsageExhaustedLocked(connection) {
			status = "cooldown"
		} else if connection.Penalty > 2 {
			status = "degraded"
		}
		primaryUsage := accountPrimaryUsageLocked(connection)
		secondaryUsage := accountSecondaryUsageLocked(connection)
		primaryReset := resetMinutes(now, connection.Usage.PrimaryResetAt)
		secondaryReset := resetMinutes(now, connection.Usage.SecondaryResetAt)
		breakdown := scoreBreakdown{}
		score := float64(0)
		if !connection.Dead && !connection.Disabled {
			breakdown = scoreAccountBreakdownLocked(connection, now)
			score = breakdown.Score
		}
		identity := connection.connectionIdentityLocked()
		cacheHitRate := float64(0)
		if connection.Totals.TotalInputTokens > 0 {
			cacheHitRate = float64(connection.Totals.TotalCachedTokens) / float64(connection.Totals.TotalInputTokens) * 100
		}
		view := ProviderConnectionStats{
			ID: hashAccountID(connection.ID), DisplayName: identity.DisplayName,
			ExternalSubject: identity.ExternalSubject, IdentityAttributes: cloneStringMap(identity.Attributes),
			UpstreamAccountID: connection.AccountID, AccountEmail: connection.Email, Type: string(connection.Type),
			PlanType: formatPlanWithTier(connection.PlanType, connection.RateLimitTier), Status: status,
			Penalty: connection.Penalty, PrimaryWindowUsed: primaryUsage * 100, SecondaryWindowUsed: secondaryUsage * 100,
			PrimaryWindowAvailable: usagePrimaryWindowAvailable(connection.Usage), SecondaryWindowAvailable: usageSecondaryWindowAvailable(connection.Usage),
			PrimaryResetMinutes: primaryReset, SecondaryResetMinutes: secondaryReset,
			PrimaryWindowMinutes: connection.Usage.PrimaryWindowMinutes, SecondaryWindowMinutes: connection.Usage.SecondaryWindowMinutes,
			PrimaryPaceRatio:   quotaPaceRatio(primaryUsage*100, primaryReset, connection.Usage.PrimaryWindowMinutes),
			SecondaryPaceRatio: quotaPaceRatio(secondaryUsage*100, secondaryReset, connection.Usage.SecondaryWindowMinutes),
			AccountAddedAt:     connection.AddedAt.UTC().Format(time.RFC3339), TotalInputTokens: connection.Totals.TotalInputTokens,
			TotalCachedTokens: connection.Totals.TotalCachedTokens, TotalOutputTokens: connection.Totals.TotalOutputTokens,
			TotalReasoningTokens: connection.Totals.TotalReasoningTokens, TotalBillableTokens: connection.Totals.TotalBillableTokens,
			CacheHitRate: cacheHitRate, HasCredits: connection.Usage.HasCredits, CreditsBalance: connection.Usage.CreditsBalance,
			Score: score, ScoreTooltip: scoreTooltipFromBreakdownLocked(connection, now, breakdown),
			ResetCreditsAvailable: connection.ResetCreditsAvailable, ResetCreditsKnown: !connection.ResetCreditsRetrievedAt.IsZero(),
		}
		for _, credit := range connection.RateLimitResetCredits {
			view.ResetCreditExpirations = append(view.ResetCreditExpirations, credit.ExpiresAt.UTC().Format(time.RFC3339Nano))
		}
		cyberEligible := connection.CyberAccess && !connection.Dead && !connection.Disabled &&
			(connection.ExpiresAt.IsZero() || connection.ExpiresAt.After(now) || disableRefresh)
		out = append(out, poolStatsConnectionSnapshot{View: view, ConnectionID: connection.ID, AddedAt: connection.AddedAt,
			PrimaryUsage: primaryUsage, SecondaryUsage: secondaryUsage, CyberEligible: cyberEligible})
		connection.mu.Unlock()
	}
	markPoolStatsPrimary(out)
	return out
}

func resetMinutes(now, resetAt time.Time) int {
	if resetAt.IsZero() {
		return 0
	}
	minutes := int(resetAt.Sub(now).Minutes())
	if minutes < 0 {
		return 0
	}
	return minutes
}

func markPoolStatsPrimary(snapshots []poolStatsConnectionSnapshot) {
	highest, index := map[string]float64{}, map[string]int{}
	for i := range snapshots {
		view := snapshots[i].View
		if (view.Status == "healthy" || view.Status == "degraded") && view.Score > highest[view.Type] {
			highest[view.Type], index[view.Type] = view.Score, i
		}
	}
	for _, i := range index {
		snapshots[i].View.IsPrimary = true
	}
}

func optionalTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	value = value.UTC()
	return &value
}

func providerConnectionRuntimeLocked(connection *ProviderConnection, now time.Time) ProviderConnectionRuntimeView {
	runtime := ProviderConnectionRuntimeView{Status: "healthy", UsageSource: connection.Usage.Source}
	primaryUsed, secondaryUsed := accountPrimaryUsageLocked(connection), accountSecondaryUsageLocked(connection)
	if usagePrimaryWindowAvailable(connection.Usage) {
		percent := primaryUsed * 100
		runtime.PrimaryUsedPercent = &percent
		if connection.Usage.PrimaryWindowMinutes > 0 {
			minutes := connection.Usage.PrimaryWindowMinutes
			runtime.PrimaryWindowMinutes = &minutes
		}
		runtime.PrimaryResetAt = optionalTime(connection.Usage.PrimaryResetAt)
	}
	if usageSecondaryWindowAvailable(connection.Usage) {
		percent := secondaryUsed * 100
		runtime.SecondaryUsedPercent = &percent
		if connection.Usage.SecondaryWindowMinutes > 0 {
			minutes := connection.Usage.SecondaryWindowMinutes
			runtime.SecondaryWindowMinutes = &minutes
		}
		runtime.SecondaryResetAt = optionalTime(connection.Usage.SecondaryResetAt)
	}
	runtime.UsageRetrievedAt = optionalTime(connection.Usage.RetrievedAt)
	if connection.RateLimitUntil.After(now) {
		runtime.RateLimitUntil = optionalTime(connection.RateLimitUntil)
	}
	switch {
	case connection.Disabled:
		runtime.Status, runtime.StatusDetail = "disabled", "Disabled by an administrator"
	case connection.Dead:
		runtime.Status, runtime.StatusDetail = "dead", "Credential or provider validation failed"
	case connection.NeedsVerification:
		runtime.Status, runtime.StatusDetail = "verification_required", "Provider verification is required"
	case accountCoolingDownLocked(connection, now):
		runtime.Status, runtime.StatusDetail = "cooldown", "Rate-limit cooldown is active"
	case primaryUsed >= primaryHardExcludeThreshold && secondaryUsed >= secondaryHardExcludeThreshold:
		runtime.Status, runtime.StatusDetail = "cooldown", "Primary and secondary quota are exhausted"
	case primaryUsed >= primaryHardExcludeThreshold:
		runtime.Status, runtime.StatusDetail = "cooldown", "Primary quota is exhausted"
	case secondaryUsed >= secondaryHardExcludeThreshold:
		runtime.Status, runtime.StatusDetail = "cooldown", "Secondary quota is exhausted"
	case connection.HealthError != "":
		runtime.Status, runtime.StatusDetail = "degraded", connection.HealthError
	case connection.Penalty > 2:
		runtime.Status, runtime.StatusDetail = "degraded", "Recent provider failures reduced routing priority"
	}
	return runtime
}

func (service *ConnectionViewService) snapshots(now time.Time) []connectionViewSnapshot {
	if service == nil || service.pool == nil {
		return []connectionViewSnapshot{}
	}
	connections := service.pool.allAccounts()
	out := make([]connectionViewSnapshot, 0, len(connections))
	for _, connection := range connections {
		if connection == nil {
			continue
		}
		connection.mu.Lock()
		identity := connection.connectionIdentityLocked()
		breakdown := scoreAccountBreakdownLocked(connection, now)
		score := breakdown.Score
		if connection.Dead || connection.Disabled {
			score = 0
		}
		resetCredits := ProviderConnectionResetCreditsView{
			Known: !connection.ResetCreditsRetrievedAt.IsZero(), AvailableCount: connection.ResetCreditsAvailable,
			RetrievedAt:               optionalTime(connection.ResetCreditsRetrievedAt),
			ManagementAvailable:       connection.Type == AccountTypeCodex && service.providerStateWritable,
			InventoryRefreshAvailable: connection.Type == AccountTypeCodex && service.providerStateWritable,
			RedemptionAvailable:       connection.Type == AccountTypeCodex && service.providerStateWritable && len(connection.RateLimitResetCredits) > 0,
			DashboardURL:              codexResetCreditsDashboardURL,
			Expirations:               make([]time.Time, 0, len(connection.RateLimitResetCredits)),
		}
		for _, credit := range connection.RateLimitResetCredits {
			resetCredits.Expirations = append(resetCredits.Expirations, credit.ExpiresAt.UTC())
		}
		canonical := OperatorProviderConnectionView{
			ID: connection.ID, PublicID: hashAccountID(connection.ID), ProviderID: connection.Type,
			Identity: identity, PlanType: connection.PlanType, Disabled: connection.Disabled, Dead: connection.Dead,
			NeedsVerification: connection.NeedsVerification, VerificationURL: connection.VerificationURL,
			HealthError: connection.HealthError, CyberAccess: connection.CyberAccess,
			Inflight: atomic.LoadInt64(&connection.Inflight), ExpiresAt: connection.ExpiresAt,
			LastRefresh: connection.LastRefresh, Penalty: connection.Penalty, Score: score,
			ScoreTooltip: scoreTooltipFromBreakdownLocked(connection, now, breakdown),
			Runtime:      providerConnectionRuntimeLocked(connection, now),
			ResetCredits: resetCredits,
			Usage:        connection.Usage, Totals: connection.Totals,
		}
		legacy := LegacyOperatorConnectionView{
			ID: canonical.ID, PublicID: canonical.PublicID, Type: canonical.ProviderID,
			DisplayName: identity.DisplayName, ExternalSubject: identity.ExternalSubject,
			IdentityAttributes: cloneStringMap(identity.Attributes), PlanType: canonical.PlanType,
			AccountID: connection.AccountID, IDTokenChatGPTAccountID: connection.IDTokenChatGPTAccountID,
			Email: connection.Email, Disabled: canonical.Disabled, Dead: canonical.Dead,
			NeedsVerification: canonical.NeedsVerification, VerificationURL: canonical.VerificationURL,
			HealthError: canonical.HealthError, CyberAccess: canonical.CyberAccess, Inflight: canonical.Inflight,
			ExpiresAt: canonical.ExpiresAt, LastRefresh: canonical.LastRefresh, Penalty: canonical.Penalty,
			Score: canonical.Score, ScoreTooltip: canonical.ScoreTooltip, Usage: canonical.Usage, Totals: canonical.Totals,
		}
		connection.mu.Unlock()
		canonical.Identity.Attributes = cloneStringMap(canonical.Identity.Attributes)
		out = append(out, connectionViewSnapshot{Canonical: canonical, Legacy: legacy})
	}
	return out
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	copy := make(map[string]string, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func markCanonicalPrimary(views []OperatorProviderConnectionView) {
	highest, index := map[ProviderID]float64{}, map[ProviderID]int{}
	for i, view := range views {
		if !view.Dead && !view.Disabled && view.Score > highest[view.ProviderID] {
			highest[view.ProviderID], index[view.ProviderID] = view.Score, i
		}
	}
	for _, i := range index {
		views[i].IsPrimary = true
	}
}

func markLegacyPrimary(views []LegacyOperatorConnectionView) {
	highest, index := map[ProviderID]float64{}, map[ProviderID]int{}
	for i, view := range views {
		if !view.Dead && !view.Disabled && view.Score > highest[view.Type] {
			highest[view.Type], index[view.Type] = view.Score, i
		}
	}
	for _, i := range index {
		views[i].IsPrimary = true
	}
}
