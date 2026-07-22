package main

import "strings"

// ConnectionIdentity is the provider-neutral presentation identity for one
// upstream credential connection. It is independent of the stable connection
// ID used by routing and usage attribution.
type ConnectionIdentity struct {
	DisplayName     string            `json:"display_name"`
	ExternalSubject string            `json:"external_subject,omitempty"`
	Attributes      map[string]string `json:"attributes,omitempty"`
}

func (a *ProviderConnection) connectionIdentityLocked() ConnectionIdentity {
	identity := a.Identity
	identity.DisplayName = strings.TrimSpace(identity.DisplayName)
	identity.ExternalSubject = strings.TrimSpace(identity.ExternalSubject)
	if identity.ExternalSubject == "" {
		identity.ExternalSubject = connectionExternalSubjectLocked(a)
	}
	if identity.Attributes == nil {
		identity.Attributes = make(map[string]string)
	} else {
		copy := make(map[string]string, len(identity.Attributes))
		for key, value := range identity.Attributes {
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)
			if key != "" && value != "" {
				copy[key] = value
			}
		}
		identity.Attributes = copy
	}
	if email := strings.TrimSpace(a.Email); email != "" {
		if _, exists := identity.Attributes["email"]; !exists {
			identity.Attributes["email"] = email
		}
	}
	if projectID := strings.TrimSpace(a.ProjectID); projectID != "" {
		if _, exists := identity.Attributes["project_id"]; !exists {
			identity.Attributes["project_id"] = projectID
		}
	}
	if identity.DisplayName == "" {
		identity.DisplayName = suggestedConnectionDisplayNameLocked(a, identity)
	}
	if len(identity.Attributes) == 0 {
		identity.Attributes = nil
	}
	return identity
}

func (a *ProviderConnection) ConnectionIdentity() ConnectionIdentity {
	if a == nil {
		return ConnectionIdentity{}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.connectionIdentityLocked()
}

func connectionExternalSubjectLocked(a *ProviderConnection) string {
	for _, value := range []string{a.AccountID, a.IDTokenChatGPTAccountID, a.AccountUUID} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func suggestedConnectionDisplayNameLocked(a *ProviderConnection, identity ConnectionIdentity) string {
	for _, value := range []string{a.Label, a.Email} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	if subject := strings.TrimSpace(identity.ExternalSubject); subject != "" {
		return string(a.Type) + " " + abbreviatedIdentity(subject)
	}
	if id := strings.TrimSpace(a.ID); id != "" {
		return string(a.Type) + " " + abbreviatedIdentity(id)
	}
	return string(a.Type) + " connection"
}

func abbreviatedIdentity(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 12 {
		return value
	}
	return value[:6] + "…" + value[len(value)-4:]
}

func persistConnectionIdentity(root map[string]any, a *ProviderConnection) {
	if root == nil || a == nil {
		return
	}
	a.mu.Lock()
	identity := a.connectionIdentityLocked()
	a.Identity = identity
	a.mu.Unlock()
	root["display_name"] = identity.DisplayName
	if identity.ExternalSubject != "" {
		root["external_subject"] = identity.ExternalSubject
	} else {
		delete(root, "external_subject")
	}
	if len(identity.Attributes) > 0 {
		root["identity_attributes"] = identity.Attributes
	} else {
		delete(root, "identity_attributes")
	}
}
