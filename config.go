package main

import (
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// ConfigFile represents the config.toml structure.
type ConfigFile struct {
	ListenAddr      string `toml:"listen_addr"`
	PoolDir         string `toml:"pool_dir"`
	DBPath          string `toml:"db_path"`
	MaxAttempts     int    `toml:"max_attempts"`
	RefreshProxyURL string `toml:"refresh_proxy_url"` // HTTP proxy for refresh operations
	PublicURL       string `toml:"public_url"`
	ModelAPIBaseURL string `toml:"model_api_base_url"`
	GrokBase        string `toml:"grok_base"`
	FriendName      string `toml:"friend_name"`
	FriendTagline   string `toml:"friend_tagline"`

	OAuthGoogleClientID     string   `toml:"oauth_google_client_id"`
	OAuthGoogleClientSecret string   `toml:"oauth_google_client_secret"`
	AllowedEmails           []string `toml:"allowed_emails"`
	AdminEmails             []string `toml:"admin_emails"`

	ModelAliases map[string]string `toml:"model_aliases"`

	PoolUsers PoolUsersConfig `toml:"pool_users"`
}

// getFriendName returns the configured friend name for the landing page.
func getFriendName() string {
	if v := os.Getenv("FRIEND_NAME"); v != "" {
		return v
	}
	if globalConfigFile != nil && globalConfigFile.FriendName != "" {
		return globalConfigFile.FriendName
	}
	return "PP" // default
}

// getFriendTagline returns the configured tagline for the landing page.
func getFriendTagline() string {
	if v := os.Getenv("FRIEND_TAGLINE"); v != "" {
		return v
	}
	if globalConfigFile != nil && globalConfigFile.FriendTagline != "" {
		return globalConfigFile.FriendTagline
	}
	return "For the few who know, the pool awaits. Unlimited resources. Zero friction."
}

// getEmailList returns a lowercased, trimmed list of emails (or, for
// allowed_emails, bare domains) from either a comma-separated env var or a
// config.toml array. The env var, when set, replaces the config file list
// entirely (same precedence convention as getConfigString). Used for both
// ALLOWED_EMAILS (login gate) and ADMIN_EMAILS (operator gate).
func getEmailList(envKey string, fileEmails []string) []string {
	var raw []string
	if v := os.Getenv(envKey); v != "" {
		raw = strings.Split(v, ",")
	} else {
		raw = fileEmails
	}
	out := make([]string, 0, len(raw))
	for _, email := range raw {
		email = strings.ToLower(strings.TrimSpace(email))
		if email != "" {
			out = append(out, email)
		}
	}
	return out
}

// PoolUsersConfig is the [pool_users] section.
type PoolUsersConfig struct {
	JWTSecret   string `toml:"jwt_secret"`
	StoragePath string `toml:"storage_path"`
}

// loadConfigFile loads config.toml if it exists.
// Returns nil if the file doesn't exist.
func loadConfigFile(path string) (*ConfigFile, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}

	var cfg ConfigFile
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// getConfigString returns the config value with priority: env var > config file > default.
func getConfigString(envKey string, configValue string, defaultValue string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	if configValue != "" {
		return configValue
	}
	return defaultValue
}

// getConfigInt returns the config value with priority: env var > config file > default.
func getConfigInt(envKey string, configValue int, defaultValue int) int {
	if v := os.Getenv(envKey); v != "" {
		if n, err := parseInt64(v); err == nil && n > 0 {
			return int(n)
		}
	}
	if configValue > 0 {
		return configValue
	}
	return defaultValue
}
