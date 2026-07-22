package main

import "strings"

// ProtocolStreamDetector centralizes protocol-level stream recognition. An
// explicit content type wins; path predicates are used only when upstreams omit
// a useful content type.
type ProtocolStreamDetector struct {
	PathIsStreaming func(string) bool
}

func (detector ProtocolStreamDetector) Detect(path, contentType string) bool {
	contentType = strings.ToLower(contentType)
	if strings.Contains(contentType, "text/event-stream") {
		return true
	}
	if strings.Contains(contentType, "application/json") || strings.Contains(contentType, "text/plain") {
		return false
	}
	return detector.PathIsStreaming != nil && detector.PathIsStreaming(path)
}

var eventStreamDetector ProtocolStreamDetector
var geminiStreamDetector = ProtocolStreamDetector{PathIsStreaming: func(path string) bool {
	return strings.Contains(path, "stream")
}}
var antigravityStreamDetector = ProtocolStreamDetector{PathIsStreaming: func(path string) bool {
	return strings.Contains(path, "streamGenerateContent")
}}
var responsesStreamDetector = ProtocolStreamDetector{PathIsStreaming: func(path string) bool {
	return path == "/responses" || path == "/v1/responses" || strings.HasPrefix(path, "/responses/compact") || strings.HasPrefix(path, "/v1/responses/compact")
}}
