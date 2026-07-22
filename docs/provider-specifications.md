# Declarative provider specifications

Standard providers can be loaded at runtime from strict JSON specifications layered on a shared protocol engine.

## Enable runtime specifications

Set:

```text
PROVIDER_SPECS_DIR=/path/to/provider-specs
```

The directory may contain `*.json` files. See `provider-specs.example/` for DeepSeek and Z.ai examples.

At startup, every file is decoded with unknown-field rejection and schema validation. Any invalid file prevents startup rather than partially applying the directory.

While the gateway is running, the directory is watched. A change causes the complete directory to be loaded and validated before one atomic registry swap. If parsing, validation, provider construction, or duplicate-ID detection fails:

- the previous provider registry remains active;
- the current provider connection pool remains active;
- the error is logged;
- no partial provider snapshot is visible to requests.

After a successful registry swap, credentials are reloaded against the new provider snapshot.

## Credential files

A provider ID is also its credential subdirectory. For a specification with:

```json
{
  "id": "example",
  "credential_field": "api_key"
}
```

place credentials under:

```text
POOL_DIR/example/connection.json
```

with content such as:

```json
{
  "api_key": "secret",
  "display_name": "Example production connection"
}
```

Provider IDs must be lowercase directory-safe slugs containing only `a-z`, `0-9`, and `-`.

## Supported schema

```json
{
  "id": "example",
  "protocol": "anthropic-messages",
  "base_url": "https://api.example.com/anthropic",
  "plan_type": "example",
  "credential_field": "api_key",
  "auth": {
    "type": "bearer"
  },
  "models": [
    {
      "id": "example-model",
      "display_name": "Example Model",
      "description": "Optional catalog description.",
      "aliases": ["example"],
      "context_window": 128000,
      "max_output_tokens": 32000,
      "reasoning": true,
      "input": ["text"]
    }
  ]
}
```

Current protocol support:

- `anthropic-messages`
- `openai-chat`

Optional usage profiles are evaluated in order until one recognizes an event:

- `anthropic-messages`
- `openai-chat`
- `openai-chat-kimi-billing` — preserves Kimi's cache-inclusive historical billing
- `openai-responses`

When `usage_profiles` is omitted, it defaults to the selected `protocol`.

Optional routing fields:

- `model_prefix` matches arbitrary model names with the given prefix.
- `strip_model_prefix` removes that prefix before upstream forwarding.

Optional quota profiles:

- `minimax` parses MiniMax request/token limit headers.

Current authentication support:

- `{"type":"bearer"}` — sends `Authorization: Bearer <credential>`
- `{"type":"header","header":"X-Api-Key"}` — sends the credential in the named header
- either strategy may define a literal `prefix`

Models in runtime specifications participate in:

- normal model routing;
- streamed large-body model routing;
- canonical upstream model rewriting;
- `/api/pool/catalog` and unified OpenAI model discovery;
- provider-connection availability counts.

## Intentional limits

The schema is not a programming language. OAuth, request signing, model discovery, custom quota APIs, bespoke WebSockets, whole-document transformations, Codex, and Antigravity remain code-backed plugins.
