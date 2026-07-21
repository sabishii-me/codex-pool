package main

import "testing"

func TestEmailAllowed(t *testing.T) {
	allowlist := []string{"friend@example.com", "other@example.com"}

	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{"exact match", "friend@example.com", true},
		{"case insensitive", "Friend@Example.com", true},
		{"whitespace trimmed", "  friend@example.com  ", true},
		{"not on list", "stranger@example.com", false},
		{"empty email", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := emailAllowed(allowlist, tt.email); got != tt.want {
				t.Errorf("emailAllowed(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}

func TestEmailAllowedEmptyAllowlistDeniesEveryone(t *testing.T) {
	if emailAllowed(nil, "anyone@example.com") {
		t.Fatal("empty allowlist must deny everyone, not act as an open gate")
	}
	if emailAllowed([]string{}, "anyone@example.com") {
		t.Fatal("empty allowlist must deny everyone, not act as an open gate")
	}
}

func TestEmailAllowedBareDomainMatchesAnyAddressOnThatDomain(t *testing.T) {
	allowlist := []string{"sabishii.me", "c0dt.app"}

	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{"first domain", "spj@sabishii.me", true},
		{"second domain", "spj@c0dt.app", true},
		{"different local part still matches domain", "anyone@c0dt.app", true},
		{"case insensitive domain", "spj@Sabishii.ME", true},
		{"unrelated domain rejected", "spj@example.com", false},
		{"subdomain is not the domain itself", "spj@mail.c0dt.app", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := emailAllowed(allowlist, tt.email); got != tt.want {
				t.Errorf("emailAllowed(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}
