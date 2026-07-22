package main

import "testing"

type protocolUsageTestProvider struct{ Provider }

func (protocolUsageTestProvider) ParseUsage(object map[string]any) *RequestUsage {
	return parseAnthropicUsage(object)
}

func TestProtocolUsageObserverDecodesAndMergesSplitEvents(t *testing.T) {
	var emitted []*RequestUsage
	observer := newProtocolUsageObserver(protocolUsageTestProvider{}, "fallback-model", func(usage *RequestUsage, _ bool) {
		copy := *usage
		emitted = append(emitted, &copy)
	})
	start := []byte(`{"type":"message_start","message":{"model":"native-model","usage":{"input_tokens":100,"cache_read_input_tokens":20,"cache_creation_input_tokens":10}}}`)
	delta := []byte(`{"type":"message_delta","usage":{"output_tokens":25,"reasoning_tokens":7}}`)
	if err := observer.Observe(start); err != nil {
		t.Fatal(err)
	}
	if len(emitted) != 0 {
		t.Fatalf("message_start emitted early: %#v", emitted)
	}
	if err := observer.Observe(delta); err != nil {
		t.Fatal(err)
	}
	if len(emitted) != 1 {
		t.Fatalf("emitted=%d, want 1", len(emitted))
	}
	usage := emitted[0]
	if usage.InputTokens != 100 || usage.CachedInputTokens != 20 || usage.CacheCreationTokens != 10 || usage.OutputTokens != 25 || usage.ReasoningTokens != 7 || usage.BillableTokens != 95 || usage.Model != "native-model" {
		t.Fatalf("merged usage=%#v", usage)
	}
}

func TestProtocolUsageObserverAcceptsGeminiArrayEnvelope(t *testing.T) {
	var got *RequestUsage
	observer := newProtocolUsageObserver(&GeminiProvider{}, "fallback-model", func(usage *RequestUsage, _ bool) { got = usage })
	if err := observer.Observe([]byte(`[{"usageMetadata":{"promptTokenCount":40,"cachedContentTokenCount":5,"candidatesTokenCount":10,"thoughtsTokenCount":2}}]`)); err != nil {
		t.Fatal(err)
	}
	if got == nil || got.InputTokens != 40 || got.CachedInputTokens != 5 || got.OutputTokens != 10 || got.ReasoningTokens != 2 || got.Model != "fallback-model" {
		t.Fatalf("array usage=%#v", got)
	}
}

func TestProtocolUsageObserverFlushesIncompleteStart(t *testing.T) {
	var got *RequestUsage
	var pending bool
	observer := newProtocolUsageObserver(protocolUsageTestProvider{}, "fallback-model", func(usage *RequestUsage, isPending bool) {
		got, pending = usage, isPending
	})
	if err := observer.Observe([]byte(`{"type":"message_start","message":{"usage":{"input_tokens":12}}}`)); err != nil {
		t.Fatal(err)
	}
	observer.Flush()
	if got == nil || got.InputTokens != 12 || got.Model != "fallback-model" || !pending {
		t.Fatalf("flushed usage=%#v pending=%v", got, pending)
	}
}

func TestProtocolUsageObserverRejectsInvalidPayload(t *testing.T) {
	observer := newProtocolUsageObserver(&GeminiProvider{}, "", nil)
	if err := observer.Observe([]byte(`not-json`)); err == nil {
		t.Fatal("invalid payload accepted")
	}
}
