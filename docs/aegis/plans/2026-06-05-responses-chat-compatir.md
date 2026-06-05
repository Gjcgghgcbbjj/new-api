# Responses <-> Chat CompatIR Implementation Plan

## Goal

Create a canonical intermediate representation for the `Responses <-> Chat Completions` compatibility path so request, response, and stream conversions are owned by one package instead of by pairwise bridge helpers.

## Architecture

- New canonical owner: `service/compatir`.
- Existing public service functions remain stable and become wrappers:
  - `service.ChatCompletionsRequestToResponsesRequest`
  - `service.ResponsesRequestToChatCompletionsRequest`
  - `service.ChatCompletionsResponseToResponsesResponse`
  - `service.ResponsesResponseToChatCompletionsResponse`
- Relay handlers keep their current entrypoints and response shapes.
- Old pairwise conversion logic in `service/openaicompat` is retired or reduced to wrapper-only code as each IR slice lands.
- Claude/Gemini conversion stays out of scope.

## Tech Stack

- Go 1.25.1.
- Existing DTOs in `dto`.
- Existing helper APIs in `common`.
- Tests use package-level Go tests and existing relay handler test style.

## Baseline / Authority Refs

- Current bridge entrypoints:
  - `relay/chat_completions_via_responses.go`
  - `relay/responses_via_chat.go`
  - `relay/channel/openai/chat_via_responses.go`
  - `relay/channel/openai/responses_via_chat.go`
- Current pairwise converters:
  - `service/openaicompat/chat_to_responses.go`
  - `service/openaicompat/responses_chat_bridge.go`
  - `service/openaicompat/responses_to_chat.go`
- Tests added for mixed text + tool calls:
  - `service/openaicompat/responses_chat_bridge_test.go`
  - `relay/channel/openai/responses_via_chat_test.go`

## Compatibility Boundary

- Do not change public request or response JSON fields.
- Do not change routing, model mapping, pricing, quota, or database behavior.
- Preserve existing best-effort mappings where they are semantically valid.
- Replace silent loss with either explicit `ConversionWarning` or an error for unsupported fields that would change semantics.
- Keep bridge handlers callable through current service wrappers.

## Verification

Run after each slice:

```bash
PATH=/root/.local/go1.25.1/bin:$PATH go test ./service/openaicompat ./service/compatir ./relay ./relay/channel/openai -run 'Responses|Chat|Bridge|CompatIR' -count=1
git diff --check
```

Run before commit:

```bash
PATH=/root/.local/go1.25.1/bin:$PATH go test ./service/... ./relay/... -count=1
git diff --check
```

## Architecture Integrity Lens

- Invariant: one canonical owner must decide how Chat and Responses concepts map, including lossy conversion warnings.
- Canonical owner / contract: `service/compatir` owns the IR types, format decoders, format encoders, warning/error policy, and stable ordering rules.
- Responsibility overlap: `service/openaicompat` currently owns direct pairwise conversions; after this plan it must only expose compatibility wrappers.
- Higher-level simplification: pairwise helpers become `format -> IR -> format`, so future bugs are fixed in the IR owner instead of adding local bridge branches.
- Retirement / falsifier: if any old pairwise helper continues to carry mapping logic after its slice lands, the slice is incomplete.
- Verdict: proceed with scoped IR for `Responses <-> Chat`; do not include Claude/Gemini in this workstream.

## Plan Pressure Test

- Owner / contract / retirement: proceed only if each migrated path removes active mapping logic from `openaicompat`.
- Architecture integrity / higher-level path: use `compatir` for request and response conversion before touching stream conversion.
- Verification scope: add IR unit tests plus existing wrapper/relay tests.
- Task executability: split request, response, and stream into separate commits.
- Pressure result: proceed.

## Plan-Time Complexity Check

- Target files: `service/openaicompat/*.go`, new `service/compatir/*.go`, relay OpenAI stream bridge files.
- Existing size / shape signals: `service/convert.go` is large and out of scope; avoid adding IR logic there.
- Owner fit: new package is a better owner than growing relay handlers or `openaicompat`.
- Add-in-place risk: adding more branches in pairwise converters would preserve duplicate owners.
- Better file boundary: new `service/compatir` package with request, response, stream, warning, and tests.
- Recommendation: add owner file/package, then reduce old files to wrappers.

## Task 1: Add CompatIR Core Types

Files:

- Create `service/compatir/types.go`
- Create `service/compatir/warnings.go`
- Create `service/compatir/types_test.go`

Why:

- Establish a single internal contract for Chat and Responses conversion.

Impact / Compatibility:

- No public API changes.
- No relay behavior changes yet.

Steps:

1. Write tests for content part, message, tool call, warning append, and stable tool ordering helpers.
2. Verify RED:

   ```bash
   PATH=/root/.local/go1.25.1/bin:$PATH go test ./service/compatir -run CompatIR -count=1
   ```

3. Implement minimal IR structs and helpers:
   - `Request`
   - `Response`
   - `Message`
   - `ContentPart`
   - `ToolDefinition`
   - `ToolChoice`
   - `ToolCall`
   - `Usage`
   - `ConversionWarning`
4. Verify GREEN with the same command.
5. Commit:

   ```bash
   git add service/compatir
   git commit -m "refactor: add compatir core types"
   ```

Repair Track:

- Root cause: conversion rules are duplicated across pairwise functions.
- Canonical owner: `service/compatir`.
- Minimal repair: add the new owner before migrating callers.
- Compat boundary: no runtime behavior changes.
- Verification: package tests only.

Retirement Track:

- Old owner: `service/openaicompat` pairwise mapping helpers.
- Active status: still active after Task 1.
- Deletion trigger: retired per converter slice in Tasks 2-4.

## Task 2: Migrate Request Conversion Through CompatIR

Files:

- Create `service/compatir/chat_request.go`
- Create `service/compatir/responses_request.go`
- Create `service/compatir/request_test.go`
- Modify `service/openaicompat/chat_to_responses.go`
- Modify `service/openaicompat/responses_chat_bridge.go`

Why:

- Make `Chat request <-> Responses request` use the same semantic owner.

Impact / Compatibility:

- Public wrapper functions keep identical signatures.
- Existing accepted request mappings remain compatible.
- Unsupported lossy fields produce warnings or errors according to IR policy.

Steps:

1. Write tests for:
   - text-only request
   - multimodal request
   - tool definitions
   - tool choice
   - tool result
   - assistant tool call
   - reasoning
   - parallel tool calls
   - lossy Responses fields warnings/errors
2. Verify RED:

   ```bash
   PATH=/root/.local/go1.25.1/bin:$PATH go test ./service/compatir -run 'Request|CompatIR' -count=1
   ```

3. Implement:
   - `FromChatRequest(*dto.GeneralOpenAIRequest) (*Request, error)`
   - `ToResponsesRequest(*Request) (*dto.OpenAIResponsesRequest, error)`
   - `FromResponsesRequest(*dto.OpenAIResponsesRequest) (*Request, error)`
   - `ToChatRequest(*Request) (*dto.GeneralOpenAIRequest, error)`
4. Replace old pairwise request converter bodies in `openaicompat` with wrapper calls into `compatir`.
5. Verify GREEN:

   ```bash
   PATH=/root/.local/go1.25.1/bin:$PATH go test ./service/compatir ./service/openaicompat -run 'Request|Responses|Chat|Bridge|CompatIR' -count=1
   ```

6. Commit:

   ```bash
   git add service/compatir service/openaicompat/chat_to_responses.go service/openaicompat/responses_chat_bridge.go
   git commit -m "refactor: route chat responses requests through compatir"
   ```

Repair Track:

- Root cause: request conversion rules are duplicated and silently lossy.
- Canonical owner: `service/compatir`.
- Minimal repair: wrapper signatures remain; mapping logic moves to IR.
- Compat boundary: relay/service signatures unchanged.
- Verification: IR request tests plus existing bridge tests.

Retirement Track:

- Old owner: request mapping logic in `openaicompat`.
- Active status after task: wrapper-only.
- Deletion trigger: no remaining private request mapping helpers in `openaicompat`.

## Task 3: Migrate Non-Stream Response Conversion Through CompatIR

Files:

- Create `service/compatir/chat_response.go`
- Create `service/compatir/responses_response.go`
- Create `service/compatir/response_test.go`
- Modify `service/openaicompat/responses_to_chat.go`
- Modify `service/openaicompat/responses_chat_bridge.go`

Why:

- Preserve text, tool calls, ordering, usage, and warnings in one response owner.

Impact / Compatibility:

- Public response JSON remains the same.
- Existing mixed text + tool call fix must remain covered.

Steps:

1. Write tests for:
   - Chat text response to Responses
   - Chat tool-only response to Responses
   - Chat mixed text + tool response to Responses
   - Responses text response to Chat
   - Responses tool-only response to Chat
   - Responses mixed text + tool response to Chat
   - usage normalization
   - stable output order
2. Verify RED:

   ```bash
   PATH=/root/.local/go1.25.1/bin:$PATH go test ./service/compatir -run 'Response|CompatIR' -count=1
   ```

3. Implement:
   - `FromChatResponse(*dto.OpenAITextResponse) (*Response, error)`
   - `ToResponsesResponse(*Response, original *dto.OpenAIResponsesRequest, id string) (*dto.OpenAIResponsesResponse, *dto.Usage, error)`
   - `FromResponsesResponse(*dto.OpenAIResponsesResponse) (*Response, error)`
   - `ToChatResponse(*Response, id string) (*dto.OpenAITextResponse, *dto.Usage, error)`
4. Replace old pairwise non-stream response converter bodies in `openaicompat` with wrapper calls into `compatir`.
5. Verify GREEN:

   ```bash
   PATH=/root/.local/go1.25.1/bin:$PATH go test ./service/compatir ./service/openaicompat ./relay/channel/openai -run 'Response|Responses|Chat|Bridge|CompatIR' -count=1
   ```

6. Commit:

   ```bash
   git add service/compatir service/openaicompat
   git commit -m "refactor: route chat responses through compatir"
   ```

Repair Track:

- Root cause: response conversion had multiple local owners and dropped mixed output shapes.
- Canonical owner: `service/compatir`.
- Minimal repair: response wrappers delegate to IR.
- Compat boundary: output shapes remain unchanged.
- Verification: IR response tests plus relay handler tests.

Retirement Track:

- Old owner: response mapping logic in `openaicompat`.
- Active status after task: wrapper-only.
- Deletion trigger: no remaining private non-stream response mapping helpers in `openaicompat`.

## Task 4: Migrate Stream Event Conversion Through CompatIR

Files:

- Create `service/compatir/stream.go`
- Create `service/compatir/stream_test.go`
- Modify `relay/channel/openai/chat_via_responses.go`
- Modify `relay/channel/openai/responses_via_chat.go`

Why:

- Stream conversions currently encode event semantics inside relay handlers.
- Moving event normalization to IR reduces handler-specific bugs.

Impact / Compatibility:

- SSE event names and payload shapes remain unchanged.
- Relay handlers still own HTTP/SSE writing and upstream IO.
- `compatir` owns event semantic conversion only.

Steps:

1. Write tests for:
   - Chat content delta to Responses output text delta
   - Chat tool call delta to Responses function call events
   - Chat usage chunk to IR usage
   - Responses output text delta to Chat delta
   - Responses function call events to Chat tool call delta
   - Responses completed event to Chat stop chunk
   - mixed text + tool stream
2. Verify RED:

   ```bash
   PATH=/root/.local/go1.25.1/bin:$PATH go test ./service/compatir -run 'Stream|CompatIR' -count=1
   ```

3. Implement IR stream event types and conversion helpers without taking over HTTP/SSE writes.
4. Update relay stream handlers to call `compatir` helpers for event transformation.
5. Verify GREEN:

   ```bash
   PATH=/root/.local/go1.25.1/bin:$PATH go test ./service/compatir ./relay/channel/openai -run 'Stream|Responses|Chat|Bridge|CompatIR' -count=1
   ```

6. Commit:

   ```bash
   git add service/compatir relay/channel/openai
   git commit -m "refactor: route chat responses streams through compatir"
   ```

Repair Track:

- Root cause: stream semantic conversion is embedded in relay IO handlers.
- Canonical owner: `service/compatir` for event semantics, relay for IO.
- Minimal repair: extract semantic event transformation only.
- Compat boundary: SSE output contract unchanged.
- Verification: IR stream tests plus existing relay stream tests.

Retirement Track:

- Old owner: semantic conversion blocks in relay stream handlers.
- Active status after task: relay writes events, `compatir` decides semantic mapping.
- Deletion trigger: no handler-local semantic tool/text conversion branches except IO orchestration.

## Task 5: Final Regression, Push, and Image Build

Files:

- No code files unless verification exposes a defect.
- Update plan/evidence only if Aegis checkpoint records are used.

Why:

- Confirm behavior and publish deployable artifact.

Steps:

1. Run full verification:

   ```bash
   PATH=/root/.local/go1.25.1/bin:$PATH go test ./service/... ./relay/... -count=1
   git diff --check
   ```

2. Check lingering mapping logic:

   ```bash
   rg -n "func .*RequestTo.*Request|func .*ResponseTo.*Response|function_call|output_text" service/openaicompat service/compatir relay/channel/openai
   ```

3. Commit any final verification-only adjustments.
4. Push:

   ```bash
   git push origin HEAD:codex/responses-chat-bridge
   ```

5. Confirm GHCR workflow:

   ```bash
   gh run list --repo Gjcgghgcbbjj/new-api --branch codex/responses-chat-bridge --limit 3
   ```

Repair Track:

- Root cause: previous bridge fragility from pairwise owner sprawl.
- Canonical owner: `service/compatir`.
- Verification: full service/relay tests and Actions image build.

Retirement Track:

- Old owner: `openaicompat` and relay-local semantic conversion.
- Final state: wrappers and IO orchestration remain; mapping semantics live in `compatir`.
- Follow-up trigger: if Claude/Gemini are later included, write a separate plan and do not reuse this plan as authority.

## Risks

- IR can become a second DTO layer if old mapping helpers are not retired.
- Stream conversion may need a narrower first pass if event semantics are more coupled to SSE writes than expected.
- Unsupported field policy may reveal existing client requests that relied on silent loss.

## Rollback

- Revert the per-task commit that introduced the failing slice.
- Since no DB/API/route changes are allowed, rollback is code-only.

## ADR Signal

This introduces a durable canonical owner for `Responses <-> Chat` conversion. After implementation, create or update an ADR describing:

- Why `service/compatir` owns this boundary.
- Why Claude/Gemini were excluded from v1.
- What old pairwise owners were retired.
- What warnings/errors define lossy conversion policy.
