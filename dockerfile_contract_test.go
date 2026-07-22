package main

import (
	"os"
	"strings"
	"testing"
)

func TestDockerWebBuildIncludesAPISchemas(t *testing.T) {
	data, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, fragment := range []string{"WORKDIR /workspace/web", "COPY schemas/ ../schemas/", "COPY --from=web-build /workspace/web/dist ./web/dist"} {
		if !strings.Contains(source, fragment) {
			t.Errorf("Dockerfile lacks schema-generation build contract %q", fragment)
		}
	}
}
