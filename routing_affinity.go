package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
)

// AffinityKind keeps cache locality, client sessions, and provider-owned state
// distinct. This first slice admits only soft client/cache affinity to ordinary
// connection selection; strict provider-state ownership is implemented in the
// next planned slice.
type AffinityKind string

const (
	AffinityPromptCacheKey     AffinityKind = "prompt_cache_key"
	AffinityClientSession      AffinityKind = "client_session"
	AffinityClientConversation AffinityKind = "client_conversation"
)

// RequestRoutingContext is the protocol-owned input to connection selection.
// Typed request-routing affinity replaces the historical untyped
// conversation/session collapse on ordinary paths.
// Raw signal values exist only while deriving the private routing key. An
// activated request may retain the value until request processing completes;
// inactive contexts clear it immediately.
type RequestRoutingContext struct {
	Provider       ProviderID
	Protocol       string
	CanonicalModel string
	SoftAffinity   ClientAffinitySignal // request-scoped; do not persist or log Value
	AffinityKey    string
}

type ClientAffinitySignal struct {
	Kind   AffinityKind
	Source string
	Value  string // request-scoped only; never log or persist this raw value
}

func buildRequestRoutingContext(path string, body []byte, headers http.Header, gatewayUserID string, targetProvider ProviderID, canonicalModel, secret string) RequestRoutingContext {
	signal := extractClientAffinitySignal(path, body, headers, targetProvider)
	protocol := affinityProtocolForPath(path)
	routingContext := RequestRoutingContext{
		Provider:       targetProvider,
		Protocol:       protocol,
		CanonicalModel: canonicalModel,
		SoftAffinity:   signal,
		AffinityKey:    affinityRoutingKey(secret, gatewayUserID, targetProvider, canonicalModel, protocol, signal),
	}
	// Do not carry a raw declaration through paths where affinity cannot activate.
	if routingContext.AffinityKey == "" {
		routingContext.SoftAffinity.Value = ""
	}
	return routingContext
}

func affinityProtocolForPath(path string) string {
	path = strings.ToLower(strings.TrimSpace(path))
	switch {
	case isOpenAIResponsesRequestPath(path):
		return "openai_responses"
	case path == "/v1/chat/completions":
		return "openai_chat"
	case path == "/v1/messages":
		return "anthropic_messages"
	default:
		return ""
	}
}

// extractClientAffinitySignal interprets only fields declared by the selected
// incoming protocol or an explicitly registered client/provider extension.
// Every accepted source below is scoped by both path protocol and resolved
// provider/client profile. Unknown metadata and lookalike headers deliberately
// have no routing meaning.
func extractClientAffinitySignal(path string, body []byte, headers http.Header, targetProvider ProviderID) ClientAffinitySignal {
	path = strings.ToLower(strings.TrimSpace(path))
	var obj map[string]any
	if len(body) > 0 {
		if err := json.Unmarshal(body, &obj); err != nil {
			obj = nil
		}
	}

	if isOpenAIResponsesRequestPath(path) && targetProvider == AccountTypeCodex {
		if value := affinityStringField(obj, "prompt_cache_key"); value != "" {
			return ClientAffinitySignal{Kind: AffinityPromptCacheKey, Source: "openai.responses.prompt_cache_key", Value: value}
		}
		// conversation_id is a supported Codex client extension, not an
		// OpenAI provider Conversation object. It therefore remains soft.
		if value := affinityStringField(obj, "conversation_id"); value != "" {
			return ClientAffinitySignal{Kind: AffinityClientConversation, Source: "client.codex.conversation_id", Value: value}
		}
	}

	// Registered Codex transport headers apply to supported inference-create
	// paths, including clients that omit a body-level conversation extension.
	if targetProvider == AccountTypeCodex && isRegisteredCodexInferencePath(path) && headers != nil {
		if value := affinityHeaderValue(headers, "X-Codex-Conversation-Id"); value != "" {
			return ClientAffinitySignal{Kind: AffinityClientConversation, Source: "client.codex.x-codex-conversation-id", Value: value}
		}
	}

	// Codex transport clients use Session_id on registered inference-create
	// operations only. This is an explicit provider/client extension, not a
	// generic header-name guess.
	if targetProvider == AccountTypeCodex && isRegisteredCodexInferencePath(path) && headers != nil {
		if value := affinityHeaderValue(headers, "Session_id"); value != "" {
			return ClientAffinitySignal{Kind: AffinityClientSession, Source: "client.codex.session_id", Value: value}
		}
	}

	return ClientAffinitySignal{}
}

func isRegisteredCodexInferencePath(path string) bool {
	return isOpenAIResponsesRequestPath(path) || path == "/v1/chat/completions" || path == "/v1/messages"
}

// isOpenAIResponsesRequestPath recognizes only the create operation; response
// retrieval/cancellation subresources must not acquire affinity semantics.
func isOpenAIResponsesRequestPath(path string) bool {
	return path == "/responses" || path == "/v1/responses"
}

func recognizedAffinityProtocol(protocol string) bool {
	switch protocol {
	case "openai_responses", "openai_chat", "anthropic_messages":
		return true
	default:
		return false
	}
}

func affinityKindAllowedForProtocolProvider(protocol string, providerID ProviderID, kind AffinityKind) bool {
	// Registered transport profiles must opt in here. Ordinary provider paths
	// are deliberately denied until they have protocol fixtures and version policy.
	if providerID != AccountTypeCodex {
		return false
	}
	switch protocol {
	case "openai_responses":
		return kind == AffinityPromptCacheKey || kind == AffinityClientSession || kind == AffinityClientConversation
	case "openai_chat", "anthropic_messages":
		return kind == AffinityClientSession || kind == AffinityClientConversation
	default:
		return false
	}
}

func recognizedAffinityKind(kind AffinityKind) bool {
	switch kind {
	case AffinityPromptCacheKey, AffinityClientSession, AffinityClientConversation:
		return true
	default:
		return false
	}
}

func affinityHeaderValue(headers http.Header, name string) string {
	for key, values := range headers {
		if !strings.EqualFold(strings.TrimSpace(key), name) {
			continue
		}
		for _, value := range values {
			if value = validAffinityDeclaration(value); value != "" {
				return value
			}
		}
		return ""
	}
	return ""
}

func validAffinityDeclaration(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 4096 || strings.ContainsRune(value, '\x00') {
		return ""
	}
	return value
}

func affinityStringField(obj map[string]any, key string) string {
	if obj == nil {
		return ""
	}
	value, _ := obj[key].(string)
	return validAffinityDeclaration(value)
}

// affinityRoutingKey creates a privacy-safe, user-isolated local key. The
// client value is still forwarded in the body/header unchanged; only this HMAC
// enters ProviderPool.convPin. Missing secret, GatewayUser, provider, protocol,
// canonical model, or kind disables soft affinity rather than weakening its
// namespace or persisting a reversible/raw identifier.
func affinityRoutingKey(secret, gatewayUserID string, providerID ProviderID, canonicalModel, protocol string, signal ClientAffinitySignal) string {
	secret = strings.TrimSpace(secret)
	raw := strings.TrimSpace(signal.Value)
	gatewayUserID = strings.TrimSpace(gatewayUserID)
	providerID = ProviderID(strings.ToLower(strings.TrimSpace(string(providerID))))
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	canonicalModel = strings.ToLower(strings.TrimSpace(canonicalModel))
	signal.Kind = AffinityKind(strings.ToLower(strings.TrimSpace(string(signal.Kind))))
	if len(raw) > 4096 || strings.ContainsRune(raw, '\x00') || strings.ContainsRune(gatewayUserID, '\x00') || strings.ContainsRune(canonicalModel, '\x00') || strings.ContainsRune(protocol, '\x00') || strings.ContainsRune(string(providerID), '\x00') || strings.ContainsRune(string(signal.Kind), '\x00') {
		return ""
	}
	if secret == "" || gatewayUserID == "" || raw == "" {
		return ""
	}
	if canonicalModel == "" {
		return ""
	}
	if !recognizedAffinityProtocol(protocol) || !recognizedAffinityKind(signal.Kind) || providerID == "" || !affinityKindAllowedForProtocolProvider(protocol, providerID, signal.Kind) {
		return ""
	}
	payload := strings.Join([]string{
		"v1",
		gatewayUserID,
		string(providerID),
		canonicalModel,
		protocol,
		string(signal.Kind),
		raw,
	}, "\x00")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return "aff:v1:" + hex.EncodeToString(mac.Sum(nil))
}
