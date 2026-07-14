package productbrain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/synergyai-os/Mindline/internal/routing"
)

func deliveryReviewPacket(outbox Outbox, history DeliveryHistory, summary DeliverySummary) string {
	var b strings.Builder
	b.WriteString("# Mindline First-Slice Review Packet\n\n")
	b.WriteString("## Bound lineage\n\n")
	b.WriteString(fmt.Sprintf("- Routing decisions fingerprint: `%s`\n- Outbox fingerprint: `%s`\n- Delivery profile fingerprint: `%s`\n- Expected destination: `%s` (`%s`)\n- Expected non-secret credential key ID: `%s`\n- Required entry actor: `%s`; relation initiator: `agent_operator`; judgment method: `operator_agent_review`\n\n", outbox.RoutingFingerprint, outbox.Fingerprint, outbox.ProfileFingerprint, outbox.ProfileSnapshot.ExpectedWorkspaceSlug, outbox.ProfileSnapshot.ExpectedWorkspaceID, outbox.ProfileSnapshot.ExpectedKeyID, ExpectedCreatedBy))

	latest := latestOperationResults(history)
	operations := map[string]OutboxOperation{}
	for _, operation := range outbox.Operations {
		operations[operation.OperationID] = operation
	}

	b.WriteString("## Original capture ledger\n\n")
	b.WriteString("| Capture | Public source / evidence | Enrichment / missingness | Meaning | Context lenses | Disposition | Nodes / edges | Destination / readback | Replay / manual action |\n|---|---|---|---|---|---|---|---|---|\n")
	for _, capture := range outbox.ReviewContext.Captures {
		duplicate := ""
		if capture.DuplicateOf != "" {
			duplicate = " (duplicate of " + capture.DuplicateOf + ")"
		}
		meaning := capture.SemanticAssessment.PrimaryRole
		if capture.SemanticAssessment.Summary != "" {
			meaning += ": " + capture.SemanticAssessment.Summary
		}
		lenses := make([]string, 0, len(capture.LensResults))
		for _, lens := range capture.LensResults {
			lenses = append(lenses, fmt.Sprintf("%s=%s (%.2f): %s", lens.LensID, lens.Result, lens.Confidence, inline(lens.Rationale)))
		}
		evidence := make([]string, 0, len(capture.PublicExcerpts))
		for _, excerpt := range capture.PublicExcerpts {
			evidence = append(evidence, excerpt.ExcerptID+"@"+inline(excerpt.Locator))
		}
		structure := make([]string, 0, len(capture.SemanticNodes)+len(capture.SemanticEdges))
		for _, node := range capture.SemanticNodes {
			structure = append(structure, node.SemanticNodeID+":"+node.Role)
		}
		for _, edge := range capture.SemanticEdges {
			structure = append(structure, edge.From+"-"+edge.Type+"->"+edge.To)
		}
		destination := make([]string, 0, len(capture.DestinationOperationIDs))
		for _, operationID := range capture.DestinationOperationIDs {
			result := latest[operationID]
			operation := operations[operationID]
			if operation.Entry != nil {
				destination = append(destination, fmt.Sprintf("entry:%s/%s -> %s (%s,draft=%t,actor=%t)", operation.Entry.CollectionSlug, operation.Entry.EntryID, result.RemoteObjectID, result.State, result.DraftVerified, result.ActorVerified))
			} else if operation.Relation != nil {
				destination = append(destination, fmt.Sprintf("relation:%s/%s -> %s (%s,attribution=%t)", operation.Relation.Type, operation.Relation.RelationIdentity, result.RemoteObjectID, result.State, result.AttributionVerified))
			}
		}
		b.WriteString(fmt.Sprintf("| %s%s | %s; evidence: %s | %s; missing: %s | %s | %s | %s | %s | %s | zero-new=%t; action=review |\n", capture.CaptureRef, duplicate, capture.CanonicalURL, escapeCell(joinOrNone(evidence)), capture.EnrichmentState, escapeCell(joinOrNone(capture.Missingness)), escapeCell(meaning), escapeCell(strings.Join(lenses, "; ")), escapeCell(capture.Disposition+": "+capture.DispositionRationale), escapeCell(joinOrNone(structure)), escapeCell(joinOrNone(destination)), summary.ReplayZeroMutation))
	}

	b.WriteString("\n## Evidence and routing details\n\n")
	for _, capture := range outbox.ReviewContext.Captures {
		b.WriteString("### " + capture.CaptureRef + "\n\n")
		b.WriteString(fmt.Sprintf("- Public source: %s (`%s`)\n- Enrichment: `%s`; missingness: %s\n", capture.CanonicalURL, capture.CanonicalURLID, capture.EnrichmentState, joinOrNone(capture.Missingness)))
		writePublicEvidence(&b, capture.PublicMetadata, capture.PublicExcerpts)
		assessment := capture.SemanticAssessment
		b.WriteString(fmt.Sprintf("- Stable meaning: `%s` at %.2f — %s; evidence: %s; missingness: %s\n", assessment.PrimaryRole, assessment.Confidence, inline(assessment.Summary), joinOrNone(assessment.EvidenceRefs), joinOrNone(assessment.Missingness)))
		b.WriteString("- Lens judgments:\n")
		for _, lens := range capture.LensResults {
			b.WriteString(fmt.Sprintf("  - `%s`: `%s` at %.2f — %s; evidence: %s; missingness: %s\n", lens.LensID, lens.Result, lens.Confidence, inline(lens.Rationale), joinOrNone(lens.EvidenceRefs), joinOrNone(lens.Missingness)))
		}
		b.WriteString(fmt.Sprintf("- Disposition: `%s` — %s\n", capture.Disposition, inline(capture.DispositionRationale)))
		b.WriteString("- Semantic nodes:\n")
		if len(capture.SemanticNodes) == 0 {
			b.WriteString("  - none\n")
		}
		for _, node := range capture.SemanticNodes {
			b.WriteString(fmt.Sprintf("  - `%s` `%s` at %.2f: %s — %s; lenses: %s; evidence: %s\n", node.SemanticNodeID, node.Role, node.Confidence, inline(node.Name), inline(node.Description), joinOrNone(node.LensRefs), joinOrNone(node.EvidenceRefs)))
		}
		b.WriteString("- Semantic edges:\n")
		if len(capture.SemanticEdges) == 0 {
			b.WriteString("  - none\n")
		}
		for _, edge := range capture.SemanticEdges {
			b.WriteString(fmt.Sprintf("  - `%s` --`%s`--> `%s`: %s; evidence: %s\n", edge.From, edge.Type, edge.To, inline(edge.Rationale), joinOrNone(edge.EvidenceRefs)))
		}
		b.WriteString("- Product Brain operations:\n")
		if len(capture.DestinationOperationIDs) == 0 {
			b.WriteString("  - none\n")
		}
		for _, operationID := range capture.DestinationOperationIDs {
			writeDestinationOperation(&b, operations[operationID], latest[operationID])
		}
		b.WriteString("\n")
	}

	b.WriteString("## Admitted depth-1 sources\n\n")
	for _, source := range outbox.ReviewContext.DepthOneSources {
		b.WriteString(fmt.Sprintf("### %s\n\n- Parent: %s\n- Enrichment: `%s`; missingness: %s\n", source.CanonicalURL, source.ParentCanonicalURL, source.EnrichmentState, joinOrNone(source.Missingness)))
		writePublicEvidence(&b, source.PublicMetadata, source.PublicExcerpts)
		b.WriteString(fmt.Sprintf("- Meaning: `%s` at %.2f — %s; evidence: %s\n- Disposition: `%s`\n- Lenses:\n", source.SemanticAssessment.PrimaryRole, source.SemanticAssessment.Confidence, inline(source.SemanticAssessment.Summary), joinOrNone(source.SemanticAssessment.EvidenceRefs), source.Disposition))
		for _, lens := range source.LensResults {
			b.WriteString(fmt.Sprintf("  - `%s`: `%s` at %.2f — %s; evidence: %s; missingness: %s\n", lens.LensID, lens.Result, lens.Confidence, inline(lens.Rationale), joinOrNone(lens.EvidenceRefs), joinOrNone(lens.Missingness)))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Delivery history and replay\n\n")
	for _, run := range history.Runs {
		acknowledged := 0
		for _, operation := range run.Operations {
			if operation.Acknowledged {
				acknowledged++
			}
		}
		b.WriteString(fmt.Sprintf("- Run %d `%s`: `%s`; sealed ref `%s`; preflight `%s` at `%s`; preflight mutations %d; external preconditions repeated %t; acknowledgements %d/%d; entry mutations %d; relation mutations %d.\n", run.Sequence, run.InvocationID, run.Outcome, history.RunRefs[run.Sequence-1], run.PreflightFingerprint, run.PreflightSnapshotRef, run.PreflightMutationCalls, run.ExternalPreconditionsRepeated, acknowledged, len(run.Operations), run.EntriesCreated, run.RelationsCreated))
	}
	b.WriteString(fmt.Sprintf("\n## Totals and limits\n\n- Captures represented: %d\n- Entries acknowledged: %d\n- Relations acknowledged: %d\n- Product Brain mutations across retained successful lineage: %d\n- Latest replay entry mutations: %d\n- Latest replay relation mutations: %d\n- Replay zero mutation: %t\n- Draft-only readback: %t\n- Entry actor verified: %t\n- Relation attribution verified: %t\n- Public privacy findings: %d\n- Claim: private, curated, operator/agent-judged, sample-bound, non-generalizable; no autonomy claim.\n", len(outbox.ReviewContext.Captures), summary.EntriesAcknowledged, summary.RelationsAcknowledged, summary.ProductBrainWrites, summary.LatestRunEntryMutations, summary.LatestRunRelationMutations, summary.ReplayZeroMutation, summary.DraftOnly, summary.EntryActorVerified, summary.RelationAttributionVerified, summary.PrivacyFindingCount))
	b.WriteString("\n## Randy review checklist\n\n")
	for _, action := range outbox.ReviewContext.PendingActions {
		b.WriteString("- [ ] " + action + "\n")
	}
	return b.String()
}

func latestOperationResults(history DeliveryHistory) map[string]DeliveryOperationResult {
	latest := map[string]DeliveryOperationResult{}
	for _, run := range history.Runs {
		for _, operation := range run.Operations {
			prior, exists := latest[operation.OperationID]
			if !exists || operation.Acknowledged || !prior.Acknowledged {
				latest[operation.OperationID] = operation
			}
		}
	}
	return latest
}

func writePublicEvidence(b *strings.Builder, metadata *routing.PublicMetadata, excerpts []routing.PublicExcerpt) {
	if metadata == nil {
		b.WriteString("- Public metadata: none retained in this lineage\n")
	} else {
		b.WriteString(fmt.Sprintf("- Public metadata: title=%s; author=%s; published=%s\n", valueOrNone(metadata.Title), valueOrNone(metadata.Author), valueOrNone(metadata.PublishedAt)))
	}
	b.WriteString("- Public evidence:\n")
	if len(excerpts) == 0 {
		b.WriteString("  - none\n")
	}
	for _, excerpt := range excerpts {
		b.WriteString(fmt.Sprintf("  - `%s` at `%s`: %s\n", excerpt.ExcerptID, inline(excerpt.Locator), inline(excerpt.Text)))
	}
}

func writeDestinationOperation(b *strings.Builder, operation OutboxOperation, result DeliveryOperationResult) {
	if operation.Entry != nil {
		entry := operation.Entry
		b.WriteString(fmt.Sprintf("  - entry `%s`: `%s` / `%s` / `%s`; expected actor `%s`; force draft %t; state `%s`; actual entry `%s`; doc `%s`; draft verified %t; actor verified %t\n", operation.OperationID, entry.CollectionSlug, entry.EntryID, inline(entry.Name), entry.CreatedBy, entry.ForceDraft, result.State, result.RemoteObjectID, result.EntryDocID, result.DraftVerified, result.ActorVerified))
		return
	}
	if operation.Relation != nil {
		relation := operation.Relation
		metadata, _ := json.Marshal(relation.Metadata)
		b.WriteString(fmt.Sprintf("  - relation `%s`: identity `%s`; `%s` --`%s`--> `%s`; if missing %t; metadata `%s`; state `%s`; actual relation `%s`; attribution verified %t\n", operation.OperationID, relation.RelationIdentity, relation.FromEntryID, relation.Type, relation.ToEntryID, relation.IfMissing, inline(string(metadata)), result.State, result.RemoteObjectID, result.AttributionVerified))
	}
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	out := append([]string{}, values...)
	sort.Strings(out)
	return strings.Join(out, ", ")
}

func valueOrNone(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return inline(value)
}

func inline(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\r", " "), "\n", " "))
}

func escapeCell(value string) string {
	return strings.ReplaceAll(inline(value), "|", "\\|")
}
