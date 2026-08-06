package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServeCodexSetupScript_PowerShell(t *testing.T) {
	h := &proxyHandler{}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/setup/codex/testtoken?shell=powershell", nil)
	rr := httptest.NewRecorder()
	h.serveCodexSetupScript(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain*", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Set-StrictMode -Version Latest") {
		t.Fatalf("expected PowerShell script body, got:\n%s", body)
	}
	if !strings.Contains(body, "Join-Path $HOME '.codex'") {
		t.Fatalf("expected codex paths in script body, got:\n%s", body)
	}
	if !strings.Contains(body, "model_catalog_json = ") {
		t.Fatalf("expected model catalog config in script body, got:\n%s", body)
	}
	if !strings.Contains(body, "[mcp_servers.model_sync]") {
		t.Fatalf("expected MCP sidecar config in script body, got:\n%s", body)
	}
	if !strings.Contains(body, "model_sync.ps1") {
		t.Fatalf("expected MCP sidecar script install in PowerShell body, got:\n%s", body)
	}
	if !strings.Contains(body, "$firstLine = [Console]::In.ReadLine()") {
		t.Fatalf("expected MCP JSONL transport support in PowerShell body, got:\n%s", body)
	}
}

func TestServeCodexSetupScript_Bash(t *testing.T) {
	h := &proxyHandler{}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/setup/codex/testtoken", nil)
	rr := httptest.NewRecorder()
	h.serveCodexSetupScript(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/x-shellscript") {
		t.Fatalf("Content-Type = %q, want text/x-shellscript*", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "model_sync.sh") {
		t.Fatalf("expected MCP sidecar script install in bash body, got:\n%s", body)
	}
	if !strings.Contains(body, "model_catalog_json = ") {
		t.Fatalf("expected model catalog config in bash script body, got:\n%s", body)
	}
	if !strings.Contains(body, "[mcp_servers.model_sync]") {
		t.Fatalf("expected MCP sidecar config in bash script body, got:\n%s", body)
	}
	if !strings.Contains(body, "MCP_TRANSPORT_MODE=\"jsonl\"") {
		t.Fatalf("expected MCP JSONL transport support in bash body, got:\n%s", body)
	}
}

func TestServeGrokSetupScript_Bash(t *testing.T) {
	h := &proxyHandler{}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/setup/grok/testtoken", nil)
	rr := httptest.NewRecorder()
	h.serveGrokSetupScript(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	for _, want := range []string{"[endpoints]", `models_base_url = \"`, `[model."%s"]`, "grok-build", "gpt-5.6-luna", "auth.json.before-codex-pool", "/config/grok/$TOKEN"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected Grok setup script to contain %q", want)
		}
	}
}

func TestServeGrokSetupScript_PowerShell(t *testing.T) {
	h := &proxyHandler{}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/setup/grok/testtoken?shell=powershell", nil)
	rr := httptest.NewRecorder()
	h.serveGrokSetupScript(rr, req)

	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "models_base_url") || !strings.Contains(rr.Body.String(), "[model.\"' + $Model.Id + '\"]") {
		t.Fatalf("PowerShell Grok setup missing proxy endpoint or model credentials: status=%d", rr.Code)
	}
}

func TestServeGrokSetupScript_BashPreservesConfigAndIsIdempotent(t *testing.T) {
	h := &proxyHandler{}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/setup/grok/testtoken", nil)
	rr := httptest.NewRecorder()
	h.serveGrokSetupScript(rr, req)

	home := t.TempDir()
	configDir := filepath.Join(home, ".grok")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(configDir, "config.toml")
	initial := "[cli]\nauto_update = true\n\n[models]\ndefault = \"grok-build\"\n"
	if err := os.WriteFile(configFile, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	authFile := filepath.Join(configDir, "auth.json")
	if err := os.WriteFile(authFile, []byte(`{"oauth":"credential"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	fakeCurl := "#!/bin/sh\nprintf '%s\\n' '{\"api_key\":\"pool-jwt\"}'\n"
	if err := os.WriteFile(filepath.Join(binDir, "curl"), []byte(fakeCurl), 0o700); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		cmd := exec.Command("bash")
		cmd.Stdin = strings.NewReader(rr.Body.String())
		cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+binDir+":"+os.Getenv("PATH"))
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("run installer: %v\n%s", err, output)
		}
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}
	config := string(data)
	for _, want := range []string{"[cli]", "auto_update = true", `default = "grok-build"`, `[endpoints]`, `models_base_url = "http://example.com/v1"`, `api_key = "pool-jwt"`} {
		if !strings.Contains(config, want) {
			t.Fatalf("installed config missing %q:\n%s", want, config)
		}
	}
	if strings.Contains(config, "codex-pool-grok") {
		t.Fatalf("installer must not create or select a synthetic model:\n%s", config)
	}
	if count := strings.Count(config, `[model."grok-build"]`); count != 1 {
		t.Fatalf("grok-build credential override count = %d, want 1:\n%s", count, config)
	}
	if _, err := os.Stat(authFile); !os.IsNotExist(err) {
		t.Fatalf("active Grok OAuth file still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "auth.json.before-codex-pool")); err != nil {
		t.Fatalf("Grok OAuth backup missing: %v", err)
	}
}

func TestServePiSetupScriptMergesProviders(t *testing.T) {
	h := &proxyHandler{}
	for _, target := range []string{
		"http://example.com/setup/pi/testtoken",
		"http://example.com/setup/pi/testtoken?shell=powershell",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rr := httptest.NewRecorder()
		h.servePiSetupScript(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status = %d", target, rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "/config/pi/testtoken") || !strings.Contains(body, "providers") {
			t.Fatalf("%s did not generate a merging Pi installer", target)
		}
	}
}

func TestServeGeminiSetupScript_PowerShell(t *testing.T) {
	secret := "test-secret-key-12345678901234567890"
	t.Setenv("POOL_JWT_SECRET", secret)

	tmpDir := t.TempDir()
	usersPath := filepath.Join(tmpDir, "pool_users.json")
	store, err := newGatewayUserStore(usersPath)
	if err != nil {
		t.Fatalf("newGatewayUserStore: %v", err)
	}

	user := &GatewayUser{
		ID:        "user123",
		Token:     "tok123",
		Email:     "test@example.com",
		PlanType:  "pro",
		CreatedAt: time.Now(),
	}
	if err := store.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	h := &proxyHandler{poolUsers: store}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/setup/gemini/tok123?shell=powershell", nil)
	rr := httptest.NewRecorder()
	h.serveGeminiSetupScript(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain*", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "$env:CODE_ASSIST_ENDPOINT = $BaseUrl") {
		t.Fatalf("expected PowerShell env setup in body, got:\n%s", body)
	}
	if strings.Contains(body, "`") {
		t.Fatalf("PowerShell script should not contain backticks (Go raw string safety), got:\n%s", body)
	}
}

func newTestPoolUserStoreWithUser(t *testing.T, token string) *GatewayUserStore {
	t.Helper()
	tmpDir := t.TempDir()
	usersPath := filepath.Join(tmpDir, "pool_users.json")
	store, err := newGatewayUserStore(usersPath)
	if err != nil {
		t.Fatalf("newGatewayUserStore: %v", err)
	}
	user := &GatewayUser{ID: "user-" + token, Token: token, Email: token + "@example.com", PlanType: "pro", CreatedAt: time.Now()}
	if err := store.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return store
}
