package main

import (
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"
)

// contractState distinguishes characterized behavior from roadmap gaps. A gap
// is valid in the baseline matrix but must be explicit and explained; adding a
// provider without declaring every dimension fails this test.
type contractState string

const (
	contractVerified contractState = "verified"
	contractPartial  contractState = "partial"
	contractGap      contractState = "gap"
	contractNA       contractState = "n/a"
)

type capabilityContract struct {
	State contractState
	Note  string
}

type providerContract struct {
	Provider     Provider
	Protocol     string
	Routing      string
	Streaming    capabilityContract
	NonStreaming capabilityContract
	CacheRead    capabilityContract
	CacheWrite   capabilityContract
	Reasoning    capabilityContract
	Translation  capabilityContract
	LargeBody    capabilityContract
	ExactlyOnce  capabilityContract
}

func verified(note string) capabilityContract {
	return capabilityContract{State: contractVerified, Note: note}
}
func partial(note string) capabilityContract {
	return capabilityContract{State: contractPartial, Note: note}
}
func gap(note string) capabilityContract { return capabilityContract{State: contractGap, Note: note} }
func notApplicable(note string) capabilityContract {
	return capabilityContract{State: contractNA, Note: note}
}

func contractURL(raw string) *url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return parsed
}

func providerContractMatrix() []providerContract {
	anthropic := contractURL("https://anthropic.test")
	openAI := contractURL("https://openai.test")
	gemini := contractURL("https://gemini.test")
	return []providerContract{
		{NewCodexProvider(openAI, openAI, openAI), "openai-responses-custom", "path+model", verified("Responses SSE and websocket regression coverage"), verified("Responses buffering/translation coverage"), verified("cached_tokens parsed"), notApplicable("Responses usage exposes cache reads but no cache-creation dimension"), verified("reasoning output tokens parsed"), verified("Claude/chat/completions to Responses paths covered"), partial("large-body path exists; full parity matrix pending"), partial("several paths covered, not yet one canonical durable event")},
		{NewClaudeProvider(anthropic), "anthropic-messages-custom", "path", verified("native Messages SSE coverage"), verified("native Messages JSON coverage"), verified("cache_read_input_tokens parsed"), verified("cache_creation_input_tokens parsed"), verified("shared Anthropic reasoning fixture"), verified("OpenAI/Responses to Claude paths covered"), partial("large-body path exists; full parity matrix pending"), verified("canonical event asserted once with root request ID and attribution")},
		{NewGeminiProvider(gemini, gemini), "gemini", "path", verified("streamGenerateContent path detection and parser"), verified("generateContent parser"), verified("cachedContentTokenCount parsed"), notApplicable("Gemini usage exposes cached-content reads but no cache-creation dimension"), verified("thoughtsTokenCount parsed"), partial("Gemini protocol transformations are bespoke"), gap("large-body provider parity not characterized"), gap("canonical event persistence not characterized")},
		{NewAntigravityProvider(gemini, gemini), "gemini-antigravity-custom", "model", verified("stream response parser exists"), partial("provider parser exists; end-to-end JSON fixture pending"), verified("cached content usage parsed"), notApplicable("Gemini usage exposes cached-content reads but no cache-creation dimension"), verified("thought usage parsed"), partial("bespoke Cloud Code Assist transformation"), gap("large-body provider parity not characterized"), gap("canonical event persistence not characterized")},
		{NewKimiProvider(anthropic), "anthropic-messages", "model", verified("shared stream fixture and proxy integrity contract"), verified("shared JSON fixture and proxy exactly-once contract"), verified("cache-read usage parsed"), verified("canonical usage event persists cache-write tokens"), verified("shared reasoning fixture"), verified("shared Anthropic translation path"), verified("large-body route preserves payload and canonical model"), verified("canonical event asserted once with root request ID and attribution")},
		{NewKimiPlatformProvider(anthropic), "anthropic-messages", "model", verified("shared stream fixture and proxy integrity contract"), verified("shared JSON fixture and proxy exactly-once contract"), verified("cache-read usage parsed"), verified("canonical usage event persists cache-write tokens"), verified("shared reasoning fixture"), verified("shared Anthropic translation path"), verified("large-body route preserves payload and canonical model"), verified("canonical event asserted once with root request ID and attribution")},
		{NewMinimaxProvider(anthropic), "anthropic-messages", "model", verified("shared stream fixture and proxy integrity contract"), verified("shared JSON fixture and proxy exactly-once contract"), verified("cache-read usage parsed"), verified("canonical usage event persists cache-write tokens"), verified("shared reasoning fixture"), verified("shared Anthropic translation path"), verified("large-body route preserves payload and canonical model"), verified("canonical event asserted once with root request ID and attribution")},
		{NewZAIProvider(anthropic), "anthropic-messages", "model", verified("shared stream fixture and proxy integrity contract"), verified("shared JSON fixture and proxy exactly-once contract"), verified("cache-read usage parsed"), verified("canonical usage event persists cache-write tokens"), verified("shared reasoning fixture"), verified("shared Anthropic translation path"), verified("large-body route preserves payload and canonical model"), verified("canonical event asserted once with root request ID and attribution")},
		{NewXiaomiProvider(anthropic), "anthropic-messages", "model", verified("shared stream fixture and proxy integrity contract"), verified("shared JSON fixture and proxy exactly-once contract"), verified("cache-read usage parsed"), verified("canonical usage event persists cache-write tokens"), verified("shared reasoning fixture"), verified("shared Anthropic translation path"), verified("large body model peek plus shared route/body integrity"), verified("canonical event asserted once with root request ID and attribution")},
		{NewGrokProvider(openAI), "openai-responses-custom", "model", verified("Responses usage parser coverage"), verified("Responses usage parser coverage"), verified("cached token details parsed"), notApplicable("protocol usage exposes cache reads but no cache-creation dimension"), verified("reasoning token details parsed"), verified("translated request sanitization covered"), gap("large-body routing parity not characterized"), gap("canonical event persistence not characterized")},
		{NewDeepSeekProvider(anthropic), "anthropic-messages", "model", verified("shared stream fixture and proxy integrity contract"), verified("shared JSON fixture and proxy exactly-once contract"), verified("cache-read usage parsed"), verified("canonical usage event persists cache-write tokens"), verified("shared reasoning fixture"), verified("shared Anthropic translation path"), verified("large-body route preserves payload and canonical model"), verified("canonical event asserted once with root request ID and attribution")},
		{NewQwenProvider(anthropic), "anthropic-messages", "model", verified("shared stream fixture and proxy integrity contract"), verified("shared JSON fixture and proxy exactly-once contract"), verified("cache-read usage parsed"), verified("canonical usage event persists cache-write tokens"), verified("shared reasoning fixture"), verified("shared Anthropic translation path"), verified("large-body route preserves payload and canonical model"), verified("canonical event asserted once with root request ID and attribution")},
		{NewOpenRouterProvider(anthropic), "anthropic-messages", "model", verified("shared stream fixture and proxy integrity contract"), verified("shared JSON fixture and proxy exactly-once contract"), verified("cache-read usage parsed"), verified("canonical usage event persists cache-write tokens"), verified("shared reasoning fixture; routed upstream semantics can vary"), verified("shared Anthropic translation path"), verified("large-body route preserves payload and canonical model"), verified("canonical event asserted once with root request ID and attribution")},
		{NewNvidiaProvider(openAI), "openai-chat", "model", verified("final stream usage chunk parser"), verified("non-streaming usage parser"), verified("nested prompt cached tokens parsed"), notApplicable("protocol usage exposes cache reads but no cache-creation dimension"), verified("nested completion reasoning tokens parsed"), verified("Claude to OpenAI Chat translation path"), verified("small and large native Chat payload integrity asserted"), verified("canonical event asserted once with root request ID and attribution")},
	}
}

func registryForContracts(matrix []providerContract) *ProviderRegistry {
	byType := make(map[AccountType]Provider, len(matrix))
	providers := make([]Provider, 0, len(matrix))
	for _, contract := range matrix {
		provider := contract.Provider
		providers = append(providers, provider)
		byType[provider.Type()] = provider
	}
	return &ProviderRegistry{providers: providers, byType: byType}
}

func TestProviderContractMatrixCoversRegistry(t *testing.T) {
	matrix := providerContractMatrix()
	registry := registryForContracts(matrix)
	wantTypes := []AccountType{
		AccountTypeCodex, AccountTypeClaude, AccountTypeGemini, AccountTypeAntigravity,
		AccountTypeKimi, AccountTypeKimiPlatform, AccountTypeMinimax, AccountTypeZAI,
		AccountTypeXiaomi, AccountTypeGrok, AccountTypeDeepSeek, AccountTypeQwen,
		AccountTypeOpenRouter, AccountTypeNvidia,
	}
	seen := make(map[AccountType]bool, len(matrix))
	for _, contract := range matrix {
		t.Run(string(contract.Provider.Type()), func(t *testing.T) {
			providerType := contract.Provider.Type()
			if seen[providerType] {
				t.Fatalf("duplicate contract for %s", providerType)
			}
			seen[providerType] = true
			if contract.Protocol == "" || contract.Routing == "" {
				t.Fatal("protocol and routing strategy are required")
			}
			capabilities := map[string]capabilityContract{
				"streaming": contract.Streaming, "non_streaming": contract.NonStreaming,
				"cache_read": contract.CacheRead, "cache_write": contract.CacheWrite,
				"reasoning": contract.Reasoning, "translation": contract.Translation,
				"large_body": contract.LargeBody, "exactly_once": contract.ExactlyOnce,
			}
			for name, capability := range capabilities {
				switch capability.State {
				case contractVerified, contractPartial, contractGap, contractNA:
				default:
					t.Errorf("%s has invalid/empty state %q", name, capability.State)
				}
				if capability.State != contractVerified && strings.TrimSpace(capability.Note) == "" {
					t.Errorf("%s state %s requires an explanatory note", name, capability.State)
				}
			}
			if registry.ForType(providerType) != contract.Provider {
				t.Fatal("registry does not return the contracted provider")
			}
			if contract.Provider.UpstreamURL("/contract") == nil {
				t.Fatal("provider returned a nil upstream URL")
			}
			path := "/contract/path"
			if first, second := contract.Provider.NormalizePath(path), contract.Provider.NormalizePath(path); first != second {
				t.Fatalf("NormalizePath is not deterministic: %q != %q", first, second)
			}
			if contract.Provider.DetectsSSE("/v1/messages", "text/event-stream") == false && providerType != AccountTypeGemini {
				t.Error("provider did not detect an explicit SSE content type")
			}
		})
	}
	for _, providerType := range wantTypes {
		if !seen[providerType] {
			t.Errorf("provider %s is missing from the contract matrix", providerType)
		}
	}
	if len(matrix) != len(wantTypes) {
		actual := make([]string, 0, len(matrix))
		for _, contract := range matrix {
			actual = append(actual, string(contract.Provider.Type()))
		}
		sort.Strings(actual)
		t.Fatalf("contract count = %d, provider count = %d; contracts=%v", len(matrix), len(wantTypes), actual)
	}
}

func TestProviderContractUsageFixtures(t *testing.T) {
	anthropicStart := map[string]any{
		"type": "message", "model": "contract-model", "usage": map[string]any{
			"input_tokens": float64(100), "cache_read_input_tokens": float64(20), "cache_creation_input_tokens": float64(10),
			"output_tokens": float64(25), "reasoning_tokens": float64(5),
		},
	}
	openAIUsage := map[string]any{"model": "contract-model", "usage": map[string]any{
		"prompt_tokens": float64(100), "completion_tokens": float64(25),
		"prompt_tokens_details":     map[string]any{"cached_tokens": float64(20)},
		"completion_tokens_details": map[string]any{"reasoning_tokens": float64(5)},
	}}
	responsesUsage := map[string]any{"type": "response.completed", "response": map[string]any{
		"model": "contract-model", "usage": map[string]any{
			"input_tokens": float64(100), "output_tokens": float64(25),
			"input_tokens_details":  map[string]any{"cached_tokens": float64(20)},
			"output_tokens_details": map[string]any{"reasoning_tokens": float64(5)},
		},
	}}
	geminiUsage := map[string]any{"usageMetadata": map[string]any{
		"promptTokenCount": float64(100), "candidatesTokenCount": float64(25),
		"cachedContentTokenCount": float64(20), "thoughtsTokenCount": float64(5),
	}}
	for _, contract := range providerContractMatrix() {
		contract := contract
		t.Run(string(contract.Provider.Type()), func(t *testing.T) {
			var fixture map[string]any
			switch contract.Protocol {
			case "anthropic-messages", "anthropic-messages-custom":
				fixture = anthropicStart
			case "openai-chat":
				fixture = openAIUsage
			case "openai-responses-custom":
				fixture = responsesUsage
			case "gemini", "gemini-antigravity-custom":
				fixture = geminiUsage
			default:
				t.Fatalf("protocol %q has no executable usage fixture", contract.Protocol)
			}
			usage := contract.Provider.ParseUsage(fixture)
			if usage == nil {
				t.Fatal("declared protocol parser rejected its baseline usage fixture")
			}
			if usage.InputTokens != 100 || usage.CachedInputTokens != 20 || usage.OutputTokens != 25 || usage.ReasoningTokens != 5 {
				t.Fatalf("normalized usage = %+v; want input=100 cache-read=20 output=25 reasoning=5", usage)
			}
			wantBillable := int64(105)
			if contract.Protocol == "anthropic-messages" || contract.Protocol == "anthropic-messages-custom" {
				if usage.CacheCreationTokens != 10 {
					t.Fatalf("cache-write tokens = %d, want 10", usage.CacheCreationTokens)
				}
				wantBillable = 95
			}
			if usage.BillableTokens != wantBillable {
				t.Fatalf("billable tokens = %d, want %d", usage.BillableTokens, wantBillable)
			}
		})
	}
}

func TestModelRoutedProvidersDoNotWinPathRouting(t *testing.T) {
	for _, contract := range providerContractMatrix() {
		if contract.Routing != "model" {
			continue
		}
		if contract.Provider.MatchesPath("/v1/messages") || contract.Provider.MatchesPath("/v1/chat/completions") {
			t.Errorf("%s is model-routed but wins path routing", contract.Provider.Type())
		}
	}
}

func TestProviderContractSSEContentTypeIsCaseInsensitive(t *testing.T) {
	for _, contract := range providerContractMatrix() {
		if contract.Provider.Type() == AccountTypeGemini {
			continue // Gemini detects streaming from the RPC path.
		}
		if !contract.Provider.DetectsSSE("/contract", http.CanonicalHeaderKey("TEXT/EVENT-STREAM")) {
			t.Errorf("%s SSE detection is case-sensitive", contract.Provider.Type())
		}
	}
}
