package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestStagingAndDevelopmentComposeStayIsolated(t *testing.T) {
	stagingBytes, err := os.ReadFile("docker-compose.staging.yml")
	if err != nil {
		t.Fatal(err)
	}
	developmentBytes, err := os.ReadFile("docker-compose.dev.yml")
	if err != nil {
		t.Fatal(err)
	}
	staging, development := string(stagingBytes), string(developmentBytes)
	for _, required := range []string{"${STAGING_IMAGE:?set STAGING_IMAGE to the immutable image promoted from Test}", "staging-data-permissions:", "chown -R codex:codex /app/data /app/pool /app/provider-specs", "condition: service_completed_successfully", "127.0.0.1:18990:8989", "./staging/pool:/app/pool", "./staging/data:/app/data", "PROXY_MAX_ATTEMPTS: 3", "PROXY_USAGE_REFRESH_SECONDS: 300"} {
		if !strings.Contains(staging, required) {
			t.Errorf("staging Compose lacks %q", required)
		}
	}
	if strings.Contains(staging, "build:") || strings.Contains(staging, "codex-pool:dev") || strings.Contains(staging, "./dev/") || strings.Contains(staging, "STAGING_IMAGE:-") {
		t.Fatal("staging Compose can be rebuilt, shares active development resources, or pins a default image instead of requiring promotion")
	}
	for _, required := range []string{"codex-pool:dev", "127.0.0.1:18991:8989", "./dev/pool:/app/pool", "./dev/data:/app/data", "PROXY_MAX_ATTEMPTS: 3", "PROXY_USAGE_REFRESH_SECONDS: 300"} {
		if !strings.Contains(development, required) {
			t.Errorf("development Compose lacks %q", required)
		}
	}
	if strings.Contains(development, "./staging/") || strings.Contains(development, "${STAGING_") {
		t.Fatal("active development Compose shares staging resources")
	}
	for path, source := range map[string]string{"staging": staging, "development": development} {
		for _, forbidden := range []string{"- ./pool:/app/pool", "- ./data:/app/data", "codex-pool:latest"} {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s Compose uses production resource %q", path, forbidden)
			}
		}
	}
	dockerIgnore, err := os.ReadFile(".dockerignore")
	if err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{"dev", "staging"} {
		if !strings.Contains("\n"+string(dockerIgnore)+"\n", "\n"+directory+"\n") {
			t.Errorf(".dockerignore does not exclude secret state directory %q", directory)
		}
	}
}

func TestRuntimeHasNoBehaviorBypassSwitches(t *testing.T) {
	forbidden := regexp.MustCompile(`(?i)(PROXY_DISABLE_REFRESH|disable_refresh|LOCAL_DEV_SESSION|PROXY_LOG_BODIES|PROXY_BODY_LOG_LIMIT|PROXY_CLAUDE_TRACE|PROXY_IMAGE_TRACE|PROXY_DEBUG|DEV_PROXY_DEBUG|STAGING_PROXY_DEBUG|WEBSOCKET_COMPRESSION|CODEX_TLS_FINGERPRINT|CODEX_FINGERPRINT_AUTO_UPDATE|CODEX_FINGERPRINT_UPDATE_SECONDS|TIER_THRESHOLD|tier_threshold)`)
	paths := []string{"main.go", "config.go", "config.toml.example", "codex_fingerprint.go", "image_trace.go", "docker-compose.yml", "docker-compose.dev.yml", "docker-compose.staging.yml", ".env.dev.example", ".env.staging.example"}
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if match := forbidden.Find(body); match != nil {
			t.Errorf("%s reintroduces behavioral switch %q", path, match)
		}
	}
}

func TestReleaseEnvironmentsUseSameRetryAndPollingBehavior(t *testing.T) {
	for _, path := range []string{"docker-compose.yml", "docker-compose.dev.yml", "docker-compose.staging.yml"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(body)
		for _, required := range []string{"PROXY_MAX_ATTEMPTS", "3", "PROXY_USAGE_REFRESH_SECONDS", "300"} {
			if !strings.Contains(source, required) {
				t.Errorf("%s lacks common behavior value %q", path, required)
			}
		}
	}
}
