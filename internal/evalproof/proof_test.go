package evalproof

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/evalreadback"
	"github.com/synergyai-os/Mindline/internal/privateio"
	"github.com/synergyai-os/Mindline/internal/productbrain"
)

func TestDeliveryProofPassesOnlyForAcknowledgedReplay(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	routingFP := "routing-fingerprint"
	profile := proofDeliveryProfile()
	profileFP := proofArtifactFingerprint(profile)
	writeProofArtifact(t, filepath.Join(root, "routing", "route-summary.json"), map[string]any{"schema_version": "mindline-strategic-routing-summary/v0.1", "route_decisions_fingerprint": routingFP, "input_record_count": 12, "url_occurrence_count": 12, "primary_canonical_url_count": 11, "depth_one_url_count": 1, "canonical_source_count": 12, "duplicate_occurrence_count": 1, "lens_count": 2, "required_lens_result_count": 24, "lens_result_count": 24, "validation_failure_count": 0, "outbound_privacy_findings": 0, "operator_judged": true, "eval_projection": map[string]any{"sample_status": "private_curated_sample", "held_out": false, "generalizable": false}})
	operations := []any{}
	entries := []map[string]any{}
	entryOperationIDs := []string{}
	sourceRef := "https://example.com/source"
	entryDefinitions := []struct {
		collection, prefix, nodeID, name, description string
		data                                          map[string]any
	}{
		{"landscape", "LAND", "entity", "Entity", "External entity.", map[string]any{"description": "External entity.", "url": sourceRef}},
		{"insights", "INS", "insight", "Insight", "Evidence-backed finding.", map[string]any{"description": "Evidence-backed finding.", "source": sourceRef, "evidenceStrength": "strong"}},
		{"tensions", "TEN", "tension", "Tension", "Unresolved tension.", map[string]any{"description": "Unresolved tension."}},
	}
	for _, definition := range entryDefinitions {
		entryID := proofDeterministicEntryID(definition.prefix, sourceRef, definition.nodeID, definition.collection)
		opID := "op-entry-" + proofSHA256Hex(entryID+"|"+sourceRef+"|"+definition.nodeID)
		entry := map[string]any{"collection_slug": definition.collection, "entry_id": entryID, "name": definition.name, "data": definition.data, "source_ref": sourceRef, "source_excerpt": "Public evidence.", "created_by": "mindline:agent-operator", "force_draft": true}
		entries = append(entries, entry)
		entryOperationIDs = append(entryOperationIDs, opID)
		payload := map[string]any{"operation_id": opID, "kind": "entry", "dependencies": []any{}, "payload_fingerprint": "", "entry": entry}
		payload["payload_fingerprint"] = proofArtifactFingerprint(payload)
		operations = append(operations, payload)
	}
	for _, target := range []int{1, 2} {
		identity := proofSHA256Hex("mindline/relation/v0.1|" + entries[0]["entry_id"].(string) + "|related_to|" + entries[target]["entry_id"].(string))
		relation := map[string]any{"relation_identity": identity, "from_entry_id": entries[0]["entry_id"], "to_entry_id": entries[target]["entry_id"], "type": "related_to", "metadata": map[string]any{"evidence_refs": []string{"public-evidence"}, "lens_refs": []string{"building-product", "team-design"}, "rationale": "Evidence relation.", "initiator_type": "agent_operator", "judgment_method": "operator_agent_review", "credential_key_id": "safe-key-id"}, "if_missing": true}
		dependencies := []string{entryOperationIDs[0], entryOperationIDs[target]}
		sort.Strings(dependencies)
		payload := map[string]any{"operation_id": "op-relation-" + identity, "kind": "relation", "dependencies": dependencies, "payload_fingerprint": "", "relation": relation}
		payload["payload_fingerprint"] = proofArtifactFingerprint(payload)
		operations = append(operations, payload)
	}
	outbox := map[string]any{"schema_version": "productbrain-outbox/v0.1", "routing_fingerprint": routingFP, "profile_fingerprint": profileFP, "delivery_profile_snapshot": proofDeliveryProfileSnapshot(), "review_context": proofReviewContext(operations), "operations": operations, "privacy_findings": []any{}, "operator_judged": true, "held_out": false, "generalizable": false, "autonomy_claim": false}
	writeProofArtifact(t, filepath.Join(root, "outbox", "outbox.json"), outbox)
	outboxData, err := os.ReadFile(filepath.Join(root, "outbox", "outbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := productbrain.DecodeOutbox(outboxData); err != nil {
		t.Fatalf("proof fixture outbox is invalid: %v", err)
	}
	outboxFP := outbox["fingerprint"].(string)
	writeProofArtifact(t, filepath.Join(root, "outbox", "outbox-summary.json"), map[string]any{"schema_version": "productbrain-outbox-summary/v0.1", "outbox_fingerprint": outboxFP, "operation_count": 5, "entry_operation_count": 3, "relation_operation_count": 2, "privacy_finding_count": 0, "draft_only": true, "operator_judged": true, "held_out": false, "generalizable": false})
	contracts := []any{map[string]any{"slug": "insights", "fingerprint": "schema-insights"}, map[string]any{"slug": "landscape", "fingerprint": "schema-landscape"}, map[string]any{"slug": "tensions", "fingerprint": "schema-tensions"}}
	actuals := map[string]string{"trusted_origin": "https://gateway.productbrain.io", "runtime_secret_scan": "zero findings", "workspace_id": "ws-test", "workspace_slug": "test", "governance_mode": "open", "key_scope": "readwrite", "key_id": "safe-key-id", "collection_contract:insights": "schema-insights", "collection_contract:landscape": "schema-landscape", "collection_contract:tensions": "schema-tensions"}
	gates := []any{}
	for _, name := range []string{"trusted_origin", "runtime_secret_scan", "workspace_id", "workspace_slug", "governance_mode", "key_scope", "key_id", "collection_contract:insights", "collection_contract:landscape", "collection_contract:tensions"} {
		gates = append(gates, map[string]any{"name": name, "verdict": "pass", "actual": actuals[name]})
	}
	preflight := map[string]any{"schema_version": "productbrain-preflight/v0.1", "outbox_fingerprint": outboxFP, "profile_fingerprint": profileFP, "expected_origin": "https://gateway.productbrain.io", "workspace": map[string]any{"id": "ws-test", "slug": "test", "governance_mode": "open", "key_scope": "readwrite", "key_id": "safe-key-id"}, "collection_contracts": contracts, "mutation_calls": 0, "verdict": "pass", "gates": gates}
	writeProofArtifact(t, filepath.Join(root, "preflight", "preflight.json"), preflight)
	preflightFP := preflight["fingerprint"].(string)
	writeProofArtifact(t, filepath.Join(root, "delivery", "preflight-snapshots", preflightFP+".json"), preflight)
	operationResults := func(mutations bool) []any {
		values := []any{}
		for index, entry := range entries {
			docID := fmt.Sprintf("doc-%d", index+1)
			readback := proofEntryReadback(entry, docID)
			result := map[string]any{"operation_id": entryOperationIDs[index], "kind": "entry", "state": "acknowledged", "attempts": 1, "mutation_observed": mutations, "acknowledged": true, "entry_doc_id": docID, "remote_object_id": entry["entry_id"], "readback_fingerprint": proofArtifactFingerprint(readback), "draft_verified": true, "actor_verified": true}
			values = append(values, result)
		}
		for _, operation := range operations[3:] {
			op := operation.(map[string]any)
			relation := op["relation"].(map[string]any)
			remoteID := "remote-" + relation["relation_identity"].(string)[:8]
			readback := map[string]any{"relation_id": remoteID, "identity": relation["relation_identity"], "metadata": relation["metadata"]}
			values = append(values, map[string]any{"operation_id": op["operation_id"], "kind": "relation", "state": "acknowledged", "attempts": 1, "mutation_observed": mutations, "acknowledged": true, "remote_object_id": remoteID, "readback_fingerprint": proofArtifactFingerprint(readback), "attribution_verified": true})
		}
		return values
	}
	run1 := map[string]any{"schema_version": "productbrain-delivery-run/v0.1", "sequence": 1, "invocation_id": "first", "outbox_fingerprint": outboxFP, "profile_fingerprint": profileFP, "preflight_fingerprint": preflightFP, "preflight_snapshot_ref": "preflight-snapshots/" + preflightFP + ".json", "preflight_mutation_calls": 0, "external_preconditions_repeated": true, "outcome": "completed", "entries_created_this_run": 3, "relations_created_this_run": 2, "operations": operationResults(true)}
	run1["fingerprint"] = proofArtifactFingerprint(run1)
	run2 := map[string]any{"schema_version": "productbrain-delivery-run/v0.1", "sequence": 2, "invocation_id": "replay", "outbox_fingerprint": outboxFP, "profile_fingerprint": profileFP, "preflight_fingerprint": preflightFP, "preflight_snapshot_ref": "preflight-snapshots/" + preflightFP + ".json", "preflight_mutation_calls": 0, "external_preconditions_repeated": true, "outcome": "completed", "entries_created_this_run": 0, "relations_created_this_run": 0, "operations": operationResults(false)}
	run2["fingerprint"] = proofArtifactFingerprint(run2)
	writeProofArtifact(t, filepath.Join(root, "delivery", "delivery-runs", "000001-first.json"), run1)
	writeProofArtifact(t, filepath.Join(root, "delivery", "delivery-runs", "000002-replay.json"), run2)
	writeProofArtifact(t, filepath.Join(root, "delivery", "delivery-history.json"), map[string]any{"schema_version": "productbrain-delivery-history/v0.1", "outbox_fingerprint": outboxFP, "profile_fingerprint": profileFP, "run_refs": []string{"delivery-runs/000001-first.json", "delivery-runs/000002-replay.json"}, "runs": []any{run1, run2}})
	writeProofArtifact(t, filepath.Join(root, "delivery", "delivery-summary.json"), map[string]any{"schema_version": "productbrain-delivery-summary/v0.1", "outbox_fingerprint": outboxFP, "profile_fingerprint": profileFP, "preflight_lineage_verified": true, "run_count": 2, "completed_run_count": 2, "interrupted_run_count": 0, "failed_run_count": 0, "expected_operation_count": 5, "entries_acknowledged": 3, "relations_acknowledged": 2, "blocked": 0, "mismatches": 0, "first_run_entry_mutations": 3, "first_run_relation_mutations": 2, "latest_run_entry_mutations": 0, "latest_run_relation_mutations": 0, "replay_zero_mutation": true, "draft_only": true, "entry_actor_verified": true, "relation_attribution_verified": true, "privacy_finding_count": 0, "operator_judged": true, "held_out": false, "generalizable": false, "autonomy_claim": false, "destination_writes": 5, "product_brain_writes": 5})
	if err := privateio.ValidateContained(root, filepath.Join(root, "outbox", "outbox.json")); err != nil {
		t.Fatalf("proof fixture permissions: %v", err)
	}
	packet, err := Build(root, t.TempDir(), Options{Claim: ClaimDelivery})
	if err != nil {
		t.Fatalf("Build delivery proof: %v", err)
	}
	if packet.Verdict != VerdictPass {
		t.Fatalf("expected delivery proof pass: %+v", packet.MandatoryGates)
	}
	baseOutbox := cloneProofMap(outbox)
	basePreflight := cloneProofMap(preflight)
	baseRun1 := cloneProofMap(run1)
	baseRun2 := cloneProofMap(run2)
	outboxPath := filepath.Join(root, "outbox", "outbox.json")
	if err := os.Chmod(outboxPath, 0o644); err != nil {
		t.Fatal(err)
	}
	widenedAuthorityPacket, err := Build(root, t.TempDir(), Options{Claim: ClaimDelivery})
	if err != nil {
		t.Fatal(err)
	}
	if widenedAuthorityPacket.Verdict == VerdictPass {
		t.Fatal("delivery proof accepted permission-widened top-level outbox authority")
	}
	if err := os.Chmod(outboxPath, 0o600); err != nil {
		t.Fatal(err)
	}
	unknownStateRun := cloneProofMap(baseRun1)
	unknownStateRun["outcome"] = "interrupted"
	unknownStateRun["entries_created_this_run"] = 2
	unknownOperation := unknownStateRun["operations"].([]any)[0].(map[string]any)
	unknownOperation["state"] = "mystery_state"
	unknownOperation["attempts"] = 0
	unknownOperation["acknowledged"] = false
	unknownOperation["mutation_observed"] = false
	for _, key := range []string{"entry_doc_id", "remote_object_id", "readback_fingerprint", "draft_verified", "actor_verified"} {
		delete(unknownOperation, key)
	}
	rebindDeliveryProofFixture(t, root, baseOutbox, basePreflight, unknownStateRun, baseRun2, profileFP)
	unknownStatePacket, err := Build(root, t.TempDir(), Options{Claim: ClaimDelivery})
	if err != nil {
		t.Fatal(err)
	}
	if unknownStatePacket.Verdict == VerdictPass {
		t.Fatal("delivery proof accepted an interrupted sealed run with an unknown operation state")
	}
	rebindDeliveryProofFixture(t, root, baseOutbox, basePreflight, baseRun1, baseRun2, profileFP)
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"human entry actor", func(value map[string]any) {
			operation := value["operations"].([]any)[0].(map[string]any)
			operation["entry"].(map[string]any)["created_by"] = "human:user"
			operation["payload_fingerprint"] = ""
			operation["payload_fingerprint"] = proofArtifactFingerprint(operation)
		}},
		{"wrong relation key id", func(value map[string]any) {
			operation := value["operations"].([]any)[3].(map[string]any)
			operation["relation"].(map[string]any)["metadata"].(map[string]any)["credential_key_id"] = "wrong-key"
			operation["payload_fingerprint"] = ""
			operation["payload_fingerprint"] = proofArtifactFingerprint(operation)
		}},
		{"extra proposedBy metadata", func(value map[string]any) {
			operation := value["operations"].([]any)[3].(map[string]any)
			operation["relation"].(map[string]any)["metadata"].(map[string]any)["proposedBy"] = "user"
			operation["payload_fingerprint"] = ""
			operation["payload_fingerprint"] = proofArtifactFingerprint(operation)
		}},
		{"unsupported profile mapping", func(value map[string]any) {
			value["delivery_profile_snapshot"].(map[string]any)["role_mappings"].(map[string]any)["reference_resource"] = map[string]any{"collection_slug": "resources", "id_prefix": "RES"}
		}},
		{"unauthorized review lifecycle action", func(value map[string]any) {
			value["review_context"].(map[string]any)["pending_actions"].([]any)[1] = "Retire the temporary Product Brain key immediately without review"
		}},
		{"new outbox reusing delivered legacy actions", func(value map[string]any) {
			value["review_context"].(map[string]any)["pending_actions"] = []any{
				"Review the three Product Brain drafts and routing judgments",
				"Retire the temporary Product Brain key after review",
				"Confirm owner-validated private runtime cleanup after key retirement",
			}
		}},
	} {
		t.Run("rejects "+test.name, func(t *testing.T) {
			badOutbox := cloneProofMap(baseOutbox)
			test.mutate(badOutbox)
			rebindDeliveryProofFixture(t, root, badOutbox, basePreflight, baseRun1, baseRun2, profileFP)
			badPacket, err := Build(root, t.TempDir(), Options{Claim: ClaimDelivery})
			if err != nil {
				t.Fatal(err)
			}
			if badPacket.Verdict == VerdictPass {
				t.Fatal("delivery proof accepted malformed outbox authority")
			}
		})
	}
	rebindDeliveryProofFixture(t, root, baseOutbox, basePreflight, baseRun1, baseRun2, profileFP)
	bogusPreflight := cloneProofMap(basePreflight)
	bogusContracts := []any{
		map[string]any{"slug": "bogus-a", "fingerprint": "schema-bogus-a"},
		map[string]any{"slug": "bogus-b", "fingerprint": "schema-bogus-b"},
		map[string]any{"slug": "bogus-c", "fingerprint": "schema-bogus-c"},
	}
	bogusPreflight["collection_contracts"] = bogusContracts
	baseGates := bogusPreflight["gates"].([]any)[:7]
	bogusPreflight["gates"] = append(baseGates,
		map[string]any{"name": "collection_contract:bogus-a", "verdict": "pass", "actual": "schema-bogus-a"},
		map[string]any{"name": "collection_contract:bogus-b", "verdict": "pass", "actual": "schema-bogus-b"},
		map[string]any{"name": "collection_contract:bogus-c", "verdict": "pass", "actual": "schema-bogus-c"},
	)
	rebindDeliveryProofFixture(t, root, baseOutbox, bogusPreflight, baseRun1, baseRun2, profileFP)
	bogusPacket, err := Build(root, t.TempDir(), Options{Claim: ClaimDelivery})
	if err != nil {
		t.Fatal(err)
	}
	if bogusPacket.Verdict == VerdictPass {
		t.Fatal("delivery proof accepted unrelated preflight collection contracts")
	}
	wrongWorkspacePreflight := cloneProofMap(basePreflight)
	wrongWorkspacePreflight["workspace"].(map[string]any)["id"] = "wrong-workspace"
	wrongWorkspacePreflight["workspace"].(map[string]any)["slug"] = "wrong-slug"
	wrongWorkspacePreflight["workspace"].(map[string]any)["key_id"] = "wrong-key"
	wrongGates := wrongWorkspacePreflight["gates"].([]any)
	wrongGates[2].(map[string]any)["actual"] = "wrong-workspace"
	wrongGates[3].(map[string]any)["actual"] = "wrong-slug"
	wrongGates[6].(map[string]any)["actual"] = "wrong-key"
	rebindDeliveryProofFixture(t, root, baseOutbox, wrongWorkspacePreflight, baseRun1, baseRun2, profileFP)
	wrongWorkspacePacket, err := Build(root, t.TempDir(), Options{Claim: ClaimDelivery})
	if err != nil {
		t.Fatal(err)
	}
	if wrongWorkspacePacket.Verdict == VerdictPass {
		t.Fatal("delivery proof accepted a self-consistent preflight for the wrong workspace/key identity")
	}
	rebindDeliveryProofFixture(t, root, baseOutbox, basePreflight, baseRun1, baseRun2, profileFP)
	readback, err := evalreadback.BuildSummary(root, evalreadback.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if readback.SampleStatus != "private_curated_sample" || readback.Guardrails.DestinationWrites != 5 || readback.Guardrails.ProductBrainWrites != 5 {
		t.Fatalf("readback lost sample or side-effect truth: %+v %+v", readback.SampleStatus, readback.Guardrails)
	}
	readbackOut := t.TempDir()
	if _, err := evalreadback.Build(root, readbackOut, evalreadback.Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(filepath.Join(readbackOut, evalreadback.DirName), t.TempDir(), Options{Claim: ClaimDelivery}); err == nil {
		t.Fatal("delivery proof trusted a cached readback summary without reopening sealed authority")
	}
	sealedReplay := filepath.Join(root, "delivery", "delivery-runs", "000002-replay.json")
	historyPath := filepath.Join(root, "delivery", "delivery-history.json")
	badFirstRun := cloneProofMap(run1)
	badFirstOperations := badFirstRun["operations"].([]any)
	badFirstOperations[0].(map[string]any)["remote_object_id"] = "WRONG-EARLIER-DESTINATION"
	badFirstOperations[0].(map[string]any)["readback_fingerprint"] = "wrong-earlier-readback"
	writeProofArtifact(t, filepath.Join(root, "delivery", "delivery-runs", "000001-first.json"), badFirstRun)
	writeProofArtifact(t, historyPath, map[string]any{"schema_version": "productbrain-delivery-history/v0.1", "outbox_fingerprint": outboxFP, "profile_fingerprint": profileFP, "run_refs": []string{"delivery-runs/000001-first.json", "delivery-runs/000002-replay.json"}, "runs": []any{badFirstRun, run2}})
	packet, err = Build(root, t.TempDir(), Options{Claim: ClaimDelivery})
	if err != nil {
		t.Fatal(err)
	}
	if packet.Verdict == VerdictPass {
		t.Fatal("delivery proof accepted a wrong destination/readback identity in an earlier mutating run")
	}
	writeProofArtifact(t, filepath.Join(root, "delivery", "delivery-runs", "000001-first.json"), run1)
	writeProofArtifact(t, historyPath, map[string]any{"schema_version": "productbrain-delivery-history/v0.1", "outbox_fingerprint": outboxFP, "profile_fingerprint": profileFP, "run_refs": []string{"delivery-runs/000001-first.json", "delivery-runs/000002-replay.json"}, "runs": []any{run1, run2}})

	badRun := cloneProofMap(run2)
	badOperations := badRun["operations"].([]any)
	badOperations[0].(map[string]any)["remote_object_id"] = "WRONG-DESTINATION-ID"
	writeProofArtifact(t, sealedReplay, badRun)
	writeProofArtifact(t, historyPath, map[string]any{"schema_version": "productbrain-delivery-history/v0.1", "outbox_fingerprint": outboxFP, "profile_fingerprint": profileFP, "run_refs": []string{"delivery-runs/000001-first.json", "delivery-runs/000002-replay.json"}, "runs": []any{run1, badRun}})
	packet, err = Build(root, t.TempDir(), Options{Claim: ClaimDelivery})
	if err != nil {
		t.Fatal(err)
	}
	if packet.Verdict == VerdictPass {
		t.Fatal("delivery proof passed an acknowledgement bound to the wrong destination identity")
	}
	writeProofArtifact(t, sealedReplay, run2)
	writeProofArtifact(t, historyPath, map[string]any{"schema_version": "productbrain-delivery-history/v0.1", "outbox_fingerprint": outboxFP, "profile_fingerprint": profileFP, "run_refs": []string{"delivery-runs/000001-first.json", "delivery-runs/000002-replay.json"}, "runs": []any{run1, run2}})
	badSchema := cloneProofMap(run2)
	badSchema["schema_version"] = "productbrain-delivery-run/v9"
	writeProofArtifact(t, sealedReplay, badSchema)
	writeProofArtifact(t, historyPath, map[string]any{"schema_version": "productbrain-delivery-history/v0.1", "outbox_fingerprint": outboxFP, "profile_fingerprint": profileFP, "run_refs": []string{"delivery-runs/000001-first.json", "delivery-runs/000002-replay.json"}, "runs": []any{run1, badSchema}})
	packet, err = Build(root, t.TempDir(), Options{Claim: ClaimDelivery})
	if err != nil {
		t.Fatal(err)
	}
	if packet.Verdict == VerdictPass {
		t.Fatal("delivery proof accepted a sealed run with an unsupported schema")
	}
	writeProofArtifact(t, sealedReplay, run2)
	writeProofArtifact(t, historyPath, map[string]any{"schema_version": "productbrain-delivery-history/v0.1", "outbox_fingerprint": outboxFP, "profile_fingerprint": profileFP, "run_refs": []string{"delivery-runs/000001-first.json", "delivery-runs/000002-replay.json"}, "runs": []any{run1, run2}})
	if err := os.Remove(sealedReplay); err != nil {
		t.Fatal(err)
	}
	packet, err = Build(root, t.TempDir(), Options{Claim: ClaimDelivery})
	if err != nil {
		t.Fatalf("Build missing-sealed-run proof: %v", err)
	}
	if packet.Verdict == VerdictPass {
		t.Fatal("delivery proof passed without its referenced sealed run")
	}
	externalRun := filepath.Join(t.TempDir(), "outside-run.json")
	writeProofArtifact(t, externalRun, run2)
	if err := os.Symlink(externalRun, sealedReplay); err != nil {
		t.Fatal(err)
	}
	packet, err = Build(root, t.TempDir(), Options{Claim: ClaimDelivery})
	if err != nil {
		t.Fatal(err)
	}
	if packet.Verdict == VerdictPass {
		t.Fatal("delivery proof followed a symlinked sealed authority ref")
	}
	if err := os.Remove(sealedReplay); err != nil {
		t.Fatal(err)
	}
	writeProofArtifact(t, sealedReplay, run2)
	snapshot := filepath.Join(root, "delivery", "preflight-snapshots", preflightFP+".json")
	if err := os.Remove(snapshot); err != nil {
		t.Fatal(err)
	}
	packet, err = Build(root, t.TempDir(), Options{Claim: ClaimDelivery})
	if err != nil {
		t.Fatalf("Build missing-snapshot proof: %v", err)
	}
	if packet.Verdict == VerdictPass {
		t.Fatal("delivery proof passed without its referenced preflight snapshot")
	}
	writeProofArtifact(t, snapshot, preflight)
	summaryPath := filepath.Join(root, "delivery", "delivery-summary.json")
	var broken map[string]any
	data, _ := os.ReadFile(summaryPath)
	_ = json.Unmarshal(data, &broken)
	broken["replay_zero_mutation"] = false
	writeProofArtifact(t, summaryPath, broken)
	packet, err = Build(root, t.TempDir(), Options{Claim: ClaimDelivery})
	if err != nil {
		t.Fatalf("Build broken delivery proof: %v", err)
	}
	if packet.Verdict == VerdictPass {
		t.Fatal("delivery proof passed without zero-mutation replay")
	}
}

func TestBuildRejectsProtectedRootBeforeChangingIt(t *testing.T) {
	protected := t.TempDir()
	if err := os.Chmod(protected, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Build(t.TempDir(), protected, Options{Claim: ClaimSafety, ProtectedRoots: []string{protected}})
	if err == nil {
		t.Fatal("proof build accepted a protected output root")
	}
	info, statErr := os.Stat(protected)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("protected root mode was changed before rejection: %o", info.Mode().Perm())
	}
}

func proofDeliveryProfile() map[string]any {
	return map[string]any{
		"schema_version": "productbrain-delivery-profile/v0.1",
		"profile_id":     "test",
		"workspace":      map[string]any{"expected_id": "ws-test", "expected_slug": "test"},
		"transport":      map[string]any{"kind": "aki", "base_url": "https://gateway.productbrain.io", "api_path": "/api/aki"},
		"credential":     map[string]any{"provider": "environment", "name": "MINDLINE_PRODUCT_BRAIN_API_KEY", "expected_key_id": "safe-key-id"},
		"role_mappings": map[string]any{
			"external_entity":         map[string]any{"collection_slug": "landscape", "id_prefix": "LAND"},
			"evidence_backed_finding": map[string]any{"collection_slug": "insights", "id_prefix": "INS"},
			"unresolved_tension":      map[string]any{"collection_slug": "tensions", "id_prefix": "TEN"},
		},
		"relation_mappings": map[string]any{"related_to": "related_to"},
		"draft_only":        true,
	}
}

func proofDeliveryProfileSnapshot() map[string]any {
	return map[string]any{
		"profile_id":              "test",
		"transport_kind":          "aki",
		"transport_api_path":      "/api/aki",
		"expected_origin":         "https://gateway.productbrain.io",
		"expected_workspace_id":   "ws-test",
		"expected_workspace_slug": "test",
		"expected_key_id":         "safe-key-id",
		"draft_only":              true,
		"role_mappings": map[string]any{
			"external_entity":         map[string]any{"collection_slug": "landscape", "id_prefix": "LAND"},
			"evidence_backed_finding": map[string]any{"collection_slug": "insights", "id_prefix": "INS"},
			"unresolved_tension":      map[string]any{"collection_slug": "tensions", "id_prefix": "TEN"},
		},
		"relation_mappings": map[string]any{"related_to": "related_to"},
	}
}

func proofReviewContext(operations []any) map[string]any {
	lenses := []any{
		map[string]any{"lens_id": "building-product", "result": "matched", "confidence": .9, "rationale": "Relevant to the product landscape.", "evidence_refs": []string{"public-evidence"}, "missingness": []string{}},
		map[string]any{"lens_id": "team-design", "result": "matched", "confidence": .8, "rationale": "Relevant to AI-native team design.", "evidence_refs": []string{"public-evidence"}, "missingness": []string{}},
	}
	assessment := map[string]any{"primary_role": "reference_resource", "summary": "A public source with bounded strategic relevance.", "confidence": .8, "evidence_refs": []string{"public-evidence"}, "missingness": []string{}}
	captures := make([]any, 0, 12)
	for index := 1; index <= 12; index++ {
		canonicalIndex := index
		if index == 2 {
			canonicalIndex = 1
		}
		canonicalURL := fmt.Sprintf("https://example.com/source-%d", canonicalIndex)
		canonicalID := fmt.Sprintf("source-%d", canonicalIndex)
		if index == 12 {
			canonicalURL = "https://example.com/source"
			canonicalID = "source-12"
		}
		capture := map[string]any{
			"capture_ref":               fmt.Sprintf("capture-%03d", index),
			"canonical_url":             canonicalURL,
			"canonical_url_id":          canonicalID,
			"enrichment_state":          "complete",
			"public_metadata":           map[string]any{"title": fmt.Sprintf("Source %d", canonicalIndex)},
			"public_excerpts":           []any{map[string]any{"excerpt_id": "public-evidence", "text": "Public evidence.", "locator": "page"}},
			"missingness":               []string{},
			"semantic_assessment":       assessment,
			"lens_results":              lenses,
			"disposition":               "hold",
			"disposition_rationale":     "Retain for later review.",
			"semantic_nodes":            []any{},
			"semantic_edges":            []any{},
			"destination_operation_ids": []string{},
		}
		if index == 2 {
			capture["duplicate_of"] = "capture-001"
		}
		if index == 12 {
			capture["disposition"] = "promote"
			capture["disposition_rationale"] = "Promote the evidence-backed constellation."
			capture["semantic_nodes"] = []any{
				map[string]any{"semantic_node_id": "entity", "role": "external_entity", "name": "Entity", "description": "External entity.", "confidence": .9, "lens_refs": []string{"building-product"}, "evidence_refs": []string{"public-evidence"}, "attributes": map[string]any{}},
				map[string]any{"semantic_node_id": "insight", "role": "evidence_backed_finding", "name": "Insight", "description": "Evidence-backed finding.", "confidence": .9, "lens_refs": []string{"building-product", "team-design"}, "evidence_refs": []string{"public-evidence"}, "attributes": map[string]any{}},
				map[string]any{"semantic_node_id": "tension", "role": "unresolved_tension", "name": "Tension", "description": "Unresolved tension.", "confidence": .9, "lens_refs": []string{"building-product", "team-design"}, "evidence_refs": []string{"public-evidence"}, "attributes": map[string]any{}},
			}
			capture["semantic_edges"] = []any{
				map[string]any{"from": "entity", "type": "related_to", "to": "insight", "rationale": "Evidence relation.", "evidence_refs": []string{"public-evidence"}},
				map[string]any{"from": "entity", "type": "related_to", "to": "tension", "rationale": "Evidence relation.", "evidence_refs": []string{"public-evidence"}},
			}
			operationIDs := make([]string, 0, len(operations))
			for _, value := range operations {
				operationIDs = append(operationIDs, value.(map[string]any)["operation_id"].(string))
			}
			capture["destination_operation_ids"] = operationIDs
		}
		captures = append(captures, capture)
	}
	return map[string]any{
		"captures": captures,
		"depth_one_sources": []any{map[string]any{
			"canonical_url":        "https://example.com/depth-one",
			"parent_canonical_url": "https://example.com/source",
			"enrichment_state":     "complete",
			"public_metadata":      map[string]any{"title": "Depth one"},
			"public_excerpts":      []any{map[string]any{"excerpt_id": "public-evidence", "text": "Public evidence.", "locator": "page"}},
			"semantic_assessment":  assessment,
			"lens_results":         lenses,
			"disposition":          "hold",
		}},
		"pending_actions": []string{
			"Review 3 Product Brain draft entries and 2 proposed relations; accept or reject the routing judgments",
			"Complete the Product Brain credential lifecycle required by the selected delivery profile",
			"Confirm private runtime retention or cleanup after review",
		},
	}
}

func rebindDeliveryProofFixture(t *testing.T, root string, outbox, preflight, run1, run2 map[string]any, profileFingerprint string) {
	t.Helper()
	outbox = cloneProofMap(outbox)
	preflight = cloneProofMap(preflight)
	run1 = cloneProofMap(run1)
	run2 = cloneProofMap(run2)
	writeProofArtifact(t, filepath.Join(root, "outbox", "outbox.json"), outbox)
	outboxFingerprint := outbox["fingerprint"].(string)
	writeProofArtifact(t, filepath.Join(root, "outbox", "outbox-summary.json"), map[string]any{"schema_version": "productbrain-outbox-summary/v0.1", "outbox_fingerprint": outboxFingerprint, "operation_count": 5, "entry_operation_count": 3, "relation_operation_count": 2, "privacy_finding_count": 0, "draft_only": true, "operator_judged": true, "held_out": false, "generalizable": false})
	preflight["outbox_fingerprint"] = outboxFingerprint
	preflight["profile_fingerprint"] = profileFingerprint
	writeProofArtifact(t, filepath.Join(root, "preflight", "preflight.json"), preflight)
	preflightFingerprint := preflight["fingerprint"].(string)
	snapshotDir := filepath.Join(root, "delivery", "preflight-snapshots")
	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if err := os.Remove(filepath.Join(snapshotDir, entry.Name())); err != nil {
			t.Fatal(err)
		}
	}
	writeProofArtifact(t, filepath.Join(snapshotDir, preflightFingerprint+".json"), preflight)
	for index, run := range []map[string]any{run1, run2} {
		run["outbox_fingerprint"] = outboxFingerprint
		run["profile_fingerprint"] = profileFingerprint
		run["preflight_fingerprint"] = preflightFingerprint
		run["preflight_snapshot_ref"] = "preflight-snapshots/" + preflightFingerprint + ".json"
		name := "000001-first.json"
		if index == 1 {
			name = "000002-replay.json"
		}
		writeProofArtifact(t, filepath.Join(root, "delivery", "delivery-runs", name), run)
	}
	writeProofArtifact(t, filepath.Join(root, "delivery", "delivery-history.json"), map[string]any{"schema_version": "productbrain-delivery-history/v0.1", "outbox_fingerprint": outboxFingerprint, "profile_fingerprint": profileFingerprint, "run_refs": []string{"delivery-runs/000001-first.json", "delivery-runs/000002-replay.json"}, "runs": []any{run1, run2}})
	writeProofArtifact(t, filepath.Join(root, "delivery", "delivery-summary.json"), map[string]any{"schema_version": "productbrain-delivery-summary/v0.1", "outbox_fingerprint": outboxFingerprint, "profile_fingerprint": profileFingerprint, "preflight_lineage_verified": true, "run_count": 2, "completed_run_count": 2, "interrupted_run_count": 0, "failed_run_count": 0, "expected_operation_count": 5, "entries_acknowledged": 3, "relations_acknowledged": 2, "blocked": 0, "mismatches": 0, "first_run_entry_mutations": 3, "first_run_relation_mutations": 2, "latest_run_entry_mutations": 0, "latest_run_relation_mutations": 0, "replay_zero_mutation": true, "draft_only": true, "entry_actor_verified": true, "relation_attribution_verified": true, "privacy_finding_count": 0, "operator_judged": true, "held_out": false, "generalizable": false, "autonomy_claim": false, "destination_writes": 5, "product_brain_writes": 5})
}

func writeProofArtifact(t *testing.T, path string, value map[string]any) {
	t.Helper()
	delete(value, "fingerprint")
	value["fingerprint"] = proofArtifactFingerprint(value)
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
func proofArtifactFingerprint(value any) string {
	data, _ := json.Marshal(value)
	var clone any
	_ = json.Unmarshal(data, &clone)
	stripProofFingerprints(clone)
	canonical, _ := json.Marshal(clone)
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func proofSHA256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func proofDeterministicEntryID(prefix, canonicalURL, nodeID, collection string) string {
	sum := sha256.Sum256([]byte("mindline/v0.1|" + canonicalURL + "|" + nodeID + "|" + collection))
	number := new(big.Int).SetBytes(sum[:10])
	return prefix + "-" + number.String()
}

func proofEntryReadback(entry map[string]any, docID string) map[string]any {
	// Re-marshal through a field-ordered struct so the proof fixture matches the
	// delivery adapter's exact EntryReadback fingerprint contract.
	value := struct {
		Found          bool           `json:"found"`
		DocID          string         `json:"doc_id,omitempty"`
		EntryID        string         `json:"entry_id,omitempty"`
		CollectionSlug string         `json:"collection_slug,omitempty"`
		Name           string         `json:"name,omitempty"`
		Status         string         `json:"status,omitempty"`
		Data           map[string]any `json:"data,omitempty"`
		SourceRef      string         `json:"source_ref,omitempty"`
		SourceExcerpt  string         `json:"source_excerpt,omitempty"`
		CreatedBy      string         `json:"created_by,omitempty"`
	}{true, docID, entry["entry_id"].(string), entry["collection_slug"].(string), entry["name"].(string), "draft", entry["data"].(map[string]any), entry["source_ref"].(string), entry["source_excerpt"].(string), entry["created_by"].(string)}
	data, _ := json.Marshal(value)
	var raw map[string]any
	_ = json.Unmarshal(data, &raw)
	return raw
}

func cloneProofMap(value map[string]any) map[string]any {
	data, _ := json.Marshal(value)
	var clone map[string]any
	_ = json.Unmarshal(data, &clone)
	return clone
}
func stripProofFingerprints(value any) {
	switch item := value.(type) {
	case map[string]any:
		delete(item, "fingerprint")
		for _, child := range item {
			stripProofFingerprints(child)
		}
	case []any:
		for _, child := range item {
			stripProofFingerprints(child)
		}
	}
}

func TestImprovementProofPassesWithComparableBaseline(t *testing.T) {
	out := t.TempDir()
	packet, err := Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), out, Options{
		BaselineRoot: filepath.Join("..", "..", "testdata", "eval-readback", "baseline"),
		Claim:        ClaimImprovement,
	})
	if err != nil {
		t.Fatalf("build proof: %v", err)
	}
	if packet.Verdict != VerdictPass || packet.ExitCode != 0 {
		t.Fatalf("expected pass, got %+v", packet)
	}
	if gateVerdict(packet, "improvement_claim") != VerdictPass {
		t.Fatalf("expected improvement gate pass: %+v", packet.MandatoryGates)
	}
	for _, rel := range []string{"proof-packet.json", "proof-report.md", "chain-capture-draft.md", filepath.Join("readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile)} {
		if _, err := os.Stat(filepath.Join(out, DirName, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	assertProofOutputSafe(t, filepath.Join(out, DirName))
}

func TestImprovementProofUsesReadbackOutputAsBaseline(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writeProofPressure(t, baseline, 0.4, 0.7, "same", completeProofGuardrails())
	writeProofPressure(t, current, 0.9, 0.2, "same", completeProofGuardrails())

	baselineReadback := filepath.Join(root, "baseline-readback")
	if _, err := evalreadback.Build(baseline, baselineReadback, evalreadback.Options{}); err != nil {
		t.Fatalf("baseline readback: %v", err)
	}

	packet, err := Build(current, filepath.Join(root, "proof"), Options{
		BaselineRoot: baselineReadback,
		Claim:        ClaimImprovement,
	})
	if err != nil {
		t.Fatalf("build proof: %v", err)
	}
	if packet.Verdict != VerdictPass || gateVerdict(packet, "improvement_claim") != VerdictPass {
		t.Fatalf("expected improvement proof to pass with readback baseline, got %+v", packet)
	}
}

func TestImprovementProofBlocksReadbackBaselineThatIsNotReplayReady(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writeProofJSON(t, filepath.Join(baseline, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version":            "corpus-pressure-summary/v0.1",
		"evidence_ready_atom_ratio": 0.4,
		"review_burden_ratio":       0.7,
		"corpus_fingerprint":        "same",
		"guardrails":                completeProofGuardrails(),
	})
	writeProofPressure(t, current, 0.9, 0.2, "same", completeProofGuardrails())

	baselineReadback := filepath.Join(root, "baseline-readback")
	if _, err := evalreadback.Build(baseline, baselineReadback, evalreadback.Options{}); err != nil {
		t.Fatalf("baseline readback: %v", err)
	}

	packet, err := Build(current, filepath.Join(root, "proof"), Options{
		BaselineRoot: baselineReadback,
		Claim:        ClaimImprovement,
	})
	if err != nil {
		t.Fatalf("build proof: %v", err)
	}
	if packet.Verdict != VerdictBlocked ||
		!gateHasReason(packet, "improvement_claim", "replay_baseline_blocked") ||
		!gateHasReason(packet, "improvement_claim", "missing_command_config_fingerprint") {
		t.Fatalf("expected blocked replay baseline reasons, got %+v", packet.MandatoryGates)
	}
}

func TestImprovementProofRefreshesLegacyReadbackBaselineBeforeComparison(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writeProofJSON(t, filepath.Join(baseline, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version":            "corpus-pressure-summary/v0.1",
		"evidence_ready_atom_ratio": 0.4,
		"review_burden_ratio":       0.7,
		"corpus_fingerprint":        "same",
		"guardrails":                completeProofGuardrails(),
	})
	writeProofJSON(t, filepath.Join(current, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version":            "corpus-pressure-summary/v0.1",
		"evidence_ready_atom_ratio": 0.9,
		"review_burden_ratio":       0.2,
		"corpus_fingerprint":        "same",
		"guardrails":                completeProofGuardrails(),
	})

	legacySummary, err := evalreadback.BuildSummary(baseline, evalreadback.Options{})
	if err != nil {
		t.Fatalf("build baseline summary: %v", err)
	}
	legacySummary.ReplayBaseline = evalreadback.ReplayBaseline{}
	legacyReadback := filepath.Join(root, "legacy-readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile)
	writeProofSummary(t, legacyReadback, legacySummary)

	packet, err := Build(current, filepath.Join(root, "proof"), Options{
		BaselineRoot: legacyReadback,
		Claim:        ClaimImprovement,
	})
	if err != nil {
		t.Fatalf("build proof: %v", err)
	}
	if packet.Verdict != VerdictBlocked ||
		!gateHasReason(packet, "improvement_claim", "replay_baseline_blocked") ||
		!gateHasReason(packet, "improvement_claim", "missing_command_config_fingerprint") {
		t.Fatalf("expected stale readback baseline to be refreshed and blocked, got %+v", packet.MandatoryGates)
	}
}

func TestSafetyProofPassesWithoutBaseline(t *testing.T) {
	packet, err := Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), t.TempDir(), Options{Claim: ClaimSafety})
	if err != nil {
		t.Fatalf("build proof: %v", err)
	}
	if packet.Verdict != VerdictPass {
		t.Fatalf("expected safety pass without baseline, got %+v", packet)
	}
	if gateVerdict(packet, "improvement_claim") != "" {
		t.Fatalf("safety claim should not require improvement gate: %+v", packet.MandatoryGates)
	}
}

func TestProofPacketJSONIncludesRequiredEmptyFields(t *testing.T) {
	packet, err := Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), t.TempDir(), Options{Claim: ClaimSafety})
	if err != nil {
		t.Fatalf("build proof: %v", err)
	}
	data, err := json.Marshal(packet)
	if err != nil {
		t.Fatalf("marshal packet: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal packet: %v", err)
	}
	for _, key := range []string{"baseline_root_label", "blocked_claims", "failed_claims", "permitted_claims"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("proof packet missing required field %q: %s", key, string(data))
		}
	}
	for _, key := range []string{"blocked_claims", "failed_claims", "permitted_claims"} {
		if _, ok := raw[key].([]any); !ok {
			t.Fatalf("proof packet field %q must serialize as an array, got %#v in %s", key, raw[key], string(data))
		}
	}
}

func TestImprovementProofBlocksWithoutBaseline(t *testing.T) {
	packet, err := Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), t.TempDir(), Options{Claim: ClaimImprovement})
	if err != nil {
		t.Fatalf("build proof: %v", err)
	}
	if packet.Verdict != VerdictBlocked || packet.ExitCode == 0 {
		t.Fatalf("expected blocked nonzero proof, got %+v", packet)
	}
	if !gateHasReason(packet, "improvement_claim", "missing_baseline") {
		t.Fatalf("expected missing_baseline, got %+v", packet.MandatoryGates)
	}
}

func TestImprovementProofBlocksSemanticReadinessCollapse(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writeProofReferenceOnlyCollapsePressure(t, baseline)
	writeProofReferenceOnlyCollapsePressure(t, current)

	packet, err := Build(current, filepath.Join(root, "proof"), Options{
		Claim:        ClaimImprovement,
		BaselineRoot: baseline,
	})
	if err != nil {
		t.Fatalf("build proof: %v", err)
	}
	if packet.Verdict != VerdictBlocked {
		t.Fatalf("expected blocked proof, got %+v", packet)
	}
	if !gateHasReason(packet, "improvement_claim", "semantic_readiness_blocked") ||
		!gateHasReason(packet, "improvement_claim", "reference_only_one_candidate_per_source") {
		t.Fatalf("expected semantic readiness block reasons, got %+v", packet.MandatoryGates)
	}
}

func TestProofPacketEmittedWhenArtifactsMissing(t *testing.T) {
	root := t.TempDir()
	packet, err := Build(root, filepath.Join(t.TempDir(), "proof"), Options{Claim: ClaimSafety})
	if err != nil {
		t.Fatalf("missing artifacts should produce proof packet, got error: %v", err)
	}
	if packet.Verdict != VerdictFail || !gateHasReason(packet, "artifact_presence", "missing_proof") {
		t.Fatalf("expected missing proof packet, got %+v", packet)
	}
}

func TestImprovementProofPreservesNotComparableReason(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writeProofPressure(t, baseline, 0.2, 0.8, "baseline-fingerprint", completeProofGuardrails())
	writeProofPressure(t, current, 0.8, 0.3, "current-fingerprint", completeProofGuardrails())

	packet, err := Build(current, filepath.Join(root, "proof"), Options{Claim: ClaimImprovement, BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("build not-comparable proof: %v", err)
	}
	if packet.Verdict != VerdictBlocked || !gateHasReason(packet, "improvement_claim", "not_comparable") {
		t.Fatalf("expected not_comparable, got %+v", packet.MandatoryGates)
	}
}

func TestReadbackDeniesSecretLikeFingerprint(t *testing.T) {
	root := t.TempDir()
	writeProofPressure(t, root, 1, 0, "sk-proj-secret-do-not-leak", completeProofGuardrails())
	packet, err := Build(root, filepath.Join(t.TempDir(), "proof"), Options{Claim: ClaimSafety})
	if err != nil {
		t.Fatalf("secret-like input should be converted into failed proof, not leak through error: %v", err)
	}
	if packet.Verdict != VerdictFail || !gateHasReason(packet, "privacy_safe_readback", "unsafe_output") {
		t.Fatalf("expected unsafe output failure, got %+v", packet.MandatoryGates)
	}
}

func TestSafetyProofBlocksIncompleteSideEffectEvidence(t *testing.T) {
	root := t.TempDir()
	writeProofPressure(t, root, 1, 0, "same", map[string]any{
		"hosted_telemetry_exports": 0,
		"hosted_inference_calls":   0,
		"destination_writes":       0,
	})
	packet, err := Build(root, filepath.Join(t.TempDir(), "proof"), Options{Claim: ClaimSafety})
	if err != nil {
		t.Fatalf("build incomplete guardrail proof: %v", err)
	}
	if packet.Verdict != VerdictBlocked || !gateHasReason(packet, "side_effect_claim", "missing_side_effect_evidence") {
		t.Fatalf("expected missing side-effect evidence, got %+v", packet.MandatoryGates)
	}
}

func TestGeneralizationAndDEC64FailWhenClaimsBlocked(t *testing.T) {
	root := t.TempDir()
	writeProofPressure(t, root, 1, 0, "same", completeProofGuardrails())
	packet, err := Build(root, filepath.Join(t.TempDir(), "generalization"), Options{Claim: ClaimGeneralization})
	if err != nil {
		t.Fatalf("build generalization proof: %v", err)
	}
	if packet.Verdict != VerdictBlocked || !gateHasReason(packet, "generalization_claim", "non_generalizable") {
		t.Fatalf("expected generalization blocked for private runtime, got %+v", packet)
	}
	packet, err = Build(root, filepath.Join(t.TempDir(), "dec64"), Options{Claim: ClaimDEC64})
	if err != nil {
		t.Fatalf("build dec64 proof: %v", err)
	}
	if packet.Verdict != VerdictBlocked || gateVerdict(packet, "dec64_no_human_claim") != VerdictBlocked {
		t.Fatalf("expected dec64 blocked, got %+v", packet)
	}
}

func TestProofFailsUnsupportedSchemaAndSideEffects(t *testing.T) {
	root := t.TempDir()
	writeProofJSON(t, filepath.Join(root, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version": "corpus-pressure-summary/v9",
	})
	packet, err := Build(root, filepath.Join(t.TempDir(), "unsupported"), Options{Claim: ClaimSafety})
	if err != nil {
		t.Fatalf("build unsupported proof: %v", err)
	}
	if packet.Verdict != VerdictFail || !gateHasReason(packet, "schema_supported", "unsupported_artifact") {
		t.Fatalf("expected unsupported schema fail, got %+v", packet.MandatoryGates)
	}

	sideEffectRoot := t.TempDir()
	guardrails := completeProofGuardrails()
	guardrails["destination_writes"] = 1
	writeProofPressure(t, sideEffectRoot, 1, 0, "same", guardrails)
	packet, err = Build(sideEffectRoot, filepath.Join(t.TempDir(), "side-effect"), Options{Claim: ClaimSafety})
	if err != nil {
		t.Fatalf("build side-effect proof: %v", err)
	}
	if packet.Verdict != VerdictFail || !gateHasReason(packet, "side_effect_claim", "guardrail_failed") {
		t.Fatalf("expected side-effect fail, got %+v", packet.MandatoryGates)
	}
}

func TestProofLoadsExistingReadbackSummary(t *testing.T) {
	root := t.TempDir()
	readbackOut := filepath.Join(root, "readback")
	if _, err := evalreadback.Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), readbackOut, evalreadback.Options{
		BaselineRoot: filepath.Join("..", "..", "testdata", "eval-readback", "baseline"),
	}); err != nil {
		t.Fatalf("build readback: %v", err)
	}
	packet, err := Build(filepath.Join(readbackOut, evalreadback.DirName, evalreadback.ReadbackSummaryFile), filepath.Join(root, "proof"), Options{Claim: ClaimImprovement})
	if err != nil {
		t.Fatalf("build proof from readback: %v", err)
	}
	expectedRef := filepath.ToSlash(filepath.Join("readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile))
	if packet.Verdict != VerdictPass || packet.ReadbackSummaryRef != expectedRef {
		t.Fatalf("unexpected proof from readback: %+v", packet)
	}
	if _, err := os.Stat(filepath.Join(root, "proof", DirName, "readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile)); err != nil {
		t.Fatalf("expected proof-local readback summary copy: %v", err)
	}
}

func TestProofPreservesNestedExistingReadbackSummaryRef(t *testing.T) {
	root := t.TempDir()
	readbackOut := filepath.Join(root, "run")
	if _, err := evalreadback.Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), readbackOut, evalreadback.Options{
		BaselineRoot: filepath.Join("..", "..", "testdata", "eval-readback", "baseline"),
	}); err != nil {
		t.Fatalf("build readback: %v", err)
	}
	packet, err := Build(readbackOut, filepath.Join(root, "proof"), Options{Claim: ClaimImprovement})
	if err != nil {
		t.Fatalf("build proof from nested readback: %v", err)
	}
	expectedRef := filepath.ToSlash(filepath.Join("readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile))
	if packet.Verdict != VerdictPass || packet.ReadbackSummaryRef != expectedRef {
		t.Fatalf("unexpected nested proof ref: %+v", packet)
	}
	if _, err := os.Stat(filepath.Join(root, "proof", DirName, "readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile)); err != nil {
		t.Fatalf("expected nested proof-local readback summary copy: %v", err)
	}
}

func TestProofAppliesBaselineToExistingReadbackSummary(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	readbackOut := filepath.Join(root, "readback")
	writeProofPressure(t, baseline, 0.2, 0.8, "same", completeProofGuardrails())
	writeProofPressure(t, current, 0.8, 0.3, "same", completeProofGuardrails())
	if _, err := evalreadback.Build(current, readbackOut, evalreadback.Options{}); err != nil {
		t.Fatalf("build readback without baseline: %v", err)
	}

	packet, err := Build(filepath.Join(readbackOut, evalreadback.DirName, evalreadback.ReadbackSummaryFile), filepath.Join(root, "proof"), Options{
		Claim:        ClaimImprovement,
		BaselineRoot: baseline,
	})
	if err != nil {
		t.Fatalf("build proof from readback with supplied baseline: %v", err)
	}
	if packet.Verdict != VerdictPass || gateVerdict(packet, "improvement_claim") != VerdictPass {
		t.Fatalf("expected supplied baseline to produce improvement proof, got %+v", packet)
	}
}

func TestProofReevaluatesCachedReadbackSummary(t *testing.T) {
	root := t.TempDir()
	run := filepath.Join(root, "run")
	writeProofPressure(t, run, 1, 0, "same", map[string]any{
		"hosted_telemetry_exports": 0,
		"hosted_inference_calls":   0,
		"destination_writes":       0,
	})
	summary, err := evalreadback.BuildSummary(run, evalreadback.Options{})
	if err != nil {
		t.Fatalf("build stale source summary: %v", err)
	}
	for i := range summary.ClaimGates {
		if summary.ClaimGates[i].Gate == "side_effect_claim" {
			summary.ClaimGates[i].Status = "pass"
			summary.ClaimGates[i].ReasonCodes = nil
		}
	}
	writeProofSummary(t, filepath.Join(run, evalreadback.DirName, evalreadback.ReadbackSummaryFile), summary)

	packet, err := Build(run, filepath.Join(root, "proof"), Options{Claim: ClaimSafety})
	if err != nil {
		t.Fatalf("build proof from stale readback: %v", err)
	}
	if packet.Verdict != VerdictBlocked || !gateHasReason(packet, "side_effect_claim", "missing_side_effect_evidence") {
		t.Fatalf("expected cached gates to be re-evaluated, got %+v", packet.MandatoryGates)
	}
}

func gateVerdict(packet Packet, gate string) string {
	for _, result := range packet.MandatoryGates {
		if result.Gate == gate {
			return result.Verdict
		}
	}
	return ""
}

func gateHasReason(packet Packet, gate string, reason string) bool {
	for _, result := range packet.MandatoryGates {
		if result.Gate != gate {
			continue
		}
		for _, actual := range result.ReasonCodes {
			if actual == reason {
				return true
			}
		}
	}
	return false
}

func writeProofPressure(t *testing.T, root string, evidenceReady, reviewBurden float64, fingerprint string, guardrails map[string]any) {
	t.Helper()
	writeProofJSON(t, filepath.Join(root, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version":             "corpus-pressure-summary/v0.1",
		"evidence_ready_atom_ratio":  evidenceReady,
		"review_burden_ratio":        reviewBurden,
		"corpus_fingerprint":         fingerprint,
		"command_config_fingerprint": "same-config",
		"guardrails":                 guardrails,
	})
}

func writeProofReferenceOnlyCollapsePressure(t *testing.T, root string) {
	t.Helper()
	sources := make([]any, 0, 50)
	for i := 0; i < 50; i++ {
		sourceID := fmt.Sprintf("source-%02d", i)
		sources = append(sources, map[string]any{
			"source_id":       sourceID,
			"source_kind":     "markdown",
			"state":           "processed",
			"reason_code":     "none",
			"candidate_count": 1,
			"candidate_kind_counts": map[string]any{
				"reference_candidate": 1,
			},
			"semantic_run_dir": filepath.ToSlash(filepath.Join("sources", sourceID)),
		})
		writeProofJSON(t, filepath.Join(root, "sources", sourceID, "document-segments", "segment-summary.json"), map[string]any{
			"schema_version": "document-segment-summary/v0.1",
			"run_id":         "run",
			"source_count":   1,
			"segment_count":  8,
			"segments":       []any{},
			"type_counts":    map[string]any{"source_note": 8},
		})
		writeProofJSON(t, filepath.Join(root, "sources", sourceID, "semantic-candidates", "semantic-summary.json"), map[string]any{
			"schema_version":    "semantic-candidate-summary/v0.1",
			"run_id":            "run",
			"source_count":      1,
			"observation_count": 1,
			"candidate_count":   1,
			"candidate_kind_counts": map[string]any{
				"reference_candidate": 1,
			},
		})
	}
	writeProofJSON(t, filepath.Join(root, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version":             "corpus-pressure-summary/v0.1",
		"corpus_id":                  "corpus-a",
		"source_count":               50,
		"processed_source_count":     50,
		"semantic_candidate_count":   50,
		"evidence_ready_atom_ratio":  1,
		"review_burden_ratio":        0,
		"corpus_fingerprint":         "same",
		"command_config_fingerprint": "same-config",
		"guardrails":                 completeProofGuardrails(),
		"sources":                    sources,
	})
}

func writeProofJSON(t *testing.T, path string, payload map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func writeProofSummary(t *testing.T, path string, summary evalreadback.Summary) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}
}

func completeProofGuardrails() map[string]any {
	return map[string]any{
		"network_fetches":             0,
		"hosted_telemetry_exports":    0,
		"hosted_inference_calls":      0,
		"browser_calls":               0,
		"slack_api_calls":             0,
		"destination_writes":          0,
		"product_brain_writes":        0,
		"tolaria_writes":              0,
		"auto_accepts":                0,
		"no_human_claims":             0,
		"committed_private_artifacts": 0,
	}
}

func assertProofOutputSafe(t *testing.T, root string) {
	t.Helper()
	denied := []string{"/private/tmp/", "/Users/", "Young Human Club Dropbox", "slack.com/archives/", "sk-proj-", "OPENAI_API_KEY"}
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, pattern := range denied {
			if strings.Contains(string(data), pattern) {
				t.Fatalf("%s leaked denied pattern %q", path, pattern)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("walk proof output: %v", err)
	}
}
