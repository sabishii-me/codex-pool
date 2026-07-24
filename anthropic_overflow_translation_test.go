package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const contextOverflowFixture = `event: response.created
data: {"type":"response.created","response":{"id":"resp_overflow","model":"gpt-5.6-sol","status":"in_progress","usage":null}}

event: response.failed
data: {"type":"response.failed","response":{"id":"resp_overflow","model":"gpt-5.6-sol","status":"failed","error":{"type":"invalid_request_error","code":"context_length_exceeded","message":"Your input exceeds the context window of this model."},"usage":null}}

`

func TestResponsesToClaudeBufferedContextOverflowIsTypedError(t *testing.T) {
	writer := &responsesToClaudeBufferingWriter{}
	for _, part := range []string{contextOverflowFixture[:41], contextOverflowFixture[41:173], contextOverflowFixture[173:]} {
		if _, err := writer.Write([]byte(part)); err != nil {
			t.Fatal(err)
		}
	}
	errorType, message, failed := writer.Failure()
	if !failed || errorType != "invalid_request_error" || !strings.Contains(message, "context window") {
		t.Fatalf("failure=(%q,%q,%v)", errorType, message, failed)
	}
	body := writer.Result()
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if response["type"] != "error" {
		t.Fatalf("response=%s", body)
	}
	if strings.Contains(string(body), `"role":"assistant"`) || strings.Contains(string(body), "[Error:") {
		t.Fatalf("overflow became assistant content: %s", body)
	}
}

func TestResponsesToClaudeStreamingContextOverflowIsErrorOnly(t *testing.T) {
	var output bytes.Buffer
	writer := &responsesToClaudeWriter{w: &output}
	for _, part := range []string{contextOverflowFixture[:23], contextOverflowFixture[23:211], contextOverflowFixture[211:]} {
		if _, err := writer.Write([]byte(part)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Finalize(); err != nil {
		t.Fatal(err)
	}
	wire := output.String()
	if !strings.Contains(wire, "event: error") || !strings.Contains(wire, `"type":"invalid_request_error"`) {
		t.Fatalf("wire=%s", wire)
	}
	for _, forbidden := range []string{"[Error:", "content_block_delta", "message_stop"} {
		if strings.Contains(wire, forbidden) {
			t.Fatalf("stream contains %q:\n%s", forbidden, wire)
		}
	}
}

func TestResponsesToClaudePreservesCompleteUsage(t *testing.T) {
	stream := `event: response.created
data: {"type":"response.created","response":{"id":"resp_usage","model":"gpt-5.6-sol","usage":null}}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"OK"}

event: response.completed
data: {"type":"response.completed","response":{"id":"resp_usage","model":"gpt-5.6-sol","status":"completed","usage":{"input_tokens":250000,"output_tokens":479,"input_tokens_details":{"cached_tokens":200000,"cache_creation_tokens":10000}}}}

`
	buffered := &responsesToClaudeBufferingWriter{}
	_, _ = buffered.Write([]byte(stream))
	var response struct {
		Usage map[string]int64 `json:"usage"`
	}
	if err := json.Unmarshal(buffered.Result(), &response); err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"input_tokens": 40000, "cache_read_input_tokens": 200000, "cache_creation_input_tokens": 10000, "output_tokens": 479}
	for key, value := range want {
		if response.Usage[key] != value {
			t.Fatalf("buffered %s=%d want %d", key, response.Usage[key], value)
		}
	}

	var output bytes.Buffer
	streaming := &responsesToClaudeWriter{w: &output}
	_, _ = streaming.Write([]byte(stream))
	if err := streaming.Finalize(); err != nil {
		t.Fatal(err)
	}
	wire := output.String()
	for _, field := range []string{`"input_tokens":40000`, `"cache_read_input_tokens":200000`, `"cache_creation_input_tokens":10000`, `"output_tokens":479`} {
		if !strings.Contains(wire, field) {
			t.Fatalf("stream missing %s:\n%s", field, wire)
		}
	}
}
