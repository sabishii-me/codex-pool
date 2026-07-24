package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed client-specs/*.json
var defaultClientSpecs embed.FS

type clientSetupSpec struct {
	ID           string                  `json:"id"`
	DisplayName  string                  `json:"display_name"`
	Description  string                  `json:"description"`
	Adapter      string                  `json:"adapter"`
	Enabled      bool                    `json:"enabled"`
	Order        int                     `json:"order"`
	Environments []clientEnvironmentSpec `json:"environments"`
}

type clientEnvironmentSpec struct {
	ID                string `json:"id"`
	Label             string `json:"label"`
	Shell             string `json:"shell"`
	SetupPath         string `json:"setup_path"`
	ConfigPath        string `json:"config_path,omitempty"`
	ConfigFile        string `json:"config_file,omitempty"`
	InstallCommand    string `json:"install_command"`
	VerifyCommand     string `json:"verify_command"`
	LaunchCommand     string `json:"launch_command"`
	ConfigurationNote string `json:"configuration_note,omitempty"`
}

type setupClientProjection struct {
	ID           string                       `json:"id"`
	DisplayName  string                       `json:"display_name"`
	Description  string                       `json:"description"`
	Environments []setupEnvironmentProjection `json:"environments"`
}

type setupEnvironmentProjection struct {
	ID                string `json:"id"`
	Label             string `json:"label"`
	Shell             string `json:"shell"`
	SetupURL          string `json:"setup_url"`
	ConfigURL         string `json:"config_url,omitempty"`
	ConfigFile        string `json:"config_file,omitempty"`
	InstallCommand    string `json:"install_command"`
	VerifyCommand     string `json:"verify_command"`
	LaunchCommand     string `json:"launch_command"`
	ConfigurationNote string `json:"configuration_note,omitempty"`
}

var setupAdapterPaths = map[string]string{
	"codex": "/setup/codex/", "claude": "/setup/claude/", "gemini": "/setup/gemini/", "grok": "/setup/grok/", "pi": "/setup/pi/",
}

func loadClientSetupSpecs() ([]clientSetupSpec, string, error) {
	var source fs.FS = defaultClientSpecs
	root := "client-specs"
	if dir := strings.TrimSpace(os.Getenv("CLIENT_SPECS_DIR")); dir != "" {
		source = os.DirFS(dir)
		root = "."
	}
	entries, err := fs.ReadDir(source, root)
	if err != nil {
		return nil, "", fmt.Errorf("read client specs: %w", err)
	}
	seen := map[string]bool{}
	var specs []clientSetupSpec
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		data, err := fs.ReadFile(source, filepath.ToSlash(filepath.Join(root, entry.Name())))
		if err != nil {
			return nil, "", fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		var spec clientSetupSpec
		if err := json.Unmarshal(data, &spec); err != nil {
			return nil, "", fmt.Errorf("decode %s: %w", entry.Name(), err)
		}
		if err := validateClientSetupSpec(spec, seen); err != nil {
			return nil, "", fmt.Errorf("%s: %w", entry.Name(), err)
		}
		seen[spec.ID] = true
		if spec.Enabled {
			specs = append(specs, spec)
		}
	}
	sort.Slice(specs, func(i, j int) bool {
		if specs[i].Order == specs[j].Order {
			return specs[i].ID < specs[j].ID
		}
		return specs[i].Order < specs[j].Order
	})
	if len(specs) == 0 {
		return nil, "", fmt.Errorf("no enabled client specs")
	}
	return specs, root, nil
}

func validateClientSetupSpec(spec clientSetupSpec, seen map[string]bool) error {
	if spec.ID == "" || spec.DisplayName == "" || spec.Description == "" {
		return fmt.Errorf("id, display_name, and description are required")
	}
	if seen[spec.ID] {
		return fmt.Errorf("duplicate client id %q", spec.ID)
	}
	prefix, ok := setupAdapterPaths[spec.Adapter]
	if !ok {
		return fmt.Errorf("unknown adapter %q", spec.Adapter)
	}
	if len(spec.Environments) == 0 {
		return fmt.Errorf("at least one environment is required")
	}
	environments := map[string]bool{}
	for _, env := range spec.Environments {
		if env.ID == "" || env.Label == "" || env.InstallCommand == "" || env.VerifyCommand == "" || env.LaunchCommand == "" {
			return fmt.Errorf("environment identity and commands are required")
		}
		if environments[env.ID] {
			return fmt.Errorf("duplicate environment id %q", env.ID)
		}
		environments[env.ID] = true
		if env.Shell != "bash" && env.Shell != "powershell" {
			return fmt.Errorf("unsupported shell %q", env.Shell)
		}
		if env.SetupPath != prefix+"{download_token}" {
			return fmt.Errorf("setup_path does not match adapter %q", spec.Adapter)
		}
		if !strings.Contains(env.InstallCommand, "{setup_url}") {
			return fmt.Errorf("install_command must contain {setup_url}")
		}
		if strings.Contains(env.ConfigPath, "{download_token}") && !strings.HasPrefix(env.ConfigPath, "/config/") {
			return fmt.Errorf("invalid config_path")
		}
	}
	return nil
}

func (h *proxyHandler) handleSetupClients(w http.ResponseWriter, r *http.Request) {
	user, ok := h.sessionUser(r)
	if !ok {
		respondJSONError(w, http.StatusUnauthorized, "not signed in")
		return
	}
	specs, source, err := loadClientSetupSpecs()
	if err != nil {
		respondJSONError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	baseURL := strings.TrimRight(h.getEffectivePublicURL(r), "/")
	clients := make([]setupClientProjection, 0, len(specs))
	for _, spec := range specs {
		client := setupClientProjection{ID: spec.ID, DisplayName: spec.DisplayName, Description: spec.Description}
		for _, env := range spec.Environments {
			setupPath := strings.ReplaceAll(env.SetupPath, "{download_token}", user.Token)
			if env.Shell == "powershell" {
				setupPath += "?shell=powershell"
			}
			setupURL := baseURL + setupPath
			configURL := ""
			if env.ConfigPath != "" {
				configURL = baseURL + strings.ReplaceAll(env.ConfigPath, "{download_token}", user.Token)
			}
			client.Environments = append(client.Environments, setupEnvironmentProjection{
				ID: env.ID, Label: env.Label, Shell: env.Shell, SetupURL: setupURL, ConfigURL: configURL, ConfigFile: env.ConfigFile,
				InstallCommand: strings.ReplaceAll(env.InstallCommand, "{setup_url}", setupURL), VerifyCommand: env.VerifyCommand,
				LaunchCommand: env.LaunchCommand, ConfigurationNote: env.ConfigurationNote,
			})
		}
		clients = append(clients, client)
	}
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, map[string]any{"clients": clients, "evidence": map[string]any{"source": "client-spec-registry:" + source, "generated_at": time.Now().UTC()}})
}
