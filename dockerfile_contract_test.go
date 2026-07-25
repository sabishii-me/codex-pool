package main

import (
	"os"
	"strings"
	"testing"
)

func TestDockerWebBuildIncludesAPISchemasAndBuildIdentity(t *testing.T) {
	data, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, fragment := range []string{"WORKDIR /workspace/web", "COPY schemas/ ../schemas/", "COPY --from=web-build /workspace/web/dist ./web/dist", "ARG BUILD_COMMIT=unknown", "org.opencontainers.image.revision=$BUILD_COMMIT", "-X main.buildCommit=${BUILD_COMMIT}"} {
		if !strings.Contains(source, fragment) {
			t.Errorf("Dockerfile lacks build contract %q", fragment)
		}
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
