# SheJane centralized diagnostics protocol

Status: P2 implementation contract. This document is subordinate to
`docs/shejane-native-app-authorization.md`; it does not change the native inference-token exchange.

## Boundaries

- Diagnostics are off until the user explicitly enables them in SheJane Client for one existing
  `shejane-official` connection.
- The inference Token is used only once to mint a distinct diagnostics credential. It is never sent to
  the diagnostics ingestion route or LangSmith.
- Diagnostics credentials use the `st-` prefix, a separate database table, middleware, rate limit, and
  operating-system credential-store entry. They are not valid on inference, account, or browser APIs.
- Device revocation invalidates both credential types. Disabling diagnostics locally deletes the local
  diagnostics credential and stops new uploads immediately.
- SheJane Runtime remains the execution-state owner. Upload is a best-effort post-commit copy and cannot
  change, fail, retry, or delay an Agent Run.
- Electron, launcher, updater, and native crashes use a separate crash-reporter path. They never use the
  Agent diagnostics credential or LangSmith service key.

## HTTP contract

### Mint a diagnostics credential

`POST /api/shejane/telemetry/token`

The request uses `Authorization: Bearer sk-...` with an active SheJane-managed inference Token. No body is
accepted. Success is `201` with `Cache-Control: no-store`:

```json
{"token_type":"Bearer","telemetry_token":"st-<opaque>","expires_at":1787875200}
```

Unknown, ordinary personal, disabled, deleted, or revoked-device inference Tokens return the same generic
`401`. The raw diagnostics credential is returned only in this response and stored only as an HMAC digest
in Cloud. Runtime persists `expires_at` beside its non-secret diagnostics setting, renews an expired
credential from the fixed official connection, and performs one immediate renewal on an ingestion `401`.
This is not an offline retry queue and remains detached from Agent execution.

### Ingest one terminal Run

`POST /api/shejane/telemetry/events`

The request uses `Authorization: Bearer st-...`, `Content-Type: application/json`, a 32 KiB body limit,
and the shared critical rate limit. Success is `202`; all responses are no-store. The exact body is:

```json
{
  "schema_version": 1,
  "event_id": "019fabaf-e535-74f2-aa69-3962a58f2d91",
  "run_id": "019fabaf-e535-74f2-aa69-3962a58f2d91",
  "attempt_id": "job-id:1",
  "release_version": "0.1.8",
  "platform": "macos",
  "status": "failed",
  "started_at": "2026-07-29T02:24:19Z",
  "ended_at": "2026-07-29T02:24:20Z",
  "duration_ms": 1000,
  "model_category": "openai_chat",
  "tool_names": ["read_file"],
  "input_tokens": 120,
  "output_tokens": 30,
  "failure_category": "provider_unavailable"
}
```

All strings and counts are bounded. `event_id` is unique per terminal Run and makes ingestion idempotent.
Allowed content is limited to the fields above. Prompt, output, message text, reasoning, tool arguments or
results, file names or paths, URLs, headers, model IDs, provider keys, inference/diagnostics credentials,
device names, usernames, and arbitrary metadata are rejected rather than ignored.

String validation is field-specific rather than a shared "safe text" expression. `event_id` and `run_id`
are the same UUID; `attempt_id` is either a UUID or `<bounded-job-id>:<decimal-attempt>`;
`release_version` is a bounded semantic version; platform, status, model category, and failure category
use closed enums. `tool_names` accepts only the reviewed built-in Runtime tool registry. Dynamic MCP or
plugin tool names are omitted by Runtime and rejected by Cloud until the protocol allowlist is explicitly
reviewed and updated in both repositories.

Failures are always eligible for upload. Successful Runs use the user's deterministic sampling rate;
zero disables successful-Run upload. There is no durable offline retry queue in P2.

## LangSmith relay

Only Cloud reads the operator-provided LangSmith service key, regional HTTPS endpoint, workspace ID, and
project name. Cloud maps one accepted event to one completed LangSmith `chain` Run with empty `inputs` and
`outputs`; the allowlisted fields are placed in metadata and a failure uses only `failure_category` as its
error. LangSmith failure returns a retryable ingestion error but never reaches Agent execution because the
Runtime sender is detached and bounded.

The relay uses LangSmith's documented `POST /runs` API and `x-api-key`; multi-workspace keys also use
`x-tenant-id`. Service keys, rather than personal access tokens, are required for production applications:

- https://docs.langchain.com/langsmith/trace-with-api
- https://docs.langchain.com/langsmith/create-account-api-key

## Retention and external decisions

Cloud stores only the diagnostics credential digest and lifecycle metadata; P2 does not add a raw-event
database or desktop retry queue. LangSmith retention, sampling defaults, privacy notice, support access,
crash-reporting vendor/endpoint, LangSmith regional endpoint, workspace, project, service key, and retention
remain operator decisions. Until they are configured, mint/ingest fail closed while Agent Runs and BYOK
remain fully usable.
