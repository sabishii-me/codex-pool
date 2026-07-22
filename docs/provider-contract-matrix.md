# Provider Contract Matrix

Status: executable baseline

Source of truth: `provider_contract_test.go`

This matrix characterizes current behavior before architectural extraction. It is intentionally not all green.

- **Verified**: executable focused or end-to-end coverage exists.
- **Partial**: implementation exists, but a required path or assertion remains incomplete.
- **Gap**: behavior is absent or has not been characterized.
- **N/A**: protocol does not expose the concept.

Adding a registered provider without a complete declared row fails `TestProviderContractMatrixCoversRegistry`.

| Provider | Protocol | Routing | Stream | JSON | Cache read | Cache write | Reasoning | Translation | Large body | Exactly once |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| Codex | OpenAI Responses custom | path + model | Verified | Verified | Verified | N/A | Verified | Verified | Partial | Partial |
| Claude | Anthropic Messages custom | path | Verified | Verified | Verified | Verified | Verified | Verified | Partial | Verified |
| Gemini | Gemini | path | Verified | Verified | Verified | N/A | Verified | Partial | Gap | Gap |
| Antigravity | Gemini custom | model | Verified | Partial | Verified | N/A | Verified | Partial | Gap | Gap |
| Kimi Coding | Anthropic Messages | model | Verified | Verified | Verified | Verified | Verified | Verified | Verified | Verified |
| Kimi Platform | Anthropic Messages | model | Verified | Verified | Verified | Verified | Verified | Verified | Verified | Verified |
| MiniMax | Anthropic Messages | model | Verified | Verified | Verified | Verified | Verified | Verified | Verified | Verified |
| Z.ai | Anthropic Messages | model | Verified | Verified | Verified | Verified | Verified | Verified | Verified | Verified |
| Xiaomi | Anthropic Messages | model | Verified | Verified | Verified | Verified | Verified | Verified | Verified | Verified |
| Grok | OpenAI Responses custom | model | Verified | Verified | Verified | N/A | Verified | Verified | Gap | Gap |
| DeepSeek | Anthropic Messages | model | Verified | Verified | Verified | Verified | Verified | Verified | Verified | Verified |
| Qwen | Anthropic Messages | model | Verified | Verified | Verified | Verified | Verified | Verified | Verified | Verified |
| OpenRouter | Anthropic Messages | model | Verified | Verified | Verified | Verified | Verified | Verified | Verified | Verified |
| NVIDIA | OpenAI Chat | model | Verified | Verified | Verified | N/A | Verified | Verified | Verified | Verified |

## Phase 0 closure work

1. Add end-to-end custom-engine proxy fixtures for Codex, Gemini, Antigravity, and Grok; Claude now participates in the canonical Anthropic harness.
2. Exercise OpenAI-target providers through translated large-body requests or explicitly reject unsupported oversized translation.
3. Add canonical exactly-once event assertions for every custom protocol path.
4. Characterize Gemini/Antigravity large-body behavior and translation parity.

Phase 0 exits only when every applicable cell is **Verified** or explicitly **N/A**.
