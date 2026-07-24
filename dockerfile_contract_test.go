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

func TestStagingCandidateComposeRequiresImmutableImageAndIsolatedState(t *testing.T) {
	data, err := os.ReadFile("docker-compose.staging-candidate.yml")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, fragment := range []string{"STAGING_CANDIDATE_IMAGE:?", "./staging-candidate/pool:/app/pool", "./staging-candidate/data:/app/data", "127.0.0.1:${STAGING_CANDIDATE_PORT:-18992}:8989"} {
		if !strings.Contains(source, fragment) {
			t.Errorf("candidate contract lacks %q", fragment)
		}
	}
	for _, forbidden := range []string{"build:", "codex-pool:dev", "codex-pool:latest", "./pool:/app/pool", "./data:/app/data", "./staging/pool", "./dev/pool"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("candidate contract contains forbidden %q", forbidden)
		}
	}
}
