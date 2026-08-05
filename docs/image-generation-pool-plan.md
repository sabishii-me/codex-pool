# Native image-generation pool

Status: BFL and Google AI Studio vertical slices implemented locally; not deployed

## Scope and terminology

Native image generation is a workload, not a property inferred from an LLM model name or its ability to accept images.

The gateway distinguishes:

- **Direct native provider APIs**: OpenAI Images, Google Gemini API, Black Forest Labs (BFL), and Stability AI.
- **Aggregator/job APIs**: Replicate and fal expose third-party models behind their own execution, billing, and lifecycle contracts.
- **Coding-agent transports**: Google Antigravity is an agent transport. Its observed ability to return image bytes does not establish it as a native image provider.
- **LLM image tools**: Codex Responses can invoke an `image_generation` tool. This compatibility path is not native image-provider pooling.

Vision/image input, tool invocation, aliases, MIME metadata, `supports_images`, and an `image` substring never qualify a route for `image_generation`.

## Authoritative provider matrix

Research date: 2026-08-01. Contracts must be rechecked before adding each adapter.

| Provider | Category | Authentication | Discovery | Generation execution | Output | Quota/rate limit and economics | Adapter status |
|---|---|---|---|---|---|---|---|
| OpenAI | Direct API | Bearer API key | Models API plus documented image model list | `POST /v1/images/generations`; edits have a separate Images contract | Documented `b64_json`; model/option-dependent URL behavior must be checked | Rate-limit headers and published model pricing; account entitlement remains account-specific | Researched, not implemented |
| Google Gemini API | Direct API | Separate Google AI Studio API key sent as `x-goog-api-key` | `GET /v1beta/models` validates credential access; native eligibility remains an exact documented allowlist | `POST /v1beta/models/{model}:generateContent` with `generationConfig.responseModalities=["IMAGE"]` | Validated inline PNG/JPEG data in response parts | Project/model quota is provider-controlled; successful model listing is not generation-capacity proof, and Antigravity quota evidence is never transferred | Vertical slice implemented; tested projects are currently quota/eligibility blocked |
| Black Forest Labs | Direct API | `x-key` | Official endpoint/model documentation; no unverified broad discovery assumed | Asynchronous: submit model endpoint, then poll returned `polling_url` | Result contains a temporary sample URL; gateway downloads and validates bytes | `/v1/credits` validates the key/account; `429` cooldown honors `Retry-After`; cost remains unknown unless authoritative per-request evidence exists | First vertical slice implemented |
| Stability AI | Direct API | Bearer API key | Official API reference/model documentation | Provider-specific generation/edit endpoints; contract must be captured from the current official reference before coding | Binary/JSON varies by endpoint | Published credits/pricing and provider rate limits; exact headers need contract fixtures | Research incomplete; blocked from implementation |
| Replicate | Aggregator/job API | `Authorization: Bearer` | Model/version APIs | `POST /v1/predictions`; asynchronous by default, optional `Prefer: wait`, polling and webhooks | Prediction output is model-specific, commonly URLs | Platform prediction billing and rate limits; model owner/version are part of provenance | Researched, not implemented |
| fal | Aggregator/job API | Provider key contract | Model-specific schemas/catalog | Queue/sync behavior is model endpoint-specific | Model-specific URL/metadata response | Platform/model pricing and queue limits; authoritative extraction still incomplete | Research incomplete; blocked from implementation |

### Official sources

- OpenAI image generation: <https://platform.openai.com/docs/guides/image-generation>
- OpenAI developer guide: <https://developers.openai.com/api/docs/guides/image-generation>
- Google Gemini image generation: <https://ai.google.dev/gemini-api/docs/image-generation>
- BFL documentation index: <https://docs.bfl.ml/llms.txt>
- BFL quick start and asynchronous polling: <https://docs.bfl.ml/quick_start/generating_images.md>
- BFL machine-readable OpenAPI: <https://api.bfl.ai/openapi.json>
- BFL FLUX.2 Pro endpoint contract: <https://docs.bfl.ml/api-reference/models/generate-or-edit-an-image-with-flux2-[pro].md>
- BFL credits contract: <https://docs.bfl.ml/api-reference/get-the-users-credits.md>
- Stability API reference: <https://platform.stability.ai/docs/api-reference>
- Replicate predictions: <https://replicate.com/docs/topics/predictions/create-a-prediction>
- fal model API example: <https://fal.ai/models/fal-ai/flux/dev/api>

Raw strings scraped from documentation pages are not sufficient evidence. An adapter requires an official request, response, authentication, errors, and lifecycle contract plus fixtures.

## Provider-neutral contract

Every routed model has explicit metadata:

- `model_kind`: `text_generation`, `image_generation`, `embedding`, `audio`, or `video`;
- `input_modalities` and `output_modalities` as separate sets;
- canonical provider and upstream model IDs;
- supported output MIME types and provider-verified constraints;
- capability provenance, source URL, and verification date;
- synchronous or asynchronous operation semantics;
- nullable economics: unknown is never represented as free or zero-cost evidence.

A native image request is eligible only when the exact canonical model descriptor has `model_kind=image_generation` and `output_modalities` contains `image`.

## Routing and execution rules

1. Resolve only an exact canonical native model ID. Compatibility aliases and name heuristics are prohibited.
2. Select an enabled, live, non-cooling connection for the descriptor's provider.
3. Attribute one request to one connection before upstream submission.
4. Do not automatically retry a submitted asynchronous generation. Submission failure can be ambiguous and may have consumed credits.
5. Honor provider `429` cooldown evidence. A cooling connection is removed from selection.
6. Validate provider-returned polling and delivery URLs against provider trust boundaries to prevent credential forwarding or SSRF.
7. Bound response bodies, image bytes, polling duration, and request lifetime.
8. Never log prompts, credentials, returned image bytes, or base64 payloads.

## Canonical accounting

Native image events record:

- request identity, timestamps, user, origin;
- workload kind, provider, canonical model, stable connection, plan;
- success/failure status;
- image count and validated MIME type on success;
- economics-known separately from nullable media cost;
- operation/job ID, dimensions, and typed failure class.

A failed provider operation also creates exactly one canonical event. Prompt and image data are excluded. Legacy text `cost_usd` remains compatible; native-media `media_cost_usd` is SQL `NULL` unless authoritative cost evidence exists. Future schema additions should include provider quota delta and provider-specific billing evidence.

## Implemented native-image vertical slices

The local BFL implementation currently includes:

- separate `bfl` credential/account type and `x-key` authentication;
- API-key contribution validated with BFL `/v1/credits`, with authoritative balance persisted separately from credentials;
- zero-credit connections excluded from routing, `402` exhaustion persisted, and successful jobs followed by a bounded credit refresh;
- explicit native descriptors for selected documented FLUX.2 endpoints;
- OpenAI-compatible `POST /v1/images/generations` facade for `n=1`;
- submit, bounded polling, trusted image download, MIME/size validation, and `b64_json`/data-URL responses;
- multi-account selection through the existing connection pool;
- inflight tracking, 429 cooldown, no automatic generation retry, and typed upstream status handling;
- canonical success/failure image accounting with nullable unknown media cost, operation ID, dimensions, and failure class preserved;
- model catalog fields that separate model kind, input modalities, output modalities, and capability provenance;
- tests preventing vision models, agent transports, aliases, and model-name substrings from entering native routing.

Image edits and image input are not yet accepted by the BFL adapter. Descriptors therefore advertise text input only, even where the broader provider model may support additional modes through separately documented endpoints.

The local Google AI Studio implementation currently includes:

- a separate `google-ai-image` credential/account type and storage directory, isolated from Gemini LLM and Antigravity OAuth accounts;
- API-key contribution validated through `GET /v1beta/models`, using `x-goog-api-key` rather than OAuth bearer authentication;
- exact native descriptors for the documented Google image-generation models supported by this gateway;
- native generation through `POST /v1beta/models/{model}:generateContent` with image-only response modalities;
- bounded response handling, base64 decoding, PNG/JPEG validation, dimensions, operation identity, and canonical success/failure accounting;
- provider-controlled sizing: the OpenAI-compatible facade accepts only `size=auto` for Google native-image requests;
- no credential sharing, quota inference, or transport fallback to Gemini LLM or Antigravity accounts.

Credential validation does not prove generation capacity. Bounded direct tests of available projects returned either quota exhaustion with zero free-tier limits or project access denial, so no currently tested Google account is considered usable capacity.

## Test requirements

Each provider adapter must prove:

- exact native-model resolution and rejection of vision/tool/agent routes;
- correct authentication without credential logging;
- request validation before upstream calls;
- synchronous or submit/poll lifecycle according to the provider contract;
- bounded and trusted output retrieval;
- cancellation and timeout behavior;
- typed auth, quota, safety, invalid-request, provider, and malformed-output failures;
- cooldown and selection exclusion;
- exactly-once canonical accounting for success and failure;
- reload-safe stable identity and separate credential storage;
- unknown economics remain unknown.

A real generation acceptance test is deliberate: one request, short hard timeout, no automatic retry, no credential/base64 output, and local file persistence only after success.

## Implementation sequence

1. **BFL pooled vertical slice** — implemented locally and contract-tested.
2. **Google AI Studio pooled vertical slice** — implemented locally and contract-tested; real tested projects remain quota/eligibility blocked.
3. Complete a deliberate real-credential BFL Test/Staging acceptance request; no Production deployment is implied.
4. Improve durable provider-neutral quota/cooldown evidence, then define quota deltas, per-operation pricing evidence, cancellation APIs, and stale-credit refresh policy.
5. Add one reviewed direct synchronous provider adapter from current official fixtures.
6. Add edits only through a separately verified edit contract; never infer edit support from generation.
7. Add aggregators only with explicit platform/model/version provenance and job cancellation semantics.
8. Perform isolated Test and Staging acceptance before any approved Production maintenance window.

## Release constraints

Nothing in this work authorizes deployment or provider-account changes. Production must not be rebuilt or restarted during active traffic. Test, Staging, and Production credentials remain separate. The existing Antigravity image observation remains evidence only about that coding-agent transport and is excluded from native-provider claims.
