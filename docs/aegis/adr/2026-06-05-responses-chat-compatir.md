# ADR: Responses Chat CompatIR Owner

- Date: 2026-06-05
- Status: accepted
- Decision owner: Responses <-> Chat compatibility path
- Scope: `Responses <-> Chat Completions` request, response, and stream semantic conversion

## Context

The compatibility bridge previously split mapping behavior across relay stream
handlers and `service/openaicompat` pairwise helpers. That made mixed content,
tool-call ordering, stream deltas, and lossy-field policy harder to reason
about because more than one local owner could decide how the same concept maps.

The implementation plan in
`docs/aegis/plans/2026-06-05-responses-chat-compatir.md` set the target
architecture: one canonical IR owner for Responses <-> Chat conversions, stable
public wrappers, and relay handlers that continue to own HTTP/SSE IO.

## Decision

`service/compatir` is the canonical owner for Responses <-> Chat semantic
conversion.

It owns:

- IR types for request, response, output item, content part, tool definition,
  tool choice, tool call, usage, and conversion warnings.
- Chat request <-> Responses request semantic mapping.
- Chat response <-> Responses response semantic mapping.
- Chat stream chunk <-> Responses stream event semantic mapping.
- Stable tool-call ordering and usage normalization inside this compatibility
  boundary.
- Warning/error policy for fields that cannot be represented without changing
  semantics.

Existing public service entrypoints remain stable wrappers:

- `service.ChatCompletionsRequestToResponsesRequest`
- `service.ResponsesRequestToChatCompletionsRequest`
- `service.ChatCompletionsResponseToResponsesResponse`
- `service.ResponsesResponseToChatCompletionsResponse`

Relay handlers keep their current entrypoints and continue to own upstream IO,
HTTP response handling, SSE formatting, format-specific downstream writing, and
error propagation. They call `service/compatir` for event semantics rather than
carrying local text/tool conversion branches.

## Non-Goals

This ADR does not make `service/compatir` the owner for Claude, Gemini, or other
provider conversion paths.

This ADR does not change routing, model mapping, pricing, quota, persistence,
database behavior, or public request/response JSON contracts.

## Consequences

Future Responses <-> Chat mapping fixes should land in `service/compatir` first.
`service/openaicompat` should stay wrapper-only for this boundary except for
policy and regex helpers that are not semantic format mapping.

Relay stream handlers may orchestrate scanner callbacks and write SSE chunks,
but they should not reintroduce local semantic mapping for Responses text,
function calls, tool-call arguments, ordering, or usage normalization.

If Claude/Gemini need equivalent IR treatment later, that requires a separate
plan and decision record. The current `compatir` contract must not be stretched
across provider-specific semantics by implication.

## Retired Owners

The following old owner responsibilities were retired:

- Direct request mapping logic in `service/openaicompat`.
- Direct non-stream response mapping logic in `service/openaicompat`.
- Responses/Chat stream text, tool-call, tool-call-argument, and usage semantic
  conversion branches in OpenAI relay bridge handlers.

The retained compatibility surfaces are wrappers and IO orchestration, not
semantic mapping owners.

## Baseline Sync

The architecture baseline for this work is the Aegis plan and work record:

- `docs/aegis/plans/2026-06-05-responses-chat-compatir.md`
- `docs/aegis/work/2026-06-05-responses-chat-compatir/10-intent.md`
- `docs/aegis/work/2026-06-05-responses-chat-compatir/20-checkpoint.md`
- `docs/aegis/work/2026-06-05-responses-chat-compatir/90-evidence.md`

This ADR records the accepted owner boundary after implementation. No
`BASELINE-GOVERNANCE.md` change is required because that file defines workspace
governance rules, not this runtime conversion owner map.

## Verification

Implementation evidence at acceptance:

```bash
PATH=/root/.local/go1.25.1/bin:$PATH go test ./service/... ./relay/... -count=1
git diff --check
```

Additional stream fixture coverage:

```bash
PATH=/root/.local/go1.25.1/bin:$PATH go test ./service/compatir -run 'Stream|CompatIR' -count=1
```
