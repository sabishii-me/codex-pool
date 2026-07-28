package main

import (
	"fmt"
	"strings"
	"time"
)

func applyUsageCostDiagnostics(value *UsageDimension, pricing *PricingData) {
	if value == nil {
		return
	}
	model, ok := findModelRouteByID(value.ID)
	if !ok || (value.ProviderID != "" && model.ProviderID != ProviderID(value.ProviderID)) {
		value.CostStatus = "unknown"
		value.CostReason = "no canonical model route"
		return
	}
	sheet := priceSheetForModel(pricing, model)
	value.CostStatus = sheet.Status
	if sheet.Status == "unknown" {
		value.CostReason = sheet.Reason
	}
}

func applyUsageCacheDiagnostics(value *UsageDimension) {
	if value == nil {
		return
	}
	switch ProviderID(strings.ToLower(strings.TrimSpace(value.ProviderID))) {
	case AccountTypeCodex, AccountTypeClaude, AccountTypeGemini, AccountTypeAntigravity:
		value.CacheSemantics = "inclusive"
		if value.InputTokens > 0 {
			share := float64(value.CachedTokens) * 100 / float64(value.InputTokens)
			value.CacheReadSharePct = &share
			if value.CachedTokens > value.InputTokens {
				value.CacheDiagnostic = "cache_read_exceeds_inclusive_input"
			}
		}
	case AccountTypeDeepSeek, AccountTypeZAI:
		value.CacheSemantics = "exclusive"
		total := value.InputTokens + value.CachedTokens + value.CacheWriteTokens
		if total > 0 {
			share := float64(value.CachedTokens) * 100 / float64(total)
			value.CacheReadSharePct = &share
		}
	default:
		value.CacheSemantics = "unknown"
		if value.CachedTokens > 0 {
			value.CacheDiagnostic = "normalization_unavailable"
		}
	}
}

func usageRangeStart(now time.Time, hours int) time.Time {
	if hours <= 0 {
		hours = 24
	}
	return now.UTC().Truncate(time.Hour).Add(-time.Duration(hours-1) * time.Hour)
}

func (s *AnalyticsStore) getUsageHourly(userID string, hours int) ([]UserHourlyUsage, error) {
	if s == nil || s.db == nil {
		return []UserHourlyUsage{}, nil
	}
	if hours <= 0 {
		hours = 24
	}
	since := usageRangeStart(time.Now(), hours).Format(time.RFC3339Nano)
	filter, args := "completed_at >= ?", []any{since}
	if strings.TrimSpace(userID) != "" {
		filter += " AND user_id = ?"
		args = append(args, userID)
	}
	rows, err := s.db.Query(fmt.Sprintf(`SELECT substr(completed_at,1,13), provider_id,
		COALESCE(SUM(input_tokens),0), COALESCE(SUM(cache_read_tokens),0), COALESCE(SUM(output_tokens),0),
		COALESCE(SUM(reasoning_tokens),0), COALESCE(SUM(billable_tokens),0), COUNT(*)
		FROM usage_events WHERE %s GROUP BY substr(completed_at,1,13), provider_id ORDER BY substr(completed_at,1,13), provider_id`, filter), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserHourlyUsage{}
	for rows.Next() {
		var value UserHourlyUsage
		if err := rows.Scan(&value.Hour, &value.AccountType, &value.InputTokens, &value.CachedTokens, &value.OutputTokens, &value.ReasoningTokens, &value.BillableTokens, &value.RequestCount); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

func (s *AnalyticsStore) getUsageModelHourly(userID string, hours int) ([]UsageModelHourly, error) {
	if s == nil || s.db == nil {
		return []UsageModelHourly{}, nil
	}
	if hours <= 0 {
		hours = 24
	}
	since := usageRangeStart(time.Now(), hours).Format(time.RFC3339Nano)
	filter, args := "completed_at >= ?", []any{since}
	if strings.TrimSpace(userID) != "" {
		filter += " AND user_id = ?"
		args = append(args, userID)
	}
	rows, err := s.db.Query(fmt.Sprintf(`SELECT substr(completed_at,1,13) || ':00:00Z', COALESCE(model_id,''), provider_id, COALESCE(SUM(billable_tokens),0), COUNT(*) FROM usage_events WHERE %s GROUP BY substr(completed_at,1,13), provider_id, model_id ORDER BY substr(completed_at,1,13), provider_id, model_id`, filter), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UsageModelHourly{}
	for rows.Next() {
		var value UsageModelHourly
		if err := rows.Scan(&value.Hour, &value.ModelID, &value.ProviderID, &value.BillableTokens, &value.Requests); err != nil {
			return nil, err
		}
		if value.ModelID == "" {
			value.ModelID = "unknown"
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

func (s *AnalyticsStore) getUsageDimensions(userID string, hours int, includeConnections bool) (models, providers, connections []UsageDimension, err error) {
	if s == nil || s.db == nil {
		return []UsageDimension{}, []UsageDimension{}, []UsageDimension{}, nil
	}
	since := usageRangeStart(time.Now(), hours).Format(time.RFC3339Nano)
	filter, args := "completed_at >= ?", []any{since}
	if strings.TrimSpace(userID) != "" {
		filter += " AND user_id = ?"
		args = append(args, userID)
	}
	query := func(group, id, provider string) ([]UsageDimension, error) {
		rows, queryErr := s.db.Query(fmt.Sprintf(`SELECT COALESCE(%s,''), COALESCE(%s,''), COUNT(*),
			COALESCE(SUM(input_tokens),0), COALESCE(SUM(cache_read_tokens),0), COALESCE(SUM(cache_write_tokens),0), COALESCE(SUM(output_tokens),0),
			COALESCE(SUM(reasoning_tokens),0), COALESCE(SUM(billable_tokens),0), COALESCE(SUM(cost_usd),0)
			FROM usage_events WHERE %s GROUP BY %s ORDER BY SUM(billable_tokens) DESC`, id, provider, filter, group), args...)
		if queryErr != nil {
			return nil, queryErr
		}
		defer rows.Close()
		out := []UsageDimension{}
		for rows.Next() {
			var value UsageDimension
			if scanErr := rows.Scan(&value.ID, &value.ProviderID, &value.Requests, &value.InputTokens, &value.CachedTokens, &value.CacheWriteTokens, &value.OutputTokens, &value.ReasoningTokens, &value.BillableTokens, &value.CostUSD); scanErr != nil {
				return nil, scanErr
			}
			if value.ID == "" {
				value.ID = "unknown"
			}
			applyUsageCacheDiagnostics(&value)
			out = append(out, value)
		}
		return out, rows.Err()
	}
	models, err = query("provider_id, model_id", "model_id", "provider_id")
	if err != nil {
		return
	}
	providers, err = query("provider_id", "provider_id", "provider_id")
	if err != nil {
		return
	}
	connections = []UsageDimension{}
	if includeConnections {
		connections, err = query("provider_id, connection_id", "connection_id", "provider_id")
	}
	return
}
