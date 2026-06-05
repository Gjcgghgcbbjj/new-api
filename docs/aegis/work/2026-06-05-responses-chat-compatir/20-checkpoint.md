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
