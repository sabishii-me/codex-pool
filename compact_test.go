package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestDetectRequestFormatMatrix(t *testing.T) {
	cases := []struct {
		path string
		want RequestFormat
	}{
		{"/v1/messages", FormatClaude},
		{"/v1/messages/compact", FormatClaude},
		{"/v1/messages?beta=true", FormatClaude},
		{"/v1/chat/completions", FormatOpenAI},
		{"/v1/completions", FormatOpenAI},
		{"/v1/responses", FormatUnknown},
		{"/v1/responses/compact", FormatUnknown},
	}
	for _, tc := range cases {
		if got := detectRequestFormat(tc.path); got != tc.want {
			t.Errorf("detectRequestFormat(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestCompactBodyTranslationMatrix(t *testing.T) {
	models := []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5", "gpt-5.4"}
	for _, m := range models {
		body := []byte(`{"model":"` + m + `","messages":[{"role":"user","content":"hi"}],"max_tokens":100}`)
		translated, err := translateClaudeToResponsesRequest(body)
		if err != nil {
			t.Fatalf("[%s] translateClaudeToResponsesRequest: %v", m, err)
		}
		final := ensureCodexResponsesCompactBody(translated)
		var obj map[string]any
		if err := json.Unmarshal(final, &obj); err != nil {
			t.Fatalf("[%s] final compact body invalid JSON: %v", m, err)
		}
		if _, ok := obj["input"]; !ok {
			t.Fatalf("[%s] compact body missing input: %s", m, final)
		}
		if _, ok := obj["model"]; !ok {
			t.Fatalf("[%s] compact body missing model: %s", m, final)
		}
		if _, ok := obj["stream"]; ok {
			t.Fatalf("[%s] compact body should not force stream: %s", m, final)
		}
	}
}

func TestNormalClaudeToResponsesStillWorks(t *testing.T) {
	// Regression: normal /v1/messages must still translate to responses input.
	body := []byte(`{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"hi"}],"max_tokens":50}`)
	translated, err := translateClaudeToResponsesRequest(body)
	if err != nil {
		t.Fatalf("translateClaudeToResponsesRequest: %v", err)
	}
	if !bytes.Contains(translated, []byte(`"input"`)) {
		t.Fatalf("normal messages translation missing input: %s", translated)
	}
	// It must NOT look like a compact-only shape (no compact-specific markers).
	if bytes.Contains(translated, []byte("compact")) {
		t.Fatalf("normal translation should not contain compact markers: %s", translated)
	}
}

func TestCompactPathRoutingHelper(t *testing.T) {
	// The codexPassthroughRewrite compact branch must map messages/compact to
	// the responses compact endpoint while keeping a translated body.
	path, body, err := codexPassthroughRewrite("/v1/messages/compact", []byte(`{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatalf("codexPassthroughRewrite(compact): %v", err)
	}
	if path != "/v1/responses/compact" {
		t.Fatalf("rewritten path = %q, want /v1/responses/compact", path)
	}
	if !bytes.Contains(body, []byte(`"input"`)) {
		t.Fatalf("rewritten compact body missing input: %s", body)
	}
}
