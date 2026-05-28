# AGENTS.md - Mindline Control Workspace

This repository is the Product Brain control workspace for **Mindline**.

Mindline is the product. It is a headless, provider-agnostic semantic ingestion engine that turns messy private source material into evidence-backed knowledge candidates, eval artifacts, review packets, and destination-ready proposals.

Product Brain and Tolaria matter, but they are not the product:

- Product Brain is the source of truth for building Mindline in this workspace.
- Product Brain may also become a premium downstream destination or authority consumer for Mindline outputs.
- Tolaria is Randy's current local PKM destination and adapter test surface.
- Other possible destinations include Notion, Obsidian, Linear, local folders, APIs, custom apps, or future Product Brain workspaces.

## Non-Negotiable Paths

- Product Brain CLI working directory: `/Users/randyhereman/Young Human Club Dropbox/01. Projects/PKM with Codex`
- Current Tolaria destination root: `/Users/randyhereman/Young Human Club Dropbox/02. Areas/PKM - Tolaria`

The repository path still says `PKM with Codex` for historical/local reasons. Do not infer product identity from the folder name. The product identity is Mindline.

## Product Brain Context

- Treat Product Brain as the source of truth for Mindline product, architecture, standards, work packages, decisions, tensions, and proof.
- Run all `pb` commands from this repository root unless Randy explicitly says otherwise.
- This repository pins the intended PB workspace via `.productbrain/config.local.json`.
- Before using PB in a new session, verify the resolved profile with `pb profile list`.
- Expected active PB profile for this workspace: `randy-s-pkm`.
- If `pb profile list` does not report `activeSource: local` and `active: randy-s-pkm`, stop and report the mismatch before running any PB read or write command.

## Product Brain SSOT Discipline

Protecting Product Brain as the source of truth is the highest-priority operating responsibility in this workspace.

- Before substantive work, orient from PB and verify the relevant decisions, standards, work packages, tensions, features, product strategy, and architecture constraints are current.
- If PB is missing an important rule, decision, work package, blocker, or scope boundary needed for the task, update PB before treating that information as authoritative.
- Operate from PB, not from chat memory alone. Chat can clarify intent, but PB is where durable system truth must live.
- During work, capture reusable system learnings, decisions, standards, risks, and scope changes in PB as they become clear.
- At the end of work, PB must reflect the truth of what was decided, changed, blocked, completed, deferred, and learned.
- Do not leave important system state only in code, markdown, Tolaria, terminal output, PostHog, or conversation history.
- When PB and local artifacts disagree, pause and reconcile the mismatch before continuing.

## Product Architecture Frame

Mindline sits between sources and destinations.

Sources are places content comes from:

- Slack
- Markdown files
- Notion exports
- transcripts
- web links
- future source adapters

Destinations are places outputs may go:

- Product Brain
- Tolaria
- Notion
- Obsidian
- Linear
- local folders
- APIs
- custom apps

The core product is not any one source or destination. The core product is the reusable processing system:

1. Ingest native source items with provenance.
2. Normalize into source-neutral candidates.
3. Decompose and structure documents.
4. Extract semantic atoms with evidence.
5. Evaluate quality with traces, answer keys, baselines, and readback gates.
6. Produce destination-neutral candidates, proposals, or write plans.
7. Let destination adapters map those outputs into a specific surface only after evidence and safety gates pass.

## Product Model Rules

- Build source-agnostic and destination-agnostic behavior by default.
- Never optimize core logic for one private `/temp` corpus, one Slack export, one workspace, one destination, one provider, one user, or one review UI unless the work explicitly declares that bounded scope.
- Product progress must generalize to the stated user, input, output, and workspace surface.
- Treat Tolaria-specific behavior as adapter-specific unless PB explicitly says it is a core Mindline rule.
- Treat Product Brain-specific proposal behavior as adapter-specific unless PB explicitly says it is a core Mindline rule.
- When adding a new feature, state which layer owns it: source adapter, normalized candidate, document structure, semantic extraction, evaluation/readback, review workflow, or destination adapter.
- If a decision would make Notion, Obsidian, Linear, Product Brain, or Tolaria harder to support later, capture the tradeoff in PB before implementing.
- Do not make destination writes, no-human approval, or autonomous action claims until the Chain says the required held-out accuracy, privacy, evidence, and safety gates are met.

## Default Eval-Driven Improvement Protocol

Product Brain is authoritative for this protocol. Current anchors include DEC-250, DEC-251, DEC-252, DEC-241, STR-3, PRI-1, BR-1, STD-17, DEC-64, FLO-1, and TEN-23.

Use this protocol for any work package, implementation, review, or proof that touches LLM calls, semantic extraction/classification, source enrichment, routing, review automation, autonomy claims, destination writes, user data/privacy, or measurable quality KRs.

Scale the protocol to risk. Mechanical changes inside a relevant work package do not need heavyweight new evals unless they are used as product proof or can affect quality, privacy, routing, autonomy, or destination behavior.

1. Define the outcome, guardrails, and evaluation projection before implementation.
2. Challenge first-round specs or plans once for a tighter, leaner, more product-general version, then reconcile that challenge against Product Brain.
3. Run against representative local fixtures or real private runtime data as allowed.
4. Produce or reference local trace/eval artifacts for every run used as proof.
5. Run readback or an equivalent claim gate to answer:
   - what evidence exists;
   - what changed versus baseline;
   - what cannot be generalized;
   - what claims are blocked;
   - what the next product-general improvement target is.
6. Compare before/after proof when claiming improvement.
7. Capture reusable learning, decisions, standards, or blockers on Chain.

Command success is not outcome success. A process exit code of `0` proves only that the command ran; product success requires the relevant eval/KR gates and guardrails to pass.

Eval projections for relevant proof must state intended users, input/source types, output or destination surfaces, workspace assumptions, provider/model assumptions, privacy boundary, sample status, held-out/generalization claim, KR thresholds, and guardrails.

Private or sample-bound proof is not generalizable proof. `/temp`, Randy Slack, one Tolaria folder, one Product Brain profile, one PostHog run, or one customer workspace can support discovery/debugging, but closure claims require a reusable fixture, held-out eval case, reusable rule, or an explicit non-generalizable flag.

A sample-specific fix must name the generalized product behavior it proves. If it cannot, treat it as local remediation rather than product progress.

Hosted observability such as PostHog is an observability and analysis plane, not the durable source of truth. Hosted telemetry must be explicit, metadata-only, allowlisted, and governed by PRI-1 and BR-1. It must not authorize product claims, destination writes, or no-human autonomy by itself.

Short term, Codex is the active operator that inspects eval artifacts, improves the system, and captures learnings. Long term, Mindline should aggregate eval runs and propose improvement work automatically; do not claim that exists until it is built and proven.

Do not add an `AGENTS.md` or process shortcut that bypasses Chain authority. If this protocol changes, update Product Brain first, then update instruction surfaces to point at the Chain truth.

## Tolaria Boundary

Tolaria is currently Randy's local PKM destination. It is useful for testing destination behavior, but it is not the product boundary.

- Read and write Tolaria notes only when the task explicitly targets Tolaria or Randy's personal PKM output.
- Do not `cd` into the Tolaria vault to run PB commands.
- When a shell command needs to inspect or edit vault files, use the vault path as the command `workdir` or as an explicit file path only for file operations.
- When a shell command needs PB context, use this repository root as the command `workdir`.
- Do not treat Tolaria folders, note types, or domain taxonomy as Mindline core schema.
- If a Tolaria rule seems reusable, capture it in PB as a candidate destination-adapter rule or core standard before generalizing it.

## Failure Mode To Avoid

The Tolaria vault does not own the PB workspace configuration. Running `pb` from the vault can fall back to the global `product-brain` profile and read or write the wrong workspace.

Safe pattern:

1. Run PB commands from `/Users/randyhereman/Young Human Club Dropbox/01. Projects/PKM with Codex`.
2. Read/write Tolaria files under `/Users/randyhereman/Young Human Club Dropbox/02. Areas/PKM - Tolaria` only when Tolaria is explicitly in scope.
3. Keep PB workspace state, Mindline product state, and destination file state conceptually separate unless a task explicitly bridges them.

## Markdown Artifact Placement

When creating any new `.md` file, decide the destination before writing it.

- Put Mindline product/system artifacts in this repository when they document implementation plans, system standards, adapter contracts, architecture notes, build decisions, reusable automation behavior, or Product Brain work package artifacts.
- Put Randy personal PKM artifacts in Tolaria only when the requested output is personal knowledge material rather than Mindline product work.
- Do not default to Tolaria for product documentation.
- Do not default to this repository for personal research, private source analysis, or destination-specific content.
- If an artifact could plausibly belong in either place, decide based on ownership:
  - product/system truth -> PB or this repository;
  - personal PKM output -> selected destination adapter, currently often Tolaria;
  - destination-neutral run output -> explicit `--out` artifact directory.

## Destination Adapter Notes

Destination adapters translate Mindline outputs into surface-specific representations. They must not shape the core semantic model unless PB explicitly promotes the pattern.

Examples:

- Product Brain adapter: proposal/authority-aware output, external identity, Chain write constraints, draft-first behavior.
- Tolaria adapter: Markdown note output for Randy's local vault.
- Linear adapter: issue/project/update output if later built.
- Notion or Obsidian adapter: page/block/file output if later built.

Adapter rules belong in PB as standards, decisions, or work packages when they become reusable.

## Tolaria Classification Model

The following taxonomy is adapter-specific guidance for Randy's current Tolaria destination. It is not Mindline core schema.

Classify Tolaria personal PKM items with three independent axes:

1. Type: what kind of workflow object this is.
2. Domain: where in Randy's life/work map it belongs.
3. Topic: what it is about.

Use a small type set:

- `Source`: raw external item captured for processing.
- `Signal`: extracted insight, claim, pattern, or learning.
- `Task` / `Commitment`: action someone needs to take.
- `Issue`: bug, problem report, or friction point.
- `Project`: active outcome with a finish line.
- `Area`: ongoing responsibility or domain.
- `Resource`: reusable reference.
- `Decision`: settled choice with rationale.
- `Note`: general fallback.

Use domains as stable areas, not folders-only categories:

- `Product Brain`
- `Product Talk`
- `Steady Finance App`
- `Tolaria PKM OS`
- `UpCurrent`
- `ZDHC x Saprolab`
- `Life Admin`
- `Health`
- `Research Landscape`
- `Inbox / Unknown`

Default uncertain Tolaria captures to `type: Source`, `domain: Inbox / Unknown` or `Research Landscape`, and `status: Inbox`. Do not invent certainty.

## Source Adapter Workflow

Slack is the first source adapter. The product model must support many future sources.

Every source adapter should process items through this pipeline:

1. Ingest native items with provenance: adapter id, external id, author, timestamp, permalink, raw text, files, urls, and thread/context.
2. Normalize into a common candidate model before any destination-specific write or proposal.
3. Enrich external links/media according to save intent, not just the first visible URL.
4. Classify by type, domain/topic or destination-neutral equivalent, confidence, and processing status.
5. Produce evidence-backed candidates or proposals.
6. Write to a destination only when that destination is explicitly selected and the applicable safety/evidence gates pass.
7. Queue uncertain items for clarification instead of guessing.
8. Capture reusable system rules, decisions, features, tensions, and learnings in PB.

Important enrichment rules:

- LinkedIn posts may require post text, author, outbound links, and sometimes comments when accessible or clearly relevant.
- If a LinkedIn post points to another page, preserve both the post and the linked page as related sources.
- YouTube links may require title, channel, transcript, key claims, timestamps, and description links.
- GitHub links require at least repo metadata and README-level understanding.
- Articles require title, author, date, thesis, and key points.
- If required context is inaccessible, mark the item as `needs_manual_processing` or `needs_clarification`; do not pretend the source is complete.

## Review Standard For This Workspace

Before opening, updating, or merging a PR that changes Mindline product behavior:

- Check PB authority first.
- Check whether the work is source/destination/provider agnostic or explicitly adapter-scoped.
- Run the relevant tests and readback/eval proof.
- Capture durable learnings or corrections in PB.
- Use LOOP reviewer sign-off when the change affects product direction, agent instructions, evaluation, trust, privacy, destination readiness, or autonomy claims.
