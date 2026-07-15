"use strict";

let sessionCapability = "";
let csrfCapability = "";
let reviewNonce = "";
let latestState = null;

const byId = (id) => document.getElementById(id);

async function bootstrap() {
  const params = new URLSearchParams(window.location.hash.slice(1));
  const token = params.get("bootstrap") || "";
  history.replaceState(null, "", window.location.pathname + window.location.search);
  if (!token) throw new Error("The private launch capability is missing. Restart Mindline.");
  const response = await fetch("/api/bootstrap", {
    method: "POST",
    headers: {"Content-Type": "application/json", "X-Mindline-Bootstrap": token},
    body: "{}"
  });
  if (!response.ok) throw new Error("Private session bootstrap was rejected.");
  const data = await response.json();
  sessionCapability = data.session;
  csrfCapability = data.csrf;
  byId("session-status").textContent = "Private local session ready. Credentials will be forgotten when this process stops.";
  await refresh();
}

function headers(json = true) {
  const value = {"X-Mindline-Session": sessionCapability, "X-Mindline-CSRF": csrfCapability};
  if (json) value["Content-Type"] = "application/json";
  return value;
}

async function api(path, body = {}) {
  const response = await fetch(path, {method: "POST", headers: headers(true), body: JSON.stringify(body)});
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || "operation_blocked");
  return data;
}

async function refresh() {
  const response = await fetch("/api/state", {headers: {"X-Mindline-Session": sessionCapability}});
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || "state_unavailable");
  byId("state").textContent = JSON.stringify(data, null, 2);
	latestState = data;
	renderGuidedState(data);
	const operations = Number(data.destination && data.destination.operation_count || 0);
	if (operations > 0) {
		byId("max-writes").value = String(operations);
		byId("max-attempts").value = String(Math.max(operations, operations * 2));
	}
}

function setSummary(id, message) { byId(id).textContent = message; }
function option(value, selected) {
  const node = document.createElement("option");
  node.value = value;
  node.textContent = value;
  node.selected = value === selected;
  return node;
}
function pill(text) {
  const node = document.createElement("span");
  node.className = "pill";
  node.textContent = text;
  return node;
}
function renderBatchPreview(preview) {
  const summary = byId("batch-summary");
  summary.replaceChildren();
  const heading = document.createElement("p");
  heading.textContent = `Exact batch ${preview.batch_fingerprint}; Product Brain workspace ${preview.destination_workspace_id}, key ${preview.destination_key_id}. ${preview.entry_operation_count} draft entries and ${preview.relation_operation_count} relations. Maximum ${preview.maximum_destination_writes} unique writes and ${preview.maximum_mutation_attempts} mutation attempts. Privacy findings: ${preview.privacy_finding_count}. Expires ${preview.expires_at}.`;
  summary.append(heading);
  const list = document.createElement("ul");
  for (const operation of preview.operations || []) {
    const row = document.createElement("li");
    row.textContent = operation.kind === "entry" ? `${operation.collection_slug}: ${operation.name} (${operation.entry_id})` : `${operation.relation_type}: ${operation.from_entry_id} → ${operation.to_entry_id}`;
    list.append(row);
  }
  summary.append(list);
  for (const gate of preview.preflight_gates || []) summary.append(pill(`${gate.name}: ${gate.verdict}${gate.actual ? ` (${gate.actual})` : ""}`));
}
function renderGuidedState(data) {
  const connections = data.connections || {};
  const inventory = data.inventory || {};
  const proof = data.proof || {};
  const destination = data.destination || {};
  const drain = data.drain || {};
  const identity = connections.destination_identity || {};
  const sourceIdentity = connections.source_identity || {};
  setSummary("connections-summary", `Slack source ${connections.source_connected ? `verified as workspace ${sourceIdentity.workspace_id || "unknown"}, channel ${sourceIdentity.channel_id || "unknown"}` : "not connected"}; inventory ${connections.source_imported ? "adopted" : "not imported"}; Product Brain ${connections.destination_connected ? `verified as ${identity.provider || "unknown provider"} workspace ${identity.workspace_id || "unknown"}, key ${identity.key_id || "unknown"}` : "not connected"}. Credentials are session-only.`);
  setSummary("inventory-summary", inventory.frozen
    ? `Frozen source ${inventory.source_identity || "unknown"} to ${inventory.watermark || "unknown watermark"}: ${inventory.source_records} Slack records → ${inventory.url_occurrences} URL occurrences → ${inventory.canonical_items} canonical items. ${inventory.selected_items} selected; ${inventory.unselected_items} durably unprocessed. Import ${inventory.file_name || "unnamed"}: ${inventory.file_bytes || 0} bytes; declared ${JSON.stringify(inventory.declared_counts || {})}; observed ${JSON.stringify(inventory.observed_counts || {})}; ${inventory.omission_count || 0} count omissions; ${inventory.duplicate_occurrences || 0} duplicate occurrences.`
    : (connections.source_imported ? `Validated ${inventory.file_name || "import"} from ${inventory.source_identity || "unknown source"}: declared ${JSON.stringify(inventory.declared_counts || {})}; observed ${JSON.stringify(inventory.observed_counts || {})}. Freeze only after checking this accounting.` : "Import an occurrence-complete Slack manifest first."));
  setSummary("proof-summary", proof.started
    ? `${proof.reviewed_count || 0}/${proof.item_count || 0} selected items reviewed. ${proof.awaiting_review_count || 0} await your judgment; ${proof.manual_support_count || 0} require explicit manual-support handling.`
    : "The capped proof has not started.");
  const stages = drain.stages || [];
  const sentences = drain.authorization_sentences || {};
  const stageLines = stages.map((stage) => {
    const blockers = (stage.blockers || []).join(", ");
    const conditions = (stage.conditions || []).join(", ");
    let authority = "Unauthorized while blockers remain.";
    if (stage.verdict === "READY") authority = sentences[stage.stage] || "No authority is implied.";
    if (stage.verdict === "CONDITIONAL") authority = "Unauthorized until every named condition passes.";
    return `${stage.stage}: ${stage.verdict}. ${authority}${blockers ? ` Blockers: ${blockers}.` : ""}${conditions ? ` Conditions: ${conditions}.` : ""}`;
  });
  setSummary("drain-summary", `${stageLines.join("\n")} Full queue accounted: ${Boolean(drain.full_inventory_queued)}. Remainder processed: ${Boolean(drain.processed_remainder)}.`);
  renderProofItems(proof.items || []);
  byId("freeze").disabled = !(connections.source_imported && data.strategy && data.strategy.configured) || Boolean(inventory.frozen);
  byId("prove").disabled = !inventory.frozen || Boolean(proof.started);
  byId("preview-form").querySelector("button").disabled = !(proof.completed && connections.destination_connected);
  const experimental = stages.find((stage) => stage.stage === "READY_TO_EXPERIMENTAL_DRAIN") || {};
  byId("confirm-drain").disabled = experimental.verdict !== "CONDITIONAL";
  byId("cancel-delivery").disabled = !destination.approval_fingerprint || Boolean(destination.cancellation_fingerprint);
  byId("resume-delivery").disabled = !destination.approval_fingerprint || destination.delivery_status === "completed" || destination.delivery_status === "zero_draft_activation_recorded" || Boolean(destination.cancellation_fingerprint);
  byId("disconnect").disabled = !connections.destination_connected;
  byId("disconnect-slack").disabled = !connections.source_connected;
	byId("slack-source-form").querySelector('button[type="submit"]').disabled = !(data.strategy && data.strategy.configured) || connections.source_imported;
  byId("slack-drain-form").querySelector('button[type="submit"]').disabled = !connections.source_connected || connections.source_imported;
  byId("destination-form").querySelector("button").disabled = Boolean(data.delivery_in_flight);
	if (destination.approval_fingerprint && destination.batch_preview) {
		renderBatchPreview(destination.batch_preview);
		byId("batch-preview").textContent = JSON.stringify(destination.batch_preview, null, 2);
		byId("approval-panel").hidden = false;
		byId("approve").disabled = true;
	} else if (!reviewNonce) {
		byId("approval-panel").hidden = true;
	}
  if (destination.receipt_fingerprint) {
    byId("receipt").value = destination.receipt_fingerprint;
    const drafts = byId("useful-drafts");
    drafts.replaceChildren();
    for (const id of destination.draft_ids || []) {
      const label = document.createElement("label");
      const checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.value = id;
      label.append(checkbox, document.createTextNode(id));
      drafts.append(label);
    }
    const zeroDraft = destination.delivery_status === "zero_draft_activation_recorded";
    byId("zero-draft").checked = zeroDraft;
    if (zeroDraft) byId("value-verdict").value = "zero_draft";
  }
}

function renderProofItems(items) {
  const container = byId("proof-items");
  container.replaceChildren();
  for (const item of items) {
    const card = document.createElement("article");
    card.className = "card";
    const title = document.createElement("h3");
    title.textContent = item.title || item.kind || "Untitled source";
    card.append(title);
    const meta = document.createElement("p");
    meta.className = "meta";
    meta.textContent = `${item.author || "Unknown author"} · ${item.format} · retrieval ${item.retrieval_state} · evidence ${item.evidence_origin || "unknown"} · ${item.canonical_url || "Sensitive URL withheld; inspect the original source item"}`;
    card.append(meta);
    if (!item.canonical_url && (item.source_references || []).length) {
      const source = document.createElement("p");
      source.className = "meta";
      source.textContent = `Original Slack source: ${(item.source_references || []).map((ref) => `${ref.native_message_id} at ${ref.native_timestamp} (link ${ref.url_ordinal + 1})`).join(", ")}`;
      card.append(source);
    }
    for (const excerpt of item.excerpts || []) {
      const quote = document.createElement("blockquote");
      quote.textContent = `${excerpt.text} — ${excerpt.locator}`;
      card.append(quote);
    }
    const evidence = document.createElement("div");
    for (const value of item.missingness || []) evidence.append(pill(`missing: ${value}`));
    for (const value of item.reason_codes || []) evidence.append(pill(value));
    for (const lens of item.lens_results || []) evidence.append(pill(`${lens.lens_id}: ${lens.result} (${lens.rationale})`));
    card.append(evidence);
    const proposal = document.createElement("p");
    proposal.textContent = `Proposed meaning: ${item.proposed_summary || "No stable meaning available"}. Proposal: ${item.proposed_role} → ${item.proposed_disposition}. ${item.proposed_rationale} Destination: ${item.destination_mapping}.`;
    card.append(proposal);
    if (item.review_status === "reviewed") {
      const reviewed = document.createElement("p");
      reviewed.className = "reviewed";
      reviewed.textContent = `Reviewed: ${item.role} → ${item.disposition}`;
      card.append(reviewed);
      container.append(card);
      continue;
    }
    const form = document.createElement("form");
    const role = document.createElement("select");
    for (const value of ["evidence_backed_finding", "external_entity", "unresolved_tension", "reference_resource", "unknown"]) role.append(option(value, item.proposed_role));
    const disposition = document.createElement("select");
    for (const value of ["promote", "hold", "clarify", "monitor", "archive"]) disposition.append(option(value, item.proposed_disposition));
    const rationale = document.createElement("textarea");
    rationale.required = true;
    rationale.value = "I reviewed the displayed evidence and confirm this proposed judgment.";
    const submit = document.createElement("button");
    submit.type = "submit";
    submit.textContent = item.requires_manual_review ? "Confirm manual-support outcome" : "Confirm or revise judgment";
    const roleLabel = document.createElement("label"); roleLabel.textContent = "Semantic role"; roleLabel.append(role);
    const dispositionLabel = document.createElement("label"); dispositionLabel.textContent = "Disposition"; dispositionLabel.append(disposition);
    const rationaleLabel = document.createElement("label"); rationaleLabel.textContent = "Your rationale"; rationaleLabel.append(rationale);
    const manualOutcome = document.createElement("select");
    if (item.requires_manual_review) {
      manualOutcome.append(option("queued_for_manual_processing", "queued_for_manual_processing"), option("confirmed_unavailable", ""));
    } else {
      manualOutcome.append(option("not_required", "not_required"));
    }
    const manualLabel = document.createElement("label"); manualLabel.textContent = "Manual-support outcome"; manualLabel.append(manualOutcome);
    form.append(roleLabel, dispositionLabel, rationaleLabel, manualLabel, submit);
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      const decision = role.value === item.proposed_role && disposition.value === item.proposed_disposition ? "accept" : "revise";
      try {
        await api("/api/commands/review-item", {item_id: item.canonical_item_id, decision, role: role.value, disposition: disposition.value, rationale: rationale.value, manual_support_outcome: manualOutcome.value});
        notice("Item judgment recorded immutably.");
        await refresh();
      } catch (error) { fail(error); }
    });
    card.append(form);
    container.append(card);
  }
}

function notice(message) { byId("notice").textContent = message; }
function fail(error) { notice(error.message || "Operation blocked."); }

byId("import-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    const input = byId("manifest");
    if (!input.files.length) throw new Error("Choose one occurrence-complete manifest.");
    const form = new FormData();
    form.append("manifest", input.files[0]);
    input.value = "";
    const response = await fetch("/api/import/external-slack", {method: "POST", headers: headers(false), body: form});
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || "import_blocked");
    notice("External inventory validated and adopted.");
    await refresh();
  } catch (error) { fail(error); }
});

byId("destination-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const input = byId("destination-key");
  const credential = input.value;
  input.value = "";
  try {
    await api("/api/commands/connect-destination", {credential});
    notice("Destination identity verified. The key remains session-only.");
    await refresh();
  } catch (error) { fail(error); }
});

byId("slack-source-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const input = byId("slack-key");
  const credential = input.value;
  input.value = "";
  try {
    await api("/api/commands/connect-slack-source", {credential, channel_id: byId("slack-channel").value.trim()});
    notice("Slack source identity verified. The token remains session-only.");
    await refresh();
  } catch (error) { fail(error); }
});

byId("slack-drain-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    await api("/api/commands/drain-slack-source", {oldest: byId("slack-oldest").value.trim(), latest: byId("slack-latest").value.trim()});
    notice("Slack source drained into an occurrence-complete, checkpointed inventory.");
    await refresh();
  } catch (error) { fail(error); }
});

byId("disconnect-slack").addEventListener("click", () => api("/api/commands/disconnect-slack-source").then(refresh).catch(fail));

byId("strategy-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    await api("/api/commands/save-strategy", {
      context_lenses: byId("context-lenses").value,
      routing_policy: byId("routing-policy").value,
      maximum_network_requests: Number(byId("drain-network").value),
      maximum_wall_time_seconds: Number(byId("drain-wall").value),
      maximum_cost_microunits: Number(byId("drain-cost").value),
      maximum_retry_attempts: Number(byId("drain-retries").value),
      manual_support_tolerance: Number(byId("drain-manual").value)
    });
    notice("Immutable strategy version saved.");
    await refresh();
  } catch (error) { fail(error); }
});

byId("freeze").addEventListener("click", () => api("/api/commands/freeze-inventory").then(refresh).catch(fail));
byId("prove").addEventListener("click", () => api("/api/commands/start-proof").then(refresh).catch(fail));
byId("refresh").addEventListener("click", () => refresh().catch(fail));
byId("confirm-drain").addEventListener("click", () => api("/api/commands/confirm-drain").then(refresh).catch(fail));
byId("help").addEventListener("click", async () => {
  byId("help-text").hidden = false;
  try { await api("/api/discovery/help"); } catch (error) { fail(error); }
});
byId("disconnect").addEventListener("click", () => api("/api/commands/disconnect").then(refresh).catch(fail));
byId("resume-delivery").addEventListener("click", async () => {
  try {
	const preview = latestState && latestState.destination && latestState.destination.batch_preview;
	if (!preview) throw new Error("The sealed exact batch preview is unavailable.");
	const confirmed = window.confirm(`Resume only sealed batch ${preview.batch_fingerprint} to workspace ${preview.destination_workspace_id}, with at most ${preview.maximum_destination_writes} unique writes and ${preview.maximum_mutation_attempts} attempts?`);
	if (!confirmed) return;
    await api("/api/commands/resume-delivery");
    notice("The existing sealed batch was resumed after fresh live checks; no replacement approval was created.");
    await refresh();
  } catch (error) { fail(error); }
});
byId("cancel-delivery").addEventListener("click", async () => {
  try {
    const fingerprint = latestState && latestState.destination && latestState.destination.approval_fingerprint;
    if (!fingerprint) throw new Error("No approved batch is cancellable.");
    await api("/api/commands/cancel", {approval_fingerprint: fingerprint});
    notice("Cancellation authority recorded; no new mutation may start.");
    await refresh();
  } catch (error) { fail(error); }
});

byId("preview-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    const data = await api("/api/commands/preview-batch", {maximum_destination_writes: Number(byId("max-writes").value), maximum_mutation_attempts: Number(byId("max-attempts").value)});
    reviewNonce = data.review_nonce;
    renderBatchPreview(data.preview);
    byId("batch-preview").textContent = JSON.stringify(data.preview, null, 2);
    byId("approval-panel").hidden = false;
	byId("approve").disabled = false;
    notice("Review the exact fingerprint, destination, operations, and budgets before approving.");
  } catch (error) { fail(error); }
});

byId("approve").addEventListener("click", async () => {
  const nonce = reviewNonce;
  reviewNonce = "";
  byId("approval-panel").hidden = true;
  try {
    await api("/api/commands/approve-batch", {review_nonce: nonce});
    notice("Exact batch approved and handed to Product Brain authority.");
    await refresh();
  } catch (error) { fail(error); }
});

byId("founder-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    const ids = Array.from(byId("useful-drafts").querySelectorAll("input:checked")).map((value) => value.value);
    await api("/api/commands/founder-review", {
      receipt_fingerprint: byId("receipt").value,
      useful_draft_ids: ids,
      value_verdict: byId("value-verdict").value,
      usefulness_reason: byId("usefulness").value,
      credential_burden: byId("credential-burden").value,
      manual_support_burden: byId("manual-burden").value,
      approval_burden: byId("approval-burden").value,
      zero_draft: byId("zero-draft").checked
    });
    notice("Founder review recorded without changing the sealed sample.");
    await refresh();
  } catch (error) { fail(error); }
});

bootstrap().catch(fail);
