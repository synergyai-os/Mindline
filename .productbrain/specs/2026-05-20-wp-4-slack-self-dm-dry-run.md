# MINDLINE-WP4-SPEC-V7

## Phase

Spec.

## Authority

Product Brain is the source of truth for this package.

- Work package: `WP-4` - Mindline Slack self-DM source adapter dry-run.
- Product: `PROD-1` - Mindline.
- Depends on: `WP-2` CLI dry-run runner and `WP-3` candidate contract fixtures.
- Sequencing decision: `DEC-6` - candidate fixtures before Slack dry-run before destination boundary.
- Architecture decisions: `DEC-2`, `DEC-3`, `DEC-4`.
- Governing standards: `STD-5`, `STD-6`, `STD-7`, `STD-11`, `STD-12`.
- Relevant learning: `INS-1`, `INS-5`, `FEAT-1`, `FEAT-2`.
- Slack API reference used through ctx7: Slack methods include `conversations.history` for message history, `chat.getPermalink` for a message permalink, and `conversations.replies` for thread context. WP-4 models those fields but does not call the live API.

## Problem

Mindline has a proven core candidate contract and dry-run processor, but no source adapter proves how a real capture surface becomes normalized candidates. Slack self-DM is the first source because Randy already captures there, but direct Tolaria writing is now the wrong layer. WP-4 must prove source-side normalization, ordering, checkpointing, save-intent preservation, and safety routing without introducing live auth, destination coupling, or personal PKM writes.

## Product Model Fit

Mindline is the headless knowledge-processing engine between capture sources and output surfaces. A Slack source adapter is a reusable engine boundary, not a Randy-only script, because any user should be able to provide Slack-like message input and receive candidate JSON that the core can process. Tolaria remains out of scope for WP-4; destination behavior belongs to WP-5.

## In Scope

WP-4 creates a dry-run Slack adapter with deterministic local inputs and outputs:

1. A Slack adapter package that converts Slack-like message records into `sbos.Candidate` values.
2. A source input model that captures message identity, channel/conversation identity, timestamp, author, text, permalink, files, attachments, and optional capture metadata.
3. Old-to-new batch ordering by Slack timestamp.
4. Explicit checkpoint metadata separated from candidate JSON.
5. URL and attachment preservation so save intent survives normalization.
6. Safety classification for blank/empty messages and secret-looking snippets.
7. Candidate output that conforms to `docs/candidate-contract.md` and the WP-3 fixture expectations.
8. A CLI dry-run command for local fixture/export files:

```bash
mindline slack normalize <slack-export.json> [--out <dir>]
```

Default behavior prints a deterministic JSON envelope to stdout and writes no files. With `--out`, the command writes one candidate JSON file per emitted candidate plus one checkpoint metadata JSON file.

## Out of Scope

WP-4 must not include:

- live Slack API calls, OAuth, bot token handling, or workspace installation
- Tolaria writes, destination previews, folder decisions, Markdown notes, or durable personal PKM output
- promotion to projects, resources, areas, decisions, or other final PKM objects
- LLM enrichment, webpage fetching, LinkedIn comment fetching, YouTube transcript fetching, or outbound network calls
- database/provider/auth choices such as Convex, Supabase, MongoDB, Clerk, WorkOS, or Descope
- hidden checkpoints outside the explicit dry-run output
- migration of existing Slack backlog data

## Input Contract

The dry-run input is a JSON object:

```json
{
  "source": {
    "workspace": "synergyai-os",
    "channel_id": "D05H52...",
    "channel_name": "self-dm",
    "adapter_id": "slack"
  },
  "messages": [
    {
      "ts": "1710000000.000001",
      "user": "U123",
      "author_name": "Randy",
      "text": "Saved source https://example.com",
      "permalink": "https://workspace.slack.com/archives/D05H52/p1710000000000001",
      "files": [],
      "attachments": [],
      "captured_at": "2026-05-20T09:00:00Z",
      "capture_metadata": {
        "save_intent_status": "clear",
        "classification_status": "clear",
        "desired_visibility_hint": "background",
        "provenance_visibility_hint": "private",
        "public_provenance_assertion": "",
        "domain_hint": "Research Landscape",
        "topic_hints": ["slack-capture"],
        "clarification_reason": ""
      }
    }
  ]
}
```

Field rules:

- `source.adapter_id` defaults to `slack` if omitted.
- `source.channel_id` is required.
- `messages[].ts` is required and remains the source timestamp.
- `messages[].user` or `messages[].author_name` is required.
- `messages[].text`, `messages[].files`, and `messages[].attachments` may be empty, but the adapter must still make an explicit safety decision.
- `messages[].permalink` may be empty in dry-run fixtures; if empty, the adapter must emit non-empty sentinel `slack://missing-permalink/<channel_id>/<normalized_ts>` with private visibility.
- `messages[].captured_at` defaults to the source timestamp converted to a stable UTC time if possible; otherwise the adapter returns a validation error.
- `messages[].files[]` accepts objects with `id`, `name`, `url_private`, `url_public`, and `title`. IDs/names/titles are attachment identifiers. `url_public` is a candidate URL only when provenance is explicitly public. `url_private` must never be emitted as an ordinary `content.urls` value; it is replaced before stdout or `--out` emission with non-sensitive sentinel `slack-file-private://<file_id>` and forces `safety.private_provenance: true`.
- `messages[].attachments[]` accepts objects with `id`, `title`, `title_link`, `from_url`, and `text`. Titles/text are attachment identifiers/content; `title_link` and `from_url` are candidate URLs unless redacted as secret-like.
- `messages[].capture_metadata` is optional and dry-run-only. It must never be treated as Slack-native API data.
- `capture_metadata.save_intent_status`: `clear` or `ambiguous`; default `clear`.
- `capture_metadata.classification_status`: `clear` or `ambiguous`; default `clear`.
- `capture_metadata.desired_visibility_hint`: `background`, `attention`, `publish`, or `clarify`; default `background`.
- `capture_metadata.provenance_visibility_hint`: `public` or `private`; default `private` for Slack self-DM dry-run input.
- `capture_metadata.public_provenance_assertion`: required non-empty string when `provenance_visibility_hint` is `public`; otherwise the adapter must treat provenance as `private`.
- `capture_metadata.domain_hint`: optional non-empty domain string; default `Research Landscape`.
- `capture_metadata.topic_hints`: optional non-empty topic list; default `slack-capture`.
- `capture_metadata.clarification_reason`: required when save intent or classification is ambiguous; otherwise empty string.

## Candidate Mapping

Each processable Slack message produces a `schema_version: v0.1` candidate:

- `candidate_id`: `slack-<channel_id>-<normalized_ts>`.
- `adapter_id`: `slack`.
- `external_id`: `<channel_id>:<ts>`.
- `captured_at`: message `captured_at` or derived timestamp.
- `idempotency_key`: `slack:<channel_id>:<ts>`.
- `provenance.permalink`: message permalink when present. If missing, use non-empty sentinel `slack://missing-permalink/<channel_id>/<normalized_ts>`. Visibility is `public` only when `capture_metadata.provenance_visibility_hint` is `public` and `capture_metadata.public_provenance_assertion` is non-empty; otherwise `private`.
- `provenance.native_timestamp`: Slack `ts`. Visibility is `public` only when `capture_metadata.provenance_visibility_hint` is `public` and `capture_metadata.public_provenance_assertion` is non-empty; otherwise `private`.
- `provenance.author`: `author_name` if present else `user`. Visibility is `public` only when `capture_metadata.provenance_visibility_hint` is `public` and `capture_metadata.public_provenance_assertion` is non-empty; otherwise `private`.
- `provenance.raw_locator`: `<workspace>/<channel_id>/<ts>`. Visibility is `public` only when `capture_metadata.provenance_visibility_hint` is `public` and `capture_metadata.public_provenance_assertion` is non-empty; otherwise `private`.
- `content.text`: normalized Slack text, except secret-like content is replaced before emission with `[REDACTED SECRET-LIKE CONTENT]`.
- `content.urls`: URLs extracted from text plus URLs from attachments/files.
- `content.attachments`: file IDs, file URLs, attachment titles, or attachment URLs.
- `content.source_title`: first useful title from attachment/file/title text, else `Slack self-DM capture`.
- `enrichment_status`: `incomplete` when URLs require later enrichment, otherwise `not_required`.
- `classification.type`: `Source`.
- `classification.domain`: `capture_metadata.domain_hint` if present, otherwise `Research Landscape`.
- `classification.topics`: `capture_metadata.topic_hints` if present and non-empty, otherwise `slack-capture`.
- `classification.confidence`: `low` unless input supplies stronger classification hints.
- `classification.needs_clarification`: true when `capture_metadata.save_intent_status` or `capture_metadata.classification_status` is `ambiguous`, otherwise false.
- `classification.clarification_reason`: `capture_metadata.clarification_reason` when clarification is needed; if omitted for an ambiguous input, use stable fallback `Ambiguous Slack capture metadata`.
- `desired_visibility`: `clarify` when save intent or classification is ambiguous; otherwise `capture_metadata.desired_visibility_hint`.
- `safety.redaction_required`: true when the adapter redacts secret-like or private content before candidate emission, otherwise false.
- `safety.empty_content`: true when text, files, and attachments contain no meaningful content.
- `safety.secret_like`: true when text or attachment fields match obvious credential/token/password patterns.
- `safety.private_provenance`: true when any provenance field is private.

The adapter must emit candidates that satisfy the existing `sbos` validator. WP-4 does not weaken the WP-3 candidate contract. Missing optional Slack source data must be converted into valid explicit sentinel values, not empty required fields.

## Required Fixtures

WP-4 must add these local dry-run fixtures:

- `examples/slack/reverse-ordered-batch.json`: three messages with `ts` values `1710000002.000001`, `1710000000.000001`, `1710000001.000001`; expected candidate order is ascending timestamp.
- `examples/slack/url-file-attachment.json`: one message with text URL `https://example.com/page`, `files[0].id: F123`, `files[0].title: Design PDF`, `files[0].url_private: https://files.slack.com/files-pri/T/F123/design.pdf`, `files[0].url_public: https://files.example/public/design.pdf`, `attachments[0].title_link: https://article.example/post`, and `attachments[0].from_url: https://source.example/root`; text URL and attachment URLs must appear in `content.urls`; `url_public` must not appear unless `public_provenance_assertion` is non-empty; private Slack file URL must not appear anywhere in normalize output; sentinel `slack-file-private://F123` must appear in `content.attachments`; and the candidate must have `safety.private_provenance: true`.
- `examples/slack/empty-content.json`: one message with blank text and empty files/attachments; expected candidate has `safety.empty_content: true` and processes to `skipped`.
- `examples/slack/secret-redaction.json`: one message containing `password=super-secret-value`, `xoxb-1234567890-abcdef`, and `api_key=sk_live_secret`; expected candidate has `safety.secret_like: true`, `safety.redaction_required: true`, redacted content fields, and no raw secret fixture strings in normalize stdout or `--out` outputs.
- `examples/slack/ambiguous-metadata.json`: one message with `capture_metadata.save_intent_status: ambiguous` and `capture_metadata.clarification_reason: Need to know whether Randy wanted the post or linked page`; expected candidate routes to clarification.
- `examples/slack/missing-permalink-publish.json`: one message with empty permalink and `capture_metadata.desired_visibility_hint: publish`; expected candidate uses `slack://missing-permalink/<channel_id>/<normalized_ts>` and private provenance blocks publish.
- `examples/slack/private-default-publish.json`: one message with a present Slack permalink and `capture_metadata.desired_visibility_hint: publish` but no public provenance assertion; expected candidate remains private provenance and blocks publish.
- `examples/slack/public-provenance-enrichment.json`: one message with a present permalink, URL, `capture_metadata.provenance_visibility_hint: public`, and `capture_metadata.public_provenance_assertion: fixture-approved-public-provenance`; expected candidate has public provenance and URL-driven `enrichment_status: incomplete`, reaching `needs_enrichment` through the existing core.

Minimum secret-like triggers for WP-4 are case-insensitive matches for:

- `password=`
- `api_key=`
- `xoxb-`
- `xoxp-`
- `bearer `
- `sk_live_`

These minimum patterns are deliberately small and fixture-driven. Broader detection can be added later only with new tests and PB authority.

## Checkpoint Metadata

The normalize command emits checkpoint metadata in the stdout envelope and writes it with `--out`:

```json
{
  "adapter_id": "slack",
  "source": "synergyai-os/D05H52...",
  "batch_order": "old_to_new",
  "input_count": 3,
  "candidate_count": 3,
  "skipped_by_adapter_count": 0,
  "first_ts": "1710000000.000001",
  "last_ts": "1710000002.000001",
  "next_oldest_exclusive_ts": "1710000002.000001"
}
```

The checkpoint is proof only. It is not hidden state, not a database cursor, and not a destination instruction.

## Safety Rules

- Empty Slack artifacts become candidates that the core routes to `skipped`; the adapter does not silently drop them.
- Secret-looking snippets become candidates with `safety.secret_like: true` and `safety.redaction_required: true`; the adapter redacts raw secret-looking text, file URLs, attachment fields, and titles before any stdout or `--out` candidate JSON emission. Only safety flags, stable IDs, non-sensitive metadata, and redacted placeholders may persist.
- Slack private file URLs are private provenance, even when not secret-like. They must be replaced with `slack-file-private://<file_id>` before stdout or `--out` emission and must not appear in candidate URLs, checkpoint metadata, generated paths, or logs.
- Slack self-DM provenance is private by default. Private provenance remains explicit and should cause publish attempts to be blocked by the core.
- The adapter must never write Tolaria files or destination Markdown.
- The adapter must never log or persist token-looking snippets outside explicit test fixture input strings. Normalize output must not contain raw secret-like strings in stdout, candidate JSON, checkpoint JSON, generated paths, or logs.

## CLI Behavior

`mindline slack normalize <slack-export.json>`:

- validates arguments and input JSON
- sorts messages old-to-new by Slack timestamp
- prints a deterministic envelope:

```json
{
  "adapter_id": "slack",
  "candidate_count": 1,
  "candidates": [],
  "checkpoint": {},
  "authority_ids": ["WP-4", "WP-3", "WP-2", "FEAT-1", "STD-5", "STD-6", "STD-7", "STD-11", "STD-12", "DEC-6"]
}
```

`mindline slack normalize <slack-export.json> --out <dir>`:

- validates and creates `--out` like the existing `process` command
- writes candidate JSON files using sanitized `candidate_id`
- writes `slack-checkpoint.json`
- prints the same envelope with candidate bodies omitted and paths included

Usage errors return exit code `1`. Invalid source payloads return exit code `2`. Artifact write errors return exit code `3`.

## Acceptance Tests

WP-4 is accepted only when fresh verification proves:

1. Slack messages are normalized old-to-new even when input is reverse ordered.
2. The CLI default writes no files and prints deterministic stdout.
3. `--out` writes candidate JSON files and `slack-checkpoint.json` only under the requested directory.
4. URL extraction preserves eligible non-private URLs from Slack text, files, and attachments, explicitly excluding `url_private` and excluding `url_public` unless public provenance is asserted.
5. Empty content candidates process through the existing core as `skipped`.
6. Secret-looking candidates process through the existing core as `skipped`.
7. Secret-looking input does not appear in normalize stdout, candidate JSON files, checkpoint files, generated paths, or logs except in explicit test fixture input strings.
8. `examples/slack/ambiguous-metadata.json` maps to `desired_visibility: clarify`, `classification.needs_clarification: true`, a non-empty `classification.clarification_reason`, and routes through the existing core as attention/clarification without Tolaria writes.
9. `examples/slack/missing-permalink-publish.json` maps to a non-empty private sentinel and reaches the existing private-provenance publish-blocking behavior.
10. `examples/slack/private-default-publish.json` proves all provenance fields remain private by default and block publish without a public provenance assertion.
11. `examples/slack/public-provenance-enrichment.json` proves explicit public provenance can reach URL-driven `needs_enrichment`.
12. Private provenance is explicit and blocks publish via the existing core behavior.
13. All emitted candidates pass the existing `sbos` validation path.
14. `examples/slack/url-file-attachment.json` proves Slack `url_private` does not appear in stdout, candidate JSON, checkpoint files, generated paths, or logs, and that `slack-file-private://F123` plus `safety.private_provenance: true` preserve the private file reference safely.
15. No code path writes Tolaria, references destination folders, or performs network/auth calls.
16. `go test -count=1 ./...` passes.

## Reviewer Panel

Spec sign-off requires all selected reviewers to return `SIGN-OFF` on `MINDLINE-WP4-SPEC-V7`:

- Chain Steward
- Domain/User Job Reviewer
- Systems Architect
- Delivery Quality Reviewer
- Risk/Safety Reviewer

Risk/Safety is required because Slack self-DM capture touches private provenance and secret-looking content.
