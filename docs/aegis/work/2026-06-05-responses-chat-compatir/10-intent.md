# Responses Chat CompatIR - Intent

## TaskIntentDraft

- Requested outcome: Implement a canonical CompatIR owner for Responses <-> Chat conversions
- Goal: Route Responses <-> Chat request, response, and stream semantic conversions through service/compatir without changing public API behavior
- Success evidence:
- service/compatir tests plus service and relay regression tests pass; old pairwise mapping logic is retired or wrapper-only
- Stop condition: done when all planned slices pass verification; blocked on repeated test failures or scope expansion beyond Responses <-> Chat; needs-verification if tests cannot run
- Non-goals:
- Claude/Gemini IR migration
- database or pricing changes
- Scope: Responses <-> Chat conversions only
- Change kinds:
- architecture
- Risk hints:
- duplicate conversion owners and lossy field handling

## BaselineReadSetHint

- docs/aegis/plans/2026-06-05-responses-chat-compatir.md

## ImpactStatementDraft

- Compatibility boundary: no Claude/Gemini, database, quota, or route behavior changes
- Affected layers:
- service
- relay
- Owners:
- service/compatir
- Invariants:
- public API JSON, routing, quota, and pricing behavior remain unchanged
- Non-goals:
- Claude/Gemini IR migration
- database or pricing changes

These records are Method Pack drafts / hints, not authoritative runtime decisions.
