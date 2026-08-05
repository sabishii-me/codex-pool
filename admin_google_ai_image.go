package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (h *proxyHandler) handleGoogleAIImageAdd(w http.ResponseWriter, r *http.Request) {
	var input struct {
		APIKey string `json:"api_key"`
	}
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			respondJSONError(w, http.StatusBadRequest, "invalid json: "+err.Error())
			return
		}
	} else {
		input.APIKey = r.FormValue("api_key")
	}
	key := strings.TrimSpace(input.APIKey)
	if key == "" {
		respondJSONError(w, http.StatusBadRequest, "api_key is required")
		return
	}

	validationURL := strings.TrimRight(h.cfg.googleAIImageBase.String(), "/") + "/v1beta/models"
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, validationURL, nil)
	req.Header.Set("x-goog-api-key", key)
	req.Header.Set("accept", "application/json")
	resp, err := h.transport.RoundTrip(req)
	if err != nil {
		respondJSONError(w, http.StatusBadGateway, "failed to validate key: "+err.Error())
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		respondJSONError(w, http.StatusBadRequest, "invalid Google AI Studio API key (authentication failed)")
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respondJSONError(w, http.StatusBadGateway, fmt.Sprintf("key validation returned status %d", resp.StatusCode))
		return
	}
	for _, account := range h.pool.allAccounts() {
		if account.Type != AccountTypeGoogleAIImage {
			continue
		}
		account.mu.Lock()
		matches := subtle.ConstantTimeCompare([]byte(account.AccessToken), []byte(key)) == 1
		accountID := account.ID
		account.mu.Unlock()
		if matches {
			restoreValidatedAccount(account, "Google AI Studio key validation")
			respondJSON(w, map[string]any{"success": true, "account_id": accountID, "existing": true})
			return
		}
	}
	h.saveAPIKeyAccountFile(w, AccountTypeGoogleAIImage, "google-ai-image", key, map[string]any{"type": string(AccountTypeGoogleAIImage), "plan_type": "ai-studio"})
}
