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
