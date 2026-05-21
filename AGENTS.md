# AGENTS.md - PKM with Codex

This repository is the Product Brain control workspace for Randy's PKM/Tolaria system.

## Non-negotiable Paths

- Product Brain CLI working directory: `/Users/randyhereman/Young Human Club Dropbox/01. Projects/PKM with Codex`
- Tolaria vault file target: `/Users/randyhereman/Young Human Club Dropbox/02. Areas/PKM - Tolaria`

## Product Brain Context

- Treat Product Brain as the source of truth for building and system decisions.
- Run all `pb` commands from this repository root unless Randy explicitly says otherwise.
- This repository pins the intended PB workspace via `.productbrain/config.local.json`.
- Before using PB in a new session, verify the resolved profile with `pb profile list`.
- Expected active PB profile for this workspace: `randy-s-pkm`.
- If `pb profile list` does not report `activeSource: local` and `active: randy-s-pkm`, stop and report the mismatch before running any PB read or write command.

## Product Brain SSOT Discipline

Protecting Product Brain as the source of truth is the highest-priority operating responsibility in this workspace.

- Before doing substantive work, orient from PB and verify the relevant decisions, standards, work packages, tensions, and features are current.
- If PB is missing an important rule, decision, work package, blocker, or scope boundary needed for the task, update PB before treating that information as authoritative.
- Operate from PB, not from chat memory alone. Chat can clarify intent, but PB is where durable system truth must live.
- During work, capture reusable system learnings, decisions, standards, risks, and scope changes in PB as they become clear.
- At the end of work, PB must reflect the truth of what was decided, changed, blocked, completed, deferred, and learned.
- Do not leave important system state only in code, markdown, Tolaria, terminal output, or conversation history.
- When PB and local artifacts disagree, pause and reconcile the mismatch before continuing.

## Tolaria Vault Access

- Read and write Tolaria notes in `/Users/randyhereman/Young Human Club Dropbox/02. Areas/PKM - Tolaria`.
- Do not `cd` into the Tolaria vault to run PB commands.
- When a shell command needs to inspect or edit vault files, use the vault path as the command `workdir` or as an explicit file path only for file operations.
- When a shell command needs PB context, use this repository root as the command `workdir`.

## Failure Mode To Avoid

The Tolaria vault does not own the PB workspace configuration. Running `pb` from the vault can fall back to the global `product-brain` profile and read or write the wrong workspace. That is a bug-class operational error.

Safe pattern:

1. Run PB commands from `/Users/randyhereman/Young Human Club Dropbox/01. Projects/PKM with Codex`.
2. Read/write vault Markdown files under `/Users/randyhereman/Young Human Club Dropbox/02. Areas/PKM - Tolaria`.
3. Keep PB workspace state and vault file state conceptually separate unless a task explicitly bridges them.

## Current Build Frame

- We are building Randy's OS around Product Brain and Tolaria.
- Slack is the first source adapter, but the model must support many future sources.
- Build for our own workflow first, while keeping adapter boundaries clean enough to open source later.

## PB vs Tolaria Boundary

- Product Brain is the source of truth for the open-source system we are building: product decisions, standards, principles, business rules, tensions, features, architecture, flows, patterns, questions, tasks, adapter contracts, and implementation learnings.
- Tolaria is Randy's personal PKM output surface: captured sources, processed notes, personal resources, signals, tasks, issues, decisions, and domain organization bespoke to Randy.
- Do not put Randy's personal captured content into PB unless it is about the PKM OS itself.
- Do put reusable system learnings, adapter rules, classification decisions, and automation behavior into PB continuously.

## Markdown Artifact Placement

When creating any new `.md` file, decide the destination before writing it.

- Default personal PKM artifacts to the Tolaria vault, not this repository.
- Put research reports, source analyses, company/product analyses, processed notes, personal resources, personal signals, and general reference documents in Tolaria.
- Use Tolaria folders by workflow object:
  - `00-inbox/` for uncertain or unprocessed captures.
  - `10-projects/` for active personal/project outcome notes.
  - `20-areas/` for ongoing area/domain notes.
  - `30-resources/` for reusable references, research reports, and evergreen analyses.
  - `40-archives/` for inactive material.
- Only create Markdown files in this Product Brain control repository when the file is explicitly about the PKM OS/Product Brain system itself: repo documentation, implementation plans, system standards, adapter contracts, architecture notes, build decisions, or reusable automation behavior.
- If a Markdown artifact could plausibly belong in either place, choose Tolaria unless it directly changes or documents how the PKM OS is built.
- Never create a repo-local `docs/` Markdown file for personal research or external-company analysis unless Randy explicitly asks for repo documentation.

## Tolaria Classification Model

Classify personal PKM items with three independent axes:

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

Use topics as flexible tags or linked topic notes, such as `ai-tools`, `organization-design`, `competitors`, `ai-image-generation`, `product-ops`, `knowledge-management`, `moving`, `nutrition`, and `leadership-circle`.

Default uncertain captures to `type: Source`, `domain: Inbox / Unknown` or `Research Landscape`, and `status: Inbox`. Do not invent certainty.

## Source Adapter Workflow

Slack is the first adapter. The target behavior is: Randy captures freely; the OS enriches, classifies, writes confident items to Tolaria, and asks only when save intent or classification is genuinely ambiguous.

Every source adapter should process items through this pipeline:

1. Ingest native items with provenance: adapter id, external id, author, timestamp, permalink, raw text, files, urls, and thread/context.
2. Normalize into a common candidate model before any Tolaria write.
3. Enrich external links/media according to save intent, not just the first visible URL.
4. Classify by type, domain, topic, confidence, and processing status.
5. Write high-confidence personal PKM outputs to Tolaria.
6. Queue uncertain items for clarification instead of guessing.
7. Capture reusable system rules, decisions, features, tensions, and learnings in PB.

Important enrichment rules:

- LinkedIn posts may require post text, author, outbound links, and sometimes comments when accessible or clearly relevant.
- If a LinkedIn post points to another page, preserve both the post and the linked page as related sources.
- YouTube links may require title, channel, transcript, key claims, timestamps, and description links.
- GitHub links require at least repo metadata and README-level understanding.
- Articles require title, author, date, thesis, and key points.
- If required context is inaccessible, mark the item as `needs_manual_processing` or `needs_clarification`; do not pretend the source is complete.
