package main

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const localDevelopmentUserID = "local-development"

func localDevSessionURLAllowed(rawURL string) bool {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return loopbackHostname(parsed.Hostname())
}

func loopbackHostname(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func localDevRequestAllowed(r *http.Request) bool {
	if r == nil {
		return false
	}
	// In Docker, traffic accepted by a host-side 127.0.0.1 published port
	// reaches the container with a bridge peer address, so RemoteAddr cannot
	// prove whether the host listener was loopback-only. The deployment guard
	// is the explicit LOCAL_DEV_SESSION flag plus loopback PUBLIC_URL validation
	// at startup and a loopback Host here. docker-compose.dev.yml additionally
	// publishes only 127.0.0.1:18990.
	host := r.Host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	return loopbackHostname(host)
}

func ensureLocalDevelopmentUser(store *GatewayUserStore) error {
	if store == nil {
		return fmt.Errorf("gateway user store is nil")
	}
	if user := store.Get(localDevelopmentUserID); user != nil {
		if user.Disabled {
			return fmt.Errorf("local development user is disabled")
		}
		return nil
	}
	return store.Create(&GatewayUser{
		ID:        localDevelopmentUserID,
		Token:     randomHex(16),
		Email:     "developer@localhost.invalid",
		PlanType:  "development",
		CreatedAt: time.Now(),
	})
}

func (h *proxyHandler) localDevelopmentUser(r *http.Request) (*GatewayUser, bool) {
	if h == nil || h.cfg == nil || !h.cfg.localDevSession || !localDevRequestAllowed(r) || h.poolUsers == nil {
		return nil, false
	}
	user := h.poolUsers.Get(localDevelopmentUserID)
	return user, user != nil && !user.Disabled
}
