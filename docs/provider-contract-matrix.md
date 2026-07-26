# Provider Contract Matrix

Status: executable baseline

Executable Phase 0 source of truth: `provider_contract_test.go`

This matrix contains the executable Phase 0 baseline plus a clearly marked planned affinity/state contract expansion. The original accounting and transport capabilities are executable and verified; unsupported protocol dimensions are explicitly N/A. The added cache-declaration and stateful-continuation columns are planning status until their fixtures become executable.

- **Verified**: executable focused or end-to-end coverage exists.
- **Partial**: implementation exists, but a required path or assertion remains incomplete.
- **Gap**: behavior is absent or has not been characterized.
- **N/A**: protocol does not expose the concept.

Adding a registered provider without a complete original Phase 0 row fails `TestProviderContractMatrixCoversRegistry`. The planned columns must move into an executable registry before they can be marked **Verified**.

| Provider | Protocol | Routing | Stream | JSON | Cache read | Cache write | Reasoning | Cache declaration | Stateful continuation | Translation | Large body | Exactly once |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| Codex | OpenAI Responses custom | path + model | Verified | Verified | Verified | N/A | Verified | Partial | Partial | Verified | Verified | Verified |
| Claude | Anthropic Messages custom | path | Verified | Verified | Verified | Verified | Verified | Partial | N/A | Verified | Verified | Verified |
| Gemini | Gemini | path | Verified | Verified | Verified | N/A | Verified | N/A | N/A | N/A | Verified | Verified |
| Antigravity | Gemini custom | model | Verified | Verified | Verified | N/A | Verified | N/A | N/A | Verified | Verified | Verified |
| Kimi Coding | Anthropic Messages | model | Verified | Verified | Verified | Verified | Verified | Partial | N/A | Verified | Verified | Verified |
| Kimi Platform | Anthropic Messages | model | Verified | Verified | Verified | Verified | Verified | Partial | N/A | Verified | Verified | Verified |
| MiniMax | Anthropic Messages | model | Verified | Verified | Verified | Verified | Verified | Partial | N/A | Verified | Verified | Verified |
| Z.ai | Anthropic Messages | model | Verified | Verified | Verified | Verified | Verified | Partial | N/A | Verified | Verified | Verified |
| Xiaomi | Anthropic Messages | model | Verified | Verified | Verified | Verified | Verified | Partial | N/A | Verified | Verified | Verified |
| Grok | OpenAI Responses custom | model | Verified | Verified | Verified | N/A | Verified | Partial | N/A | Verified | Verified | Verified |
| DeepSeek | Anthropic Messages | model | Verified | Verified | Verified | Verified | Verified | Partial | N/A | Verified | Verified | Verified |
| Qwen | Anthropic Messages | model | Verified | Verified | Verified | Verified | Verified | Partial | N/A | Verified | Verified | Verified |
| OpenRouter | Anthropic Messages | model | Verified | Verified | Verified | Verified | Verified | Partial | N/A | Verified | Verified | Verified |
| NVIDIA | OpenAI Chat | model | Verified | Verified | Verified | N/A | Verified | Partial | N/A | Verified | Verified | Verified |

## Phase 0 closure

`TestProviderContractPhaseZeroIsClosed` continues to protect the original Phase 0 accounting and transport capabilities. The added **Cache declaration** and **Stateful continuation** columns are a separate planned contract expansion and may remain **Partial** without reopening the historical Phase 0 closure. They track the work in [`protocol-affinity-and-codex-balancing-plan.md`](protocol-affinity-and-codex-balancing-plan.md).

`Cache declaration` means recognized client/provider cache instructions are preserved and typed without synthesizing routing semantics. `Stateful continuation` means provider-owned response/conversation references have explicit connection ownership and safe retry behavior. Cache usage parsing alone does not prove either contract.

Completed closure work:

1. Canonical SQLite events assert root request identity, provider, connection, user, cache read/write, reasoning, billable tokens, and exactly-once persistence.
2. Every provider participates in shared parser fixtures and an end-to-end canonical proxy contract.
3. Native large-body routes preserve payload integrity; protocols requiring whole-body transformation or sanitization explicitly reject oversized requests before upstream transmission.
4. Cross-protocol translation is executable for applicable providers; Gemini-native routing declares translation N/A.
