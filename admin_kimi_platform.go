package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// serveKimiPlatformAdmin routes Kimi Platform admin requests (auth already checked by router)
func (h *proxyHandler) serveKimiPlatformAdmin(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/kimi-platform")
	if path == "" {
		path = "/"
	}

	switch {
	case path == "/" && r.Method == http.MethodGet:
		h.handleAPIKeyList(w, AccountTypeKimiPlatform)

	case path == "/add" && r.Method == http.MethodPost:
		h.handleKimiPlatformAdd(w, r)

	case strings.HasSuffix(path, "/remove") && r.Method == http.MethodPost:
		id := strings.TrimPrefix(path, "/")
		id = strings.TrimSuffix(id, "/remove")
		h.handleAPIKeyRemove(w, AccountTypeKimiPlatform, id)

	default:
		http.NotFound(w, r)
	}
}

// POST /admin/kimi-platform/add - add a Kimi/Moonshot Open Platform API key
func (h *proxyHandler) handleKimiPlatformAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey string `json:"api_key"`
	}

	if r.Header.Get("Content-Type") == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSONError(w, http.StatusBadRequest, "invalid json: "+err.Error())
			return
		}
	} else {
		req.APIKey = r.FormValue("api_key")
	}

	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		respondJSONError(w, http.StatusBadRequest, "api_key is required")
		return
	}

	// Validate by calling GET /v1/models on the OpenAI-compatible endpoint.
	// Per the official Moonshot/Kimi Open Platform docs, this endpoint
	// authenticates via Bearer token and returns the list of available
	// models. A 200 confirms the key is valid; 401/403 means it is not.
	// This avoids depending on a specific model ID for validation, which
	// would conflate product type (Coding Plan vs Open Platform) with
	// credential validity.
	//
	// The OpenAI-compatible base is derived from the active declarative
	// provider specification's Anthropic-compatible base by taking only
	// scheme+host (stripping the "/anthropic" path). This construction uses
	// url.JoinPath for safe path joining rather than string concatenation.
	if h == nil || h.registry == nil {
		respondJSONError(w, http.StatusServiceUnavailable, "Kimi Platform provider is unavailable")
		return
	}
	provider := h.registry.ForType(AccountTypeKimiPlatform)
	if provider == nil || provider.UpstreamURL("") == nil {
		respondJSONError(w, http.StatusServiceUnavailable, "Kimi Platform provider is unavailable")
		return
	}
	base := provider.UpstreamURL("")
	openAIBase := &url.URL{Scheme: base.Scheme, Host: base.Host}
	validationURL, err := url.JoinPath(openAIBase.String(), "/v1/models")
	if err != nil {
		respondJSONError(w, http.StatusInternalServerError, "failed to build validation URL: "+err.Error())
		return
	}
	validReq, _ := http.NewRequest(http.MethodGet, validationURL, nil)
	validReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := h.transport.RoundTrip(validReq)
	if err != nil {
		respondJSONError(w, http.StatusBadGateway, "failed to validate key: "+err.Error())
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	// 401/403 can mean:
	//   - The API key is genuinely invalid/revoked.
	//   - The key is hitting the wrong endpoint (e.g. an Anthropic-compatible
	//     URL that doesn't serve /v1/models). The upstream error body
	//     is included so operators can distinguish "invalid key" from
	//     "wrong endpoint/method."
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		summary := truncateString(string(respBody), 200)
		respondJSONError(w, http.StatusBadRequest, "API key validation rejected (status "+resp.Status+"): "+summary)
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		summary := truncateString(string(respBody), 200)
		respondJSONError(w, http.StatusBadGateway, fmt.Sprintf("key validation returned status %d: %s", resp.StatusCode, summary))
		return
	}

	h.saveAPIKeyAccountFile(w, AccountTypeKimiPlatform, "kimi-platform", apiKey)
}

func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
