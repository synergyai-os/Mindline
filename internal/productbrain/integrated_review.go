package productbrain

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/synergyai-os/Mindline/internal/privateio"
	"github.com/synergyai-os/Mindline/internal/routing"
)

// WriteIntegratedReview binds the destination-neutral routing evidence to an
// already sealed delivery lineage. A legacy immutable v0.1 outbox may omit
// public evidence fields, but every retained field and the complete ordered
// source/lens/destination matrix must match routing authority before projection.
func WriteIntegratedReview(outDir, deliveryDir string, route routing.Result, outbox Outbox, profile DeliveryProfile) (DeliverySummary, error) {
	if route.Decisions.Fingerprint == "" || route.Decisions.Fingerprint != outbox.RoutingFingerprint {
		return DeliverySummary{}, errors.New("routing_outbox_binding_mismatch")
	}
	if err := ValidateOutbox(outbox); err != nil {
		return DeliverySummary{}, err
	}
	history, err := loadStrictDeliveryHistoryReadOnly(deliveryDir, outbox, profile)
	if err != nil {
		return DeliverySummary{}, err
	}
	augmented, err := augmentReviewEvidence(outbox, route)
	if err != nil {
		return DeliverySummary{}, err
	}
	summary := summarizeDelivery(outbox, history, latestOperationSlice(history))
	summary.ProfileFingerprint = hashValue(profile)
	summary.Fingerprint = hashValue(summary)
	if err := privateio.PrepareDir(outDir); err != nil {
		return DeliverySummary{}, err
	}
	if err := privateio.WriteFile(filepath.Join(outDir, "mindline-review-packet.md"), []byte(deliveryReviewPacket(augmented, history, summary)), false); err != nil {
		return DeliverySummary{}, err
	}
	return summary, nil
}

func loadStrictDeliveryHistoryReadOnly(deliveryDir string, outbox Outbox, profile DeliveryProfile) (DeliveryHistory, error) {
	profileFingerprint := hashValue(profile)
	projected, exists, err := readProjectedHistory(deliveryDir, filepath.Join(deliveryDir, "delivery-history.json"), outbox.Fingerprint, profileFingerprint)
	if err != nil || !exists {
		if err == nil {
			err = errors.New("delivery history projection missing")
		}
		return DeliveryHistory{}, err
	}
	reconstructed, err := reconstructDeliveryHistory(deliveryDir, outbox.Fingerprint, profileFingerprint)
	if err != nil {
		return DeliveryHistory{}, err
	}
	if !canonicalEqual(projected, reconstructed) {
		return DeliveryHistory{}, errors.New("delivery history authority mismatch")
	}
	if err := validatePreflightSnapshotsReadOnly(deliveryDir, projected, outbox, profile); err != nil {
		return DeliveryHistory{}, err
	}
	return projected, nil
}

func validatePreflightSnapshotsReadOnly(deliveryDir string, history DeliveryHistory, outbox Outbox, profile DeliveryProfile) error {
	expected := map[string]bool{}
	for _, run := range history.Runs {
		safePreconditionFailure := run.Outcome == "failed" && !run.ExternalPreconditionsRepeated && run.EntriesCreated == 0 && run.RelationsCreated == 0 && !deliveryRunObservedMutation(run)
		if run.Outcome == "failed" && (run.EntriesCreated != 0 || run.RelationsCreated != 0 || deliveryRunObservedMutation(run)) || run.PreflightFingerprint == "" || run.PreflightSnapshotRef != filepath.ToSlash(filepath.Join("preflight-snapshots", run.PreflightFingerprint+".json")) || run.PreflightMutationCalls != 0 || !run.ExternalPreconditionsRepeated && !safePreconditionFailure {
			return errors.New("invalid delivery preflight lineage")
		}
		expected[filepath.Base(run.PreflightSnapshotRef)] = true
		path := filepath.Join(deliveryDir, filepath.FromSlash(run.PreflightSnapshotRef))
		if err := privateio.ValidateContained(deliveryDir, path); err != nil {
			return err
		}
		artifact, err := LoadPreflight(path)
		if err != nil {
			return err
		}
		if artifact.Fingerprint != run.PreflightFingerprint {
			return errors.New("preflight snapshot fingerprint mismatch")
		}
		if err := ValidatePreflight(artifact, outbox, profile); err != nil {
			return err
		}
	}
	dir := filepath.Join(deliveryDir, "preflight-snapshots")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != len(expected) {
		return errors.New("preflight snapshot authority set mismatch")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !expected[entry.Name()] {
			return fmt.Errorf("unexpected preflight snapshot: %s", entry.Name())
		}
	}
	return nil
}

func augmentReviewEvidence(outbox Outbox, route routing.Result) (Outbox, error) {
	operationsByCanonical, err := reviewOperationsByCanonical(route, outbox)
	if err != nil {
		return Outbox{}, err
	}
	profile := DeliveryProfileFromSnapshot(outbox.ProfileSnapshot)
	expected := buildReviewContext(route, operationsByCanonical, profile, outbox.Operations)
	if err := validateProductBrainPendingActions(outbox); err != nil {
		return Outbox{}, err
	}
	// Pending actions are already checked centrally against either the current
	// profile/count derivation or the one exact immutable v0.1 legacy set.
	expected.PendingActions = append([]string{}, outbox.ReviewContext.PendingActions...)
	if len(outbox.ReviewContext.Captures) != len(expected.Captures) || len(outbox.ReviewContext.DepthOneSources) != len(expected.DepthOneSources) || !canonicalEqual(outbox.ReviewContext.PendingActions, expected.PendingActions) {
		return Outbox{}, errors.New("routing_review_context_mismatch")
	}
	for index := range expected.Captures {
		actual := outbox.ReviewContext.Captures[index]
		want := expected.Captures[index]
		if actual.PublicMetadata != nil && !canonicalEqual(actual.PublicMetadata, want.PublicMetadata) || len(actual.PublicExcerpts) > 0 && !canonicalEqual(actual.PublicExcerpts, want.PublicExcerpts) || len(actual.Missingness) > 0 && !canonicalEqual(actual.Missingness, want.Missingness) {
			return Outbox{}, errors.New("routing_review_context_mismatch")
		}
		actual.PublicMetadata, want.PublicMetadata = nil, nil
		actual.PublicExcerpts, want.PublicExcerpts = nil, nil
		actual.Missingness, want.Missingness = nil, nil
		if !canonicalEqual(actual, want) {
			return Outbox{}, errors.New("routing_review_context_mismatch")
		}
		outbox.ReviewContext.Captures[index] = expected.Captures[index]
	}
	for index := range expected.DepthOneSources {
		actual := outbox.ReviewContext.DepthOneSources[index]
		want := expected.DepthOneSources[index]
		if actual.EnrichmentState != "" && actual.EnrichmentState != want.EnrichmentState || actual.PublicMetadata != nil && !canonicalEqual(actual.PublicMetadata, want.PublicMetadata) || len(actual.PublicExcerpts) > 0 && !canonicalEqual(actual.PublicExcerpts, want.PublicExcerpts) || len(actual.Missingness) > 0 && !canonicalEqual(actual.Missingness, want.Missingness) {
			return Outbox{}, errors.New("routing_review_context_mismatch")
		}
		actual.EnrichmentState, want.EnrichmentState = "", ""
		actual.PublicMetadata, want.PublicMetadata = nil, nil
		actual.PublicExcerpts, want.PublicExcerpts = nil, nil
		actual.Missingness, want.Missingness = nil, nil
		if !canonicalEqual(actual, want) {
			return Outbox{}, errors.New("routing_review_context_mismatch")
		}
		outbox.ReviewContext.DepthOneSources[index] = expected.DepthOneSources[index]
	}
	return outbox, nil
}

func reviewOperationsByCanonical(route routing.Result, outbox Outbox) (map[string][]string, error) {
	canonicalByURL := map[string]string{}
	for _, source := range route.Decisions.Sources {
		canonicalByURL[source.CanonicalURL] = source.CanonicalURLID
	}
	entryCanonical := map[string]string{}
	result := map[string][]string{}
	for _, operation := range outbox.Operations {
		if operation.Entry == nil {
			continue
		}
		canonicalID := canonicalByURL[operation.Entry.SourceRef]
		if canonicalID == "" {
			return nil, errors.New("routing_review_context_mismatch")
		}
		entryCanonical[operation.Entry.EntryID] = canonicalID
		result[canonicalID] = append(result[canonicalID], operation.OperationID)
	}
	for _, operation := range outbox.Operations {
		if operation.Relation == nil {
			continue
		}
		from := entryCanonical[operation.Relation.FromEntryID]
		to := entryCanonical[operation.Relation.ToEntryID]
		if from == "" || from != to {
			return nil, errors.New("routing_review_context_mismatch")
		}
		result[from] = append(result[from], operation.OperationID)
	}
	return result, nil
}

func latestOperationSlice(history DeliveryHistory) []DeliveryOperationResult {
	latest := latestOperationResults(history)
	values := make([]DeliveryOperationResult, 0, len(latest))
	for _, operation := range latest {
		values = append(values, operation)
	}
	return sortedOperations(values)
}
