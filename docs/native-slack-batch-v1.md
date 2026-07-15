# Native Slack batch v1

`mindline_native_slack_batch/v1` is the source-integration handoff between a
credential-owning Slack connector and Mindline's Slack source adapter. It lets
an external connector drain a conversation without transferring its OAuth
credential to Mindline.

## Ownership boundary

The connector owns and declares:

- workspace and conversation identity;
- the inclusive native timestamp window and watermark;
- whether channel and thread pagination are exhausted;
- whether threads and replies are included;
- the number of native source records;
- native message identity, timestamp, thread parent, text, edit/delete state,
  attachment count, and private-file count.

Mindline owns and derives:

- every URL occurrence;
- canonical URLs and canonical item identities;
- retrieval strategy, format, and strata;
- completeness evidence and the sealed content fingerprint;
- resource caps and the destination-neutral inventory.

Connectors must not reproduce Mindline URL extraction or classification rules.
`lower_inclusive` and `upper_inclusive` are the exhausted connector query-window
bounds, not required observed-message extrema. Every emitted message timestamp
must fall numerically within that window, and `watermark` equals its upper bound.

## JSON frame

The command accepts one compact JSON frame on standard input. The exact byte
length and SHA-256 are supplied as non-secret command flags so the handoff does
not depend on terminal EOF and cannot silently truncate or combine frames.

```json
{
  "schema_version": "mindline_native_slack_batch/v1",
  "workspace_id": "T_WORKSPACE",
  "channel_id": "D_CONVERSATION",
  "lower_inclusive": "1710000000.000001",
  "upper_inclusive": "1720000000.000001",
  "watermark": "1720000000.000001",
  "include_threads": true,
  "include_replies": true,
  "pagination_exhausted": true,
  "thread_pagination_exhausted": true,
  "declared_source_records": 1,
  "messages": [
    {
      "native_message_id": "1720000000.000001",
      "timestamp": "1720000000.000001",
      "text": "https://example.com/resource",
      "attachment_count": 0,
      "private_file_count": 0
    }
  ]
}
```

The bridge is available only to a clean build with a fresh, commit- and
configuration-bound pre-live receipt:

```text
mindline activation build-slack-manifest \
  --runtime <private-runtime> \
  --receipt <pre-live-receipt.json> \
  --out <private-runtime/slack-manifest.json> \
  --payload-bytes <exact-byte-count> \
  --payload-sha256 <sha256>
```

Native content crosses only standard input. The normalized output is a
no-replace `0600` private artifact and does not retain raw message text; it
retains native provenance fingerprints and URL occurrences. Current hard caps
are 64 MiB per frame, 20,000 native messages, 50,000 extracted URL occurrences,
and 64 MiB for the resulting import artifact.
