# WP-6 Local Pipeline Runner and Method Routing Spec

Spec version: `MINDLINE-WP6-SPEC-V12`
Date: 2026-05-20
Status: Revised draft for LOOP Spec review. Not delivery authority until signed in Product Brain.

## Product Brain Authority

- Product: `PROD-1` - Mindline.
- Shape: `MINDLINE-WP6-SHAPE-V1`.
- Method boundary: `DEC-15` - Mindline core is method-neutral; BASB/PARA/CODE is the first method profile, not core architecture.
- Source sequence: `DEC-6` - normalized candidate fixtures before Slack dry-run before destination boundary.
- Source adapter authority: `DEC-7`, `DEC-8`, `DEC-10`, `DEC-11`.
- Destination adapter authority: `DEC-12`, `DEC-13`, WP-5.
- Standards: `STD-5`, `STD-6`, `STD-7`, `STD-10`, `STD-11`, `STD-12`.
- Git dependency: WP-5 delivery is in PR #2, currently open and merge-clean. WP-6 delivery must not start from `main` until PR #2 is merged, or until an explicit branch-base decision says to build on top of `codex/wp-5-destination-dry-run`.

Important Chain hygiene note: there is an accidental dropped `WP-6` capture from the WP-5 close-out. This spec uses `WP-6` as the human-readable next package number, but the durable Product Brain work item must be created or updated only after this spec signs off and must not treat the dropped accidental entry as authority.

Until that governed Product Brain work item exists, required artifact examples must use current real authority ids only. After the real work item is created, implementation fixtures may add that real id to `authority_ids`; they must not use the dropped accidental capture as authority.

## Problem

Mindline has proven isolated dry-run slices:

1. candidate validation and core routing;
2. Slack-like local source normalization;
3. destination-neutral operations and Tolaria dry-run planning.

It has not proven the full local headless flow. Without a pipeline runner, users cannot see how a capture moves from source input through candidate processing, method policy, processor routing, and destination dry-run output in one deterministic run.

Two architecture risks must be controlled before live Slack or real enrichment processors:

1. BASB/PARA/CODE could leak into core logic instead of staying a user-selectable method profile.
2. Processor-specific behavior for YouTube, LinkedIn, websites, PDFs, mixed links, unknown sources, private provenance, and secret-like content could become ad hoc instead of policy-driven and testable.

Approval friction is also a product problem. Randy should not approve every routine local step, but Mindline must still distinguish safe dry-run actions from reviewable writes, live writes, and destructive actions.

## Selected Approach

Add a local-only pipeline dry-run runner that composes existing contracts and introduces two new boundaries:

1. **Method profile boundary** - a versioned policy module selected by method id. WP-6 includes only `basb-para-code`.
2. **Processor routing boundary** - a deterministic plan describing which processors would be needed for content units, without running live network processors.

The pipeline should run:

```text
pipeline input -> method profile loading -> source normalization or candidate/result loading -> SBOS processing -> processor routing plan with method policy -> destination dry-run plan -> artifact writer -> pipeline summary
```

WP-6 must not implement real network enrichment. It only detects content units already present in local input and emits a reviewable processing plan.

## CLI Contract

The command shape is:

```text
mindline pipeline dry-run <pipeline-input.json> --method basb-para-code --destination tolaria --out <dir>
```

The command must:

- require `--out`;
- require `--method`;
- require `--destination`;
- support only `--method basb-para-code` in WP-6;
- support only `--destination tolaria` in WP-6;
- write only under the explicit output directory;
- reject output at or under the protected Tolaria vault path, including symlinks and final-file symlink escapes;
- never call live Slack, network, browser, auth, LLM, database, or destination APIs;
- print the same deterministic summary JSON that it writes to `pipeline-summary.json`;
- include `run_mode: "dry_run"` in every pipeline-owned summary and artifact envelope. WP-5 destination operation envelopes keep their existing `write_mode: "dry_run"` field as the destination write-mode contract and do not add a separate `run_mode` field.

## Pipeline Input Contract

The versioned input envelope is:

```json
{
  "schema_version": "pipeline-input/v0.1",
  "run_mode": "dry_run",
  "source": {
    "kind": "slack_export",
    "path": "examples/slack/reverse-ordered-batch.json"
  },
  "method": {
    "id": "basb-para-code"
  },
  "destination": {
    "id": "tolaria"
  },
  "authority_ids": ["DEC-15", "DEC-6", "DEC-12", "DEC-13"]
}
```

Allowed `source.kind` values for WP-6:

- `slack_export` - load a local Slack-like export and normalize it using the WP-4 adapter.
- `candidate` - load one local normalized candidate JSON file.
- `candidate_batch` - load local candidate JSON files listed in `source.paths`.

`process_result` is intentionally out of scope for WP-6 because the pipeline must prove candidate-to-destination composition first. Existing destination dry-run input/result parsing remains owned by WP-5.

Path references must resolve relative to the pipeline input file directory, not process cwd. The local bundle root is the parent of the input directory when the input file lives under a directory named `inputs`; otherwise the local bundle root is the input file directory. Source paths and `local_artifacts[].path` values must resolve under the local bundle root after symlink evaluation. Symlink escapes outside the local bundle root must be rejected. The WP-6 fixture bundle root is `testdata/pipeline/`, so `testdata/pipeline/inputs/pipeline-text-only.json` may reference `../candidates/pipeline-text-only.json` but may not reference `../../private.json`.

Candidate fixture JSON used by `candidate` and `candidate_batch` sources must use this minimal local envelope:

```json
{
  "schema_version": "candidate/v0.1",
  "candidate_id": "pipeline-text-only",
  "source_adapter_id": "fixture",
  "external_id": "fixture:pipeline-text-only",
  "captured_at": "2026-05-20T09:00:00Z",
  "author": {
    "name": "Example Author"
  },
  "visibility": "public",
  "text": "Mindline should keep raw capture, method policy, and destination preview separate.",
  "urls": [],
  "local_artifacts": []
}
```

`visibility` values are `public` or `private`. `urls[]` entries must preserve input order and use `{ "url": "...", "kind": "youtube_url|linkedin_url|pdf_url|web_url|unknown" }`. `local_artifacts[]` entries use `{ "unit_url": "...", "kind": "youtube_transcript|linkedin_post_context|pdf_text|web_metadata", "path": "../artifacts/<file>.json" }`. WP-6 fixtures intentionally omit local artifacts.

Slack export fixtures must normalize into the same candidate envelope before SBOS processing. The Slack batch fixture must contain two messages in input order: one text-only message whose normalized id is `slack-text-only`, and one YouTube message whose normalized id is `slack-youtube-url`.

## Pipeline Summary Contract

The summary file is `pipeline-summary.json` with fields in this order:

```json
{
  "schema_version": "pipeline-summary/v0.1",
  "run_mode": "dry_run",
  "method_id": "basb-para-code",
  "destination_adapter_id": "tolaria",
  "source_kind": "slack_export",
  "candidate_count": 0,
  "result_count": 0,
  "operation_count": 0,
  "blocked_count": 0,
  "items": [],
  "authority_ids": ["DEC-15", "DEC-6", "DEC-12", "DEC-13"]
}
```

Authority propagation is deterministic for WP-6: the pipeline validates `authority_ids` from the input envelope and copies that ordered list unchanged into `pipeline-summary.json`, every `results/<record_id>.json` envelope, every `processor-plans/<record_id>.json`, every `destination-summary.json`, and every destination operation JSON. The artifact writer must not add, remove, sort, or infer authority ids. Until a real governed Product Brain work item exists, fixtures must use `["DEC-15", "DEC-6", "DEC-12", "DEC-13"]`. After the real work item exists, a later implementation revision may append that real id at the end of the input list and all derived artifacts must preserve the new order.

Authority validation is intentionally strict in WP-6. Input `authority_ids` must be a non-empty ordered list drawn from this allowlist: `DEC-15`, `DEC-6`, `DEC-7`, `DEC-8`, `DEC-10`, `DEC-11`, `DEC-12`, `DEC-13`, `STD-5`, `STD-6`, `STD-7`, `STD-10`, `STD-11`, `STD-12`, and the future real governed work item id after it exists. Unknown ids, malformed ids, duplicate ids, empty ids, and the dropped accidental `WP-6` id must be rejected before any output is written.

Each `items[]` entry must include:

- `source_candidate_id`;
- `record_id`;
- `state`;
- `visibility_lane`;
- `method_profile_id`;
- `processor_plan_path`;
- `destination_summary_path`;
- `blocked`;
- `blockers`.

Item values are deterministic:

- `source_candidate_id` is the original candidate id.
- `record_id` is the path-safe slug of `source_candidate_id`.
- `state` is `ready_for_destination` when the item has zero blockers, otherwise `needs_manual_processing`.
- `visibility_lane` is `visible` when the item is unblocked and safe to preview, otherwise `review_only`.
- `method_profile_id` is `basb-para-code`.
- `processor_plan_path` is `processor-plans/<record_id>.json`.
- `destination_summary_path` is `destinations/<record_id>/destination-summary.json`.
- `blocked` is `true` when blockers are non-empty.
- `blockers` is the ordered list of blocker reason strings emitted by processor routing and safety checks.

## Pipeline Result Contract

Each `results/<record_id>.json` file is a pipeline-owned result envelope. It bridges SBOS processing to processor planning and destination dry-run planning, but it does not replace the WP-5 destination input/result contract.

The unblocked text-only result must use this exact body:

```json
{
  "schema_version": "pipeline-result/v0.1",
  "run_mode": "dry_run",
  "record_id": "pipeline-text-only",
  "source_candidate_id": "pipeline-text-only",
  "state": "ready_for_destination",
  "visibility_lane": "visible",
  "method_profile_id": "basb-para-code",
  "title": "Processed source pipeline-text-only",
  "snapshot": "Mindline should keep raw capture, method policy, and destination preview separate.",
  "safety": {
    "private_provenance": false,
    "secret_like": false,
    "redaction_required": false
  },
  "blockers": [],
  "authority_ids": ["DEC-15", "DEC-6", "DEC-12", "DEC-13"]
}
```

Blocked result envelopes must use the same fields with `state: "needs_manual_processing"`, `visibility_lane: "review_only"`, `snapshot: ""`, and the fixture's ordered `blockers`. Private and secret-like fixture bodies must not appear in `snapshot`, `title`, `record_id`, `source_candidate_id`, or any other generated result field.

## Processor Routing Contract

Processor routing emits one processor plan per candidate/result:

```json
{
  "schema_version": "processor-plan/v0.1",
  "run_mode": "dry_run",
  "source_candidate_id": "candidate-youtube",
  "content_units": [
    {
      "unit_id": "url-1",
      "unit_type": "youtube_url",
      "source": "https://www.youtube.com/watch?v=example",
      "visibility": "public"
    }
  ],
  "steps": [
    {
      "processor_id": "youtube_transcript",
      "requirement": "required",
      "status": "planned",
      "reason": "youtube_url_requires_transcript_or_manual_processing"
    }
  ],
  "blockers": [],
  "authority_ids": ["DEC-15", "DEC-6", "DEC-12", "DEC-13"]
}
```

WP-6 supported content-unit detection:

- `youtube_url` for YouTube URLs.
- `linkedin_url` for LinkedIn post/article URLs.
- `pdf_url` for URLs ending in `.pdf` or carrying PDF-like path hints.
- `web_url` for regular HTTP(S) URLs.
- `text` for meaningful non-URL text content.
- `unknown` when content exists but cannot be classified.

WP-6 supported processor step ids:

- `text_capture_review`;
- `web_page_metadata`;
- `youtube_transcript`;
- `linkedin_post_context`;
- `pdf_text_extract`;
- `manual_processing_required`;
- `secret_skip`;
- `private_provenance_block`.

Requirement values:

- `required`;
- `optional`;
- `blocked`;
- `not_applicable`.

Status values:

- `planned`;
- `skipped`;
- `blocked`;

Routing rules:

- YouTube URL -> `youtube_transcript` required. If no transcript artifact is present locally, add `manual_processing_required` with `requirement: "blocked"`, `status: "blocked"`, and reason `missing_local_youtube_transcript`.
- LinkedIn URL -> `linkedin_post_context` required. If no local LinkedIn context artifact is present, add `manual_processing_required` with `requirement: "blocked"`, `status: "blocked"`, and reason `missing_local_linkedin_context`. If the candidate also contains outbound web/PDF/YouTube URLs, preserve related units and emit steps for each unit.
- PDF URL -> `pdf_text_extract` required. If no local PDF text artifact is present, add `manual_processing_required` with `requirement: "blocked"`, `status: "blocked"`, and reason `missing_local_pdf_text`.
- Web URL -> `web_page_metadata` required. If no local webpage metadata artifact is present, add `manual_processing_required` with `requirement: "blocked"`, `status: "blocked"`, and reason `missing_local_web_metadata`.
- Text-only capture -> `text_capture_review` required.
- Unknown content -> `manual_processing_required` with `requirement: "blocked"`, `status: "blocked"`, and reason `unknown_content_type`.
- Secret-like content -> `secret_skip` blocked and no destination preview.
- Private provenance -> `private_provenance_block` blocked for publish, with no visible Tolaria Inbox output unless the core state already requires attention and the body is safe/redacted.

For WP-6, external content units are considered inaccessible unless the input fixture provides an explicit local artifact reference for that unit. Inaccessible required context makes the item blocked, increments `blocked_count`, and suppresses preview Markdown for that item. The destination summary may still be written as a blocked dry-run diagnostic, but it must not contain fabricated source understanding.

Multiple missing external artifacts on the same item produce one processor step per detected content unit plus one `manual_processing_required` step. The `manual_processing_required.reason` must be `missing_required_local_artifacts` when more than one artifact is missing. The plan-level `blockers[]` must still include every specific missing-artifact reason in content-unit order.

The processor plan is planning-only. It must not fetch, scrape, parse, transcribe, browse, call an LLM, or access network.

## Method Profile Contract

Method profiles are policy modules. They may influence:

- organization hints;
- routing preferences;
- processor requirement defaults;
- destination visibility guidance;
- output section expectations;
- clarification behavior.

They may not own:

- source normalization schema;
- provenance truth;
- private/secret safety flags;
- core state names;
- destination operation schema;
- live write authorization.

WP-6 implements one profile:

```json
{
  "schema_version": "method-profile/v0.1",
  "method_id": "basb-para-code",
  "run_mode": "dry_run",
  "organize": {
    "default_model": "PARA",
    "source_note_sections": [
      "Snapshot",
      "Source Content",
      "Key Details",
      "Relevance",
      "Signals",
      "Related Sources",
      "Next Action"
    ]
  },
  "processor_policy": {
    "youtube_url": {
      "required_processor": "youtube_transcript",
      "missing_artifact_status": "blocked",
      "missing_artifact_reason": "missing_local_youtube_transcript"
    },
    "linkedin_url": {
      "required_processor": "linkedin_post_context",
      "missing_artifact_status": "blocked",
      "missing_artifact_reason": "missing_local_linkedin_context"
    },
    "pdf_url": {
      "required_processor": "pdf_text_extract",
      "missing_artifact_status": "blocked",
      "missing_artifact_reason": "missing_local_pdf_text"
    },
    "web_url": {
      "required_processor": "web_page_metadata",
      "missing_artifact_status": "blocked",
      "missing_artifact_reason": "missing_local_web_metadata"
    },
    "unknown": {
      "required_processor": "manual_processing_required",
      "missing_artifact_status": "blocked",
      "missing_artifact_reason": "unknown_content_type"
    }
  }
}
```

The profile id must appear in pipeline summaries and item outputs. The strings `PARA`, `CODE`, `BASB`, and Tolaria note-section rules must not appear in core source adapter packages or core SBOS state logic.

## Output Layout

With `--out <dir>`, the pipeline writes:

- `pipeline-summary.json`;
- `candidates/<candidate_id>.json` for normalized candidates when the source is `slack_export`;
- `results/<record_id>.json` for SBOS result envelopes;
- `processor-plans/<record_id>.json`;
- `destinations/<record_id>/destination-summary.json`;
- `destinations/<record_id>/operations/*.json`;
- `destinations/<record_id>/previews/*.md` only when the destination dry-run operation allows previews.

Only the artifact writer package may create WP-7 pipeline output files. The runner, processor router, method profile loader, source loaders, SBOS/core, and the new pipeline CLI route must return in-memory envelopes or operations to the writer. Existing legacy CLI file helpers for pre-WP-7 commands are out of scope; WP-7 must not add direct file creation to the `pipeline dry-run` CLI route. The planned writer package path is `internal/pipeline/artifacts`.

The artifact writer boundary must be enforced by tests or static scans against common Go write surfaces. Outside `internal/pipeline/artifacts`, WP-6 pipeline production code must not call `os.WriteFile`, `os.Create`, `os.CreateTemp`, `os.OpenFile`, `os.Mkdir`, `os.MkdirAll`, `ioutil.WriteFile`, `io.Copy` to file handles, or package-local helper functions whose purpose is file creation. The CLI pipeline route must be covered by a test proving it delegates to the pipeline runner/artifact writer and does not add a direct write path. Reads are allowed only for pipeline input, candidate fixtures, Slack export fixtures, and local artifact references that passed bundle-root validation.

All filenames must be deterministic and path-safe. Private or secret-like values must not appear in filenames, summary paths, operation ids, processor plans, or preview bodies.

File naming rules:

- `candidate_id` and `record_id` filenames use lowercase ASCII slugs.
- Allowed slug characters are `a-z`, `0-9`, and `-`.
- Replace every run of unsupported characters with one `-`.
- Trim leading and trailing `-`.
- If the resulting slug is empty, use `item`.
- Preserve input order. When two items resolve to the same slug, append `-2`, `-3`, and so on.
- All paths written into JSON summaries must be relative to `--out` and use `/` separators.
- Destination operation filenames must be `001-<operation_type>.json`, `002-<operation_type>.json`, and so on in operation order.

## Approval and Run-Mode Boundary

WP-6 must define and emit run-mode vocabulary:

- `dry_run` - local deterministic artifacts only; allowed for routine automation.
- `reviewable_write` - future mode for proposed writes requiring review; not implemented in WP-6.
- `live_write` - future mode for writing to external systems; not implemented in WP-6.
- `destructive` - future mode for deletion or irreversible mutation; not implemented in WP-6.

WP-6 CLI must reject any input run mode other than `dry_run`.

## Golden Fixture Matrix

Fixture inputs live in `testdata/pipeline/inputs/`. Golden expected outputs live in `testdata/pipeline/expected/<fixture-name>/`. Every fixture uses this input envelope unless explicitly stated:

```json
{
  "schema_version": "pipeline-input/v0.1",
  "run_mode": "dry_run",
  "source": {
    "kind": "candidate",
    "path": "../candidates/<fixture-name>.json"
  },
  "method": {
    "id": "basb-para-code"
  },
  "destination": {
    "id": "tolaria"
  },
  "authority_ids": ["DEC-15", "DEC-6", "DEC-12", "DEC-13"]
}
```

Candidate fixtures live in `testdata/pipeline/candidates/`. Candidate ids must exactly match the fixture stem.

Exact candidate fixture bodies:

```json
[
  {
    "schema_version": "candidate/v0.1",
    "candidate_id": "pipeline-text-only",
    "source_adapter_id": "fixture",
    "external_id": "fixture:pipeline-text-only",
    "captured_at": "2026-05-20T09:00:00Z",
    "author": {"name": "Example Author"},
    "visibility": "public",
    "text": "Mindline should keep raw capture, method policy, and destination preview separate.",
    "urls": [],
    "local_artifacts": []
  },
  {
    "schema_version": "candidate/v0.1",
    "candidate_id": "pipeline-youtube-url",
    "source_adapter_id": "fixture",
    "external_id": "fixture:pipeline-youtube-url",
    "captured_at": "2026-05-20T09:01:00Z",
    "author": {"name": "Example Author"},
    "visibility": "public",
    "text": "Watch this for Mindline processor routing: https://www.youtube.com/watch?v=wp6example",
    "urls": [{"url": "https://www.youtube.com/watch?v=wp6example", "kind": "youtube_url"}],
    "local_artifacts": []
  },
  {
    "schema_version": "candidate/v0.1",
    "candidate_id": "pipeline-linkedin-with-website",
    "source_adapter_id": "fixture",
    "external_id": "fixture:pipeline-linkedin-with-website",
    "captured_at": "2026-05-20T09:02:00Z",
    "author": {"name": "Example Author"},
    "visibility": "public",
    "text": "LinkedIn post with outbound context: https://www.linkedin.com/posts/example-mindline and https://example.com/mindline-routing",
    "urls": [
      {"url": "https://www.linkedin.com/posts/example-mindline", "kind": "linkedin_url"},
      {"url": "https://example.com/mindline-routing", "kind": "web_url"}
    ],
    "local_artifacts": []
  },
  {
    "schema_version": "candidate/v0.1",
    "candidate_id": "pipeline-pdf-url",
    "source_adapter_id": "fixture",
    "external_id": "fixture:pipeline-pdf-url",
    "captured_at": "2026-05-20T09:03:00Z",
    "author": {"name": "Example Author"},
    "visibility": "public",
    "text": "PDF source: https://example.com/reports/mindline.pdf",
    "urls": [{"url": "https://example.com/reports/mindline.pdf", "kind": "pdf_url"}],
    "local_artifacts": []
  },
  {
    "schema_version": "candidate/v0.1",
    "candidate_id": "pipeline-mixed-links",
    "source_adapter_id": "fixture",
    "external_id": "fixture:pipeline-mixed-links",
    "captured_at": "2026-05-20T09:04:00Z",
    "author": {"name": "Example Author"},
    "visibility": "public",
    "text": "Mixed source set: https://www.youtube.com/watch?v=wp6example https://example.com/mindline-routing https://example.com/reports/mindline.pdf",
    "urls": [
      {"url": "https://www.youtube.com/watch?v=wp6example", "kind": "youtube_url"},
      {"url": "https://example.com/mindline-routing", "kind": "web_url"},
      {"url": "https://example.com/reports/mindline.pdf", "kind": "pdf_url"}
    ],
    "local_artifacts": []
  },
  {
    "schema_version": "candidate/v0.1",
    "candidate_id": "pipeline-unknown-source",
    "source_adapter_id": "fixture",
    "external_id": "fixture:pipeline-unknown-source",
    "captured_at": "2026-05-20T09:05:00Z",
    "author": {"name": "Example Author"},
    "visibility": "public",
    "text": "unclassified://mindline/local-capture",
    "urls": [{"url": "unclassified://mindline/local-capture", "kind": "unknown"}],
    "local_artifacts": []
  },
  {
    "schema_version": "candidate/v0.1",
    "candidate_id": "pipeline-private-provenance",
    "source_adapter_id": "fixture",
    "external_id": "fixture:pipeline-private-provenance",
    "captured_at": "2026-05-20T09:06:00Z",
    "author": {"name": "Example Author"},
    "visibility": "private",
    "text": "PRIVATE_DM_SENTINEL_DO_NOT_WRITE",
    "urls": [],
    "local_artifacts": []
  },
  {
    "schema_version": "candidate/v0.1",
    "candidate_id": "pipeline-secret-like",
    "source_adapter_id": "fixture",
    "external_id": "fixture:pipeline-secret-like",
    "captured_at": "2026-05-20T09:07:00Z",
    "author": {"name": "Example Author"},
    "visibility": "public",
    "text": "sk-test-secret-do-not-leak",
    "urls": [],
    "local_artifacts": []
  }
]
```

Exact Slack export batch body:

```json
{
  "schema_version": "slack-export/v0.1",
  "workspace": "fixture-workspace",
  "channel": "self-dm",
  "messages": [
    {
      "external_id": "slack-text-only",
      "ts": "2026-05-20T09:10:00Z",
      "author": "Example Author",
      "text": "Mindline should keep raw capture, method policy, and destination preview separate.",
      "urls": []
    },
    {
      "external_id": "slack-youtube-url",
      "ts": "2026-05-20T09:11:00Z",
      "author": "Example Author",
      "text": "Watch this for Mindline processor routing: https://www.youtube.com/watch?v=wp6example",
      "urls": ["https://www.youtube.com/watch?v=wp6example"]
    }
  ]
}
```

| Input fixture | Candidate body signal | Expected counts | Expected processor steps | Expected blocked reasons | Expected output files |
| --- | --- | --- | --- | --- | --- |
| `pipeline-text-only.json` | `Mindline should keep raw capture, method policy, and destination preview separate.` | candidates `1`, results `1`, operations `1`, blocked `0` | `text_capture_review:required:planned` | none | `pipeline-summary.json`, `results/pipeline-text-only.json`, `processor-plans/pipeline-text-only.json`, `destinations/pipeline-text-only/destination-summary.json`, `destinations/pipeline-text-only/operations/001-create_note.json`, `destinations/pipeline-text-only/previews/pipeline-text-only.md` |
| `pipeline-youtube-url.json` | `https://www.youtube.com/watch?v=wp6example` with no local transcript artifact | candidates `1`, results `1`, operations `0`, blocked `1` | `youtube_transcript:required:planned`, `manual_processing_required:blocked:blocked` | `missing_local_youtube_transcript` | `pipeline-summary.json`, `results/pipeline-youtube-url.json`, `processor-plans/pipeline-youtube-url.json`, `destinations/pipeline-youtube-url/destination-summary.json`; no `previews/` files |
| `pipeline-linkedin-with-website.json` | LinkedIn post URL plus `https://example.com/mindline-routing` with no local LinkedIn or webpage artifact | candidates `1`, results `1`, operations `0`, blocked `1` | `linkedin_post_context:required:planned`, `web_page_metadata:required:planned`, `manual_processing_required:blocked:blocked` | `missing_local_linkedin_context`, `missing_local_web_metadata` | `pipeline-summary.json`, `results/pipeline-linkedin-with-website.json`, `processor-plans/pipeline-linkedin-with-website.json`, `destinations/pipeline-linkedin-with-website/destination-summary.json`; no `previews/` files |
| `pipeline-pdf-url.json` | `https://example.com/reports/mindline.pdf` with no local PDF text artifact | candidates `1`, results `1`, operations `0`, blocked `1` | `pdf_text_extract:required:planned`, `manual_processing_required:blocked:blocked` | `missing_local_pdf_text` | `pipeline-summary.json`, `results/pipeline-pdf-url.json`, `processor-plans/pipeline-pdf-url.json`, `destinations/pipeline-pdf-url/destination-summary.json`; no `previews/` files |
| `pipeline-mixed-links.json` | YouTube, website, and PDF URLs with no local artifacts | candidates `1`, results `1`, operations `0`, blocked `1` | `youtube_transcript:required:planned`, `web_page_metadata:required:planned`, `pdf_text_extract:required:planned`, `manual_processing_required:blocked:blocked` | `missing_local_youtube_transcript`, `missing_local_web_metadata`, `missing_local_pdf_text` | `pipeline-summary.json`, `results/pipeline-mixed-links.json`, `processor-plans/pipeline-mixed-links.json`, `destinations/pipeline-mixed-links/destination-summary.json`; no `previews/` files |
| `pipeline-unknown-source.json` | `unclassified://mindline/local-capture` | candidates `1`, results `1`, operations `0`, blocked `1` | `manual_processing_required:blocked:blocked` | `unknown_content_type` | `pipeline-summary.json`, `results/pipeline-unknown-source.json`, `processor-plans/pipeline-unknown-source.json`, `destinations/pipeline-unknown-source/destination-summary.json`; no `previews/` files |
| `pipeline-private-provenance.json` | Body contains `PRIVATE_DM_SENTINEL_DO_NOT_WRITE`; provenance visibility is `private` | candidates `1`, results `1`, operations `0`, blocked `1` | `private_provenance_block:blocked:blocked` | `private_provenance_requires_review` | `pipeline-summary.json`, `results/pipeline-private-provenance.json`, `processor-plans/pipeline-private-provenance.json`, `destinations/pipeline-private-provenance/destination-summary.json`; no `previews/` files |
| `pipeline-secret-like.json` | Body contains `sk-test-secret-do-not-leak` | candidates `1`, results `1`, operations `0`, blocked `1` | `secret_skip:blocked:blocked` | `secret_like_content_detected` | `pipeline-summary.json`, `results/pipeline-secret-like.json`, `processor-plans/pipeline-secret-like.json`, `destinations/pipeline-secret-like/destination-summary.json`; no `previews/` files |
| `pipeline-slack-export-batch.json` | Local Slack export containing one text-only capture and one YouTube capture | candidates `2`, results `2`, operations `1`, blocked `1` | text item: `text_capture_review:required:planned`; YouTube item: `youtube_transcript:required:planned`, `manual_processing_required:blocked:blocked` | YouTube item: `missing_local_youtube_transcript` | `pipeline-summary.json`, two files under `candidates/`, two files under `results/`, two files under `processor-plans/`, two `destination-summary.json` files, one text-only preview, no YouTube preview |

For all single-candidate fixtures, `pipeline-summary.json.items[0]` must be:

```json
{
  "source_candidate_id": "<fixture-stem>",
  "record_id": "<fixture-stem>",
  "state": "ready_for_destination or needs_manual_processing",
  "visibility_lane": "visible or review_only",
  "method_profile_id": "basb-para-code",
  "processor_plan_path": "processor-plans/<fixture-stem>.json",
  "destination_summary_path": "destinations/<fixture-stem>/destination-summary.json",
  "blocked": "<true-or-false>",
  "blockers": ["<ordered blocker reasons>"]
}
```

Unblocked text-only sets `state: "ready_for_destination"`, `visibility_lane: "visible"`, `blocked: false`, and `blockers: []`. Every other single-candidate fixture sets `state: "needs_manual_processing"`, `visibility_lane: "review_only"`, and `blocked: true`.

Content-unit ids are deterministic: `text-1` for text, then `url-1`, `url-2`, `url-3` in URL input order. Processor steps preserve content-unit order; safety blockers (`private_provenance_block`, `secret_skip`) precede content processors and suppress them when triggered.

Blocked `destination-summary.json` files must use this shape:

```json
{
  "schema_version": "destination-summary/v0.1",
  "run_mode": "dry_run",
  "destination_adapter_id": "tolaria",
  "record_id": "<record-id>",
  "status": "blocked",
  "operation_count": 0,
  "preview_count": 0,
  "blockers": ["<ordered blocker reasons>"],
  "message": "Destination preview suppressed because required local processing is incomplete.",
  "authority_ids": ["DEC-15", "DEC-6", "DEC-12", "DEC-13"]
}
```

The unblocked text-only `destinations/pipeline-text-only/destination-summary.json` must use this exact body:

```json
{
  "schema_version": "destination-summary/v0.1",
  "run_mode": "dry_run",
  "destination_adapter_id": "tolaria",
  "record_id": "pipeline-text-only",
  "status": "planned",
  "operation_count": 1,
  "preview_count": 1,
  "blockers": [],
  "message": "Destination preview planned.",
  "authority_ids": ["DEC-15", "DEC-6", "DEC-12", "DEC-13"]
}
```

The unblocked text-only `destinations/pipeline-text-only/operations/001-create_note.json` must use this exact body:

```json
{
  "schema_version": "destination-operation/v0.1",
  "operation_id": "tolaria-pipeline-text-only-create-note-04725ffeb88f99b6",
  "destination_adapter_id": "tolaria",
  "source_candidate_id": "pipeline-text-only",
  "source_record_id": "pipeline-text-only",
  "idempotency_key": "pipeline-text-only",
  "operation_type": "create_note",
  "write_mode": "dry_run",
  "visibility_lane": "publish",
  "planned_locator": "30-resources/pipeline-text-only.md",
  "title": "Processed source pipeline-text-only",
  "body": "# Processed source pipeline-text-only\n\n## Snapshot\n\nMindline should keep raw capture, method policy, and destination preview separate.\n",
  "metadata": {
    "state": "ready_for_destination",
    "method_profile_id": "basb-para-code"
  },
  "blockers": [],
  "authority_ids": ["DEC-15", "DEC-6", "DEC-12", "DEC-13"]
}
```

The unblocked text-only `destinations/pipeline-text-only/previews/pipeline-text-only.md` must use this exact body:

```markdown
# Processed source pipeline-text-only

## Snapshot

Mindline should keep raw capture, method policy, and destination preview separate.
```

Invalid-case fixtures live in `testdata/pipeline/invalid/` and must assert deterministic failures:

| Case | Command/input | Expected exit | Expected stderr substring |
| --- | --- | --- | --- |
| missing out | omit `--out` | `2` | `missing required --out` |
| unsupported method | `--method zettelkasten` | `2` | `unsupported method: zettelkasten` |
| unsupported destination | `--destination notion` | `2` | `unsupported destination: notion` |
| unsupported run mode | input has `"run_mode": "live_write"` | `2` | `unsupported run_mode: live_write` |
| invalid schema | input has `"schema_version": "pipeline-input/v9"` | `2` | `unsupported schema_version: pipeline-input/v9` |
| unsafe input path | source path resolves outside fixture root through symlink | `2` | `unsafe input path escapes pipeline input directory` |
| protected output path | `--out` is the Tolaria vault or resolves inside it through symlink | `2` | `refusing to write pipeline output inside protected Tolaria vault` |
| missing authority ids | input omits `authority_ids` | `2` | `authority_ids are required` |
| empty authority ids | input contains `"authority_ids": []` | `2` | `authority_ids are required` |
| empty authority id | input contains `"authority_ids": [""]` | `2` | `authority_id must not be empty` |
| dropped authority id | input contains `"authority_ids": ["WP-6", "DEC-15"]` | `2` | `dropped or unauthorized authority_id: WP-6` |
| unknown authority id | input contains `"authority_ids": ["DEC-999"]` | `2` | `unknown authority_id: DEC-999` |
| duplicate authority id | input contains `"authority_ids": ["DEC-15", "DEC-15"]` | `2` | `duplicate authority_id: DEC-15` |
| malformed authority id | input contains `"authority_ids": ["not an id"]` | `2` | `malformed authority_id: not an id` |

Golden tests must compare:

- exact summary counts;
- exact item ordering;
- exact relative paths;
- exact processor ids, requirement values, status values, and reason strings;
- exact blocked reasons;
- exact authority id propagation across `pipeline-summary.json`, result envelopes, processor plans, destination summaries, and operation JSON;
- preview presence or absence;
- absence of sentinel strings from every generated file and stdout/stderr.

## Acceptance Criteria

1. `mindline pipeline dry-run <input> --method basb-para-code --destination tolaria --out <dir>` succeeds for valid fixtures and writes the output layout exactly.
2. Missing `--out`, unsupported method, unsupported destination, unsupported run mode, invalid schema, unsafe input paths, and protected output paths fail with deterministic errors.
3. The pipeline composes local Slack normalization, candidate processing, processor planning, method policy, and Tolaria destination dry-run without live APIs.
4. Processor plans match exact expected content units and steps for text-only, YouTube, LinkedIn+website, PDF, mixed links, unknown, private provenance, secret-like, and Slack export batch fixtures.
5. `run_mode: "dry_run"` appears in the pipeline input validation, method profile output, processor plans, pipeline summary, and destination summary surfaces. WP-5 destination operation JSON must keep `write_mode: "dry_run"`.
6. Private and secret-like fixture strings do not appear in generated files, stdout, filenames, processor plans, destination summaries, operation JSON, or preview Markdown.
7. Static boundary checks prove no live Slack API, no network HTTP client, no browser automation, no LLM calls, no auth/provider/database integration, and no Tolaria vault writes.
8. Tests prove BASB/PARA/CODE-specific strings and profile policy do not appear in `internal/sbos` or source adapter packages.
9. Tests prove WP-6 implementation refuses to run delivery behavior unless WP-5 destination code is present on the implementation base branch.
10. README documents the local pipeline dry-run, run modes, method profile boundary, processor routing boundary, and no-live-action guardrails.

## Exact Validation Commands

Implementation review must run these commands from the repository root:

```bash
go test -count=1 ./...
go test -json ./... > /tmp/mindline-wp7-go-test.json
rm -rf /tmp/mindline-wp7-output /tmp/mindline-wp7-private-output /tmp/mindline-wp7-secret-output /tmp/mindline-wp7-stdout.txt /tmp/mindline-wp7-stderr.txt /tmp/mindline-wp7-private-stdout.txt /tmp/mindline-wp7-private-stderr.txt /tmp/mindline-wp7-secret-stdout.txt /tmp/mindline-wp7-secret-stderr.txt
go run ./cmd/mindline pipeline dry-run testdata/pipeline/inputs/pipeline-text-only.json --method basb-para-code --destination tolaria --out /tmp/mindline-wp7-output > /tmp/mindline-wp7-stdout.txt 2> /tmp/mindline-wp7-stderr.txt
go run ./cmd/mindline pipeline dry-run testdata/pipeline/inputs/pipeline-private-provenance.json --method basb-para-code --destination tolaria --out /tmp/mindline-wp7-private-output > /tmp/mindline-wp7-private-stdout.txt 2> /tmp/mindline-wp7-private-stderr.txt
go run ./cmd/mindline pipeline dry-run testdata/pipeline/inputs/pipeline-secret-like.json --method basb-para-code --destination tolaria --out /tmp/mindline-wp7-secret-output > /tmp/mindline-wp7-secret-stdout.txt 2> /tmp/mindline-wp7-secret-stderr.txt
rg -n 'slack\.com/api|net/http|http\.Client|chromedp|playwright|puppeteer|openai|anthropic|claude|convex|supabase|mongodb|mongo\.Connect|clerk|workos|descope|oauth2' internal/pipeline internal/adapters internal/cli internal/destinations internal/sbos -g '!**/*_test.go'
rg -n 'os\.WriteFile\(|os\.Create\(|os\.CreateTemp\(|os\.OpenFile\(|os\.Mkdir\(|os\.MkdirAll\(|ioutil\.WriteFile\(|io\.Copy\(' internal/pipeline -g '!internal/pipeline/artifacts/**' -g '!**/*_test.go'
rg -n 'os\.WriteFile\(|os\.Create\(|os\.CreateTemp\(|os\.OpenFile\(|os\.Mkdir\(|os\.MkdirAll\(|ioutil\.WriteFile\(|io\.Copy\(' internal/cli/runner.go
rg -n 'PARA|CODE|BASB|Snapshot|Source Content|Key Details|Relevance|Signals|Related Sources|Next Action' internal/sbos internal/adapters -g '!**/*_test.go'
rg -n 'PRIVATE_DM_SENTINEL_DO_NOT_WRITE|sk-test-secret-do-not-leak' /tmp/mindline-wp7-output /tmp/mindline-wp7-private-output /tmp/mindline-wp7-secret-output /tmp/mindline-wp7-stdout.txt /tmp/mindline-wp7-stderr.txt /tmp/mindline-wp7-private-stdout.txt /tmp/mindline-wp7-private-stderr.txt /tmp/mindline-wp7-secret-stdout.txt /tmp/mindline-wp7-secret-stderr.txt
```

The two `go test` commands must pass. All five `rg` commands must return no matches and exit with status `1`. If the implementation places pipeline code outside `internal/pipeline`, the static grep target list must be expanded before review; it must not be narrowed. Test files are excluded from static architecture scans because fixture strings and expected Markdown can legitimately mention method/profile terms.

## Out of Scope

- No live Slack API.
- No live Tolaria vault writes.
- No real YouTube transcript fetching.
- No LinkedIn scraping.
- No website crawling.
- No PDF download or parsing from the network.
- No browser automation.
- No LLM calls.
- No database, auth, OAuth, Clerk, WorkOS, Descope, Supabase, Convex, or MongoDB integration.
- No implementation of Zettelkasten beyond preserving method-profile extensibility.
- No UI.
- No live approval workflow beyond dry-run run-mode representation.

## Delivery Preconditions

Implementation may start only after:

1. this spec receives LOOP Spec sign-off;
2. a matching implementation plan receives LOOP Plan sign-off;
3. both are captured in Product Brain;
4. WP-5 PR #2 is merged into `main`, or Product Brain captures an explicit decision to build WP-6 on top of `codex/wp-5-destination-dry-run`.

## Verification Expectations

- TDD red/green for pipeline input parsing.
- TDD red/green for method profile loading.
- TDD red/green for processor routing.
- TDD red/green for CLI behavior and output layout.
- TDD red/green for safety/no-leak behavior.
- `go test -count=1 ./...`.
- `go test -json ./...`.
- Static grep for forbidden live integrations.
- Generated artifact scans for private/secret sentinels.
- LOOP delivery review before future close-out.
