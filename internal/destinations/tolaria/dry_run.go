package tolaria

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/synergyai-os/Mindline/internal/destinations"
)

const AdapterID = "tolaria"

func Plan(result destinations.InputResult) ([]destinations.Operation, error) {
	return PlanBatch([]destinations.InputResult{result})
}

func PlanBatch(results []destinations.InputResult) ([]destinations.Operation, error) {
	operations := make([]destinations.Operation, 0, len(results))
	for _, result := range results {
		operation, err := planOne(result)
		if err != nil {
			return nil, err
		}
		operations = append(operations, operation)
	}
	return destinations.ResolveConflicts(operations), nil
}

func planOne(result destinations.InputResult) (destinations.Operation, error) {
	switch result.State {
	case "dry_run_published":
		if result.Safety.PrivateProvenance || result.Safety.RedactionRequired || result.Safety.SecretLike {
			return baseOperation(result, destinations.OperationBlocked, destinations.VisibilityBlocked, "", "Publish blocked", "", privacyBlockers(result.Safety)), nil
		}
		body, ok := artifactBody(result, "dry_run_publish")
		if !ok {
			return destinations.Operation{}, fmt.Errorf("dry_run_published requires dry_run_publish artifact")
		}
		return baseOperation(result, destinations.OperationCreateNote, destinations.VisibilityPublish, "30-resources/"+safeSlug(result)+".md", "Processed source "+safeDisplayID(result), body, nil), nil
	case "attention_ready":
		if len(privacyBlockers(result.Safety)) > 0 {
			return baseOperation(result, destinations.OperationBlocked, destinations.VisibilityBlocked, "", "Attention preview blocked", "", privacyBlockers(result.Safety)), nil
		}
		body, ok := artifactBody(result, "attention_preview")
		if !ok {
			return destinations.Operation{}, fmt.Errorf("attention_ready requires attention_preview artifact")
		}
		return baseOperation(result, destinations.OperationAttentionPreview, destinations.VisibilityAttention, "00-inbox/"+safeSlug(result)+".md", "Attention needed "+safeDisplayID(result), body, []string{"needs_attention"}), nil
	case "background_ready":
		return baseOperation(result, destinations.OperationBackgroundRecord, destinations.VisibilityBackground, "40-archives/background/"+safeSlug(result)+".json", "Background record "+safeDisplayID(result), "state: background_ready\nsource_candidate_id: "+safeDisplayID(result)+"\n", privacyBlockers(result.Safety)), nil
	case "skipped":
		return baseOperation(result, destinations.OperationSkip, destinations.VisibilitySkip, "", "", "", []string{"skipped"}), nil
	case "needs_enrichment":
		return baseOperation(result, destinations.OperationBlocked, destinations.VisibilityBlocked, "", "Needs enrichment", "", []string{"needs_enrichment"}), nil
	default:
		return baseOperation(result, destinations.OperationBlocked, destinations.VisibilityBlocked, "", "Unsupported state", "", []string{"unsupported_state"}), nil
	}
}

func baseOperation(result destinations.InputResult, operationType destinations.OperationType, lane destinations.VisibilityLane, locator, title, body string, blockers []string) destinations.Operation {
	private := len(privacyBlockers(result.Safety)) > 0
	idempotencyKey := destinations.SafeDiagnosticValue(result.IdempotencyKey)
	sourceCandidateID := destinations.SafeDiagnosticValue(result.SourceCandidateID)
	recordID := destinations.SafeDiagnosticValue(result.RecordID)
	operationID := destinations.GenerateOperationID(AdapterID, result.SourceCandidateID, operationType)
	if private {
		idempotencyKey = "fingerprint:" + destinations.StableFingerprint(result.IdempotencyKey)
		sourceCandidateID = "fingerprint:" + destinations.StableFingerprint(result.SourceCandidateID)
		recordID = "fingerprint:" + destinations.StableFingerprint(result.RecordID)
		operationID = destinations.GeneratePrivateOperationID(AdapterID, result.SourceCandidateID, operationType)
	}
	return destinations.Operation{
		SchemaVersion:        destinations.OperationSchemaVersion,
		OperationID:          operationID,
		DestinationAdapterID: AdapterID,
		SourceCandidateID:    sourceCandidateID,
		SourceRecordID:       recordID,
		IdempotencyKey:       idempotencyKey,
		OperationType:        operationType,
		WriteMode:            destinations.WriteModeDryRun,
		VisibilityLane:       lane,
		PlannedLocator:       locator,
		Title:                title,
		Body:                 body,
		Metadata:             map[string]any{"state": result.State},
		Blockers:             blockers,
		AuthorityIDs:         append([]string(nil), result.AuthorityIDs...),
	}
}

func artifactBody(result destinations.InputResult, kind string) (string, bool) {
	for _, artifact := range result.Artifacts {
		if artifact.Kind == kind && artifact.Body != "" {
			return artifact.Body, true
		}
	}
	return "", false
}

func privacyBlockers(safety destinations.InputSafety) []string {
	var blockers []string
	if safety.PrivateProvenance {
		blockers = append(blockers, "private_provenance")
	}
	if safety.RedactionRequired {
		blockers = append(blockers, "redaction_required")
	}
	if safety.SecretLike {
		blockers = append(blockers, "secret_like")
	}
	return blockers
}

func displayID(value string) string {
	if destinations.IsSensitive(value) {
		return destinations.SafeDiagnosticValue(value)
	}
	return value
}

func safeDisplayID(result destinations.InputResult) string {
	if len(privacyBlockers(result.Safety)) > 0 {
		return "source-" + destinations.StableFingerprint(result.SourceCandidateID)
	}
	return displayID(result.SourceCandidateID)
}

func safeSlug(result destinations.InputResult) string {
	if len(privacyBlockers(result.Safety)) > 0 {
		return "source-" + destinations.StableFingerprint(result.SourceCandidateID)
	}
	return slug(result.SourceCandidateID)
}

func slug(value string) string {
	if destinations.IsSensitive(value) {
		return "source-" + destinations.StableFingerprint(value)
	}
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	cleaned := strings.Trim(b.String(), "-")
	if cleaned == "" {
		return "source"
	}
	return cleaned
}
