package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func reserveBrokerTestPorts(t *testing.T, count int) []int {
	t.Helper()
	ports := make([]int, 0, count)
	for len(ports) < count {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()
		duplicate := false
		for _, existing := range ports {
			if existing == port {
				duplicate = true
			}
		}
		if !duplicate {
			ports = append(ports, port)
		}
	}
	return ports
}

func prepareBrokerLease(t *testing.T, broker *codexOAuthBroker, origin string) codexOAuthBrokerLeaseResponse {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"gateway_origin": origin})
	request := httptest.NewRequest(http.MethodPost, "/v1/leases", bytes.NewReader(body))
	request.Header.Set("Origin", origin)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	broker.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("prepare status = %d, body = %s", response.Code, response.Body.String())
	}
	var lease codexOAuthBrokerLeaseResponse
	if err := json.Unmarshal(response.Body.Bytes(), &lease); err != nil {
		t.Fatal(err)
	}
	return lease
}

func waitForPortAvailable(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		listener, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(port))
		if err == nil {
			listener.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("port %d was not released", port)
}

func TestCodexOAuthBrokerHandlesThreeSequentialLeasesAndReleasesPorts(t *testing.T) {
	ports := reserveBrokerTestPorts(t, 2)
	broker, err := newCodexOAuthBroker([]string{"http://localhost:8989"}, ports, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	for i := 0; i < 3; i++ {
		lease := prepareBrokerLease(t, broker, "http://localhost:8989")
		callback := "http://127.0.0.1:" + strconv.Itoa(lease.Port) + "/auth/callback?code=code-" + strconv.Itoa(i) + "&state=state-" + strconv.Itoa(i)
		response, err := client.Get(callback)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusSeeOther {
			t.Fatalf("callback status = %d", response.StatusCode)
		}
		location, _ := url.Parse(response.Header.Get("Location"))
		if location.Scheme+"://"+location.Host+location.Path != "http://localhost:8989/auth/callback/codex" || location.Query().Get("code") != "code-"+strconv.Itoa(i) {
			t.Fatalf("callback location = %q", location.String())
		}
		waitForPortAvailable(t, lease.Port)
	}
}

func TestCodexOAuthBrokerFallsBackAndReportsBothPortsBusy(t *testing.T) {
	ports := reserveBrokerTestPorts(t, 2)
	first, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(ports[0]))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	broker, _ := newCodexOAuthBroker([]string{"http://localhost:8989"}, ports, time.Minute)
	lease := prepareBrokerLease(t, broker, "http://localhost:8989")
	if lease.Port != ports[1] {
		t.Fatalf("selected port = %d, want %d", lease.Port, ports[1])
	}
	broker.releaseLease(lease.LeaseID)
	second, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(ports[1]))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err := broker.prepareLease("http://localhost:8989"); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("both-busy error = %v", err)
	}
}

func TestCodexOAuthBrokerRejectsOriginsAndConcurrentLease(t *testing.T) {
	ports := reserveBrokerTestPorts(t, 1)
	broker, _ := newCodexOAuthBroker([]string{"http://localhost:8989"}, ports, time.Minute)
	request := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	request.Header.Set("Origin", "https://evil.example")
	response := httptest.NewRecorder()
	broker.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("evil origin status = %d", response.Code)
	}
	lease := prepareBrokerLease(t, broker, "http://localhost:8989")
	if _, err := broker.prepareLease("http://localhost:8989"); err == nil || !strings.Contains(err.Error(), "already pending") {
		t.Fatalf("concurrent error = %v", err)
	}
	broker.releaseLease(lease.LeaseID)
}

func TestCodexOAuthBrokerTimeoutReleasesPort(t *testing.T) {
	ports := reserveBrokerTestPorts(t, 1)
	broker, _ := newCodexOAuthBroker([]string{"http://localhost:8989"}, ports, 25*time.Millisecond)
	lease := prepareBrokerLease(t, broker, "http://localhost:8989")
	waitForPortAvailable(t, lease.Port)
}
