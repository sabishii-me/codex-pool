package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestCodexAnthropicReadToolImageReachesUpstreamUnmodified(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "contract-secret")
	var forwarded map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &forwarded); err != nil {
			t.Errorf("upstream request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_read\",\"model\":\"gpt-5.6-sol\",\"status\":\"completed\",\"usage\":{\"input_tokens\":20,\"output_tokens\":1}}}\n\n"))
	}))
	defer upstream.Close()
	base, _ := url.Parse(upstream.URL)
	connection := &ProviderConnection{Type: AccountTypeCodex, ID: "codex_read_image", AccessToken: "contract-key", PlanType: "plus"}
	handler := &proxyHandler{
		cfg:       &config{requestTimeout: 10 * time.Second, streamTimeout: 10 * time.Second, maxInMemoryBodyBytes: 1024 * 1024},
		transport: http.DefaultTransport, pool: newProviderPool([]*ProviderConnection{connection}),
		registry: NewProviderRegistry(NewCodexProvider(base, base, base), NewGeminiProvider(base, base)),
		metrics:  newMetrics(), recent: newRecentErrors(5), aliases: newModelAliases(nil),
	}
	body := `{"model":"gpt-5.6-sol","max_tokens":64,"stream":false,"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"read_1","name":"Read","input":{"path":"screen.png"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"read_1","content":[{"type":"text","text":"Image dimensions: 2x2"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aW1hZ2U="}}]}]}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("contract-secret", "contract-user"))
	response := httptest.NewRecorder()
	handler.proxyRequest(response, request, "read-image-contract")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	input := forwarded["input"].([]any)
	result := input[1].(map[string]any)
	output := result["output"].([]any)
	image := output[1].(map[string]any)
	if image["type"] != "input_image" || image["image_url"] != "data:image/png;base64,aW1hZ2U=" {
		t.Fatalf("forwarded image=%#v", image)
	}
	if forwarded["instructions"] != nil {
		t.Fatalf("gateway injected instructions=%#v", forwarded["instructions"])
	}
}
