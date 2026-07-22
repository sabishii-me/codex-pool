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
| Codex | OpenAI Responses custom | path + model | Verified | Verified | Verified | Gap | Verified | Verified | Partial | Partial |
| Claude | Anthropic Messages custom | path | Verified | Verified | Verified | Verified | Partial | Verified | Partial | Partial |
| Gemini | Gemini | path | Verified | Verified | Verified | Gap | Verified | Partial | Gap | Gap |
| Antigravity | Gemini custom | model | Verified | Partial | Verified | Gap | Verified | Partial | Gap | Gap |
| Kimi Coding | Anthropic Messages | model | Verified | Verified | Verified | Partial | Verified | Verified | Partial | Partial |
| Kimi Platform | Anthropic Messages | model | Verified | Verified | Verified | Partial | Verified | Verified | Partial | Partial |
| MiniMax | Anthropic Messages | model | Verified | Verified | Verified | Partial | Verified | Verified | Partial | Partial |
| Z.ai | Anthropic Messages | model | Verified | Verified | Verified | Partial | Verified | Verified | Partial | Partial |
| Xiaomi | Anthropic Messages | model | Verified | Verified | Verified | Partial | Verified | Verified | Verified | Partial |
| Grok | OpenAI Responses custom | model | Verified | Verified | Verified | Gap | Verified | Verified | Gap | Gap |
| DeepSeek | Anthropic Messages | model | Verified | Verified | Verified | Partial | Verified | Verified | Partial | Partial |
| Qwen | Anthropic Messages | model | Verified | Verified | Verified | Partial | Verified | Verified | Partial | Partial |
| OpenRouter | Anthropic Messages | model | Verified | Verified | Verified | Partial | Verified | Verified | Partial | Partial |
| NVIDIA | OpenAI Chat | model | Verified | Verified | Verified | Gap | Verified | Verified | Partial | Gap |

## Phase 0 closure work

1. Persist cache-creation tokens in analytics and aggregate projections, not only parser/request records.
2. Add custom-engine fixtures for Codex, Claude, Gemini, Antigravity, and Grok to the shared harness.
3. Exercise OpenAI-target providers through translated large-body requests or explicitly reject unsupported oversized translation.
4. Replace legacy analytics exactly-once assertions with one canonical event assertion and request ID for every row.

Phase 0 exits only when every applicable cell is **Verified** or explicitly **N/A**.
