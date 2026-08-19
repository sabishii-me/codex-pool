package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTranslateClaudeRespToOpenAISurfacesCache(t *testing.T) {
	// DeepSeek (anthropic-messages upstream) reports cache reads; Harness
	// receives OpenAI chat. The translated usage must include
	// prompt_tokens_details.cached_tokens so the downstream sees a real hit.
	claudeBody := []byte(`{"id":"msg_1","type":"message","role":"assistant","model":"deepseek-v4-pro","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":100,"cache_read_input_tokens":80,"output_tokens":20}}`)
	out, err := translateClaudeRespToOpenAI(claudeBody)
	if err != nil {
		t.Fatal(err)
	}
	var oai map[string]any
	if err := json.Unmarshal(out, &oai); err != nil {
		t.Fatal(err)
	}
	usage, _ := oai["usage"].(map[string]any)
	if usage == nil {
		t.Fatalf("no usage: %s", out)
	}
	details, _ := usage["prompt_tokens_details"].(map[string]any)
	if details == nil {
		t.Fatalf("prompt_tokens_details missing: %#v", usage)
	}
	if cached, ok := details["cached_tokens"]; !ok || cached != float64(80) {
		t.Fatalf("cached_tokens=%v ok=%v, want 80", cached, ok)
	}
}

func TestTranslateClaudeRespToOpenAIOmitsCacheWhenUnreported(t *testing.T) {
	// Upstream did NOT report cache: the OpenAI-chat response must NOT include
	// prompt_tokens_details (absent = unavailable, not zero).
	claudeBody := []byte(`{"id":"msg_1","type":"message","role":"assistant","model":"deepseek-v4-pro","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":100,"output_tokens":20}}`)
	out, err := translateClaudeRespToOpenAI(claudeBody)
	if err != nil {
		t.Fatal(err)
	}
	var oai map[string]any
	if err := json.Unmarshal(out, &oai); err != nil {
		t.Fatal(err)
	}
	usage, _ := oai["usage"].(map[string]any)
	if usage == nil {
		t.Fatalf("no usage: %s", out)
	}
	if _, ok := usage["prompt_tokens_details"]; ok {
		t.Fatalf("prompt_tokens_details must be omitted when unreported: %#v", usage)
	}
}

func TestClaudeToOAISSEWriterSurfacesCache(t *testing.T) {
	// Feed a Claude message_start with cache_read_input_tokens, then a
	// message_delta; the emitted OpenAI chat final chunk must carry
	// prompt_tokens_details.cached_tokens.
	start := []byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m1\",\"model\":\"deepseek-v4-pro\",\"usage\":{\"input_tokens\":100,\"cache_read_input_tokens\":80}}}\n\n")
	delta := []byte("event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":20}}\n\n")

	buf := &capturingWriter{}
	sw2 := &sseTranslateWriter{w: buf, direction: TranslateClaudeToOAI}
	sw2.Write(start)
	sw2.Write(delta)
	out := string(buf.data)
	if !jsonContainsKey(out, "prompt_tokens_details") {
		t.Fatalf("final chunk missing prompt_tokens_details: %q", out)
	}
	if !jsonContainsKeyValue(out, "cached_tokens", "80") {
		t.Fatalf("final chunk missing cached_tokens=80: %q", out)
	}
}

func TestClaudeToOAISSEWriterOmitsCacheWhenUnreported(t *testing.T) {
	// Upstream did NOT report cache_read_input_tokens: the OpenAI-chat final
	// chunk must NOT include prompt_tokens_details (absent = unavailable).
	start := []byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m1\",\"model\":\"deepseek-v4-pro\",\"usage\":{\"input_tokens\":100}}}\n\n")
	delta := []byte("event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":20}}\n\n")
	buf := &capturingWriter{}
	sw2 := &sseTranslateWriter{w: buf, direction: TranslateClaudeToOAI}
	sw2.Write(start)
	sw2.Write(delta)
	out := string(buf.data)
	if jsonContainsKey(out, "prompt_tokens_details") {
		t.Fatalf("prompt_tokens_details must be omitted when unreported: %q", out)
	}
}

type capturingWriter struct{ data []byte }

func (c *capturingWriter) Write(p []byte) (int, error) {
	c.data = append(c.data, p...)
	return len(p), nil
}

func jsonContainsKey(s, key string) bool {
	return jsonContainsKeyInData(s, key, "")
}

func jsonContainsKeyValue(s, key, value string) bool {
	return jsonContainsKeyInData(s, key, value)
}

func jsonContainsKeyInData(s, key, value string) bool {
	for _, line := range splitSSEDataLines(s) {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) != nil {
			continue
		}
		if value == "" {
			if containsKeyRecursive(m, key) {
				return true
			}
		} else if containsKeyValueRecursive(m, key, value) {
			return true
		}
	}
	return false
}

func splitSSEDataLines(s string) []string {
	var lines []string
	for _, part := range splitOn(s, "\n\ndata: ") {
		for _, p := range splitOn(part, "data: ") {
			if json.Valid([]byte(strings.TrimSpace(p))) {
				lines = append(lines, strings.TrimSpace(p))
			}
		}
	}
	return lines
}

func splitOn(s, sep string) []string {
	var out []string
	for {
		idx := strings.Index(s, sep)
		if idx < 0 {
			out = append(out, s)
			return out
		}
		out = append(out, s[:idx])
		s = s[idx+len(sep):]
	}
}

func containsKeyRecursive(m map[string]any, key string) bool {
	for k, v := range m {
		if k == key {
			return true
		}
		if sub, ok := v.(map[string]any); ok && containsKeyRecursive(sub, key) {
			return true
		}
	}
	return false
}

func containsKeyValueRecursive(m map[string]any, key, value string) bool {
	for k, v := range m {
		if k == key {
			switch tv := v.(type) {
			case float64:
				if jsonNumberEqual(tv, value) {
					return true
				}
			case json.Number:
				if tv.String() == value {
					return true
				}
			}
		}
		if sub, ok := v.(map[string]any); ok && containsKeyValueRecursive(sub, key, value) {
			return true
		}
	}
	return false
}

func jsonNumberEqual(v float64, s string) bool {
	var n float64
	if json.Unmarshal([]byte(s), &n) == nil {
		return v == n
	}
	return false
}
