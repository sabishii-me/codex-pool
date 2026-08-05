package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func (h *proxyHandler) serveBFLAdmin(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/bfl")
	switch {
	case (path == "" || path == "/") && r.Method == http.MethodGet:
		h.handleAPIKeyList(w, AccountTypeBFL)
	case path == "/add" && r.Method == http.MethodPost:
		h.handleBFLAdd(w, r)
	case strings.HasSuffix(path, "/remove") && r.Method == http.MethodPost:
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/remove")
		h.handleAPIKeyRemove(w, AccountTypeBFL, id)
	default:
		http.NotFound(w, r)
	}
}

func (h *proxyHandler) handleBFLAdd(w http.ResponseWriter, r *http.Request) {
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
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, strings.TrimRight(h.cfg.bflBase.String(), "/")+"/v1/credits", nil)
	req.Header.Set("x-key", key)
	req.Header.Set("accept", "application/json")
	resp, err := h.transport.RoundTrip(req)
	if err != nil {
		respondJSONError(w, http.StatusBadGateway, "failed to validate key: "+err.Error())
		return
	}
	defer resp.Body.Close()
	var credits struct {
		Credits *float64 `json:"credits"`
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&credits); err != nil || credits.Credits == nil || *credits.Credits < 0 {
			respondJSONError(w, http.StatusBadGateway, "key validation returned an invalid credits response")
			return
		}
	} else {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		respondJSONError(w, http.StatusBadRequest, "invalid API key (authentication failed)")
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respondJSONError(w, http.StatusBadGateway, fmt.Sprintf("key validation returned status %d", resp.StatusCode))
		return
	}
	for _, account := range h.pool.allAccounts() {
		if account.Type != AccountTypeBFL {
			continue
		}
		account.mu.Lock()
		matches := subtle.ConstantTimeCompare([]byte(account.AccessToken), []byte(key)) == 1
		accountID := account.ID
		account.mu.Unlock()
		if matches {
			account.mu.Lock()
			account.Usage.CreditsBalance, account.Usage.HasCredits, account.Usage.RetrievedAt, account.Usage.Source = *credits.Credits, true, time.Now().UTC(), "bfl_credits"
			account.Usage.creditsSet = true
			account.mu.Unlock()
			restoreValidatedAccount(account, "bfl credits")
			respondJSON(w, map[string]any{"success": true, "account_id": accountID, "existing": true})
			return
		}
	}
	h.saveAPIKeyAccountFile(w, AccountTypeBFL, "bfl", key, map[string]any{"credits_balance": *credits.Credits, "credits_retrieved_at": time.Now().UTC().Format(time.RFC3339Nano)})
}
