package main

import (
	"net/http"
	"time"
)

// redeemCodexResetCredit consumes the earliest-expiring credit owned by the
// selected Codex connection. The browser never supplies a provider credit ID.
func (h *proxyHandler) redeemCodexResetCredit(w http.ResponseWriter, r *http.Request, connectionID string) {
	if h == nil || h.cfg == nil || h.cfg.disableRefresh {
		respondJSONError(w, http.StatusConflict, "this gateway is not the provider-state authority; use the official Codex dashboard")
		return
	}
	var connection *ProviderConnection
	for _, candidate := range h.pool.allAccounts() {
		if candidate != nil && candidate.ID == connectionID {
			connection = candidate
			break
		}
	}
	if connection == nil {
		respondJSONError(w, http.StatusNotFound, "provider connection not found")
		return
	}

	now := time.Now()
	connection.mu.Lock()
	if connection.Type != AccountTypeCodex {
		connection.mu.Unlock()
		respondJSONError(w, http.StatusConflict, "reset credits are available only for Codex connections")
		return
	}
	if connection.ResetCreditRedeeming {
		connection.mu.Unlock()
		respondJSONError(w, http.StatusConflict, "a reset credit redemption is already in progress")
		return
	}
	var selected *RateLimitResetCredit
	for _, credit := range connection.RateLimitResetCredits {
		if credit.ID == "" || !credit.ExpiresAt.After(now) {
			continue
		}
		if selected == nil || credit.ExpiresAt.Before(selected.ExpiresAt) {
			copy := credit
			selected = &copy
		}
	}
	if selected == nil {
		connection.mu.Unlock()
		respondJSONError(w, http.StatusConflict, "no available Codex reset credit")
		return
	}
	connection.ResetCreditRedeeming = true
	connection.mu.Unlock()
	defer func() {
		connection.mu.Lock()
		connection.ResetCreditRedeeming = false
		connection.mu.Unlock()
	}()

	code, windowsReset, err := h.consumeCodexResetCredit(connection, *selected)
	if err != nil {
		respondJSONError(w, http.StatusBadGateway, "provider reset-credit redemption failed")
		return
	}
	creditsErr := h.fetchCodexResetCredits(connection)
	usageErr := h.fetchUsage(time.Now(), connection)
	if creditsErr != nil || usageErr != nil {
		respondJSON(w, map[string]any{
			"status": "redeemed", "code": code, "windows_reset": windowsReset,
			"refresh_complete": false, "dashboard_url": "https://chatgpt.com/",
		})
		return
	}
	respondJSON(w, map[string]any{
		"status": "redeemed", "code": code, "windows_reset": windowsReset,
		"refresh_complete": true, "dashboard_url": "https://chatgpt.com/",
	})
}
