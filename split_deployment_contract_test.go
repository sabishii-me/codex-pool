package main

import (
	"os"
	"strings"
	"testing"
)

func TestSplitProductionComposeKeepsServicesIndependent(t *testing.T) {
	data, err := os.ReadFile("docker-compose.split.yml")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, fragment := range []string{"api:", "web:", "ingress:", "Dockerfile.api", "Dockerfile.web", "Dockerfile.ingress", "./pool:/app/pool", "./data:/app/data", `"0.0.0.0:8989:8989"`} {
		if !strings.Contains(source, fragment) {
			t.Errorf("split Compose lacks %q", fragment)
		}
	}
}

func TestSplitIngressRoutesExactSPAPagesAndDynamicallyResolvesServices(t *testing.T) {
	data, err := os.ReadFile("deploy/nginx/ingress.conf")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, fragment := range []string{"resolver 127.0.0.11", "set $api_upstream http://api:8989", "set $web_upstream http://web:8080", "location = /admin/connections", "location /assets/", "proxy_buffering off", "proxy_request_buffering off"} {
		if !strings.Contains(source, fragment) {
			t.Errorf("ingress config lacks %q", fragment)
		}
	}
}

func TestIndependentDeployScriptsUseNoDeps(t *testing.T) {
	for _, path := range []string{"scripts/deploy-production-web.ps1", "scripts/deploy-production-api.ps1"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(data)
		if !strings.Contains(source, "--no-deps") {
			t.Errorf("%s can recreate dependencies", path)
		}
		if strings.Contains(source, " down") {
			t.Errorf("%s stops the stack", path)
		}
	}
}
