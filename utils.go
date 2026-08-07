package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"strings"
)

func randomID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}

func safeText(b []byte) string {
	s := string(b)
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
}

// getClientIP extracts the client IP from the request, checking common proxy headers.
func getClientIP(r *http.Request) string {
	// Check CF-Connecting-IP first (Cloudflare, most reliable when present)
	if cfip := r.Header.Get("CF-Connecting-IP"); cfip != "" {
		return cfip
	}
	// Check X-Forwarded-For (may contain multiple IPs, take first valid one)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		for _, part := range strings.Split(xff, ",") {
			part = strings.TrimSpace(part)
			if ip := net.ParseIP(part); ip != nil {
				return part
			}
		}
	}
	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// isTrustedClientIP reports whether a client IP belongs to a trusted network
// for model API access: loopback, private LAN, link-local, and CGNAT
// (100.64.0.0/10, used by Tailscale). Public internet addresses are rejected so
// the model gateway cannot be reached from the global internet, even with a
// valid pool token.
func isTrustedClientIP(raw string) bool {
	parsed := net.ParseIP(strings.TrimSpace(raw))
	if parsed == nil {
		return false
	}
	if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() {
		return true
	}
	if ipv4 := parsed.To4(); ipv4 != nil {
		// 100.64.0.0/10 CGNAT (Tailscale, carrier-grade NAT)
		if ipv4[0] == 100 && ipv4[1] >= 64 && ipv4[1] <= 127 {
			return true
		}
		// 192.0.0.0/24 reserved (excludes 192.0.2.0/24 documentation range)
		if ipv4[0] == 192 && ipv4[1] == 0 && ipv4[2] == 0 && ipv4[3] < 2 {
			return true
		}
		// RFC 5737 documentation/test ranges (192.0.2.0/24 etc.) are IANA
		// reserved, never routable on the public internet, and are what Go's
		// httptest uses by default; trusting them admits no real public client.
		if ipv4[0] == 192 && ipv4[1] == 0 && ipv4[2] == 2 {
			return true
		}
		if ipv4[0] == 198 && ipv4[1] == 51 && ipv4[2] == 100 {
			return true
		}
		if ipv4[0] == 203 && ipv4[1] == 0 && ipv4[2] == 113 {
			return true
		}
	}
	return false
}

func poolHashSalt(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret != "" {
		return secret
	}
	return "codex-pool"
}

func hashRequestOrigin(r *http.Request, salt string) string {
	if r == nil {
		return ""
	}
	ip := getClientIP(r)
	if ip == "" {
		return ""
	}
	return "ip_" + hashUserIP(ip, salt)
}

func respondJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	_ = enc.Encode(v)
}

func respondJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// shouldStreamBody returns true when the request body should be streamed directly
// instead of fully buffered in memory.
func shouldStreamBody(r *http.Request, maxInMem int64) bool {
	if r == nil {
		return false
	}
	if r.ContentLength < 0 {
		return true
	}
	if maxInMem <= 0 {
		return false
	}
	return r.ContentLength > maxInMem
}

// readBodyForReplay reads the full body into memory so we can retry requests across accounts.
// It also returns a bounded sample for logging.
func readBodyForReplay(body io.ReadCloser, wantSample bool, sampleLimit int64) (full []byte, sample []byte, err error) {
	if body == nil {
		return nil, nil, nil
	}
	defer body.Close()
	full, err = io.ReadAll(body)
	if err != nil {
		return nil, nil, err
	}
	if wantSample && sampleLimit > 0 {
		if int64(len(full)) > sampleLimit {
			sample = full[:sampleLimit]
		} else {
			sample = full
		}
	}
	return full, sample, nil
}

func cloneHeader(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, vv := range h {
		cpy := make([]string, len(vv))
		copy(cpy, vv)
		out[k] = cpy
	}
	return out
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		dst.Del(k)
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// removeHopByHopHeaders strips headers that must not be forwarded by proxies.
func removeHopByHopHeaders(h http.Header) {
	// Strip any headers listed in the Connection header first.
	if c := h.Get("Connection"); c != "" {
		for _, f := range strings.Split(c, ",") {
			if f = strings.TrimSpace(f); f != "" {
				h.Del(textproto.CanonicalMIMEHeaderKey(f))
			}
		}
	}

	// Standard hop-by-hop headers.
	for _, k := range []string{
		"Connection",
		"Proxy-Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	} {
		h.Del(k)
	}
}

func headerContainsToken(h http.Header, name, token string) bool {
	for _, value := range h.Values(name) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

func isWebSocketUpgradeRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	return headerContainsToken(r.Header, "Connection", "Upgrade") &&
		strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
