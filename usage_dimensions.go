package main

import (
	"fmt"
	"strings"
	"time"
)

func (s *AnalyticsStore) getUsageDimensions(userID string, days int, includeConnections bool) (models, providers, connections []UsageDimension, err error) {
	if s == nil || s.db == nil {
		return []UsageDimension{}, []UsageDimension{}, []UsageDimension{}, nil
	}
	if days <= 0 {
		days = 30
	}
	since := time.Now().UTC().AddDate(0, 0, -days).Format(time.RFC3339Nano)
	filter, args := "completed_at >= ?", []any{since}
	if strings.TrimSpace(userID) != "" {
		filter += " AND user_id = ?"
		args = append(args, userID)
	}
	query := func(group, id, provider string) ([]UsageDimension, error) {
		rows, queryErr := s.db.Query(fmt.Sprintf(`SELECT COALESCE(%s,''), COALESCE(%s,''), COUNT(*),
			COALESCE(SUM(input_tokens),0), COALESCE(SUM(cache_read_tokens),0), COALESCE(SUM(output_tokens),0),
			COALESCE(SUM(reasoning_tokens),0), COALESCE(SUM(billable_tokens),0), COALESCE(SUM(cost_usd),0)
			FROM usage_events WHERE %s GROUP BY %s ORDER BY SUM(billable_tokens) DESC`, id, provider, filter, group), args...)
		if queryErr != nil {
			return nil, queryErr
		}
		defer rows.Close()
		out := []UsageDimension{}
		for rows.Next() {
			var value UsageDimension
			if scanErr := rows.Scan(&value.ID, &value.ProviderID, &value.Requests, &value.InputTokens, &value.CachedTokens, &value.OutputTokens, &value.ReasoningTokens, &value.BillableTokens, &value.CostUSD); scanErr != nil {
				return nil, scanErr
			}
			if value.ID == "" {
				value.ID = "unknown"
			}
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
