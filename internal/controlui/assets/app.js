"use strict";

let sessionCapability = "";
let csrfCapability = "";
let reviewNonce = "";
let latestState = null;
let pairingAbort = null;
let settingsBaseline = null;
let settingsDirty = false;
let runSelectionBaseline = null;

const SESSION_STORAGE_KEY = "mindline.session.v1";
const CSRF_STORAGE_KEY = "mindline.csrf.v1";
const PAIRING_ATTEMPT_STORAGE_KEY = "mindline.pairing-attempt.v1";
const PAIRING_CHALLENGE_STORAGE_KEY = "mindline.pairing-challenge.v1";
const ORIGIN = "http://127.0.0.1:9876";
const RUN_RECOVERY_ACKNOWLEDGEMENT = "I understand this changes only the explicit Mindline run selection pointer.";

const byId = (id) => document.getElementById(id);

async function bootstrap() {
  history.replaceState(null, "", window.location.pathname + window.location.search);
  sessionCapability = sessionStorage.getItem(SESSION_STORAGE_KEY) || "";
  csrfCapability = sessionStorage.getItem(CSRF_STORAGE_KEY) || "";
  if (sessionCapability && csrfCapability) {
    try {
      await refresh();
      showPaired();
      return;
    } catch (_) {
      clearBrowserAuthority();
    }
  }
  showLocked();
  const pendingAttempt = sessionStorage.getItem(PAIRING_ATTEMPT_STORAGE_KEY) || "";
  const pendingChallenge = sessionStorage.getItem(PAIRING_CHALLENGE_STORAGE_KEY) || "";
  if (pendingAttempt && pendingChallenge) {
    pairingAbort = new AbortController();
    showPairingChallenge(pendingChallenge);
    try {
      await waitForPairing(pendingAttempt, pairingAbort.signal);
    } catch (error) {
      clearPairingAttempt();
      showLocked();
      byId("session-status").textContent = error.message;
    } finally {
      pairingAbort = null;
    }
  } else if (pendingAttempt || pendingChallenge) {
    clearPairingAttempt();
  }
}

function headers(json = true) {
  const value = {"X-Mindline-Origin": ORIGIN, "X-Mindline-Session": sessionCapability, "X-Mindline-CSRF": csrfCapability};
  if (json) value["Content-Type"] = "application/json";
  return value;
}

async function api(path, body = {}, method = "POST", localAuthority = null) {
  const requestHeaders = headers(true);
  if (localAuthority) {
    requestHeaders["X-Mindline-Session"] = localAuthority.session;
    requestHeaders["X-Mindline-CSRF"] = localAuthority.csrf;
  }
  const response = await fetch(path, {method, headers: requestHeaders, body: JSON.stringify(body)});
  const data = await response.json();
  if (!response.ok) {
    if (response.status === 401 || data.error_code === "session_stale" || data.error === "unauthorized") {
      clearBrowserAuthority();
      showLocked();
    }
    const error = new Error(data.user_message || data.error_code || data.error || "operation_blocked");
    error.payload = data;
    error.status = response.status;
    throw error;
  }
  return data;
}

async function refresh() {
  const response = await fetch("/api/state", {headers: {"X-Mindline-Origin": ORIGIN, "X-Mindline-Session": sessionCapability}});
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || "state_unavailable");
  byId("state").textContent = JSON.stringify(data, null, 2);
	latestState = data;
	hydrateSettings(data.settings);
	renderRunState(data);
	renderGuidedState(data);
	const operations = Number(data.destination && data.destination.operation_count || 0);
	if (operations > 0) {
		byId("max-writes").value = String(operations);
		byId("max-attempts").value = String(Math.max(operations, operations * 2));
	}
}

function clearBrowserAuthority() {
  sessionStorage.removeItem(SESSION_STORAGE_KEY);
  sessionStorage.removeItem(CSRF_STORAGE_KEY);
  sessionCapability = "";
  csrfCapability = "";
  reviewNonce = "";
  latestState = null;
  runSelectionBaseline = null;
  byId("state").textContent = "";
  byId("proof-items").replaceChildren();
  byId("batch-preview").textContent = "";
}

function clearPairingAttempt() {
  sessionStorage.removeItem(PAIRING_ATTEMPT_STORAGE_KEY);
  sessionStorage.removeItem(PAIRING_CHALLENGE_STORAGE_KEY);
}

function showLocked() {
  byId("private-workspace").hidden = true;
  byId("pairing-panel").hidden = false;
  byId("pair").hidden = false;
  byId("pairing-code-panel").hidden = true;
  byId("lock-session").hidden = true;
  byId("session-status").textContent = "This local Mindline window is locked.";
}

function showPaired() {
  clearPairingAttempt();
  byId("pairing-panel").hidden = true;
  byId("pairing-code-panel").hidden = true;
  byId("private-workspace").hidden = false;
  byId("lock-session").hidden = false;
  byId("session-status").textContent = "Private local session ready. Provider credentials remain only in this process.";
}

function showPairingChallenge(challenge) {
  byId("private-workspace").hidden = true;
  byId("pairing-panel").hidden = false;
  byId("pair").hidden = true;
  byId("pairing-code").textContent = challenge;
  byId("pairing-code-panel").hidden = false;
  byId("session-status").textContent = "Waiting for Codex to confirm the exact code…";
}

async function waitForPairing(attemptID, signal) {
  while (true) {
    if (signal.aborted) throw new DOMException("Pairing cancelled.", "AbortError");
    const response = await fetch("/api/session/pair", {
      method: "POST",
      headers: {"Content-Type": "application/json", "X-Mindline-Origin": ORIGIN},
      body: JSON.stringify({attempt_id: attemptID}),
      signal
    });
    const frame = await response.json();
    if (response.status === 202 && frame.type === "pending") {
      await new Promise((resolve) => setTimeout(resolve, 350));
      continue;
    }
    if (!response.ok || frame.type !== "paired") {
      throw new Error(frame.user_message || frame.error_code || "Pairing ended before confirmation. Create a new code.");
    }
    sessionCapability = frame.session;
    csrfCapability = frame.csrf;
    sessionStorage.setItem(SESSION_STORAGE_KEY, sessionCapability);
    sessionStorage.setItem(CSRF_STORAGE_KEY, csrfCapability);
    showPaired();
    await refresh();
    return;
  }
}

async function pairBrowser() {
  if (pairingAbort) pairingAbort.abort();
  clearPairingAttempt();
  pairingAbort = new AbortController();
  byId("pair").disabled = true;
  byId("session-status").textContent = "Creating a five-minute pairing code…";
  try {
    const response = await fetch("/api/session/pair", {
      method: "POST",
      headers: {"Content-Type": "application/json", "X-Mindline-Origin": ORIGIN},
      body: "{}",
      signal: pairingAbort.signal
    });
    const frame = await response.json();
    if (!response.ok || frame.type !== "challenge" || !frame.attempt_id || !frame.challenge) {
      throw new Error(frame.user_message || frame.error_code || "Pairing is unavailable. Restart Mindline from Codex.");
    }
    sessionStorage.setItem(PAIRING_ATTEMPT_STORAGE_KEY, frame.attempt_id);
    sessionStorage.setItem(PAIRING_CHALLENGE_STORAGE_KEY, frame.challenge);
    showPairingChallenge(frame.challenge);
    await waitForPairing(frame.attempt_id, pairingAbort.signal);
  } catch (error) {
    clearPairingAttempt();
    showLocked();
    byId("session-status").textContent = error.message;
    throw error;
  } finally {
    byId("pair").disabled = false;
    pairingAbort = null;
  }
}

function settingsDocument(settings) {
  if (!settings) return null;
  return settings.document || settings.snapshot || settings;
}

function hydrateSettings(settings) {
  const document = settingsDocument(settings);
  const draft = document && document.draft;
  if (!draft) return;
  if (!settingsDirty) {
    settingsBaseline = JSON.parse(JSON.stringify(document));
    byId("context-lenses").value = (draft.context_lenses || []).join("\n\n");
    byId("routing-policy").value = draft.routing_policy || "";
    const limits = draft.drain_policy || {};
    byId("drain-network").value = String(limits.maximum_network_requests ?? "");
    byId("drain-wall").value = String(limits.maximum_wall_time_seconds ?? "");
    byId("drain-cost").value = String(limits.maximum_cost_microunits ?? "");
    byId("drain-retries").value = String(limits.maximum_retry_attempts ?? "");
    byId("drain-manual").value = String(limits.manual_support_tolerance ?? "");
  }
  for (const field of byId("strategy-form").querySelectorAll("textarea,input,button")) field.disabled = false;
  const state = settings.state || (document.version > 0 ? "saved" : "defaults");
  byId("settings-status").textContent = state === "saved"
    ? `Saved settings version ${document.version} at ${document.saved_at}. These values survive restarts; credentials do not.`
    : "Server-owned defaults loaded. They are editable but not saved yet.";
}

function collectSettingsDraft() {
  const current = settingsBaseline && settingsBaseline.draft || {};
  return {
    context_lenses: byId("context-lenses").value.split(/\n+/).map((value) => value.trim()).filter(Boolean),
    routing_policy: byId("routing-policy").value.trim(),
    drain_policy: {
      maximum_network_requests: Number(byId("drain-network").value),
      maximum_wall_time_seconds: Number(byId("drain-wall").value),
      maximum_cost_microunits: Number(byId("drain-cost").value),
      maximum_retry_attempts: Number(byId("drain-retries").value),
      manual_support_tolerance: Number(byId("drain-manual").value)
    },
    adapter_defaults: current.adapter_defaults || [],
    expected_source_identity: current.expected_source_identity || null,
    expected_destination_identity: current.expected_destination_identity || null
  };
}

function renderRunState(data) {
  const selection = data.run_selection || {state: "none", version: 0, generation: ""};
  if (!selection.state && data.run && data.run.run_id) {
    selection.state = "compatible_selected";
    selection.selected_run_id = data.run.run_id;
  }
  runSelectionBaseline = JSON.parse(JSON.stringify(selection));
  const settings = settingsDocument(data.settings);
  const savedSettings = settings && Number(settings.version) > 0 && Boolean(settings.fingerprint);
  const active = selection.state === "compatible_selected";
  const blocked = selection.state === "blocked";
  const incompatible = selection.state === "incompatible_preserved";
  let status = "No proof is selected. Settings remain available; source and destination actions stay blocked.";
  if (active) status = `Selected proof ${selection.selected_run_id}. Selection revision ${selection.version}; no provider authority comes from this pointer.`;
  if (incompatible) status = `Prior proof ${selection.selected_run_id} is preserved but incompatible with this build. Start a new proof from saved settings or select another exact run.`;
  if (blocked) status = `Run selection is blocked (${selection.reason_code || "recovery required"}). Mindline will not infer or open another run.`;
  byId("run-status").textContent = status;
  byId("create-run").disabled = blocked || !savedSettings || settingsDirty || !Boolean(data.pre_live_ready);
  byId("select-run-form").querySelector("button").disabled = blocked;
  byId("run-recovery").hidden = !blocked || !selection.problem_fingerprint;
  byId("run-recovery-message").textContent = selection.problem_fingerprint
    ? `Recovery requires exact problem ${selection.problem_fingerprint}. Backup available: ${Boolean(selection.backup_available)}. No run evidence will be changed.`
    : "";
  byId("recover-run-explicit").disabled = !byId("select-run-id").value.trim();
  byId("use-settings").disabled = !active || settingsDirty || !savedSettings || !Boolean(data.pre_live_ready);
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
  const gateReady = Boolean(data.pre_live_ready);
  const runReady = !data.run_selection || !data.run_selection.state || data.run_selection.state === "compatible_selected";
  byId("gate-status").textContent = gateReady
    ? "Live safety gate ready. Source handoff and Product Brain connection remain explicit, bounded actions."
    : "The live safety gate is not ready yet. You can edit strategy and review saved evidence; Codex must prepare the gate before source handoff or Product Brain connection.";
  const sourceReady = Boolean(connections.source_imported);
  byId("source-handoff-status").textContent = sourceReady
    ? `Slack source ready for this proof: ${inventory.source_records || 0} messages, ${inventory.url_occurrences || 0} link occurrences, and ${inventory.canonical_items || 0} canonical items${inventory.watermark ? ` through ${inventory.watermark}` : ""}.`
    : (gateReady
      ? "Waiting for Codex to prepare and hand off the Slack source inventory. No action is required from you."
      : "Waiting for Codex to prepare the live gate and hand off the Slack source inventory. No action is required from you.");
  const destinationConnected = Boolean(connections.destination_connected);
  byId("destination-credential-panel").hidden = !gateReady || destinationConnected;
  byId("disconnect").hidden = !destinationConnected;
  byId("destination-status").textContent = destinationConnected
    ? `Connected to ${identity.provider || "Product Brain"} workspace ${identity.workspace_id || "unknown"}, key ${identity.key_id || "unknown"}. The credential remains only in this process.`
    : (gateReady
      ? "Enter your Product Brain key when you are ready to connect the draft-only destination. It remains only in this process."
      : "Waiting for Codex to prepare the live safety gate. Mindline will not request your Product Brain key before then.");
  setSummary("connections-summary", `Slack source ${sourceReady ? "ready" : "waiting for Codex"}. Product Brain ${destinationConnected ? "connected" : "not connected"}.`);
  setSummary("inventory-summary", inventory.frozen
    ? `Frozen source ${inventory.source_identity || "unknown"} to ${inventory.watermark || "unknown watermark"}: ${inventory.source_records} Slack records → ${inventory.url_occurrences} URL occurrences → ${inventory.canonical_items} canonical items. ${inventory.selected_items} selected; ${inventory.unselected_items} durably unprocessed. Import ${inventory.file_name || "unnamed"}: ${inventory.file_bytes || 0} bytes; declared ${JSON.stringify(inventory.declared_counts || {})}; observed ${JSON.stringify(inventory.observed_counts || {})}; ${inventory.omission_count || 0} count omissions; ${inventory.duplicate_occurrences || 0} duplicate occurrences.`
    : (connections.source_imported ? `Validated ${inventory.file_name || "source handoff"} from ${inventory.source_identity || "unknown source"}: declared ${JSON.stringify(inventory.declared_counts || {})}; observed ${JSON.stringify(inventory.observed_counts || {})}. Freeze only after checking this accounting.` : "Waiting for Codex to hand off a validated Slack source inventory."));
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
  renderProofItems(proof.items || [], gateReady);
  byId("freeze").disabled = !gateReady || !runReady || !(connections.source_imported && data.strategy && data.strategy.configured) || Boolean(inventory.frozen);
  byId("prove").disabled = !gateReady || !inventory.frozen || Boolean(proof.started);
  byId("preview-form").querySelector("button").disabled = !gateReady || !(proof.completed && connections.destination_connected);
  const experimental = stages.find((stage) => stage.stage === "READY_TO_EXPERIMENTAL_DRAIN") || {};
  byId("confirm-drain").disabled = !gateReady || experimental.verdict !== "CONDITIONAL";
  byId("cancel-delivery").disabled = !gateReady || !destination.approval_fingerprint || Boolean(destination.cancellation_fingerprint);
  byId("resume-delivery").disabled = !gateReady || !destination.approval_fingerprint || destination.delivery_status === "completed" || destination.delivery_status === "zero_draft_activation_recorded" || Boolean(destination.cancellation_fingerprint);
  byId("disconnect").disabled = !gateReady || !connections.destination_connected;
	byId("founder-form").querySelector('button[type="submit"]').disabled = !gateReady;
  byId("destination-form").querySelector("button").disabled = !gateReady || !runReady || Boolean(data.delivery_in_flight);
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

function renderProofItems(items, mutationsEnabled = true) {
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
    if (!mutationsEnabled) for (const field of form.querySelectorAll("select,textarea,button")) field.disabled = true;
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

byId("strategy-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    if (!settingsBaseline) throw new Error("Authoritative settings are not loaded yet.");
    const result = await api("/api/settings", {
      expected_version: settingsBaseline.version,
      expected_generation: settingsBaseline.generation,
      draft: collectSettingsDraft()
    }, "PUT");
    settingsDirty = false;
    byId("settings-conflict").hidden = true;
    hydrateSettings(result.settings || result);
    notice("Settings saved for future visits and proofs. No credential was stored.");
    await refresh();
  } catch (error) {
    if (error.status === 409 && error.payload && (error.payload.error_code === "settings_version_conflict" || error.payload.error === "settings_version_conflict")) {
      byId("settings-conflict").hidden = false;
      byId("settings-conflict-message").textContent = error.payload.user_message || "Saved settings changed elsewhere. Your edits are still here.";
      if (error.payload.current) byId("settings-conflict").dataset.current = JSON.stringify(error.payload.current);
    }
    fail(error);
  }
});

for (const field of byId("strategy-form").querySelectorAll("textarea,input")) {
  field.addEventListener("input", () => {
    settingsDirty = true;
    byId("create-run").disabled = true;
    byId("use-settings").disabled = true;
  });
}

byId("select-run-id").addEventListener("input", () => {
  byId("recover-run-explicit").disabled = !byId("select-run-id").value.trim();
});

byId("create-run").addEventListener("click", async () => {
  try {
    if (!runSelectionBaseline || !settingsBaseline || settingsDirty) throw new Error("Save settings and refresh the explicit run selection first.");
    await api("/api/runs", {
      expected_selection_version: runSelectionBaseline.version,
      expected_selection_generation: runSelectionBaseline.generation,
      settings_version: settingsBaseline.version,
      settings_generation: settingsBaseline.generation,
      settings_fingerprint: settingsBaseline.fingerprint
    });
    notice("A new immutable proof was created from the exact saved settings and explicitly selected.");
    await refresh();
  } catch (error) { fail(error); }
});

byId("select-run-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    if (!runSelectionBaseline) throw new Error("Run selection is not loaded.");
    await api("/api/runs/select", {
      expected_version: runSelectionBaseline.version,
      expected_generation: runSelectionBaseline.generation,
      run_id: byId("select-run-id").value.trim()
    });
    notice("The exact run pointer changed. Mindline did not infer a latest run or grant provider authority.");
    await refresh();
  } catch (error) { fail(error); }
});

async function recoverRunSelection(runID) {
  if (!runSelectionBaseline || !runSelectionBaseline.problem_fingerprint) throw new Error("No exact run-selection recovery problem is loaded.");
  const action = runID ? `replace the pointer with ${runID}` : "clear the corrupt pointer";
  if (!window.confirm(`Acknowledge recovery and ${action}? Run evidence will remain unchanged.`)) return;
  const readable = Boolean(runSelectionBaseline.readable_generation);
  await api("/api/runs/recover-selection", {
    problem_fingerprint: runSelectionBaseline.problem_fingerprint,
    expected_version: readable ? runSelectionBaseline.readable_version : null,
    expected_generation: readable ? runSelectionBaseline.readable_generation : "",
    acknowledgement: RUN_RECOVERY_ACKNOWLEDGEMENT,
    run_id: runID
  });
  notice("The non-authorizing run pointer was recovered explicitly; run evidence was not changed.");
  await refresh();
}

byId("recover-run-clear").addEventListener("click", () => recoverRunSelection("").catch(fail));
byId("recover-run-explicit").addEventListener("click", () => recoverRunSelection(byId("select-run-id").value.trim()).catch(fail));

byId("use-settings").addEventListener("click", async () => {
  try {
    if (settingsDirty) throw new Error("Save these edits before using them for a proof.");
    if (!settingsBaseline || !settingsBaseline.fingerprint) throw new Error("Save settings before using them for a proof.");
    await api("/api/commands/use-settings-for-proof", {
      settings_version: settingsBaseline.version,
      settings_generation: settingsBaseline.generation,
      settings_fingerprint: settingsBaseline.fingerprint
    });
    notice("The exact saved settings were applied to the open proof without changing any sealed proof.");
    await refresh();
  } catch (error) { fail(error); }
});

byId("reload-settings").addEventListener("click", () => {
  const encoded = byId("settings-conflict").dataset.current || "";
  if (!encoded) return;
  settingsDirty = false;
  hydrateSettings({state: "saved", document: JSON.parse(encoded)});
  byId("settings-conflict").hidden = true;
  notice("Your local edits were discarded and the saved version was loaded.");
});

byId("rebase-settings").addEventListener("click", () => {
  const encoded = byId("settings-conflict").dataset.current || "";
  if (!encoded) return;
  const localDraft = collectSettingsDraft();
  settingsBaseline = JSON.parse(encoded);
  byId("context-lenses").value = (localDraft.context_lenses || []).join("\n\n");
  byId("routing-policy").value = localDraft.routing_policy;
  settingsDirty = true;
  byId("settings-conflict").hidden = true;
  notice("Your edits now target the reviewed saved version. Choose Save settings to retry the exact CAS.");
});

byId("freeze").addEventListener("click", () => api("/api/commands/freeze-inventory").then(refresh).catch(fail));
byId("prove").addEventListener("click", () => api("/api/commands/start-proof").then(refresh).catch(fail));
byId("refresh").addEventListener("click", () => refresh().catch(fail));
byId("confirm-drain").addEventListener("click", () => api("/api/commands/confirm-drain").then(refresh).catch(fail));
byId("help").addEventListener("click", async () => {
  byId("help-text").hidden = false;
  if (sessionCapability) {
    try { await api("/api/discovery/help"); } catch (error) { fail(error); }
  }
});
byId("pair").addEventListener("click", () => pairBrowser().catch(fail));
byId("copy-pairing-code").addEventListener("click", async () => {
  try {
    await navigator.clipboard.writeText(byId("pairing-code").textContent);
    notice("Exact pairing code copied.");
  } catch (_) {
    notice("Copy was unavailable; select the exact code shown above.");
  }
});
byId("lock-session").addEventListener("click", async () => {
  const authority = {session: sessionCapability, csrf: csrfCapability};
  clearBrowserAuthority();
  showLocked();
  try {
    await api("/api/session/lock", {}, "POST", authority);
    byId("session-status").textContent = "This browser is locked. Slack and Product Brain connections remain process-only until explicitly disconnected or Mindline stops.";
  } catch (_) {
    byId("session-status").textContent = "Lock acknowledgement was not received. Stop Mindline to guarantee revocation.";
  }
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
