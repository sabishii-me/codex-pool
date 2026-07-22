package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func validProviderSpec() ProviderSpec {
	return ProviderSpec{
		ID: "example", Protocol: ProtocolAnthropicMessages, BaseURL: "https://api.example.test/anthropic",
		PlanType: "example", CredentialField: "api_key", Auth: ProviderAuthSpec{Type: AuthBearer},
		Models: []ModelRouteSpec{{ID: "example-model", Aliases: []string{"example"}, ContextWindow: 128000, MaxOutputTokens: 32000}},
	}
}

func TestParseProviderSpecIsStrict(t *testing.T) {
	data := []byte(`{"id":"example","protocol":"anthropic-messages","base_url":"https://api.example.test/anthropic","plan_type":"example","credential_field":"api_key","auth":{"type":"bearer"},"models":[{"id":"example-model","aliases":["example"]}]}`)
	spec, err := ParseProviderSpec(data)
	if err != nil || spec.ID != "example" || len(spec.Models) != 1 {
		t.Fatalf("spec=%#v err=%v", spec, err)
	}
	if _, err := ParseProviderSpec([]byte(`{"id":"example","protocol":"anthropic-messages","base_url":"https://api.example.test","plan_type":"example","credential_field":"api_key","auth":{"type":"bearer"},"unknown":true}`)); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := ParseProviderSpec(append(data, data...)); err == nil {
		t.Fatal("multiple JSON values accepted")
	}
}

func TestValidateProviderSpecRejectsUnsafeOrAmbiguousDefinitions(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ProviderSpec)
	}{
		{name: "missing id", mutate: func(spec *ProviderSpec) { spec.ID = "" }},
		{name: "unknown protocol", mutate: func(spec *ProviderSpec) { spec.Protocol = "custom-code" }},
		{name: "relative URL", mutate: func(spec *ProviderSpec) { spec.BaseURL = "/relative" }},
		{name: "unknown auth", mutate: func(spec *ProviderSpec) { spec.Auth.Type = "oauth" }},
		{name: "header without name", mutate: func(spec *ProviderSpec) { spec.Auth = ProviderAuthSpec{Type: AuthHeader} }},
		{name: "duplicate model", mutate: func(spec *ProviderSpec) { spec.Models = append(spec.Models, ModelRouteSpec{ID: "EXAMPLE-MODEL"}) }},
		{name: "negative context", mutate: func(spec *ProviderSpec) { spec.Models[0].ContextWindow = -1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := validProviderSpec()
			tc.mutate(&spec)
			if err := ValidateProviderSpec(spec); err == nil {
				t.Fatal("invalid spec accepted")
			}
		})
	}
}

func TestDeclarativeProviderPreservesCredentialAndProtocolContracts(t *testing.T) {
	provider, err := NewDeclarativeProvider(validProviderSpec())
	if err != nil {
		t.Fatal(err)
	}
	connection, err := provider.LoadAccount("connection.json", "/pool/example/connection.json", []byte(`{"api_key":" secret "}`))
	if err != nil {
		t.Fatal(err)
	}
	if connection == nil || connection.ID != "connection" || connection.Type != "example" || connection.AccessToken != "secret" || connection.PlanType != "example" {
		t.Fatalf("connection=%#v", connection)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	provider.SetAuthHeaders(req, connection)
	if got := req.Header.Get("Authorization"); got != "Bearer secret" {
		t.Fatalf("Authorization=%q", got)
	}
	usage := provider.ParseUsage(map[string]any{"type": "message", "model": "example-model", "usage": map[string]any{"input_tokens": 20, "output_tokens": 5}})
	if usage == nil || usage.InputTokens != 20 || usage.OutputTokens != 5 || usage.Model != "example-model" {
		t.Fatalf("usage=%#v", usage)
	}
	model, ok := provider.MatchModel("EXAMPLE")
	if !ok || model.ID != "example-model" {
		t.Fatalf("model=%#v ok=%v", model, ok)
	}
}

func TestDeclarativeProviderSupportsHeaderAuthentication(t *testing.T) {
	spec := validProviderSpec()
	spec.Auth = ProviderAuthSpec{Type: AuthHeader, Header: "X-Api-Key", Prefix: "prefix-"}
	provider, err := NewDeclarativeProvider(spec)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	provider.SetAuthHeaders(req, &ProviderConnection{AccessToken: "secret"})
	if got := req.Header.Get("X-Api-Key"); got != "prefix-secret" {
		t.Fatalf("X-Api-Key=%q", got)
	}
}

func TestDeclarativeProviderSpecIsImmutableFromCallers(t *testing.T) {
	spec := validProviderSpec()
	provider, err := NewDeclarativeProvider(spec)
	if err != nil {
		t.Fatal(err)
	}
	spec.Models[0].Aliases[0] = "mutated-input"
	copy := provider.Spec()
	copy.Models[0].Aliases[0] = "mutated-output"
	model, ok := provider.MatchModel("example")
	if !ok || model.ID != "example-model" {
		t.Fatalf("provider mutated through spec copies: model=%#v ok=%v", model, ok)
	}
}

func TestDeclarativeProviderSupportsOpenAIChatAndPrefixRouting(t *testing.T) {
	spec := validProviderSpec()
	spec.Protocol = ProtocolOpenAIChat
	spec.UsageProfiles = nil
	spec.ModelPrefix = "example/"
	spec.StripModelPrefix = true
	provider, err := NewDeclarativeProvider(spec)
	if err != nil {
		t.Fatal(err)
	}
	if provider.TargetFormat() != FormatOpenAI {
		t.Fatalf("target format=%v", provider.TargetFormat())
	}
	model, ok := provider.MatchModel("example/vendor/model")
	if !ok || model.ID != "vendor/model" {
		t.Fatalf("prefix model=%#v ok=%v", model, ok)
	}
	usage := provider.ParseUsage(map[string]any{"model": "vendor/model", "usage": map[string]any{"prompt_tokens": 100, "completion_tokens": 25, "cached_tokens": 30}})
	if usage == nil || usage.BillableTokens != 95 || usage.Model != "vendor/model" {
		t.Fatalf("OpenAI usage=%#v", usage)
	}
}

func TestDeclarativeProviderComposesOrderedUsageProfiles(t *testing.T) {
	spec := validProviderSpec()
	spec.UsageProfiles = []string{UsageAnthropicMessages, UsageOpenAIChatKimi}
	provider, err := NewDeclarativeProvider(spec)
	if err != nil {
		t.Fatal(err)
	}
	anthropic := provider.ParseUsage(map[string]any{"type": "message", "usage": map[string]any{"input_tokens": 20, "output_tokens": 5}})
	chat := provider.ParseUsage(map[string]any{"usage": map[string]any{"prompt_tokens": 20, "completion_tokens": 5, "cached_tokens": 4}})
	if anthropic == nil || anthropic.BillableTokens != 25 {
		t.Fatalf("Anthropic usage=%#v", anthropic)
	}
	if chat == nil || chat.BillableTokens != 25 || chat.CachedInputTokens != 4 {
		t.Fatalf("fallback Chat usage=%#v", chat)
	}
}

func TestDeclarativeProviderAppliesMinimaxQuotaProfile(t *testing.T) {
	spec := validProviderSpec()
	spec.QuotaProfile = QuotaMinimax
	provider, err := NewDeclarativeProvider(spec)
	if err != nil {
		t.Fatal(err)
	}
	connection := &ProviderConnection{}
	provider.ParseUsageHeaders(connection, http.Header{
		"X-Ratelimit-Remaining": []string{"75"}, "X-Ratelimit-Limit": []string{"100"},
		"Anthropic-Ratelimit-Tokens-Remaining": []string{"40"}, "Anthropic-Ratelimit-Tokens-Limit": []string{"100"},
	})
	if connection.Usage.PrimaryUsedPercent != 0.25 || connection.Usage.SecondaryUsedPercent != 0.60 {
		t.Fatalf("quota usage=%#v", connection.Usage)
	}
}

func TestValidateProviderSpecRejectsInvalidCapabilities(t *testing.T) {
	cases := []func(*ProviderSpec){
		func(spec *ProviderSpec) { spec.UsageProfiles = []string{"unknown"} },
		func(spec *ProviderSpec) { spec.QuotaProfile = "unknown" },
		func(spec *ProviderSpec) { spec.StripModelPrefix = true },
		func(spec *ProviderSpec) { spec.ModelPrefix = "bad prefix/" },
	}
	for index, mutate := range cases {
		spec := validProviderSpec()
		mutate(&spec)
		if err := ValidateProviderSpec(spec); err == nil {
			t.Errorf("invalid capability case %d accepted", index)
		}
	}
}
