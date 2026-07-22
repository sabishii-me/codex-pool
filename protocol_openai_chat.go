package main

// OpenAIChatBilling controls how a compatible provider treats cached prompt
// tokens. Most providers exclude cache reads from billable throughput; legacy
// Kimi accounting includes them. The policy is explicit so adapter delegation
// cannot silently change canonical accounting.
type OpenAIChatBilling int

const (
	OpenAIChatExcludeCachedInput OpenAIChatBilling = iota
	OpenAIChatIncludeCachedInput
)

// OpenAIChatEngine owns usage normalization for Chat Completions responses and
// final streaming chunks.
type OpenAIChatEngine struct {
	Billing OpenAIChatBilling
}

func (engine OpenAIChatEngine) ParseUsage(event map[string]any) *RequestUsage {
	usage := parseOpenAIChatUsage(event)
	if usage == nil {
		return nil
	}
	if engine.Billing == OpenAIChatIncludeCachedInput {
		usage.BillableTokens = clampNonNegative(usage.InputTokens + usage.OutputTokens)
	}
	return usage
}

var openAIChatEngine = OpenAIChatEngine{Billing: OpenAIChatExcludeCachedInput}
var openAIChatLegacyKimiEngine = OpenAIChatEngine{Billing: OpenAIChatIncludeCachedInput}
