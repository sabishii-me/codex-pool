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
| Kimi Coding | Anthropic Messages | model | Verified | Partial | Verified | Gap | Partial | Verified | Gap | Gap |
| Kimi Platform | Anthropic Messages | model | Verified | Partial | Verified | Gap | Partial | Verified | Gap | Gap |
| MiniMax | Anthropic Messages | model | Verified | Partial | Verified | Gap | Partial | Verified | Gap | Gap |
| Z.ai | Anthropic Messages | model | Verified | Partial | Verified | Verified | Partial | Verified | Gap | Verified (SSE) |
| Xiaomi | Anthropic Messages | model | Verified | Partial | Verified | Partial | Partial | Verified | Verified | Gap |
| Grok | OpenAI Responses custom | model | Verified | Verified | Verified | Gap | Verified | Verified | Gap | Gap |
| DeepSeek | Anthropic Messages | model | Verified | Verified | Verified | Verified | Partial | Verified | Gap | Verified |
| Qwen | Anthropic Messages | model | Verified | Partial | Verified | Gap | Partial | Verified | Gap | Gap |
| OpenRouter | Anthropic Messages | model | Verified | Partial | Verified | Gap | Partial | Verified | Gap | Gap |
| NVIDIA | OpenAI Chat | model | Verified | Verified | Gap | Gap | Gap | Verified | Gap | Gap |

## Phase 0 closure work

1. Add non-streaming usage fixtures for every Anthropic-compatible provider.
2. Add cache-creation persistence assertions, not only parser assertions.
3. Add reasoning-token fixtures per protocol/provider.
4. Exercise every provider through normal and large-body routing.
5. Assert response-byte integrity for pass-through paths.
6. Replace provider-specific exactly-once regressions with one canonical event assertion for every row.
7. Add custom-engine fixtures for Codex, Claude, Gemini, Antigravity, and Grok to the shared harness.

Phase 0 exits only when every applicable cell is **Verified** or explicitly **N/A**.
