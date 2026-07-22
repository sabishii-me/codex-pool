package main

import "testing"

func TestProtocolStreamDetectorHonorsContentTypeBeforePath(t *testing.T) {
	detector := ProtocolStreamDetector{PathIsStreaming: func(string) bool { return true }}
	if !detector.Detect("/stream", "TEXT/EVENT-STREAM; charset=utf-8") {
		t.Fatal("event stream content type not detected")
	}
	if detector.Detect("/stream", "application/json") {
		t.Fatal("explicit JSON response treated as stream")
	}
	if detector.Detect("/stream", "text/plain") {
		t.Fatal("explicit text response treated as stream")
	}
	if !detector.Detect("/stream", "") {
		t.Fatal("path fallback not used for missing content type")
	}
}

func TestProtocolSpecificStreamDetection(t *testing.T) {
	cases := []struct {
		name     string
		detector ProtocolStreamDetector
		path     string
		want     bool
	}{
		{name: "generic requires header", detector: eventStreamDetector, path: "/v1/messages", want: false},
		{name: "Gemini stream path", detector: geminiStreamDetector, path: "/v1beta/models/x:streamGenerateContent", want: true},
		{name: "Gemini normal path", detector: geminiStreamDetector, path: "/v1beta/models/x:generateContent", want: false},
		{name: "Antigravity stream path", detector: antigravityStreamDetector, path: "/streamGenerateContent", want: true},
		{name: "Responses path", detector: responsesStreamDetector, path: "/v1/responses", want: true},
		{name: "Responses JSON path", detector: responsesStreamDetector, path: "/v1/models", want: false},
	}
	for _, tc := range cases {
		if got := tc.detector.Detect(tc.path, ""); got != tc.want {
			t.Errorf("%s detection=%v, want %v", tc.name, got, tc.want)
		}
	}
}
