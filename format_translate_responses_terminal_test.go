package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestResponsesToClaudeFinalizesUnterminatedCompletedEvent(t *testing.T) {
	created := `event: response.created
data: {"type":"response.created","response":{"id":"resp_terminal","model":"gpt-5.6-sol"}}

`
	delta := `event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"OK"}

`
	// Deliberately omit the final SSE blank line. Codex and intermediaries may
	// close immediately after the final JSON payload.
	completed := `event: response.completed
data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":7,"output_tokens":5}}}`

	var output bytes.Buffer
	writer := &responsesToClaudeWriter{w: &output}
	stream := created + delta + completed
	for _, chunk := range []string{stream[:37], stream[37:119], stream[119:]} {
		if _, err := writer.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(output.String(), "message_stop") {
		t.Fatal("unterminated terminal event was processed before finalization")
	}
	if err := writer.Finalize(); err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(output.String(), "event: message_stop"); count != 1 {
		t.Fatalf("message_stop count=%d\n%s", count, output.String())
	}
	if strings.Contains(output.String(), "event: error") {
		t.Fatalf("successful stream emitted error:\n%s", output.String())
	}
}

func TestResponsesToClaudeMapsIncompleteTerminalEvent(t *testing.T) {
	var output bytes.Buffer
	writer := &responsesToClaudeWriter{w: &output}
	_, _ = writer.Write([]byte(`event: response.incomplete
data: {"type":"response.incomplete","response":{"status":"incomplete","usage":{"output_tokens":9}}}`))
	if err := writer.Finalize(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"stop_reason":"max_tokens"`) || !strings.Contains(output.String(), "event: message_stop") {
		t.Fatalf("incomplete terminal translation:\n%s", output.String())
	}
}

func TestResponsesToClaudeReportsTruncatedStream(t *testing.T) {
	var output bytes.Buffer
	writer := &responsesToClaudeWriter{w: &output}
	_, _ = writer.Write([]byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"))
	if err := writer.Finalize(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "event: error") || !strings.Contains(output.String(), "terminal response event") {
		t.Fatalf("truncated stream did not emit protocol error:\n%s", output.String())
	}
	if strings.Contains(output.String(), "event: message_stop") {
		t.Fatalf("truncated stream was reported as successful:\n%s", output.String())
	}
}

type failingResponseWriter struct{}

func (failingResponseWriter) Write([]byte) (int, error) { return 0, errors.New("downstream closed") }

func TestResponsesToClaudePropagatesDownstreamWriteFailure(t *testing.T) {
	writer := &responsesToClaudeWriter{w: failingResponseWriter{}}
	_, err := writer.Write([]byte("event: response.created\ndata: {\"type\":\"response.created\",\"response\":{}}\n\n"))
	if err == nil || !strings.Contains(err.Error(), "downstream closed") {
		t.Fatalf("write error=%v", err)
	}
}
