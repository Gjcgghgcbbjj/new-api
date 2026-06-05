# Responses Chat CompatIR - Checkpoint

- Task ID: 2026-06-05-responses-chat-compatir
- Current todo: Task 1: add CompatIR core types
- Active slice: Create service/compatir core structs, warnings, and tests
- Blocked on: none
- Next step: Write core type tests

## DriftCheckDraft

- Scope status: Task 1 stayed inside core types only
- Compatibility status: No runtime conversion behavior changed
- Retirement status: Old pairwise converters still active; retirement begins Task 2
- New risk signals:
- none
- Advisory decision: continue

## Checkpoint Update

- Current todo: Task 2: migrate request conversion through CompatIR
- Active slice: Request conversion IR tests and implementation
- Completed todos:
- Task 1: added CompatIR core types, warnings, stable tool ordering, and tests
- Evidence refs:
- task1-compatir-core-tests
- Blocked on: none
- Next step: Write request conversion tests in service/compatir

## DriftCheckDraft

- Scope status: Task 2 stayed inside Responses <-> Chat request conversion
- Compatibility status: Public wrappers and relay tests remained compatible
- Retirement status: Chat->Responses request direct mapper retired; Responses->Chat request exported function is wrapper; response helpers remain for Task 3
- New risk signals:
- none
- Advisory decision: continue

## Checkpoint Update

- Current todo: Task 3: migrate non-stream response conversion through CompatIR
- Active slice: Non-stream response conversion IR tests and implementation
- Completed todos:
- Task 2: migrated Chat/Responses request conversion through CompatIR and retired request-only openaicompat mapping logic
- Evidence refs:
- task2-request-compatir-tests
- Blocked on: none
- Next step: Write response conversion tests in service/compatir
