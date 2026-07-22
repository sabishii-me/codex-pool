package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestHostedMCPStreamFilterHandlesChunkedEventsAndEOF(t *testing.T) {
	var output bytes.Buffer
	filter := newHostedMCPResponseFilterWriter(&output, 1024)
	stream := strings.Join([]string{
		`event: response.output_item.done`,
		`data: {"type":"response.output_item.done","item":{"type":"mcp_call","output":"secret"}}`, "",
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"output":[{"type":"mcp_call","output":"secret"},{"type":"message","content":[]}]}}`, "",
	}, "\n")
	for _, chunk := range []string{stream[:31], stream[31:93], stream[93:]} {
		if _, err := filter.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if err := filter.Finalize(); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if strings.Contains(text, "mcp_call") || strings.Contains(text, "secret") {
		t.Fatalf("leaked hosted MCP: %s", text)
	}
	if !strings.Contains(text, "response.completed") || !strings.Contains(text, `"type":"message"`) {
		t.Fatalf("missing safe terminal event: %s", text)
	}
}

func TestHostedMCPStreamFilterRejectsOversizedIncompleteEvent(t *testing.T) {
	var output bytes.Buffer
	filter := newHostedMCPResponseFilterWriter(&output, 32)
	if _, err := filter.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"large\"}")); err == nil {
		t.Fatal("oversized event accepted")
	}
	if output.Len() != 0 {
		t.Fatalf("forwarded bytes before validation: %q", output.String())
	}
}

func TestHostedMCPStreamFilterPreservesUnchangedEventBytes(t *testing.T) {
	input := []byte("event: ping\r\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"safe\"}\r\n\r\n")
	var output bytes.Buffer
	filter := newHostedMCPResponseFilterWriter(&output, 1024)
	if _, err := filter.Write(input); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), input) {
		t.Fatalf("bytes changed:\nwant %q\ngot  %q", input, output.Bytes())
	}
}
