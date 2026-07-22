package main

import (
	"encoding/json"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	geminiRetryInPattern = regexp.MustCompile(`(?i)please retry in\s+([0-9]+(?:\.[0-9]+)?)s`)
	zaiResetAtPattern    = regexp.MustCompile(`(?i)usage limit reached[^]]*reset at\s+([0-9]{4}-[0-9]{2}-[0-9]{2}\s+[0-9]{2}:[0-9]{2}:[0-9]{2})`)
)

func parseZAIRateLimitReset(body []byte, now time.Time) (time.Time, bool) {
	match := zaiResetAtPattern.FindSubmatch(body)
	if len(match) != 2 {
		return time.Time{}, false
	}
	// Z.ai emits this timestamp in China Standard Time (UTC+8), as confirmed
	// by the timestamp prefix in the same provider request ID. A fixed zone
	// avoids depending on host/container timezone databases.
	location := time.FixedZone("ZAI-CST", 8*60*60)
	resetAt, err := time.ParseInLocation("2006-01-02 15:04:05", string(match[1]), location)
	if err != nil || !resetAt.After(now) {
		return time.Time{}, false
	}
	return resetAt, true
}

func parseGeminiRateLimitReset(body []byte, now time.Time) (time.Time, bool) {
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Details []struct {
				Metadata map[string]any `json:"metadata"`
			} `json:"details"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &payload)

	message := strings.ToLower(payload.Error.Message)
	if strings.Contains(message, "per day") || strings.Contains(message, "requests per day") {
		if reset, ok := nextGeminiPacificMidnight(now); ok {
			return reset, true
		}
	}

	for _, detail := range payload.Error.Details {
		raw, _ := detail.Metadata["quotaResetDelay"].(string)
		if duration, err := time.ParseDuration(strings.TrimSpace(raw)); err == nil && duration > 0 {
			return now.Add(time.Duration(math.Ceil(duration.Seconds())) * time.Second), true
		}
	}

	match := geminiRetryInPattern.FindStringSubmatch(string(body))
	if len(match) == 2 {
		seconds, err := strconv.ParseFloat(match[1], 64)
		if err == nil && seconds > 0 {
			return now.Add(time.Duration(math.Ceil(seconds)) * time.Second), true
		}
	}
	return time.Time{}, false
}

func nextGeminiPacificMidnight(now time.Time) (time.Time, bool) {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return time.Time{}, false
	}
	local := now.In(location)
	return time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, location), true
}

func (h *proxyHandler) applyRateLimitResponse(a *ProviderConnection, headers http.Header, body []byte) time.Duration {
	if a == nil {
		return 0
	}
	now := time.Now()
	var resetAt time.Time
	var ok bool
	switch a.Type {
	case AccountTypeGemini:
		resetAt, ok = parseGeminiRateLimitReset(body, now)
	case AccountTypeZAI:
		resetAt, ok = parseZAIRateLimitReset(body, now)
	default:
		return h.applyRateLimit(a, headers)
	}
	if !ok {
		return h.applyRateLimit(a, headers)
	}
	wait := time.Until(resetAt)
	if wait <= 0 {
		return h.applyRateLimit(a, headers)
	}
	seconds := int64(math.Ceil(wait.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	cloned := make(http.Header)
	if headers != nil {
		cloned = headers.Clone()
	}
	cloned.Set("Retry-After", strconv.FormatInt(seconds, 10))
	return h.applyRateLimit(a, cloned)
}
