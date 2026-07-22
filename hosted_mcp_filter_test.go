package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHostedMCPRequestFilterPreservesLocalAndSearchTools(t *testing.T) {
	input := []byte(`{"input":[{"type":"message","role":"user"},{"type":"mcp_approval_response","approval_request_id":"secret"}],"tools":[{"type":"mcp","server_url":"https://private.example/mcp"},{"type":"web_search"},{"type":"tool_search"},{"type":"function","name":"local_mcp_tool"}],"tool_choice":{"type":"mcp","name":"secret"}}`)
	filtered, changed, err := filterHostedMCPRequestJSON(input, int64(len(input)))
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	text := string(filtered)
	for _, forbidden := range []string{`"type":"mcp"`, "private.example", "approval_request_id", "tool_choice"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("retained %q in %s", forbidden, text)
		}
	}
	for _, required := range []string{"web_search", "tool_search", "local_mcp_tool"} {
		if !strings.Contains(text, required) {
			t.Errorf("removed %q from %s", required, text)
		}
	}
}

func TestHostedMCPResponseFilterDropsEventsAndStripsCompletedOutput(t *testing.T) {
	mcpEvent := []byte(`{"type":"response.output_item.done","item":{"type":"mcp_call","output":"secret"}}`)
	_, drop, changed, err := filterHostedMCPResponseJSON(mcpEvent, 1024)
	if err != nil || !drop || !changed {
		t.Fatalf("drop=%v changed=%v err=%v", drop, changed, err)
	}
	completed := []byte(`{"type":"response.completed","response":{"output":[{"type":"mcp_call","output":"secret"},{"type":"web_search_call"},{"type":"message","content":[]}]}}`)
	filtered, drop, changed, err := filterHostedMCPResponseJSON(completed, 1024)
	if err != nil || drop || !changed {
		t.Fatalf("drop=%v changed=%v err=%v", drop, changed, err)
	}
	if strings.Contains(string(filtered), "mcp_call") || strings.Contains(string(filtered), "secret") {
		t.Fatalf("leaked MCP output: %s", filtered)
	}
	if !strings.Contains(string(filtered), "web_search_call") {
		t.Fatalf("removed safe output: %s", filtered)
	}
}

func TestHostedMCPFiltersRejectOversizedTransformations(t *testing.T) {
	request := []byte(`{"tools":[{"type":"mcp"}]}`)
	if _, _, err := filterHostedMCPRequestJSON(request, int64(len(request)-1)); err == nil {
		t.Fatal("oversized request accepted")
	}
	response := []byte(`{"output":[{"type":"mcp_call"}]}`)
	if _, _, _, err := filterHostedMCPResponseJSON(response, int64(len(response)-1)); err == nil {
		t.Fatal("oversized response accepted")
	}
}

func TestHostedMCPFilterLeavesMalformedAndUnrelatedJSONUnchanged(t *testing.T) {
	for _, input := range [][]byte{[]byte(`not-json`), []byte(`{"tools":[{"type":"function","name":"mcp_local"}]}`)} {
		filtered, changed, err := filterHostedMCPRequestJSON(input, 1024)
		if err != nil || changed || string(filtered) != string(input) {
			t.Fatalf("input=%s filtered=%s changed=%v err=%v", input, filtered, changed, err)
		}
	}
	var decoded map[string]any
	filtered, _, changed, err := filterHostedMCPResponseJSON([]byte(`{"type":"response.completed","response":{"output":[]}}`), 1024)
	if err != nil || changed || json.Unmarshal(filtered, &decoded) != nil {
		t.Fatalf("filtered=%s changed=%v err=%v", filtered, changed, err)
	}
}
