package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const defaultNativeImageTimeout = 2 * time.Minute

type nativeImageUpstreamError struct {
	Provider   string
	Operation  string
	StatusCode int
	Headers    http.Header
}

func (err *nativeImageUpstreamError) Error() string {
	provider := err.Provider
	if provider == "" {
		provider = "BFL"
	}
	return fmt.Sprintf("%s %s failed with status %d", provider, err.Operation, err.StatusCode)
}

type nativeImageJobError struct{ Status string }

func (err *nativeImageJobError) Error() string {
	return "BFL image generation ended with status " + err.Status
}

type nativeImageResult struct {
	Bytes       []byte
	MIME        string
	OperationID string
	Width       int
	Height      int
}

type nativeImageRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

func (h *proxyHandler) handleNativeImageGeneration(w http.ResponseWriter, r *http.Request, body []byte, userID, originID, reqID string) bool {
	var input nativeImageRequest
	if json.Unmarshal(body, &input) != nil {
		return false
	}
	model, ok := resolveNativeModel(input.Model, WorkloadImageGeneration)
	if !ok {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(input.Model)), "bfl/") {
			respondJSONError(w, http.StatusBadRequest, "unknown native BFL image model")
			return true
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(input.Model)), "google-ai-image/") {
			respondJSONError(w, http.StatusBadRequest, "unknown native Google AI image model")
			return true
		}
		return false
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		respondJSONError(w, http.StatusMethodNotAllowed, "native image generation requires POST")
		return true
	}
	if strings.TrimSpace(input.Prompt) == "" {
		respondJSONError(w, http.StatusBadRequest, "prompt is required")
		return true
	}
	if input.N == 0 {
		input.N = 1
	}
	if input.N != 1 {
		respondJSONError(w, http.StatusBadRequest, "native image generation currently requires n=1")
		return true
	}
	if input.ResponseFormat != "" && input.ResponseFormat != "b64_json" && input.ResponseFormat != "url" {
		respondJSONError(w, http.StatusBadRequest, "response_format must be b64_json or url")
		return true
	}
	if _, _, err := nativeImageDimensions(input.Size); err != nil {
		respondJSONError(w, http.StatusBadRequest, err.Error())
		return true
	}
	if model.ProviderID == AccountTypeGoogleAIImage && input.Size != "" && input.Size != "auto" {
		respondJSONError(w, http.StatusBadRequest, "Google AI native image sizing is provider-controlled; size must be auto")
		return true
	}
	connection := h.connectionSelector().Select(ConnectionSelection{ProviderID: model.ProviderID, Model: model.UpstreamID})
	if connection == nil {
		respondJSONError(w, http.StatusServiceUnavailable, "no eligible connection for native image model")
		return true
	}
	provider := h.registry.ForType(model.ProviderID)
	if provider == nil {
		respondJSONError(w, http.StatusServiceUnavailable, "native image provider is unavailable")
		return true
	}
	started := time.Now().UTC()
	timeout := defaultNativeImageTimeout
	if h.cfg != nil && h.cfg.requestTimeout > 0 && h.cfg.requestTimeout < timeout {
		timeout = h.cfg.requestTimeout
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	atomic.AddInt64(&connection.Inflight, 1)
	defer atomic.AddInt64(&connection.Inflight, -1)
	var result nativeImageResult
	var err error
	switch nativeProvider := provider.(type) {
	case *BFLProvider:
		if nativeProvider.base == nil {
			err = errors.New("native image provider is unavailable")
		} else {
			result, err = executeBFLImage(ctx, h.transport, nativeProvider, connection, model, input)
		}
	case *GoogleAIImageProvider:
		if nativeProvider.base == nil {
			err = errors.New("native image provider is unavailable")
		} else {
			result, err = executeGoogleAIImage(ctx, h.transport, nativeProvider, connection, model, input)
		}
	default:
		err = errors.New("native image provider is unavailable")
	}
	if err != nil {
		h.applyNativeImageFailure(connection, err)
		failureClass := nativeImageFailureClass(err)
		if h.metrics != nil {
			h.metrics.inc("image_"+failureClass, connection.ID)
		}
		recordNativeImageUsage(h.analyticsStore, UsageEvent{RequestID: reqID, StartedAt: started, CompletedAt: time.Now().UTC(), UserID: userID, OriginID: originID, ProviderID: model.ProviderID, ConnectionID: connection.ID, ModelID: model.ID, PlanType: connectionPlan(connection), WorkloadKind: WorkloadImageGeneration, Status: "failed", FailureClass: failureClass, EconomicsKnown: false})
		if retryAfter, ok := nativeImageRetryAfter(err); ok {
			w.Header().Set("Retry-After", retryAfter)
		}
		respondJSONError(w, nativeImageErrorStatus(err), err.Error())
		return true
	}
	response := map[string]any{"created": time.Now().Unix(), "data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(result.Bytes)}}}
	if strings.EqualFold(input.ResponseFormat, "url") {
		response = map[string]any{"created": time.Now().Unix(), "data": []any{map[string]any{"url": "data:" + result.MIME + ";base64," + base64.StdEncoding.EncodeToString(result.Bytes)}}}
	}
	if h.metrics != nil {
		h.metrics.inc("image_success", connection.ID)
	}
	if bflProvider, ok := provider.(*BFLProvider); ok {
		h.refreshBFLBalance(ctx, bflProvider, connection)
	}
	if h.analyticsStore != nil {
		recordNativeImageUsage(h.analyticsStore, UsageEvent{RequestID: reqID, StartedAt: started, CompletedAt: time.Now().UTC(), UserID: userID, OriginID: originID, ProviderID: model.ProviderID, ConnectionID: connection.ID, ModelID: model.ID, PlanType: connectionPlan(connection), WorkloadKind: WorkloadImageGeneration, Status: "success", ImageCount: 1, ImageMIME: result.MIME, ImageWidth: result.Width, ImageHeight: result.Height, OperationID: result.OperationID, EconomicsKnown: false})
	}
	respondJSON(w, response)
	return true
}

func (h *proxyHandler) applyNativeImageFailure(connection *ProviderConnection, err error) {
	var upstream *nativeImageUpstreamError
	if !errors.As(err, &upstream) {
		return
	}
	switch upstream.StatusCode {
	case http.StatusPaymentRequired:
		if connection.Type != AccountTypeBFL {
			return
		}
		connection.mu.Lock()
		connection.Usage.CreditsBalance, connection.Usage.HasCredits, connection.Usage.RetrievedAt, connection.Usage.Source = 0, true, time.Now().UTC(), "bfl_credits"
		connection.Usage.creditsSet = true
		connection.mu.Unlock()
		if connection.File != "" {
			_ = saveAccount(connection)
		}
	case http.StatusTooManyRequests:
		h.applyRateLimit(connection, upstream.Headers)
		connection.mu.Lock()
		connection.Penalty += 0.2
		connection.mu.Unlock()
	case http.StatusUnauthorized, http.StatusForbidden:
		applyProxyAuthFailure(connection, false)
	}
}

func (h *proxyHandler) refreshBFLBalance(ctx context.Context, provider *BFLProvider, connection *ProviderConnection) {
	refreshCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	balance, err := provider.FetchCredits(refreshCtx, h.transport, connection)
	if err != nil {
		return
	}
	connection.mu.Lock()
	connection.Usage.CreditsBalance, connection.Usage.HasCredits, connection.Usage.RetrievedAt, connection.Usage.Source = balance, true, time.Now().UTC(), "bfl_credits"
	connection.Usage.creditsSet = true
	connection.mu.Unlock()
	if connection.File != "" {
		_ = saveAccount(connection)
	}
}

func connectionPlan(connection *ProviderConnection) string {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return connection.PlanType
}

func recordNativeImageUsage(store *AnalyticsStore, event UsageEvent) {
	if store != nil {
		_, _ = store.recordUsageEvent(event)
	}
}

func nativeImageDimensions(size string) (int, int, error) {
	if size == "" || size == "auto" {
		return 0, 0, nil
	}
	var width, height int
	if parsed, err := fmt.Sscanf(size, "%dx%d", &width, &height); err != nil || parsed != 2 || width < 64 || height < 64 {
		return 0, 0, errors.New("size must be auto or WIDTHxHEIGHT with each dimension at least 64")
	}
	return width, height, nil
}

func readBoundedBody(body io.Reader, maximum int64, label string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("%s exceeded %d bytes", label, maximum)
	}
	return data, nil
}

func executeBFLImage(ctx context.Context, transport http.RoundTripper, provider *BFLProvider, connection *ProviderConnection, model NativeModelDescriptor, input nativeImageRequest) (nativeImageResult, error) {
	payload := map[string]any{"prompt": input.Prompt, "output_format": "png"}
	width, height, sizeErr := nativeImageDimensions(input.Size)
	if sizeErr != nil {
		return nativeImageResult{}, sizeErr
	}
	if width > 0 {
		payload["width"], payload["height"] = width, height
	}
	raw, _ := json.Marshal(payload)
	submitURL := strings.TrimRight(provider.base.String(), "/") + "/v1/" + url.PathEscape(model.UpstreamID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, submitURL, bytes.NewReader(raw))
	provider.SetAuthHeaders(req, connection)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		return nativeImageResult{}, err
	}
	defer resp.Body.Close()
	responseBody, err := readBoundedBody(resp.Body, 2<<20, "BFL submit response")
	if err != nil {
		return nativeImageResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nativeImageResult{}, &nativeImageUpstreamError{Operation: "submit", StatusCode: resp.StatusCode, Headers: resp.Header.Clone()}
	}
	var submitted struct {
		ID         string `json:"id"`
		PollingURL string `json:"polling_url"`
	}
	if json.Unmarshal(responseBody, &submitted) != nil || submitted.ID == "" || submitted.PollingURL == "" {
		return nativeImageResult{}, errors.New("BFL submit response omitted id or polling_url")
	}
	pollURL, err := url.Parse(submitted.PollingURL)
	if err != nil || !trustedBFLURL(pollURL, provider.base, false) {
		return nativeImageResult{}, errors.New("BFL returned an invalid polling_url")
	}
	for {
		select {
		case <-ctx.Done():
			return nativeImageResult{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		poll, _ := http.NewRequestWithContext(ctx, http.MethodGet, pollURL.String(), nil)
		provider.SetAuthHeaders(poll, connection)
		statusResp, err := transport.RoundTrip(poll)
		if err != nil {
			return nativeImageResult{}, err
		}
		statusBody, readErr := readBoundedBody(statusResp.Body, 2<<20, "BFL polling response")
		statusResp.Body.Close()
		if readErr != nil {
			return nativeImageResult{}, readErr
		}
		if statusResp.StatusCode < 200 || statusResp.StatusCode >= 300 {
			return nativeImageResult{}, &nativeImageUpstreamError{Operation: "polling", StatusCode: statusResp.StatusCode, Headers: statusResp.Header.Clone()}
		}
		var status struct {
			Status string `json:"status"`
			Result struct {
				Sample string `json:"sample"`
			} `json:"result"`
		}
		if json.Unmarshal(statusBody, &status) != nil {
			return nativeImageResult{}, errors.New("invalid BFL polling response")
		}
		switch status.Status {
		case "Ready":
			result, err := downloadBFLImage(ctx, transport, provider.base, status.Result.Sample)
			result.OperationID = submitted.ID
			return result, err
		case "Pending", "Reasoning", "Generating":
			continue
		case "Request Moderated", "Content Moderated", "Task not found", "Error":
			return nativeImageResult{}, &nativeImageJobError{Status: status.Status}
		default:
			return nativeImageResult{}, errors.New("invalid BFL polling status")
		}
	}
}

func trustedBFLURL(candidate, base *url.URL, delivery bool) bool {
	if candidate == nil || base == nil || candidate.User != nil || candidate.Fragment != "" {
		return false
	}
	if candidate.Port() != "" && candidate.Port() != base.Port() {
		return false
	}
	if candidate.Scheme != "https" && !(candidate.Scheme == "http" && candidate.Hostname() == base.Hostname()) {
		return false
	}
	if strings.EqualFold(candidate.Hostname(), base.Hostname()) {
		return true
	}
	host := strings.ToLower(candidate.Hostname())
	if delivery {
		return strings.HasPrefix(host, "delivery.") && strings.HasSuffix(host, ".bfl.ai")
	}
	return host == "api.bfl.ai" || strings.HasSuffix(host, ".api.bfl.ai")
}

func downloadBFLImage(ctx context.Context, transport http.RoundTripper, base *url.URL, rawURL string) (nativeImageResult, error) {
	u, err := url.Parse(rawURL)
	if err != nil || !trustedBFLURL(u, base, true) {
		return nativeImageResult{}, errors.New("BFL returned an invalid image URL")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		return nativeImageResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nativeImageResult{}, fmt.Errorf("BFL image download failed with status %d", resp.StatusCode)
	}
	imageBytes, err := readBoundedBody(resp.Body, 32<<20, "BFL image")
	if err != nil || len(imageBytes) == 0 {
		return nativeImageResult{}, errors.New("BFL returned an empty image")
	}
	mime := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if mime != "image/png" && mime != "image/jpeg" {
		return nativeImageResult{}, fmt.Errorf("unsupported BFL image MIME type %q", mime)
	}
	config, _, decodeErr := image.DecodeConfig(bytes.NewReader(imageBytes))
	if decodeErr != nil {
		return nativeImageResult{}, errors.New("BFL returned invalid image bytes")
	}
	return nativeImageResult{Bytes: imageBytes, MIME: mime, Width: config.Width, Height: config.Height}, nil
}

func nativeImageFailureClass(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "timeout"
	}
	var job *nativeImageJobError
	if errors.As(err, &job) {
		switch job.Status {
		case "Request Moderated", "Content Moderated":
			return "moderation"
		case "Task not found":
			return "operation_not_found"
		default:
			return "provider_error"
		}
	}
	var upstream *nativeImageUpstreamError
	if errors.As(err, &upstream) {
		switch upstream.StatusCode {
		case http.StatusPaymentRequired:
			return "quota_exhausted"
		case http.StatusUnauthorized, http.StatusForbidden:
			return "authentication"
		case http.StatusTooManyRequests:
			return "rate_limit"
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			return "invalid_request"
		default:
			return "provider_error"
		}
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "invalid") || strings.Contains(message, "omitted") || strings.Contains(message, "mime") || strings.Contains(message, "empty image") {
		return "malformed_provider_output"
	}
	return "transport_error"
}

func nativeImageErrorStatus(err error) int {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return http.StatusGatewayTimeout
	}
	var job *nativeImageJobError
	if errors.As(err, &job) {
		switch job.Status {
		case "Request Moderated", "Content Moderated":
			return http.StatusUnprocessableEntity
		case "Task not found":
			return http.StatusBadGateway
		}
	}
	var upstream *nativeImageUpstreamError
	if errors.As(err, &upstream) {
		switch upstream.StatusCode {
		case http.StatusBadRequest, http.StatusPaymentRequired, http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusTooManyRequests:
			return upstream.StatusCode
		}
	}
	return http.StatusBadGateway
}

func nativeImageRetryAfter(err error) (string, bool) {
	var upstream *nativeImageUpstreamError
	if !errors.As(err, &upstream) {
		return "", false
	}
	if wait, ok := parseRetryAfter(upstream.Headers); ok {
		seconds := int64(wait.Seconds())
		if seconds < 1 {
			seconds = 1
		}
		return strconv.FormatInt(seconds, 10), true
	}
	return "", false
}
