package main

import (
	"os"
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
	for _, required := range []string{"codex-pool:staging-a91560b", "127.0.0.1:18990:8989", "./staging/pool:/app/pool", "./staging/data:/app/data", "${STAGING_"} {
		if !strings.Contains(staging, required) {
			t.Errorf("staging Compose lacks %q", required)
		}
	}
	if strings.Contains(staging, "build:") || strings.Contains(staging, "codex-pool:dev") || strings.Contains(staging, "./dev/") {
		t.Fatal("staging Compose can be rebuilt or shares active development resources")
	}
	for _, required := range []string{"codex-pool:dev", "127.0.0.2:18991:8989", "./dev/pool:/app/pool", "./dev/data:/app/data", "${DEV_"} {
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
