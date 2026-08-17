package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type UsageEvidence struct {
	Kind        string    `json:"kind"`
	Source      string    `json:"source"`
	GeneratedAt time.Time `json:"generated_at"`
	DataSince   time.Time `json:"data_since,omitempty"`
}

type UsageDimension struct {
	ID                string   `json:"id"`
	ProviderID        string   `json:"provider_id,omitempty"`
	Requests          int64    `json:"requests"`
	InputTokens       int64    `json:"input_tokens"`
	CachedTokens      int64    `json:"cached_tokens"`
	CacheWriteTokens  int64    `json:"cache_write_tokens"`
	OutputTokens      int64    `json:"output_tokens"`
	ReasoningTokens   int64    `json:"reasoning_tokens"`
	BillableTokens    int64    `json:"billable_tokens"`
	CostUSD           float64  `json:"cost_usd"`
	CostStatus        string   `json:"cost_status"`
	CostReason        string   `json:"cost_reason,omitempty"`
	CacheSemantics    string   `json:"cache_semantics"`
	CacheReadSharePct *float64 `json:"cache_read_share_pct"`
	CacheDiagnostic   string   `json:"cache_diagnostic,omitempty"`
}

type UsageModelHourly struct {
	Hour           string `json:"hour"`
	ModelID        string `json:"model_id"`
	ProviderID     string `json:"provider_id"`
	BillableTokens int64  `json:"billable_tokens"`
	Requests       int64  `json:"requests"`
}

type UsageProjection struct {
	Scope           string                 `json:"scope"`
	SubjectID       string                 `json:"subject_id,omitempty"`
	RangeHours      int                    `json:"range_hours"`
	RangeDays       int                    `json:"range_days"`
	Evidence        UsageEvidence          `json:"evidence"`
	Totals          UserUsage              `json:"totals"`
	Hourly          []UserHourlyUsage      `json:"hourly"`
	Daily           []UserDailyUsage       `json:"daily"`
	ByModel         []UsageDimension       `json:"by_model"`
	ByProvider      []UsageDimension       `json:"by_provider"`
	ByConnection    []UsageDimension       `json:"by_connection,omitempty"`
	ModelHourly     []UsageModelHourly     `json:"model_hourly"`
	Economics       []SignalEconomicsPoint `json:"economics,omitempty"`
	PartialFailures []string               `json:"partial_failures"`
}

func parseUsageRange(r *http.Request) (hours, days int) {
	hours, days = 24*14, 42
	if value, err := strconv.Atoi(r.URL.Query().Get("hours")); err == nil && value > 0 {
		hours = min(value, 24*31)
	}
	if value, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && value > 0 {
		days = min(value, 90)
	}
	return
}

func (h *proxyHandler) handleUsageV2(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	user, ok := h.sessionUser(r)
	if !ok {
		respondJSONError(w, http.StatusUnauthorized, "not signed in")
		return
	}
	scope := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope")))
	if scope == "" {
		scope = "me"
	}
	if scope != "me" && !h.checkAdminAuth(w, r) {
		return
	}
	if h.store == nil {
		respondJSONError(w, http.StatusServiceUnavailable, "usage projection unavailable")
		return
	}

	hours, days := parseUsageRange(r)
	projection := UsageProjection{Scope: scope, RangeHours: hours, RangeDays: days, Evidence: UsageEvidence{Kind: "measured", Source: "canonical_usage_store", GeneratedAt: time.Now().UTC(), DataSince: time.Now().UTC().Add(-h.store.retention)}, Hourly: []UserHourlyUsage{}, Daily: []UserDailyUsage{}, ByModel: []UsageDimension{}, ByProvider: []UsageDimension{}, ByConnection: []UsageDimension{}, ModelHourly: []UsageModelHourly{}, PartialFailures: []string{}}
	var subjectID string
	switch scope {
	case "me":
		subjectID = user.ID
	case "member":
		subjectID = strings.TrimSpace(r.URL.Query().Get("member_id"))
		if subjectID == "" {
			respondJSONError(w, http.StatusBadRequest, "member_id is required")
			return
		}
	case "pool":
		projection.SubjectID = "pool"
		hourly, err := h.store.getGlobalHourlyUsage(hours)
		if err != nil {
			respondJSONError(w, http.StatusInternalServerError, "failed to load pool usage")
			return
		}
		projection.Hourly = hourly
		projection.Totals = usageTotalsFromHourly(hourly)
		if h.analyticsStore != nil {
			projection.Hourly, err = h.analyticsStore.getUsageHourly("", hours)
			if err == nil {
				projection.Totals = usageTotalsFromHourly(projection.Hourly)
				projection.ByModel, projection.ByProvider, projection.ByConnection, err = h.analyticsStore.getUsageDimensions("", hours, true)
			}
			if err == nil {
				projection.ModelHourly, err = h.analyticsStore.getUsageModelHourly("", hours)
			}
			if err != nil {
				projection.PartialFailures = append(projection.PartialFailures, "usage detail unavailable")
			} else {
				applyUsageProjectionDiagnostics(projection.ByModel, h.pricing, h.registry, true)
				applyUsageProjectionDiagnostics(projection.ByProvider, h.pricing, h.registry, false)
				applyUsageProjectionDiagnostics(projection.ByConnection, h.pricing, h.registry, false)
			}
			economics, err := h.buildSignalEconomics(projection.Evidence.GeneratedAt)
			if err != nil {
				projection.PartialFailures = append(projection.PartialFailures, "economics unavailable")
			} else {
				projection.Economics = economics
			}
		}
		respondJSON(w, projection)
		return
	}
	projection.SubjectID = subjectID
	var err error
	projection.Hourly, err = h.store.getUserHourlyUsage(subjectID, hours)
	if err != nil {
		respondJSONError(w, http.StatusInternalServerError, "failed to load hourly usage")
		return
	}
	projection.Totals = usageTotalsFromHourly(projection.Hourly)
	projection.Daily, err = h.store.getUserDailyUsage(subjectID, days)
	if err != nil {
		respondJSONError(w, http.StatusInternalServerError, "failed to load daily usage")
		return
	}
	if h.analyticsStore != nil {
		projection.Hourly, err = h.analyticsStore.getUsageHourly(subjectID, hours)
		if err == nil {
			projection.Totals = usageTotalsFromHourly(projection.Hourly)
			projection.ByModel, projection.ByProvider, _, err = h.analyticsStore.getUsageDimensions(subjectID, hours, false)
		}
		if err == nil {
			projection.ModelHourly, err = h.analyticsStore.getUsageModelHourly(subjectID, hours)
		}
		if err != nil {
			projection.PartialFailures = append(projection.PartialFailures, "usage detail unavailable")
		} else {
			applyUsageProjectionDiagnostics(projection.ByModel, h.pricing, h.registry, true)
			applyUsageProjectionDiagnostics(projection.ByProvider, h.pricing, h.registry, false)
		}
	}
	respondJSON(w, projection)
}

func applyUsageProjectionDiagnostics(values []UsageDimension, pricing *PricingData, registry *ProviderRegistry, includeCost bool) {
	for index := range values {
		applyUsageCacheDiagnostics(&values[index])
		if includeCost {
			applyUsageCostDiagnostics(&values[index], pricing, registry)
		} else {
			values[index].CostStatus = "aggregate"
		}
	}
}

func usageTotalsFromHourly(hourly []UserHourlyUsage) UserUsage {
	var totals UserUsage
	for _, row := range hourly {
		totals.TotalInputTokens += row.InputTokens
		totals.TotalCachedTokens += row.CachedTokens
		totals.TotalOutputTokens += row.OutputTokens
		totals.TotalReasoningTokens += row.ReasoningTokens
		totals.TotalBillableTokens += row.BillableTokens
		totals.RequestCount += row.RequestCount
	}
	return totals
}

func addUserUsage(target *UserUsage, value UserUsage) {
	target.TotalInputTokens += value.TotalInputTokens
	target.TotalCachedTokens += value.TotalCachedTokens
	target.TotalOutputTokens += value.TotalOutputTokens
	target.TotalReasoningTokens += value.TotalReasoningTokens
	target.TotalBillableTokens += value.TotalBillableTokens
	target.RequestCount += value.RequestCount
	if target.FirstSeen.IsZero() || (!value.FirstSeen.IsZero() && value.FirstSeen.Before(target.FirstSeen)) {
		target.FirstSeen = value.FirstSeen
	}
	if value.LastSeen.After(target.LastSeen) {
		target.LastSeen = value.LastSeen
	}
}

func (h *proxyHandler) handleUsageEconomicsV2(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) || !h.checkAdminAuth(w, r) {
		return
	}
	if strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope"))) != "pool" {
		respondJSONError(w, http.StatusBadRequest, "economics supports pool scope only")
		return
	}
	if h.analyticsStore == nil {
		respondJSONError(w, http.StatusServiceUnavailable, "economics projection unavailable")
		return
	}
	now := time.Now().UTC()
	points, err := h.buildSignalEconomics(now)
	if err != nil {
		respondJSONError(w, http.StatusInternalServerError, "failed to load economics")
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"scope": "pool", "evidence": UsageEvidence{Kind: "estimated", Source: "pricing_and_subscription_projection", GeneratedAt: now}, "points": points})
}
