package main

import (
	"sync/atomic"
	"time"
)

// OperatorProviderConnectionView is the canonical operator-facing connection
// DTO. It contains no provider-specific compatibility identity fields.
type OperatorProviderConnectionView struct {
	ID                string             `json:"id"`
	PublicID          string             `json:"public_id"`
	ProviderID        ProviderID         `json:"provider_id"`
	Identity          ConnectionIdentity `json:"identity"`
	PlanType          string             `json:"plan_type,omitempty"`
	Disabled          bool               `json:"disabled"`
	Dead              bool               `json:"dead"`
	NeedsVerification bool               `json:"needs_verification,omitempty"`
	VerificationURL   string             `json:"verification_url,omitempty"`
	HealthError       string             `json:"health_error,omitempty"`
	CyberAccess       bool               `json:"cyber_access,omitempty"`
	Inflight          int64              `json:"inflight"`
	ExpiresAt         time.Time          `json:"expires_at,omitempty"`
	LastRefresh       time.Time          `json:"last_refresh,omitempty"`
	Penalty           float64            `json:"penalty"`
	Score             float64            `json:"score"`
	ScoreTooltip      string             `json:"score_tooltip,omitempty"`
	IsPrimary         bool               `json:"is_primary"`
	Usage             UsageSnapshot      `json:"usage"`
	Totals            AccountUsage       `json:"totals"`
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

// ConnectionViewService is the only read-model component allowed to inspect
// mutable ProviderConnection state. It snapshots under the connection lock and
// returns detached DTO values to HTTP/data consumers.
type ConnectionViewService struct {
	pool *ProviderPool
}

func NewConnectionViewService(pool *ProviderPool) *ConnectionViewService {
	return &ConnectionViewService{pool: pool}
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
		canonical := OperatorProviderConnectionView{
			ID: connection.ID, PublicID: hashAccountID(connection.ID), ProviderID: connection.Type,
			Identity: identity, PlanType: connection.PlanType, Disabled: connection.Disabled, Dead: connection.Dead,
			NeedsVerification: connection.NeedsVerification, VerificationURL: connection.VerificationURL,
			HealthError: connection.HealthError, CyberAccess: connection.CyberAccess,
			Inflight: atomic.LoadInt64(&connection.Inflight), ExpiresAt: connection.ExpiresAt,
			LastRefresh: connection.LastRefresh, Penalty: connection.Penalty, Score: score,
			ScoreTooltip: scoreTooltipFromBreakdownLocked(connection, now, breakdown),
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
