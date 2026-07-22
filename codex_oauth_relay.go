package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// runCodexOAuthRelay runs a single-use loopback callback relay. OpenAI's
// Codex OAuth client only allowlists localhost:1455 and localhost:1457. The
// relay occupies one of those ports for one login, forwards the browser to
// the gateway callback, then exits so Codex, Pi, and other tools can reuse it.
func runCodexOAuthRelay(args []string) error {
	flags := flag.NewFlagSet("codex-oauth-relay", flag.ContinueOnError)
	listenAddr := flags.String("listen", "127.0.0.1:1455", "temporary callback listen address")
	gateway := flags.String("gateway", "http://127.0.0.1:8989", "gateway URL receiving the callback")
	timeout := flags.Duration("timeout", 15*time.Minute, "maximum time to wait for one callback")
	if err := flags.Parse(args); err != nil {
		return err
	}

	host, port, err := net.SplitHostPort(*listenAddr)
	if err != nil {
		return fmt.Errorf("invalid relay listen address: %w", err)
	}
	if port != "1455" && port != "1457" {
		return fmt.Errorf("Codex OAuth relay port must be 1455 or 1457")
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" && host != "0.0.0.0" && host != "::" {
		return fmt.Errorf("Codex OAuth relay must listen on a loopback or container wildcard address")
	}
	gatewayURL, err := url.Parse(strings.TrimRight(*gateway, "/"))
	if err != nil || (gatewayURL.Scheme != "http" && gatewayURL.Scheme != "https") || gatewayURL.Host == "" || gatewayURL.User != nil {
		return fmt.Errorf("invalid gateway URL")
	}
	if *timeout <= 0 || *timeout > 30*time.Minute {
		return fmt.Errorf("relay timeout must be between 1ns and 30m")
	}

	listener, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", *listenAddr, err)
	}
	defer listener.Close()

	done := make(chan struct{})
	var finish sync.Once
	handler := newCodexOAuthRelayHandler(*gatewayURL, func() { finish.Do(func() { close(done) }) })

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       10 * time.Second,
	}
	serveError := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveError <- err
			return
		}
		serveError <- nil
	}()

	fmt.Printf("Codex OAuth relay waiting on http://localhost:%s/auth/callback\n", port)
	fmt.Printf("Callback will be forwarded to %s/auth/callback/codex\n", strings.TrimRight(*gateway, "/"))

	select {
	case <-done:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-serveError
	case err := <-serveError:
		return err
	case <-time.After(*timeout):
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return fmt.Errorf("Codex OAuth relay timed out after %s", timeout.String())
	}
}

func newCodexOAuthRelayHandler(gatewayURL url.URL, finish func()) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		query := r.URL.Query()
		if strings.TrimSpace(query.Get("state")) == "" || (strings.TrimSpace(query.Get("code")) == "" && strings.TrimSpace(query.Get("error")) == "") {
			http.Error(w, "invalid Codex OAuth callback", http.StatusBadRequest)
			return
		}
		destination := gatewayURL
		destination.Path = strings.TrimRight(destination.Path, "/") + "/auth/callback/codex"
		forwarded := url.Values{}
		for _, key := range []string{"code", "state", "error", "error_description", "iss"} {
			if value := query.Get(key); value != "" {
				forwarded.Set(key, value)
			}
		}
		destination.RawQuery = forwarded.Encode()
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, destination.String(), http.StatusSeeOther)
		finish()
	})
	mux.HandleFunc("/cancel", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		finish()
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"waiting"}`))
	})
	return mux
}
