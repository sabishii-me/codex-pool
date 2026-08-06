package main

import (
	"os"
	"strings"
	"testing"
)

func TestDockerWebBuildIncludesAPISchemasAndBuildIdentity(t *testing.T) {
	data, err := os.ReadFile("Dockerfile.api")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, fragment := range []string{"ARG BUILD_COMMIT=unknown", "org.opencontainers.image.revision=$BUILD_COMMIT", "-X main.buildCommit=${BUILD_COMMIT}", "API-only", "never embeds or consumes the frontend build output"} {
		if !strings.Contains(source, fragment) {
			t.Errorf("Dockerfile.api lacks build contract %q", fragment)
		}
	}
	for _, forbidden := range []string{"web/dist", "node:22-alpine", "npm run build", "web-build"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("Dockerfile.api must not build or embed the frontend, found %q", forbidden)
		}
	}
	combined, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(combined), "web-build") || strings.Contains(string(combined), "/workspace/web") {
		t.Errorf("combined Dockerfile must no longer embed the frontend into the gateway")
	}
}
func TestReleaseBuildScriptCreatesArtifactWithoutAnotherRuntime(t *testing.T) {
	data, err := os.ReadFile("scripts/build-release-image.ps1")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, fragment := range []string{"Built immutable release image", "STAGING_IMAGE", "does not create another runtime or port"} {
		if !strings.Contains(source, fragment) {
			t.Errorf("release build script lacks %q", fragment)
		}
	}
	for _, forbidden := range []string{"18992", "STAGING_CANDIDATE", "docker-compose.staging-candidate", "codex-pool-staging-candidate"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("release build script contains obsolete runtime contract %q", forbidden)
		}
	}
	if _, err := os.Stat("docker-compose.staging-candidate.yml"); !os.IsNotExist(err) {
		t.Fatalf("obsolete fourth-runtime Compose file still exists: %v", err)
	}
}
