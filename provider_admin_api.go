package main

import (
	"net/http"
	"strings"
)

// ProviderAdminAPI owns operator-only provider-connection lifecycle routes.
// Canonical and legacy paths intentionally invoke the same mutation callbacks.
type ProviderAdminAPI struct {
	authorizeAdmin func(http.ResponseWriter, *http.Request) bool
	rename         func(http.ResponseWriter, *http.Request, string)
	setDisabled    func(http.ResponseWriter, string, bool)
	resurrect      func(http.ResponseWriter, string)
	refresh        func(http.ResponseWriter, string)
}

func validProviderConnectionPathID(id string) bool {
	return strings.TrimSpace(id) != "" && !strings.Contains(id, "/")
}

func rejectInvalidProviderConnectionPathID(w http.ResponseWriter, id string) bool {
	if validProviderConnectionPathID(id) {
		return false
	}
	http.Error(w, "invalid provider connection ID", http.StatusBadRequest)
	return true
}

func (api *ProviderAdminAPI) TryServe(w http.ResponseWriter, r *http.Request) bool {
	if api == nil || r == nil {
		return false
	}

	if strings.HasPrefix(r.URL.Path, "/api/v2/provider-connections/") && strings.HasSuffix(r.URL.Path, "/identity") {
		if !api.authorizeAdmin(w, r) {
			return true
		}
		if !requireMethod(w, r, http.MethodPatch) {
			return true
		}
		connectionID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v2/provider-connections/"), "/identity")
		if rejectInvalidProviderConnectionPathID(w, connectionID) {
			return true
		}
		api.rename(w, r, connectionID)
		return true
	}

	if !strings.HasPrefix(r.URL.Path, "/admin/accounts/") {
		return false
	}
	path := strings.TrimPrefix(r.URL.Path, "/admin/accounts/")
	switch {
	case strings.HasSuffix(path, "/identity"):
		if !api.authorizeAdmin(w, r) {
			return true
		}
		if !requireMethod(w, r, http.MethodPatch) {
			return true
		}
		connectionID := strings.TrimSuffix(path, "/identity")
		if rejectInvalidProviderConnectionPathID(w, connectionID) {
			return true
		}
		api.rename(w, r, connectionID)
		return true
	case strings.HasSuffix(path, "/enable"), strings.HasSuffix(path, "/disable"):
		if !api.authorizeAdmin(w, r) {
			return true
		}
		if !requireMethod(w, r, http.MethodPost) {
			return true
		}
		disabled := strings.HasSuffix(path, "/disable")
		connectionID := strings.TrimSuffix(strings.TrimSuffix(path, "/disable"), "/enable")
		if rejectInvalidProviderConnectionPathID(w, connectionID) {
			return true
		}
		api.setDisabled(w, connectionID, disabled)
		return true
	case strings.HasSuffix(path, "/resurrect"):
		if !api.authorizeAdmin(w, r) {
			return true
		}
		if !requireMethod(w, r, http.MethodPost) {
			return true
		}
		connectionID := strings.TrimSuffix(path, "/resurrect")
		if rejectInvalidProviderConnectionPathID(w, connectionID) {
			return true
		}
		api.resurrect(w, connectionID)
		return true
	case strings.HasSuffix(path, "/refresh"):
		if !api.authorizeAdmin(w, r) {
			return true
		}
		if !requireMethod(w, r, http.MethodPost) {
			return true
		}
		connectionID := strings.TrimSuffix(path, "/refresh")
		if rejectInvalidProviderConnectionPathID(w, connectionID) {
			return true
		}
		api.refresh(w, connectionID)
		return true
	default:
		return false
	}
}

func (h *proxyHandler) providerAdminAPIService() *ProviderAdminAPI {
	if h.providerAdminAPI != nil {
		return h.providerAdminAPI
	}
	return &ProviderAdminAPI{
		authorizeAdmin: h.checkAdminAuth,
		rename:         h.renameProviderConnection,
		setDisabled:    h.setAccountDisabled,
		resurrect:      h.resurrectAccount,
		refresh:        h.forceRefreshAccount,
	}
}
