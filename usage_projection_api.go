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
	ID              string  `json:"id"`
	ProviderID      string  `json:"provider_id,omitempty"`
	Requests        int64   `json:"requests"`
	InputTokens     int64   `json:"input_tokens"`
	CachedTokens    int64   `json:"cached_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	ReasoningTokens int64   `json:"reasoning_tokens"`
	BillableTokens  int64   `json:"billable_tokens"`
	CostUSD         float64 `json:"cost_usd"`
}

type UsageProjection struct {
	Scope           string                 `json:"scope"`
	SubjectID       string                 `json:"subject_id,omitempty"`
	Evidence        UsageEvidence          `json:"evidence"`
	Totals          UserUsage              `json:"totals"`
	Hourly          []UserHourlyUsage      `json:"hourly"`
	Daily           []UserDailyUsage       `json:"daily"`
	ByModel         []UsageDimension       `json:"by_model"`
	ByProvider      []UsageDimension       `json:"by_provider"`
	ByConnection    []UsageDimension       `json:"by_connection,omitempty"`
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
	projection := UsageProjection{Scope: scope, Evidence: UsageEvidence{Kind: "measured", Source: "canonical_usage_store", GeneratedAt: time.Now().UTC(), DataSince: time.Now().UTC().Add(-h.store.retention)}, Hourly: []UserHourlyUsage{}, Daily: []UserDailyUsage{}, ByModel: []UsageDimension{}, ByProvider: []UsageDimension{}, ByConnection: []UsageDimension{}, PartialFailures: []string{}}
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
		users, err := h.store.getAllUserUsage()
		if err != nil {
			respondJSONError(w, http.StatusInternalServerError, "failed to load pool totals")
			return
		}
		for _, totals := range users {
			addUserUsage(&projection.Totals, totals)
		}
		if h.analyticsStore != nil {
			projection.ByModel, projection.ByProvider, projection.ByConnection, err = h.analyticsStore.getUsageDimensions("", days, true)
			if err != nil {
				projection.PartialFailures = append(projection.PartialFailures, "usage detail unavailable")
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
	totals, err := h.store.getUserUsage(subjectID)
	if err != nil {
		respondJSONError(w, http.StatusInternalServerError, "failed to load usage totals")
		return
	}
	if totals != nil {
		projection.Totals = *totals
	}
	projection.Hourly, err = h.store.getUserHourlyUsage(subjectID, hours)
	if err != nil {
		respondJSONError(w, http.StatusInternalServerError, "failed to load hourly usage")
		return
	}
	projection.Daily, err = h.store.getUserDailyUsage(subjectID, days)
	if err != nil {
		respondJSONError(w, http.StatusInternalServerError, "failed to load daily usage")
		return
	}
	if h.analyticsStore != nil {
		projection.ByModel, projection.ByProvider, _, err = h.analyticsStore.getUsageDimensions(subjectID, days, false)
		if err != nil {
			projection.PartialFailures = append(projection.PartialFailures, "usage detail unavailable")
		}
	}
	respondJSON(w, projection)
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
