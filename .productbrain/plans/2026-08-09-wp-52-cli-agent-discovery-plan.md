# WP-52 — CLI-only scoped agent discovery v1 plan

Date: 2026-08-09  
Authority: WP-52, DEC-439, DEC-437  
Signed Spec: `.productbrain/specs/2026-08-09-agent-discovery-contract-v1.md` SHA-256 `009c98ca9dd975bd0472fe7127ff79fb8533493601ea593b601b527631726d0d`  
Plan version: `wp52-plan-v1`

## Delivery budget

Small appetite; one vertical slice. Preserve the Spec's claim/exclusion
boundary. The shaping audit's only warning is accepted: the solution elements
remain inline because each is a tightly coupled part of one CLI journey, not an
independently valuable work package.

## Sequence

0. **Pre-build authority**
   - Complete two clean Plan reviews, then bind this exact Plan path/SHA in
     WP-52's build contract.
   - Run and reconcile `pb audit WP-52 --phase handoff`; obtain Delivery
     Authority, then and only then move WP-52 to `building` or edit code.

1. **Shared contract and focused CLI**
   - Add shared route/trust/feedback constants and additive capability fields.
   - Add `agent help|--help`, `agent discover` with exact tuple validation and
     config propagation, and `agent feedback-token`.
   - Add closed repair-safe agent errors. Keep invalid-command compatibility.
   - Project the shared contract into the installed skill; blind proof denies it.

2. **Run-bound hydration**
   - Add an agent-state read that validates one active
     run/scope/lens/agent/record candidate tuple.
   - Add `POST /v1/scoped/get`, client input/output, and scoped CLI flags.
   - Hydrate only after authorization. Label scoped and unscoped outputs.

3. **Diagnostics and route classification**
   - Refactor compact authorization to retain only content-free pre/post counts.
   - Add required v0.4 abstention classes without changing ranking or gates.
   - Label memory and legacy routes as unapproved; preserve v0.2/v0.3 behavior.

4. **Executable proof**
   - Unit/contract tests: help, discovery positive/negatives, two-config
     separation, capability/skill drift, token entropy, closed errors.
   - State/service tests: scoped-get positive and complete negative matrix,
     zero mutation, retry/conflict/reversal/restart/isolation.
   - Retrieval tests: all abstention classes, degradation, route labels, golden
     compatibility, no rejected-evidence leakage.
   - Installer tests: candidate smoke and failure restoration.
   - Add strict content-free proof/latency receipt validation with private HMAC
     commitments and deterministic negative fixtures.

5. **Freeze and review**
   - Run formatting and focused suites, commit the candidate, and freeze its SHA.
   - Against that exact commit run `go test ./...`, `go vet ./...`,
     `go test -race ./...`, diff check, secret scan, and latency harness, then two
     clean Product/User Job, Architecture, Chain, Delivery Quality, and
     Risk/Safety tree-review passes.
   - Any defect or committed generated artifact changes the tree, requires a new
     commit/SHA, and restarts all proof and clean reviews.

6. **Install and blind proof**
   - Transactionally install the exact reviewed binary and prove status,
     capabilities, discovery, rollback/re-upgrade, and canonical evidence/state
     fingerprints.
   - Freeze a pre-existing scope/lens before questions and create only a fresh
     non-Codex actor. The blind harness denies skill access while leaving every
     receipt-tracked installed artifact byte-identical.
   - Give a fresh agent one discovery command and an answerable question; require
     citation, selective get, answer, token, exact retry, and conflict rejection.
   - Run an absent question requiring typed abstention and zero get, feedback,
     or memory fallback. Keep transcript private; validate structural receipt.

7. **Close**
   - Record exact tree/binary/receipt commitments and bounded outcome in Chain.
   - Mark WP-52/DEC-439 validated only for the proven private staging boundary,
     run Close review twice on the unchanged tree, verify entries, and close the
     PB session. Preserve excluded follow-ups as future work.

## Stop/restart conditions

Return to Spec/Chain authority for any change to identity, source/destination,
ranking/thresholds, canonical evidence, feedback blast radius, privacy/public
proof, installer mutation set, claim boundary, or exclusions. Stop install on
any failed test, review blocker, secret finding, rollback mismatch, or evidence
fingerprint change.
