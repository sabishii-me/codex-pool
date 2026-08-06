package main

import (
	"net/http"
	"testing"
)

func TestIsWebSocketUpgradeRequest(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://localhost/ws", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Connection", "keep-alive, Upgrade")
	req.Header.Set("Upgrade", "websocket")
	if !isWebSocketUpgradeRequest(req) {
		t.Fatalf("expected websocket upgrade request")
	}

	req.Header.Set("Upgrade", "h2c")
	if isWebSocketUpgradeRequest(req) {
		t.Fatalf("unexpected websocket upgrade detection for non-websocket upgrade")
	}
}
