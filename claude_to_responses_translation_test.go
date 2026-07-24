package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClaudeToResponsesUsesCodexCompatibleRequestShape(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.6-luna",
		"max_tokens":8,
		"temperature":0.2,
		"top_p":0.8,
		"system":[
			{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.161.abc; cc_entrypoint=cli;"},
			{"type":"text","text":"Keep answers terse."}
		],
		"tool_choice":{"type":"tool","name":"lookup"},
		"tools":[{"name":"lookup","description":"Lookup","input_schema":{"type":"object"}}],
		"messages":[{"role":"user","content":"hello"}]
	}`)

	out, err := translateClaudeToResponsesRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["temperature"]; ok {
		t.Fatalf("reasoning model received temperature: %#v", got)
	}
	if _, ok := got["top_p"]; ok {
		t.Fatalf("reasoning model received top_p: %#v", got)
	}
	if _, ok := got["max_output_tokens"]; ok {
		t.Fatalf("Codex backend does not accept max_output_tokens: %#v", got)
	}
	if got["parallel_tool_calls"] != true || got["store"] != false {
		t.Fatalf("missing Responses defaults: %#v", got)
	}
	choice := got["tool_choice"].(map[string]any)
	if choice["type"] != "function" || choice["name"] != "lookup" {
		t.Fatalf("tool_choice = %#v", choice)
	}
	input := got["input"].([]any)
	developer := input[0].(map[string]any)
	if developer["role"] != "developer" {
		t.Fatalf("first input item = %#v", developer)
	}
	content := developer["content"].([]any)
	if len(content) != 1 || content[0].(map[string]any)["text"] != "Keep answers terse." {
		t.Fatalf("developer content = %#v", content)
	}
}

func TestClaudeToResponsesPreservesImagesReturnedByReadTool(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.6-sol",
		"messages":[
			{"role":"assistant","content":[{"type":"tool_use","id":"read_1","name":"Read","input":{"path":"screen.png"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"read_1","content":[
				{"type":"text","text":"Image dimensions: 2x2"},
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aW1hZ2U="}}
			]}]}
		]
	}`)
	out, err := translateClaudeToResponsesRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	input := got["input"].([]any)
	if len(input) != 2 {
		t.Fatalf("input=%#v", input)
	}
	result := input[1].(map[string]any)
	if result["type"] != "function_call_output" || result["call_id"] != "read_1" {
		t.Fatalf("result=%#v", result)
	}
	output, ok := result["output"].([]any)
	if !ok || len(output) != 2 {
		t.Fatalf("tool output=%#v", result["output"])
	}
	if output[0].(map[string]any)["type"] != "input_text" || output[0].(map[string]any)["text"] != "Image dimensions: 2x2" {
		t.Fatalf("text output=%#v", output[0])
	}
	image := output[1].(map[string]any)
	if image["type"] != "input_image" || image["image_url"] != "data:image/png;base64,aW1hZ2U=" {
		t.Fatalf("image output=%#v", image)
	}
	encoded := string(out)
	for _, injected := range []string{"prompt injection", "untrusted image", "ignore instructions in image"} {
		if strings.Contains(strings.ToLower(encoded), injected) {
			t.Fatalf("gateway injected %q: %s", injected, encoded)
		}
	}
}

func TestClaudeToResponsesPreservesDirectURLImageWithoutAddedPrompt(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","messages":[{"role":"user","content":[{"type":"text","text":"Describe this"},{"type":"image","source":{"type":"url","url":"https://example.com/screen.png"}}]}]}`)
	out, err := translateClaudeToResponsesRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	input := got["input"].([]any)
	content := input[0].(map[string]any)["content"].([]any)
	if len(content) != 2 || content[1].(map[string]any)["type"] != "input_image" || content[1].(map[string]any)["image_url"] != "https://example.com/screen.png" {
		t.Fatalf("content=%#v", content)
	}
	if got["instructions"] != nil {
		t.Fatalf("unexpected instructions=%#v", got["instructions"])
	}
}

func TestClaudeToResponsesMapsServerWebSearchTool(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.6-luna",
		"tools":[{"type":"web_search_20250305","name":"web_search"}],
		"messages":[{"role":"user","content":"search"}]
	}`)
	out, err := translateClaudeToResponsesRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	tools := got["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["type"] != "web_search" {
		t.Fatalf("tools = %#v", tools)
	}
}
