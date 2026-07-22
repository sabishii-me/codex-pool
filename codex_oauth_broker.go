package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type codexOAuthBroker struct {
	mu             sync.Mutex
	allowedOrigins map[string]struct{}
	callbackPorts  []int
	leaseTTL       time.Duration
	lease          *codexOAuthBrokerLease
}

type codexOAuthBrokerLease struct {
	ID            string
	GatewayOrigin string
	Port          int
	ExpiresAt     time.Time
	listener      net.Listener
	server        *http.Server
}

type codexOAuthBrokerLeaseResponse struct {
	LeaseID   string    `json:"lease_id"`
	Port      int       `json:"port"`
	ExpiresAt time.Time `json:"expires_at"`
}

func newCodexOAuthBroker(allowedOrigins []string, callbackPorts []int, leaseTTL time.Duration) (*codexOAuthBroker, error) {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, raw := range allowedOrigins {
		origin, err := normalizedHTTPOrigin(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid allowed origin %q: %w", raw, err)
		}
		allowed[origin] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil, errors.New("at least one allowed dashboard origin is required")
	}
	if len(callbackPorts) == 0 {
		return nil, errors.New("at least one callback port is required")
	}
	ports := append([]int(nil), callbackPorts...)
	for _, port := range ports {
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid callback port %d", port)
		}
	}
	if leaseTTL <= 0 || leaseTTL > 30*time.Minute {
		return nil, errors.New("lease TTL must be between zero and 30 minutes")
	}
	return &codexOAuthBroker{allowedOrigins: allowed, callbackPorts: ports, leaseTTL: leaseTTL}, nil
}

func normalizedHTTPOrigin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("origin must be an http(s) scheme and host")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", errors.New("origin must not contain a path")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func (b *codexOAuthBroker) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", b.handleStatus)
	mux.HandleFunc("/v1/leases", b.handleLeases)
	return b.withCORS(mux)
}

func (b *codexOAuthBroker) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" {
			normalized, err := normalizedHTTPOrigin(origin)
			if err != nil || !b.originAllowed(normalized) {
				http.Error(w, "dashboard origin is not allowed", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", normalized)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
				w.Header().Set("Access-Control-Allow-Private-Network", "true")
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (b *codexOAuthBroker) originAllowed(origin string) bool {
	_, ok := b.allowedOrigins[origin]
	return ok
}

func (b *codexOAuthBroker) handleStatus(w http.ResponseWriter, _ *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	response := map[string]any{"status": "ready", "callback_ports": b.callbackPorts}
	if b.lease != nil {
		response["active_lease"] = codexOAuthBrokerLeaseResponse{LeaseID: b.lease.ID, Port: b.lease.Port, ExpiresAt: b.lease.ExpiresAt}
	}
	respondJSON(w, response)
}

func (b *codexOAuthBroker) handleLeases(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var input struct {
			GatewayOrigin string `json:"gateway_origin"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
			respondJSONError(w, http.StatusBadRequest, "invalid broker request")
			return
		}
		gatewayOrigin, err := normalizedHTTPOrigin(input.GatewayOrigin)
		if err != nil || !b.originAllowed(gatewayOrigin) {
			respondJSONError(w, http.StatusForbidden, "gateway origin is not allowed")
			return
		}
		lease, err := b.prepareLease(gatewayOrigin)
		if err != nil {
			respondJSONError(w, http.StatusConflict, err.Error())
			return
		}
		respondJSON(w, codexOAuthBrokerLeaseResponse{LeaseID: lease.ID, Port: lease.Port, ExpiresAt: lease.ExpiresAt})
	case http.MethodDelete:
		b.releaseLease("")
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "POST, DELETE, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *codexOAuthBroker) prepareLease(gatewayOrigin string) (*codexOAuthBrokerLease, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.lease != nil {
		return nil, errors.New("a Codex OAuth callback is already pending")
	}
	var listener net.Listener
	selectedPort := 0
	var failures []string
	for _, port := range b.callbackPorts {
		candidate, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(port))
		if err == nil {
			listener, selectedPort = candidate, port
			break
		}
		failures = append(failures, fmt.Sprintf("%d: %v", port, err))
	}
	if listener == nil {
		return nil, fmt.Errorf("Codex callback ports are busy (%s)", strings.Join(failures, "; "))
	}
	leaseIDBytes := make([]byte, 24)
	if _, err := rand.Read(leaseIDBytes); err != nil {
		listener.Close()
		return nil, fmt.Errorf("generate lease: %w", err)
	}
	lease := &codexOAuthBrokerLease{
		ID: base64.RawURLEncoding.EncodeToString(leaseIDBytes), GatewayOrigin: gatewayOrigin,
		Port: selectedPort, ExpiresAt: time.Now().Add(b.leaseTTL), listener: listener,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) { b.forwardCallback(lease, w, r) })
	mux.HandleFunc("/cancel", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		go b.releaseLease(lease.ID)
	})
	lease.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 10 * time.Second}
	b.lease = lease
	go func() {
		if err := lease.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			go b.releaseLease(lease.ID)
		}
	}()
	go func() {
		timer := time.NewTimer(time.Until(lease.ExpiresAt))
		defer timer.Stop()
		<-timer.C
		b.releaseLease(lease.ID)
	}()
	return lease, nil
}

func (b *codexOAuthBroker) forwardCallback(lease *codexOAuthBrokerLease, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := r.URL.Query()
	if strings.TrimSpace(query.Get("state")) == "" || (strings.TrimSpace(query.Get("code")) == "" && strings.TrimSpace(query.Get("error")) == "") {
		http.Error(w, "invalid Codex OAuth callback", http.StatusBadRequest)
		return
	}
	destination, _ := url.Parse(lease.GatewayOrigin)
	destination.Path = "/auth/callback/codex"
	forwarded := url.Values{}
	for _, key := range []string{"code", "state", "error", "error_description", "iss"} {
		if value := query.Get(key); value != "" {
			forwarded.Set(key, value)
		}
	}
	destination.RawQuery = forwarded.Encode()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, destination.String(), http.StatusSeeOther)
	go b.releaseLease(lease.ID)
}

func (b *codexOAuthBroker) releaseLease(expectedID string) {
	b.mu.Lock()
	lease := b.lease
	if lease == nil || (expectedID != "" && lease.ID != expectedID) {
		b.mu.Unlock()
		return
	}
	b.lease = nil
	b.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = lease.server.Shutdown(ctx)
	_ = lease.listener.Close()
}

func runCodexOAuthBroker(args []string) error {
	flags := flag.NewFlagSet("oauth-broker", flag.ContinueOnError)
	listen := flags.String("listen", "127.0.0.1:1460", "broker control address")
	origins := flags.String("allowed-origins", "http://localhost:8989,http://127.0.0.1:8989,http://127.0.0.1:18990,http://localhost:5173,http://127.0.0.1:5173", "comma-separated dashboard origins")
	portsRaw := flags.String("callback-ports", "1455,1457", "ordered callback ports")
	leaseTTL := flags.Duration("lease-ttl", 15*time.Minute, "callback lease lifetime")
	if err := flags.Parse(args); err != nil {
		return err
	}
	ports := []int{}
	for _, raw := range strings.Split(*portsRaw, ",") {
		port, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("invalid callback port %q", raw)
		}
		ports = append(ports, port)
	}
	broker, err := newCodexOAuthBroker(strings.Split(*origins, ","), ports, *leaseTTL)
	if err != nil {
		return err
	}
	server := &http.Server{Addr: *listen, Handler: broker.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	sortedOrigins := make([]string, 0, len(broker.allowedOrigins))
	for origin := range broker.allowedOrigins {
		sortedOrigins = append(sortedOrigins, origin)
	}
	sort.Strings(sortedOrigins)
	fmt.Printf("Codex OAuth broker listening on http://%s (origins: %s)\n", *listen, strings.Join(sortedOrigins, ", "))
	return server.ListenAndServe()
}
