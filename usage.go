package main

import (
	"encoding/json"
	"log"
	"strings"
	"time"
)

// clampNonNegative ensures a value is never negative.
// This prevents issues where CachedInputTokens > InputTokens produces negative billable tokens.
func clampNonNegative(n int64) int64 {
	if n < 0 {
		return 0
	}
	return n
}

// splitUsageAccumulator merges Anthropic's split streaming usage events into
// one request record. message_start carries input/cache usage while
// message_delta carries final output usage.
type splitUsageAccumulator struct {
	pending *RequestUsage
}

func (a *splitUsageAccumulator) add(eventType string, usage *RequestUsage) *RequestUsage {
	if usage == nil {
		return nil
	}
	switch eventType {
	case "message_start":
		a.pending = usage
		return nil
	case "message_delta":
		if a.pending == nil {
			return usage
		}
		merged := a.pending
		a.pending = nil
		// Some compatible providers repeat the final input/cache usage in
		// message_delta; prefer that authoritative terminal snapshot when present.
		if usage.InputTokens > 0 {
			merged.InputTokens = usage.InputTokens
			merged.CachedInputTokens = usage.CachedInputTokens
			merged.CacheCreationTokens = usage.CacheCreationTokens
			merged.CacheReadReported = merged.CacheReadReported || usage.CacheReadReported
			merged.CacheCreationReported = merged.CacheCreationReported || usage.CacheCreationReported
		}
		merged.OutputTokens = usage.OutputTokens
		merged.ReasoningTokens = usage.ReasoningTokens
		if merged.Model == "" {
			merged.Model = usage.Model
		}
		merged.BillableTokens = clampNonNegative(
			merged.InputTokens - merged.CachedInputTokens - merged.CacheCreationTokens + merged.OutputTokens,
		)
		return merged
	default:
		return usage
	}
}

func (a *splitUsageAccumulator) flush() *RequestUsage {
	pending := a.pending
	a.pending = nil
	return pending
}

const (
	codexFiveHourWindowMinutes = 5 * 60
	codexWeeklyWindowMinutes   = 7 * 24 * 60
)

type codexUsageWindow struct {
	UsedPercent   float64
	WindowMinutes int
	ResetAt       time.Time
}

type codexUsageSlots struct {
	Primary   *codexUsageWindow
	Secondary *codexUsageWindow
}

type codexWindowKind int

const (
	codexWindowUnknown codexWindowKind = iota
	codexWindowFiveHour
	codexWindowWeekly
)

func parseCodexRateLimitMap(rateLimit map[string]any, now time.Time, source string) (UsageSnapshot, bool) {
	if rateLimit == nil {
		return UsageSnapshot{}, false
	}
	primary := codexUsageWindowFromMap(firstMap(rateLimit, "primary_window", "primary"))
	secondary := codexUsageWindowFromMap(firstMap(rateLimit, "secondary_window", "secondary"))
	return normalizeCodexUsageWindows(primary, secondary, now, source)
}

func firstMap(m map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if value, ok := m[key].(map[string]any); ok {
			return value
		}
	}
	return nil
}

func codexUsageWindowFromMap(m map[string]any) *codexUsageWindow {
	if m == nil {
		return nil
	}
	windowMinutes := int(readFloat64(m, "window_minutes"))
	if windowMinutes == 0 {
		windowMinutes = int(readFloat64(m, "limit_window_minutes"))
	}
	if windowMinutes == 0 {
		windowSeconds := readFloat64(m, "limit_window_seconds")
		if windowSeconds == 0 {
			windowSeconds = readFloat64(m, "window_seconds")
		}
		if windowSeconds > 0 {
			windowMinutes = int(windowSeconds / 60)
		}
	}

	window := &codexUsageWindow{
		UsedPercent:   readFloat64(m, "used_percent") / 100.0,
		WindowMinutes: windowMinutes,
	}
	if resetAt := int64(readFloat64(m, "reset_at")); resetAt > 0 {
		window.ResetAt = time.Unix(resetAt, 0)
	}
	return window
}

func normalizeCodexUsageWindows(primary, secondary *codexUsageWindow, now time.Time, source string) (UsageSnapshot, bool) {
	if primary == nil && secondary == nil {
		return UsageSnapshot{}, false
	}

	snap := UsageSnapshot{
		RetrievedAt:  now,
		Source:       source,
		primarySet:   true,
		secondarySet: true,
	}
	primaryFallback := codexWindowUnknown
	secondaryFallback := codexWindowUnknown
	if primary != nil && secondary != nil {
		// When both legacy slots are present their ordering is unambiguous even if
		// an older event omitted durations.
		primaryFallback = codexWindowFiveHour
		secondaryFallback = codexWindowWeekly
	}
	assigned := assignCodexUsageWindow(&snap, primary, primaryFallback)
	assigned = assignCodexUsageWindow(&snap, secondary, secondaryFallback) || assigned
	if !assigned {
		return UsageSnapshot{}, false
	}
	return snap, true
}

func assignCodexUsageWindow(snap *UsageSnapshot, window *codexUsageWindow, fallback codexWindowKind) bool {
	if snap == nil || window == nil {
		return false
	}
	kind := classifyCodexUsageWindow(window.WindowMinutes)
	if kind == codexWindowUnknown {
		kind = fallback
	}
	switch kind {
	case codexWindowFiveHour:
		snap.PrimaryUsed = window.UsedPercent
		snap.PrimaryUsedPercent = window.UsedPercent
		snap.PrimaryWindowMinutes = window.WindowMinutes
		snap.PrimaryResetAt = window.ResetAt
		return true
	case codexWindowWeekly:
		snap.SecondaryUsed = window.UsedPercent
		snap.SecondaryUsedPercent = window.UsedPercent
		snap.SecondaryWindowMinutes = window.WindowMinutes
		snap.SecondaryResetAt = window.ResetAt
		return true
	default:
		return false
	}
}

func classifyCodexUsageWindow(windowMinutes int) codexWindowKind {
	switch {
	case windowMinutes >= 4*60 && windowMinutes <= 6*60:
		return codexWindowFiveHour
	case windowMinutes >= 6*24*60 && windowMinutes <= 8*24*60:
		return codexWindowWeekly
	default:
		return codexWindowUnknown
	}
}

func codexUsageSlotsFromSnapshot(snap UsageSnapshot) codexUsageSlots {
	var windows []codexUsageWindow
	if usagePrimaryWindowAvailable(snap) {
		minutes := snap.PrimaryWindowMinutes
		if minutes == 0 {
			minutes = codexFiveHourWindowMinutes
		}
		windows = append(windows, codexUsageWindow{
			UsedPercent:   usagePrimaryUsed(snap),
			WindowMinutes: minutes,
			ResetAt:       snap.PrimaryResetAt,
		})
	}
	if usageSecondaryWindowAvailable(snap) {
		minutes := snap.SecondaryWindowMinutes
		if minutes == 0 {
			minutes = codexWeeklyWindowMinutes
		}
		windows = append(windows, codexUsageWindow{
			UsedPercent:   usageSecondaryUsed(snap),
			WindowMinutes: minutes,
			ResetAt:       snap.SecondaryResetAt,
		})
	}

	var slots codexUsageSlots
	if len(windows) > 0 {
		slots.Primary = &windows[0]
	}
	if len(windows) > 1 {
		slots.Secondary = &windows[1]
	}
	return slots
}

func usagePrimaryUsed(snap UsageSnapshot) float64 {
	if snap.PrimaryUsedPercent != 0 {
		return snap.PrimaryUsedPercent
	}
	return snap.PrimaryUsed
}

func usageSecondaryUsed(snap UsageSnapshot) float64 {
	if snap.SecondaryUsedPercent != 0 {
		return snap.SecondaryUsedPercent
	}
	return snap.SecondaryUsed
}

func usagePrimaryWindowAvailable(snap UsageSnapshot) bool {
	return snap.PrimaryWindowMinutes > 0 || !snap.PrimaryResetAt.IsZero() || usagePrimaryUsed(snap) > 0
}

func usageSecondaryWindowAvailable(snap UsageSnapshot) bool {
	return snap.SecondaryWindowMinutes > 0 || !snap.SecondaryResetAt.IsZero() || usageSecondaryUsed(snap) > 0
}

// parseTokenCountEvent extracts usage from Codex token_count SSE events.
// Format: {type: "token_count", info: {last_token_usage: {...}, total_token_usage: {...}}, rate_limits: {...}}
func parseTokenCountEvent(obj map[string]any) *RequestUsage {
	info, ok := obj["info"].(map[string]any)
	if !ok || info == nil {
		return nil
	}

	// Prefer last_token_usage (per-request) over total_token_usage (cumulative)
	var usageMap map[string]any
	if ltu, ok := info["last_token_usage"].(map[string]any); ok {
		usageMap = ltu
	} else if ttu, ok := info["total_token_usage"].(map[string]any); ok {
		usageMap = ttu
	}
	if usageMap == nil {
		return nil
	}

	ru := &RequestUsage{Timestamp: time.Now()}
	ru.InputTokens = readInt64(usageMap, "input_tokens")
	ru.CachedInputTokens = readInt64(usageMap, "cached_input_tokens")
	if _, ok := usageMap["cached_input_tokens"]; ok {
		ru.CacheReadReported = true
	}
	ru.OutputTokens = readInt64(usageMap, "output_tokens")
	ru.ReasoningTokens = readInt64(usageMap, "reasoning_output_tokens")

	// Calculate billable tokens (input - cached + output)
	// Clamp to non-negative since cached can exceed input in some cases
	ru.BillableTokens = clampNonNegative(ru.InputTokens - ru.CachedInputTokens + ru.OutputTokens)

	if ru.InputTokens == 0 && ru.OutputTokens == 0 {
		return nil
	}

	// Normalize rate limit windows by duration. OpenAI can place the weekly
	// window in the upstream primary slot when the five-hour limit is absent.
	if rl, ok := obj["rate_limits"].(map[string]any); ok {
		if snap, ok := parseCodexRateLimitMap(rl, ru.Timestamp, "token_count"); ok {
			ru.PrimaryUsedPct = usagePrimaryUsed(snap)
			ru.SecondaryUsedPct = usageSecondaryUsed(snap)
		}
	}

	// Extract model from info or top-level
	if m, ok := info["model"].(string); ok && m != "" {
		ru.Model = m
	} else if m, ok := obj["model"].(string); ok && m != "" {
		ru.Model = m
	}

	return ru
}

func (h *proxyHandler) recordUsageForRequest(a *ProviderConnection, ru RequestUsage, requestID string) {
	if ru.RequestID == "" {
		ru.RequestID = requestID
	}
	h.recordUsage(a, ru)
}

func (h *proxyHandler) recordUsage(a *ProviderConnection, ru RequestUsage) {
	if a == nil {
		return
	}
	ru = ru.canonicalIdentity()
	a.mu.Lock()
	if ru.PrimaryResetAt.IsZero() {
		ru.PrimaryResetAt = a.Usage.PrimaryResetAt
	}
	if ru.SecondaryResetAt.IsZero() {
		ru.SecondaryResetAt = a.Usage.SecondaryResetAt
	}
	if ru.PrimaryWindowMinutes == 0 {
		ru.PrimaryWindowMinutes = a.Usage.PrimaryWindowMinutes
	}
	if ru.SecondaryWindowMinutes == 0 {
		ru.SecondaryWindowMinutes = a.Usage.SecondaryWindowMinutes
	}
	a.mu.Unlock()

	var costUSD float64
	if h.pricing != nil && ru.Model != "" {
		costUSD = h.pricing.calculateCost(ru)
	}

	// SQLite is the authoritative event boundary. No projection changes until
	// this immutable event commits, and duplicate identities stop here.
	if h.analyticsStore != nil {
		if strings.TrimSpace(ru.RequestID) == "" {
			log.Printf("canonical usage rejected without request identity account=%s", ru.AccountID)
			if h.recent != nil {
				h.recent.add("canonical usage rejected without request identity")
			}
			return
		}
		recorded, err := h.analyticsStore.recordUsageEvent(usageEventFromRequest(ru, costUSD))
		if err != nil {
			log.Printf("canonical usage persistence failed request=%s account=%s: %v", ru.RequestID, ru.AccountID, err)
			if h.recent != nil {
				h.recent.add("canonical usage persistence failed: " + err.Error())
			}
			return
		}
		if !recorded {
			return
		}
	}

	// BoltDB remains a compatibility projection during migration. If SQLite is
	// unavailable it temporarily retains its old authority for local-mode tests.
	if h.store != nil {
		recorded, err := h.store.recordIfNew(ru)
		if err != nil {
			log.Printf("legacy usage projection failed request=%s account=%s: %v", ru.RequestID, ru.AccountID, err)
			if h.recent != nil {
				h.recent.add("legacy usage projection failed: " + err.Error())
			}
			if h.analyticsStore == nil {
				return
			}
		} else if !recorded && h.analyticsStore == nil {
			return
		}
	}
	a.applyRequestUsage(ru)
	if costUSD > 0 {
		a.mu.Lock()
		a.Totals.TotalCostEstimate += costUSD
		a.mu.Unlock()
	}

}

func parseRequestUsage(obj map[string]any) *RequestUsage {
	return sampledResponsesEngine.ParseUsage(obj)
}

func readInt64(m map[string]any, key string) int64 {
	if v, ok := m[key]; ok {
		switch t := v.(type) {
		case float64:
			return int64(t)
		case int64:
			return t
		case int:
			return int64(t)
		case json.Number:
			if n, err := t.Int64(); err == nil {
				return n
			}
		}
	}
	return 0
}

func readFloat64(m map[string]any, key string) float64 {
	if v, ok := m[key]; ok {
		switch t := v.(type) {
		case float64:
			return t
		case int64:
			return float64(t)
		case int:
			return float64(t)
		case json.Number:
			if f, err := t.Float64(); err == nil {
				return f
			}
		}
	}
	return 0
}
