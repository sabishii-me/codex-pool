package main

import "testing"

func TestIsTrustedClientIP(t *testing.T) {
	trusted := []string{
		"127.0.0.1",   // loopback
		"192.168.31.5", // private LAN
		"10.0.0.5",    // private
		"172.16.0.5",  // private
		"169.254.10.1", // link-local
		"100.64.0.1",  // CGNAT (Tailscale)
		"100.127.255.255",
		"192.0.2.1",   // RFC 5737 documentation (httptest default)
		"198.51.100.7",
		"203.0.113.9",
	}
	for _, ip := range trusted {
		if !isTrustedClientIP(ip) {
			t.Errorf("expected %s to be trusted", ip)
		}
	}

	public := []string{
		"8.8.8.8",
		"104.21.26.250",
		"172.67.139.177",
		"1.1.1.1",
		"203.0.114.1", // just outside RFC 5737 test range
		"",
		"not-an-ip",
	}
	for _, ip := range public {
		if isTrustedClientIP(ip) {
			t.Errorf("expected %s to be rejected as public", ip)
		}
	}
}
