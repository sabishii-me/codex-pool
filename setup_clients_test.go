package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadClientSetupSpecsUsesProductionAdapters(t *testing.T) {
	t.Setenv("CLIENT_SPECS_DIR", "")
	specs, _, err := loadClientSetupSpecs()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"codex-cli", "gemini-cli", "grok-build", "pi"}
	if len(specs) != len(want) {
		t.Fatalf("spec count = %d, want %d", len(specs), len(want))
	}
	for index, id := range want {
		if specs[index].ID != id {
			t.Fatalf("spec[%d] = %q, want %q", index, specs[index].ID, id)
		}
		if specs[index].Adapter == "cute-code" {
			t.Fatal("removed adapter was loaded")
		}
	}
}

func TestLoadClientSetupSpecsRejectsUnknownAdapter(t *testing.T) {
	dir := t.TempDir()
	data := `{"id":"bad","display_name":"Bad","description":"Bad adapter","adapter":"arbitrary","enabled":true,"environments":[{"id":"shell","label":"Shell","shell":"bash","setup_path":"/setup/bad/{download_token}","install_command":"curl {setup_url}","verify_command":"bad --version","launch_command":"bad"}]}`
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLIENT_SPECS_DIR", dir)
	if _, _, err := loadClientSetupSpecs(); err == nil || !strings.Contains(err.Error(), "unknown adapter") {
		t.Fatalf("error = %v", err)
	}
}

func TestSetupClientsProjectionIsAuthenticatedAndPersonalized(t *testing.T) {
	secret := "test-secret-key-12345678901234567890"
	t.Setenv("POOL_JWT_SECRET", secret)
	t.Setenv("PUBLIC_URL", "")
	store, err := newGatewayUserStore(filepath.Join(t.TempDir(), "users.json"))
	if err != nil {
		t.Fatal(err)
	}
	user := &GatewayUser{ID: "setup-user", Token: "download-token", Email: "setup@example.com", PlanType: "pro", CreatedAt: time.Now()}
	if err := store.Create(user); err != nil {
		t.Fatal(err)
	}
	h := &proxyHandler{cfg: &config{}, poolUsers: store}

	unauthorized := httptest.NewRecorder()
	h.handleSetupClients(unauthorized, httptest.NewRequest(http.MethodGet, "http://example.com/api/v2/setup/clients", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	token, err := signJWT(secret, map[string]any{"typ": "session", "sub": user.ID, "email": user.Email, "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/v2/setup/clients", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()
	h.handleSetupClients(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var response struct {
		Clients []setupClientProjection `json:"clients"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Clients) != 4 {
		t.Fatalf("client count = %d", len(response.Clients))
	}
	for _, client := range response.Clients {
		for _, environment := range client.Environments {
			if !strings.Contains(environment.SetupURL, "download-token") || !strings.Contains(environment.InstallCommand, environment.SetupURL) {
				t.Fatalf("environment not personalized: %#v", environment)
			}
			if environment.Shell == "powershell" && !strings.Contains(environment.SetupURL, "shell=powershell") {
				t.Fatalf("PowerShell URL missing shell query: %s", environment.SetupURL)
			}
		}
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
}
