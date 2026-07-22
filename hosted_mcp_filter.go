package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const hostedMCPToolType = "mcp"

var hostedMCPItemTypes = map[string]struct{}{
	"mcp_list_tools": {}, "mcp_call": {}, "mcp_approval_request": {}, "mcp_approval_response": {},
}

func isHostedMCPItemType(value any) bool {
	typ, ok := value.(string)
	if !ok {
		return false
	}
	_, ok = hostedMCPItemTypes[strings.TrimSpace(typ)]
	return ok
}

func isCodexResponsesPath(path string) bool {
	return path == "/responses" || path == "/v1/responses" || strings.HasPrefix(path, "/responses/") || strings.HasPrefix(path, "/v1/responses/")
}

func filterHostedMCPHTTPRequest(r *http.Request, data []byte, limit int64) ([]byte, bool, error) {
	decoded := data
	compressed := r != nil && strings.Contains(strings.ToLower(r.Header.Get("Content-Encoding")), "gzip")
	if compressed {
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, false, fmt.Errorf("decode hosted MCP request: %w", err)
		}
		decompressed, readErr := io.ReadAll(io.LimitReader(reader, limit+1))
		closeErr := reader.Close()
		if readErr != nil {
			return nil, false, fmt.Errorf("decode hosted MCP request: %w", readErr)
		}
		if closeErr != nil {
			return nil, false, fmt.Errorf("decode hosted MCP request: %w", closeErr)
		}
		if limit > 0 && int64(len(decompressed)) > limit {
			return nil, false, fmt.Errorf("hosted MCP request transformation exceeds %d bytes", limit)
		}
		decoded = decompressed
	}
	filtered, changed, err := filterHostedMCPRequestJSON(decoded, limit)
	if err != nil || !changed {
		return data, false, err
	}
	if r != nil {
		r.Header.Del("Content-Encoding")
		r.Header.Del("Content-Length")
		r.ContentLength = int64(len(filtered))
	}
	return filtered, true, nil
}

func stripHostedMCPFromResponsesRequest(request map[string]any) bool {
	changed := false
	if tools, ok := request["tools"].([]any); ok {
		kept := make([]any, 0, len(tools))
		for _, raw := range tools {
			tool, _ := raw.(map[string]any)
			if typ, _ := tool["type"].(string); strings.TrimSpace(typ) == hostedMCPToolType {
				changed = true
				continue
			}
			kept = append(kept, raw)
		}
		request["tools"] = kept
	}
	if input, ok := request["input"].([]any); ok {
		kept := make([]any, 0, len(input))
		for _, raw := range input {
			item, _ := raw.(map[string]any)
			if isHostedMCPItemType(item["type"]) {
				changed = true
				continue
			}
			kept = append(kept, raw)
		}
		request["input"] = kept
	}
	if choice, ok := request["tool_choice"].(map[string]any); ok {
		if typ, _ := choice["type"].(string); strings.TrimSpace(typ) == hostedMCPToolType {
			delete(request, "tool_choice")
			changed = true
		}
	} else if choice, ok := request["tool_choice"].(string); ok && strings.TrimSpace(choice) == hostedMCPToolType {
		delete(request, "tool_choice")
		changed = true
	}
	return changed
}

func filterHostedMCPRequestJSON(data []byte, limit int64) ([]byte, bool, error) {
	if limit > 0 && int64(len(data)) > limit {
		return nil, false, fmt.Errorf("hosted MCP request transformation exceeds %d bytes", limit)
	}
	var request map[string]any
	if err := json.Unmarshal(data, &request); err != nil {
		return data, false, nil
	}
	if !stripHostedMCPFromResponsesRequest(request) {
		return data, false, nil
	}
	filtered, err := json.Marshal(request)
	if err != nil {
		return nil, false, fmt.Errorf("encode hosted MCP-filtered request: %w", err)
	}
	return filtered, true, nil
}

func stripHostedMCPItems(items []any) ([]any, bool) {
	kept := make([]any, 0, len(items))
	changed := false
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if isHostedMCPItemType(item["type"]) {
			changed = true
			continue
		}
		kept = append(kept, raw)
	}
	return kept, changed
}

func filterHostedMCPHTTPResponse(response *http.Response, limit int64) error {
	if response == nil || response.Body == nil {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	_ = response.Body.Close()
	if err != nil {
		return err
	}
	if limit > 0 && int64(len(body)) > limit {
		return fmt.Errorf("hosted MCP response transformation exceeds %d bytes", limit)
	}
	filtered, drop, changed, err := filterHostedMCPResponseJSON(body, limit)
	if err != nil {
		return err
	}
	if drop {
		filtered, changed = []byte(`{}`), true
	}
	if !changed {
		filtered = body
	}
	response.Body = io.NopCloser(bytes.NewReader(filtered))
	response.ContentLength = int64(len(filtered))
	response.Header.Set("Content-Length", fmt.Sprintf("%d", len(filtered)))
	return nil
}

func filterHostedMCPResponseJSON(data []byte, limit int64) (filtered []byte, drop, changed bool, err error) {
	if limit > 0 && int64(len(data)) > limit {
		return nil, false, false, fmt.Errorf("hosted MCP response transformation exceeds %d bytes", limit)
	}
	var event map[string]any
	if decodeErr := json.Unmarshal(data, &event); decodeErr != nil {
		return data, false, false, nil
	}
	if eventType, _ := event["type"].(string); strings.HasPrefix(eventType, "response.mcp_") || isHostedMCPItemType(eventType) {
		return nil, true, true, nil
	}
	if item, _ := event["item"].(map[string]any); item != nil && isHostedMCPItemType(item["type"]) {
		return nil, true, true, nil
	}
	stripOutput := func(container map[string]any) {
		if output, ok := container["output"].([]any); ok {
			if kept, removed := stripHostedMCPItems(output); removed {
				container["output"] = kept
				changed = true
			}
		}
	}
	stripOutput(event)
	if response, _ := event["response"].(map[string]any); response != nil {
		stripOutput(response)
	}
	if !changed {
		return data, false, false, nil
	}
	filtered, err = json.Marshal(event)
	if err != nil {
		return nil, true, true, fmt.Errorf("encode hosted MCP-filtered response: %w", err)
	}
	return filtered, false, true, nil
}
