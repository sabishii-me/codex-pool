package main

import (
	"crypto/sha256"
	"net/http"
	"strings"
	"testing"
)

func TestExtractAntigravityConversationIDIgnoresGenericMetadata(t *testing.T) {
	if got := extractAntigravityConversationID([]byte(`{"metadata":{"user_id":"must-not-route"},"conversation":"lookalike"}`)); got != "" {
		t.Fatalf("generic Antigravity metadata created affinity %q", got)
	}
	for field, value := range map[string]string{"conversation_id": "declared-conversation", "session_id": "declared-session"} {
		body := []byte(`{"` + field + `":"` + value + `","metadata":{"user_id":"ignore"}}`)
		if got := extractAntigravityConversationID(body); got != value {
			t.Fatalf("declared Antigravity identity=%q, want %q", got, value)
		}
	}
}

func TestAffinityProtocolForPathIsOperationExact(t *testing.T) {
	for path, want := range map[string]string{
		"/v1/responses":              "openai_responses",
		"/v1/chat/completions":       "openai_chat",
		"/v1/messages":               "anthropic_messages",
		"/v1/responses/resp-1":       "",
		"/v1/messages/count_tokens":  "",
		"/v1/chat/completions/other": "",
	} {
		if got := affinityProtocolForPath(path); got != want {
			t.Fatalf("protocol(%q)=%q, want %q", path, got, want)
		}
	}
}

func TestAffinityKindProtocolProviderMatrix(t *testing.T) {
	tests := []struct {
		protocol string
		provider ProviderID
		kind     AffinityKind
		want     bool
	}{
		{protocol: "openai_responses", provider: AccountTypeCodex, kind: AffinityClientConversation, want: true},
		{protocol: "openai_responses", provider: AccountTypeCodex, kind: AffinityPromptCacheKey, want: true},
		{protocol: "openai_chat", provider: AccountTypeCodex, kind: AffinityClientSession, want: true},
		{protocol: "anthropic_messages", provider: AccountTypeCodex, kind: AffinityClientSession, want: true},
		{protocol: "openai_chat", provider: AccountTypeCodex, kind: AffinityPromptCacheKey, want: false},
		{protocol: "anthropic_messages", provider: AccountTypeCodex, kind: AffinityPromptCacheKey, want: false},
		{protocol: "openai_responses", provider: AccountTypeClaude, kind: AffinityPromptCacheKey, want: false},
	}
	for _, test := range tests {
		if got := affinityKindAllowedForProtocolProvider(test.protocol, test.provider, test.kind); got != test.want {
			t.Fatalf("allowed(%q,%q,%q)=%t, want %t", test.protocol, test.provider, test.kind, got, test.want)
		}
	}
}

func TestBuildRequestRoutingContextHeaderPrecedenceIsDeclared(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Codex-Conversation-Id", "conversation")
	headers.Set("Session_id", "session")
	context := buildRequestRoutingContext("/v1/responses", nil, headers, "user", AccountTypeCodex, "model", "secret")
	if context.SoftAffinity.Kind != AffinityClientConversation || context.SoftAffinity.Value != "conversation" || context.AffinityKey == "" {
		t.Fatalf("unexpected declared precedence: %+v", context)
	}
}

func TestBuildRequestRoutingContextPreservesRecognizedClientDeclaration(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","prompt_cache_key":"client-owned"}`)
	context := buildRequestRoutingContext("/v1/responses", body, nil, "user", AccountTypeCodex, "gpt-5.6-sol", "secret")
	if context.SoftAffinity.Value != "client-owned" || string(body) != `{"model":"gpt-5.6-sol","prompt_cache_key":"client-owned"}` {
		t.Fatalf("client declaration was rewritten: context=%+v body=%s", context, body)
	}
}

func TestBuildRequestRoutingContextUsesTypedPrivateAffinity(t *testing.T) {
	context := buildRequestRoutingContext(
		"/v1/responses",
		[]byte(`{"prompt_cache_key":"raw-key"}`),
		nil,
		"user-a",
		AccountTypeCodex,
		"gpt-5.6-sol",
		"secret",
	)
	if context.Provider != AccountTypeCodex || context.Protocol != "openai_responses" || context.CanonicalModel != "gpt-5.6-sol" {
		t.Fatalf("context=%+v", context)
	}
	if len(context.AffinityKey) != len("aff:v1:")+sha256.Size*2 {
		t.Fatalf("unexpected private key size: %q", context.AffinityKey)
	}
	if context.SoftAffinity.Kind != AffinityPromptCacheKey || context.AffinityKey == "" || context.AffinityKey == context.SoftAffinity.Value {
		t.Fatalf("unsafe or missing typed affinity: %+v", context)
	}
	if got := buildRequestRoutingContext("/v1/responses", []byte(`{"prompt_cache_key":"raw-key"}`), nil, "user-a", AccountTypeCodex, "gpt-5.6-sol", ""); got.AffinityKey != "" || got.SoftAffinity.Value != "" {
		t.Fatalf("missing secret must disable and clear raw affinity: %+v", got)
	}
	if got := buildRequestRoutingContext("/v1/responses", []byte(`{"prompt_cache_key":"raw-key"}`), nil, "", AccountTypeCodex, "gpt-5.6-sol", "secret"); got.AffinityKey != "" || got.SoftAffinity.Value != "" {
		t.Fatalf("missing GatewayUser must disable affinity: %+v", got)
	}
}

func TestExtractClientAffinitySignalUsesDeclaredProtocolFields(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		body   string
		header http.Header
		kind   AffinityKind
		source string
		value  string
	}{
		{name: "unknown metadata ignored even with declared cache", path: "/v1/responses", body: `{"prompt_cache_key":"cache-a","metadata":{"user_id":"not-affinity"}}`, kind: AffinityPromptCacheKey, source: "openai.responses.prompt_cache_key", value: "cache-a"},
		{name: "responses codex conversation extension", path: "/responses", body: `{"conversation_id":"conv-a"}`, kind: AffinityClientConversation, source: "client.codex.conversation_id", value: "conv-a"},
		{name: "responses codex session extension", path: "/v1/responses", body: `{}`, header: http.Header{"Session_id": []string{"codex-session"}}, kind: AffinityClientSession, source: "client.codex.session_id", value: "codex-session"},
		{name: "chat codex conversation extension", path: "/v1/chat/completions", body: `{}`, header: http.Header{"X-Codex-Conversation-Id": []string{"codex-chat-conversation"}}, kind: AffinityClientConversation, source: "client.codex.x-codex-conversation-id", value: "codex-chat-conversation"},
		{name: "chat codex session extension", path: "/v1/chat/completions", body: `{}`, header: http.Header{"Session_id": []string{"codex-chat-session"}}, kind: AffinityClientSession, source: "client.codex.session_id", value: "codex-chat-session"},
		{name: "messages codex session extension", path: "/v1/messages", body: `{}`, header: http.Header{"Session_id": []string{"codex-messages-session"}}, kind: AffinityClientSession, source: "client.codex.session_id", value: "codex-messages-session"},
		{name: "claude code profile", path: "/v1/messages", body: `{"metadata":{"user_id":"not-affinity"}}`, header: http.Header{"X-Claude-Code-Session-Id": []string{"claude-session"}}, kind: AffinityClientSession, source: "client.claude-code.x-claude-code-session-id", value: "claude-session"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := extractClientAffinitySignal(test.path, []byte(test.body), test.header, AccountTypeCodex)
			if got.Kind != test.kind || got.Source != test.source || got.Value != test.value {
				t.Fatalf("signal=%+v", got)
			}
		})
	}
}

func TestExtractClientAffinitySignalRegisteredHeaderIsCaseInsensitive(t *testing.T) {
	headers := http.Header{"x-codex-conversation-id": []string{"lowercase"}}
	if got := extractClientAffinitySignal("/v1/responses", nil, headers, AccountTypeCodex); got.Value != "lowercase" {
		t.Fatalf("canonical header lookup failed: %+v", got)
	}
}

func TestExtractClientAffinitySignalRejectsUnsafeHeaders(t *testing.T) {
	for name, value := range map[string]string{
		"nul":       "bad\x00value",
		"oversized": strings.Repeat("x", 4097),
	} {
		t.Run(name, func(t *testing.T) {
			if got := extractClientAffinitySignal("/v1/responses", nil, http.Header{"Session_id": []string{value}}, AccountTypeCodex); got.Value != "" {
				t.Fatalf("unsafe header created affinity: %+v", got)
			}
		})
	}
}

func TestExtractClientAffinitySignalRejectsMalformedJSON(t *testing.T) {
	if got := extractClientAffinitySignal("/v1/responses", []byte(`{"prompt_cache_key":`), nil, AccountTypeCodex); got.Value != "" {
		t.Fatalf("malformed JSON created affinity: %+v", got)
	}
}

func TestExtractClientAffinitySignalDoesNotGuess(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		body     string
		header   http.Header
		provider ProviderID
	}{
		{name: "unrelated route ignores codex session", path: "/v1/models", header: http.Header{"Session_id": []string{"wrong-operation"}}},
		{name: "messages subresource is not create", path: "/v1/messages/count_tokens", header: http.Header{"X-Claude-Code-Session-Id": []string{"wrong-operation"}}},
		{name: "responses subresource is not create", path: "/v1/responses/resp-1", body: `{"prompt_cache_key":"wrong-operation"}`},
		{name: "metadata user", path: "/v1/responses", body: `{"metadata":{"user_id":"user-guess"}}`},
		{name: "chat undeclared cache lookalike", path: "/v1/chat/completions", body: `{"prompt_cache_key":"not-contracted"}`},
		{name: "generic session body", path: "/v1/responses", body: `{"session_id":"session-guess"}`},
		{name: "generic conversation object", path: "/v1/responses", body: `{"conversation":"conv-provider-state"}`},
		{name: "lookalike header on unrelated protocol", path: "/v1/messages", header: http.Header{"Session-Id": []string{"header-guess"}}},
		{name: "claude cache markers have no unique key", path: "/v1/messages", body: `{"system":[{"type":"text","text":"x","cache_control":{"type":"ephemeral"}}]}`},
		{name: "response state is not soft affinity", path: "/v1/responses", body: `{"previous_response_id":"resp-owned"}`},
		{name: "provider SSE fields are not request affinity", path: "/v1/responses", body: `{"type":"response.completed","response":{"id":"resp-sse-owned","conversation_id":"provider-conversation"}}`},
		{name: "responses cache declaration ignored for unrelated provider", path: "/v1/responses", body: `{"prompt_cache_key":"codex-profile-only"}`, provider: AccountTypeClaude},
		{name: "claude code header ignored for unrelated provider", path: "/v1/messages", header: http.Header{"X-Claude-Code-Session-Id": []string{"codex-transport-only"}}, provider: AccountTypeClaude},
		{name: "codex conversation ignored for other provider", path: "/v1/responses", body: `{"conversation_id":"codex-only"}`, provider: AccountTypeClaude},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			targetProvider := test.provider
			if targetProvider == "" {
				targetProvider = AccountTypeCodex
			}
			if got := extractClientAffinitySignal(test.path, []byte(test.body), test.header, targetProvider); got.Value != "" {
				t.Fatalf("unexpected guessed signal=%+v", got)
			}
		})
	}
}

func TestWebSocketHeaderAffinityWaitsForCanonicalModel(t *testing.T) {
	routing := buildRequestRoutingContext(
		"/v1/responses",
		nil,
		http.Header{"Session_id": []string{"thread-ws-1"}},
		"gateway-user",
		AccountTypeCodex,
		"",
		"secret",
	)
	if routing.SoftAffinity.Source != "client.codex.session_id" || routing.SoftAffinity.Value != "" || routing.AffinityKey != "" {
		t.Fatalf("websocket routing context must wait for a canonical model: %+v", routing)
	}
}

func TestRequestRoutingContextDoesNotRetainRawValueInPool(t *testing.T) {
	context := buildRequestRoutingContext("/v1/responses", []byte(`{"prompt_cache_key":"raw-secret-value"}`), nil, "user", AccountTypeCodex, "model", "secret")
	if context.AffinityKey == "" || context.SoftAffinity.Value == "" {
		t.Fatal("active request context did not retain request-scoped declaration")
	}
	pool := newProviderPool(nil, false)
	if !pool.bindAffinity(context.AffinityKey, "account") {
		t.Fatal("private affinity binding was rejected")
	}
	for key := range pool.convPin {
		if strings.Contains(key, context.SoftAffinity.Value) {
			t.Fatalf("pool retained raw affinity value in key %q", key)
		}
	}
}

func TestAffinityRoutingKeyRejectsUnregisteredProviderProfile(t *testing.T) {
	signal := ClientAffinitySignal{Kind: AffinityClientSession, Value: "same-session"}
	codex := affinityRoutingKey("secret", "user", AccountTypeCodex, "model", "anthropic_messages", signal)
	claude := affinityRoutingKey("secret", "user", AccountTypeClaude, "model", "anthropic_messages", signal)
	if codex == "" || claude != "" {
		t.Fatalf("provider profile admission failed: codex=%q claude=%q", codex, claude)
	}
}

func TestAffinityRoutingKeySeparatesRegisteredProtocols(t *testing.T) {
	signal := ClientAffinitySignal{Kind: AffinityClientSession, Value: "same-session"}
	responses := affinityRoutingKey("secret", "user", AccountTypeCodex, "model", "openai_responses", signal)
	chat := affinityRoutingKey("secret", "user", AccountTypeCodex, "model", "openai_chat", signal)
	messages := affinityRoutingKey("secret", "user", AccountTypeCodex, "model", "anthropic_messages", signal)
	if responses == "" || chat == "" || messages == "" || responses == chat || responses == messages || chat == messages {
		t.Fatalf("protocol namespace collision: responses=%q chat=%q messages=%q", responses, chat, messages)
	}
}

func TestAffinityRoutingKeyIsPrivateAndNamespaced(t *testing.T) {
	signal := ClientAffinitySignal{Kind: AffinityPromptCacheKey, Value: "raw-client-cache-key"}
	base := affinityRoutingKey("secret", "user-a", AccountTypeCodex, "gpt-5.6-sol", "openai_responses", signal)
	if base == "" || base == signal.Value || base != affinityRoutingKey("secret", "user-a", AccountTypeCodex, "gpt-5.6-sol", "openai_responses", signal) {
		t.Fatalf("unsafe affinity key %q", base)
	}
	if !strings.HasPrefix(base, "aff:v1:") || strings.Contains(base, signal.Value) {
		t.Fatalf("affinity key leaked raw value: %q", base)
	}
	if got := affinityRoutingKey("secret", " user-a ", ProviderID(" CODEX "), " GPT-5.6-SOL ", " OPENAI_RESPONSES ", ClientAffinitySignal{Kind: " PROMPT_CACHE_KEY ", Value: signal.Value}); got != base {
		t.Fatalf("canonical namespace normalization changed key: %q != %q", got, base)
	}
	for name, got := range map[string]string{
		"user":  affinityRoutingKey("secret", "user-b", AccountTypeCodex, "gpt-5.6-sol", "openai_responses", signal),
		"model": affinityRoutingKey("secret", "user-a", AccountTypeCodex, "gpt-5.5", "openai_responses", signal),
		"kind":  affinityRoutingKey("secret", "user-a", AccountTypeCodex, "gpt-5.6-sol", "openai_responses", ClientAffinitySignal{Kind: AffinityClientSession, Value: signal.Value}),
	} {
		if got == base {
			t.Fatalf("%s namespace collided", name)
		}
	}
	if got := affinityRoutingKey("secret", "user\x00a", AccountTypeCodex, "model", "openai_responses", signal); got != "" {
		t.Fatalf("NUL namespace should be rejected, got %q", got)
	}
	if got := affinityRoutingKey("secret", "user-a", AccountTypeCodex, "model", "openai_responses", ClientAffinitySignal{Kind: AffinityPromptCacheKey, Value: "bad\x00value"}); got != "" {
		t.Fatalf("NUL affinity declaration should be rejected, got %q", got)
	}
	if got := affinityRoutingKey("secret", "user-a", AccountTypeCodex, "model", "openai_responses", ClientAffinitySignal{Kind: AffinityPromptCacheKey, Value: strings.Repeat("x", 4097)}); got != "" {
		t.Fatalf("oversized affinity declaration should be rejected, got %q", got)
	}
	if got := affinityRoutingKey("secret", "user-a", AccountTypeCodex, "gpt-5.6-sol", "openai_chat", signal); got != "" {
		t.Fatalf("Chat prompt cache key has no registered contract, got %q", got)
	}
	if got := affinityRoutingKey("secret", "user-a", AccountTypeClaude, "gpt-5.6-sol", "openai_responses", signal); got != "" {
		t.Fatalf("Responses cache key should require the registered Codex profile, got %q", got)
	}
	if got := affinityRoutingKey("secret", "user-a", AccountTypeCodex, "gpt-5.6-sol", "openai_responses", ClientAffinitySignal{Kind: "unknown", Value: "x"}); got != "" {
		t.Fatalf("unknown affinity kind should be rejected, got %q", got)
	}
	if got := affinityRoutingKey("secret", "user-a", AccountTypeCodex, "", "openai_responses", signal); got != "" {
		t.Fatalf("missing canonical model should disable affinity, got %q", got)
	}
	if got := affinityRoutingKey("secret", "user-a", AccountTypeCodex, "gpt-5.6-sol", "invented_protocol", signal); got != "" {
		t.Fatalf("unknown protocol should disable affinity, got %q", got)
	}
	if got := affinityRoutingKey("secret", "user-a", AccountTypeCodex, "gpt-5.6-sol", "", signal); got != "" {
		t.Fatalf("undeclared protocol should disable affinity, got %q", got)
	}
	if got := affinityRoutingKey("secret", "user-a", "", "gpt-5.6-sol", "openai_responses", signal); got != "" {
		t.Fatalf("missing provider should disable affinity, got %q", got)
	}
	if got := affinityRoutingKey("", "user-a", AccountTypeCodex, "gpt-5.6-sol", "openai_responses", signal); got != "" {
		t.Fatalf("missing secret should disable affinity, got %q", got)
	}
}
