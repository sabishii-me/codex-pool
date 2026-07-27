package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"
)

// StatusData contains the machine-readable gateway status projection.
type StatusData struct {
	GeneratedAt     time.Time
	Uptime          time.Duration
	TotalCount      int
	CodexCount      int
	GeminiCount     int
	ClaudeCount     int
	KimiCount       int
	MinimaxCount    int
	ZAICount        int
	GrokCount       int
	PoolUsers       int
	Accounts        []AccountStatus
	TokenAnalytics  *TokenAnalytics
	PoolUtilization []PoolUtilization `json:"pool_utilization,omitempty"`
}

// TokenAnalytics contains capacity estimation data for status projections.
type TokenAnalytics struct {
	PlanCapacities []PlanCapacityView
	TotalSamples   int64
	ModelInfo      string
}

// PlanCapacityView is the serialized plan-capacity projection.
type PlanCapacityView struct {
	PlanType                   string
	SampleCount                int64
	Confidence                 string
	TotalInputTokens           int64
	TotalOutputTokens          int64
	TotalCachedTokens          int64
	TotalReasoningTokens       int64
	TotalBillableTokens        int64
	OutputMultiplier           float64
	EffectivePerPrimaryPct     int64
	EffectivePerSecondaryPct   int64
	EstimatedPrimaryCapacity   string // e.g., "~2.5M tokens"
	EstimatedSecondaryCapacity string
}

// AccountStatus is the serialized status of one provider connection.
type AccountStatus struct {
	ID                 string
	Type               string
	PlanType           string
	Disabled           bool
	Dead               bool
	CoolingDown        bool
	PrimaryUsed        float64
	SecondaryUsed      float64
	EffectivePrimary   float64 // After applying plan weight
	EffectiveSecondary float64
	PrimaryResetIn     string
	SecondaryResetIn   string
	CooldownIn         string
	ExpiresIn          string
	LastUsed           string
	Score              float64
	ScoreTooltip       string
	Inflight           int64
	TotalTokens        int64
}

func (h *proxyHandler) serveStatusPage(w http.ResponseWriter, r *http.Request) {
	h.pool.mu.RLock()
	defer h.pool.mu.RUnlock()

	now := time.Now()
	data := StatusData{
		GeneratedAt: now,
		Uptime:      now.Sub(h.startTime),
		TotalCount:  len(h.pool.accounts),
	}

	if h.poolUsers != nil {
		data.PoolUsers = len(h.poolUsers.List())
	}

	for _, a := range h.pool.accounts {
		a.mu.Lock()

		switch a.Type {
		case AccountTypeCodex:
			data.CodexCount++
		case AccountTypeGemini:
			data.GeminiCount++
		case AccountTypeClaude:
			data.ClaudeCount++
		case AccountTypeKimi:
			data.KimiCount++
		case AccountTypeMinimax:
			data.MinimaxCount++
		case AccountTypeZAI:
			data.ZAICount++
		case AccountTypeGrok:
			data.GrokCount++
		}

		primaryUsed := a.Usage.PrimaryUsedPercent
		if primaryUsed == 0 {
			primaryUsed = a.Usage.PrimaryUsed
		}
		secondaryUsed := a.Usage.SecondaryUsedPercent
		if secondaryUsed == 0 {
			secondaryUsed = a.Usage.SecondaryUsed
		}

		// Effective usage is now just the raw usage (no capacity weighting)
		effectivePrimary := primaryUsed
		effectiveSecondary := secondaryUsed
		scoreBreakdown := scoreAccountBreakdownLocked(a, now)

		status := AccountStatus{
			ID:                 a.ID,
			Type:               string(a.Type),
			PlanType:           a.PlanType,
			Disabled:           a.Disabled,
			Dead:               a.Dead,
			CoolingDown:        accountCoolingDownLocked(a, now),
			PrimaryUsed:        primaryUsed * 100,
			SecondaryUsed:      secondaryUsed * 100,
			EffectivePrimary:   effectivePrimary * 100,
			EffectiveSecondary: effectiveSecondary * 100,
			Score:              scoreBreakdown.Score,
			ScoreTooltip:       scoreTooltipFromBreakdownLocked(a, now, scoreBreakdown),
			Inflight:           a.Inflight,
			TotalTokens:        a.Totals.TotalBillableTokens,
		}

		// Format time strings
		if !a.Usage.PrimaryResetAt.IsZero() && a.Usage.PrimaryResetAt.After(now) {
			status.PrimaryResetIn = formatDuration(a.Usage.PrimaryResetAt.Sub(now))
		} else if a.Usage.PrimaryWindowMinutes > 0 {
			status.PrimaryResetIn = fmt.Sprintf("~%dm", a.Usage.PrimaryWindowMinutes)
		}

		if !a.Usage.SecondaryResetAt.IsZero() && a.Usage.SecondaryResetAt.After(now) {
			status.SecondaryResetIn = formatDuration(a.Usage.SecondaryResetAt.Sub(now))
		} else if a.Usage.SecondaryWindowMinutes > 0 {
			status.SecondaryResetIn = fmt.Sprintf("~%dd", a.Usage.SecondaryWindowMinutes/60/24)
		}

		if status.CoolingDown {
			status.CooldownIn = formatDuration(a.RateLimitUntil.Sub(now))
		}

		if !a.ExpiresAt.IsZero() {
			if a.ExpiresAt.Before(now) {
				status.ExpiresIn = "EXPIRED"
			} else {
				status.ExpiresIn = formatDuration(a.ExpiresAt.Sub(now))
			}
		}

		if !a.LastUsed.IsZero() {
			status.LastUsed = formatDuration(now.Sub(a.LastUsed)) + " ago"
		} else {
			status.LastUsed = "never"
		}

		a.mu.Unlock()
		data.Accounts = append(data.Accounts, status)
	}

	// Sort by score descending (best accounts first)
	sort.Slice(data.Accounts, func(i, j int) bool {
		return data.Accounts[i].Score > data.Accounts[j].Score
	})

	// Load token analytics
	if h.store != nil {
		data.TokenAnalytics = h.loadTokenAnalytics()
	}

	// Compute per-provider time-weighted utilization
	data.PoolUtilization = h.pool.getPoolUtilization()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	return fmt.Sprintf("%.1fd", d.Hours()/24)
}

func (h *proxyHandler) loadTokenAnalytics() *TokenAnalytics {
	caps, err := h.store.loadAllPlanCapacity()
	if err != nil || len(caps) == 0 {
		return nil
	}

	analytics := &TokenAnalytics{
		ModelInfo: "effective = input + (cached × 0.1) + (output × mult) + (reasoning × mult)",
	}

	for planType, cap := range caps {
		analytics.TotalSamples += cap.SampleCount

		confidence := "low"
		if cap.SampleCount >= 20 {
			confidence = "high"
		} else if cap.SampleCount >= 5 {
			confidence = "medium"
		}

		mult := cap.OutputMultiplier
		if mult == 0 {
			mult = 4.0
		}

		view := PlanCapacityView{
			PlanType:                 planType,
			SampleCount:              cap.SampleCount,
			Confidence:               confidence,
			TotalInputTokens:         cap.TotalInputTokens,
			TotalOutputTokens:        cap.TotalOutputTokens,
			TotalCachedTokens:        cap.TotalCachedTokens,
			TotalReasoningTokens:     cap.TotalReasoningTokens,
			TotalBillableTokens:      cap.TotalTokens,
			OutputMultiplier:         mult,
			EffectivePerPrimaryPct:   int64(cap.EffectivePerPrimaryPct),
			EffectivePerSecondaryPct: int64(cap.EffectivePerSecondaryPct),
		}

		// Format capacity estimates
		if cap.EffectivePerPrimaryPct > 0 {
			total := int64(cap.EffectivePerPrimaryPct * 100)
			view.EstimatedPrimaryCapacity = formatTokenCount(total)
		}
		if cap.EffectivePerSecondaryPct > 0 {
			total := int64(cap.EffectivePerSecondaryPct * 100)
			view.EstimatedSecondaryCapacity = formatTokenCount(total)
		}

		analytics.PlanCapacities = append(analytics.PlanCapacities, view)
	}

	// Sort by plan type
	sort.Slice(analytics.PlanCapacities, func(i, j int) bool {
		order := map[string]int{"team": 0, "pro": 1, "prolite": 2, "plus": 3, "gemini": 4}
		return order[analytics.PlanCapacities[i].PlanType] < order[analytics.PlanCapacities[j].PlanType]
	})

	return analytics
}

func formatTokenCount(n int64) string {
	if n >= 1_000_000_000 {
		return fmt.Sprintf("~%.1fB", float64(n)/1_000_000_000)
	}
	if n >= 1_000_000 {
		return fmt.Sprintf("~%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("~%.0fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
