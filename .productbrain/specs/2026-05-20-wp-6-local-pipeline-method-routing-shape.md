# WP-6 Local Pipeline Runner and Method Routing Shape

Shape version: `MINDLINE-WP6-SHAPE-V1`
Date: 2026-05-20
Status: Shape draft for LOOP review. Not delivery authority.

## Product Brain Authority

- Product: `PROD-1` - Mindline.
- Sequence: `DEC-6` - contract fixtures before Slack dry-run before destination boundary.
- Method boundary: `DEC-15` - Mindline core is method-neutral; BASB/PARA/CODE is the first method profile, not core architecture.
- Prior source boundary: `DEC-7`, `DEC-8`, `DEC-10`, `DEC-11`, WP-4 Slack dry-run.
- Prior destination boundary: `DEC-12`, `DEC-13`, WP-5 destination adapter contract and Tolaria dry-run boundary.
- Git state: PR #2 contains WP-5 delivery and is open/clean. WP-6 spec/plan may be prepared now, but implementation must depend on WP-5 being merged or explicitly rebased onto its branch.
- Standards: `STD-5`, `STD-6`, `STD-7`, `STD-10`, `STD-11`, `STD-12`.

## Problem

Mindline now has isolated local source, core, and destination dry-run slices, but it does not yet prove that a capture can move through the whole headless system as one coherent local run.

The next risk is architectural: if we connect Slack directly to Tolaria-style output, BASB/PARA/CODE and first-source assumptions can leak into the core. If we over-generalize too early, we will build abstract framework machinery before proving an actual useful flow.

Randy also wants fewer approval interruptions, without weakening safety. That requires explicit run modes and permission boundaries, not ad hoc human approval at every CLI step.

The system also needs flexible processing paths. A captured item may contain a YouTube link, LinkedIn post, website, PDF, or multiple related URLs. Each content unit may need a different processor, and user preferences should control what is required, optional, or blocked.

## Selected Direction

WP-6 should build a local-only pipeline runner that composes the existing slices:

```text
source input -> normalized candidates -> core processing -> method profile policy -> destination dry-run operations
```

The first method profile is `basb-para-code`, because it matches Randy's current workflow. It is a profile/policy layer, not core logic. The profile may influence routing, visible lanes, organization hints, and destination planning inputs. It must not replace source normalization, safety, provenance, or destination operation contracts.

WP-6 should add a processor-routing boundary, but only enough to plan and represent processing paths locally. It should not implement live YouTube, LinkedIn, PDF, browser, network, or LLM processors. It should detect content units from local candidate data and produce a deterministic processing plan with processor steps such as `web_page_metadata`, `youtube_transcript_required`, `linkedin_post_context`, `pdf_text_extract`, or `manual_processing_required`.

The local pipeline runner should emit reviewable artifacts only under `--out`, with no live source API, no live destination write, no network requirement, no auth, and no hidden state.

## Candidate Work Package

Name: `WP: Mindline local pipeline runner and method-routing boundary`

Work type: `bet`

Appetite: `small`

Lifecycle target for this loop: `shaped` with signed spec and plan. Delivery starts later.

## In Scope

- A versioned local pipeline input envelope that can reference existing Slack dry-run exports or candidate/result files.
- A pipeline runner command shape, likely:

```text
mindline pipeline dry-run <pipeline-input.json> --method basb-para-code --destination tolaria --out <dir>
```

- A method profile contract with a first `basb-para-code` profile.
- A processor routing contract that detects content units and emits a processing plan.
- Local deterministic processor-plan fixtures for text-only, YouTube URL, LinkedIn URL with outbound website, PDF URL, mixed URL set, secret-like content, private provenance, and inaccessible/unknown source.
- Composition of existing source/core/destination dry-run behavior into one deterministic summary.
- Run-mode/approval policy vocabulary for future automation: `dry_run`, `reviewable_write`, `live_write`, `destructive`.
- Tests proving no network/auth/live Slack/live Tolaria behavior.

## Out of Scope

- No live Slack API.
- No live Tolaria vault writes.
- No real YouTube transcript fetching.
- No LinkedIn scraping.
- No PDF downloading or parsing from the network.
- No browser automation.
- No LLM calls.
- No database/auth/provider integration.
- No full Zettelkasten implementation.
- No user-facing UI.
- No merge or delivery implementation before WP-5 is merged or explicitly used as the base.

## Product Model Fit

This is a canonical Mindline engine slice, not a Randy-only script:

- Source adapters remain responsible for normalized capture candidates.
- The core remains responsible for provenance, safety, enrichment status, and routing state.
- Processor routing becomes a portable boundary for deciding what enrichment steps are needed.
- Method profiles become portable policy modules for organizing and expressing knowledge.
- Destination adapters remain responsible for translating operations into tool-specific dry-run/write surfaces.

## Outcome Proof

The signed spec should be considered successful when it makes WP-6 fail-able:

1. Reviewers can tell whether method profiles are separate from core logic.
2. Reviewers can tell whether processor routing is flexible enough for YouTube, LinkedIn, website, PDF, mixed-source, unknown, private, and secret-like captures.
3. Reviewers can tell whether approval/run modes reduce human approvals without allowing unsafe live actions.
4. Reviewers can tell whether WP-6 is local-only and cannot write to Tolaria or call live APIs.
5. The implementation plan can be executed with TDD after WP-5 merge/rebase.

## Open Risks

- If the profile abstraction is too broad, WP-6 may become framework work instead of pipeline proof.
- If the profile abstraction is too narrow, BASB/PARA/CODE leaks into the core and future Zettelkasten support becomes expensive.
- If processor routing requires real network processors now, the slice becomes too large and approval-heavy.
- If approval modes are only documented but not represented in pipeline output, future live automation will remain ambiguous.

