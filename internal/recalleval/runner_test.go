package recalleval

import (
	"context"
	"fmt"
	"testing"
)

func TestRunHydratesAndVerifiesCompactCitationsEndToEnd(t *testing.T) {
	manifest, port := syntheticOwnerManifest(t)
	baselineBinding := manifest.Baseline
	candidateBinding := baselineBinding
	candidateBinding.BuildFingerprint = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	baseline, err := Run(context.Background(), manifest, baselineBinding, port, port)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := Run(context.Background(), manifest, candidateBinding, port, port)
	if err != nil {
		t.Fatal(err)
	}
	result, err := CompareRuns(manifest, baseline, candidate)
	if err != nil || !result.Passed {
		t.Fatalf("CompareRuns = %+v, %v", result, err)
	}
	if port.getCalls != 24 {
		t.Fatalf("GetCanonicalEvidence calls = %d, want 24", port.getCalls)
	}
}

func TestRunScoresNonCurrentCanonicalEvidenceAsInvalidCitation(t *testing.T) {
	manifest, port := syntheticOwnerManifest(t)
	port.evidence["record-01"] = CanonicalEvidence{RecordID: "record-01", SourceCommitment: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", AuthorityClass: "personal_evidence", Current: false, ContentHash: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", LibraryFingerprint: manifest.LibraryFingerprint}
	result, err := Run(context.Background(), manifest, manifest.Baseline, port, port)
	if err != nil {
		t.Fatal(err)
	}
	citation := result.Evaluation.Cases[0].Citations[0]
	if citation.Valid || citation.RecordFingerprint != invalidCitationCommitment("record-01") {
		t.Fatalf("non-current citation was not scored invalid: %+v", citation)
	}
	structural, err := structuralManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := Score(structural, result.Evaluation)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.CitationCompleteness >= 1 {
		t.Fatalf("invalid citation did not lower completeness: %+v", metrics)
	}
}

func TestRunRejectsLibraryChangeDuringSearchOrHydration(t *testing.T) {
	manifest, port := syntheticOwnerManifest(t)
	firstQuery := manifest.Cases[0].Query
	packet := port.results[firstQuery]
	packet.LibraryFingerprint = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	port.results[firstQuery] = packet
	if _, err := Run(context.Background(), manifest, manifest.Baseline, port, port); err == nil {
		t.Fatal("evaluation accepted search from a changed library")
	}
	manifest, port = syntheticOwnerManifest(t)
	evidence := port.evidence["record-01"]
	evidence.LibraryFingerprint = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	port.evidence["record-01"] = evidence
	if _, err := Run(context.Background(), manifest, manifest.Baseline, port, port); err == nil {
		t.Fatal("evaluation accepted hydration from a changed library")
	}
}

func TestRunPropagatesFrozenLibraryBindingFailureOnFinalHydration(t *testing.T) {
	manifest, port := syntheticOwnerManifest(t)
	port.getErrors["record-12"] = fmt.Errorf("%w: changed during final hydration", ErrFrozenLibraryBinding)
	if result, err := Run(context.Background(), manifest, manifest.Baseline, port, port); err == nil || len(result.Evaluation.Cases) != 0 {
		t.Fatalf("frozen-library failure produced a run result: %+v err=%v", result, err)
	}
}

func TestCompareRunsRejectsEvaluationBuildOutsideBinding(t *testing.T) {
	manifest, port := syntheticOwnerManifest(t)
	baseline, err := Run(context.Background(), manifest, manifest.Baseline, port, port)
	if err != nil {
		t.Fatal(err)
	}
	candidateBinding := manifest.Baseline
	candidateBinding.BuildFingerprint = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	candidate, err := Run(context.Background(), manifest, candidateBinding, port, port)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Evaluation.BuildFingerprint = manifest.Baseline.BuildFingerprint
	if _, err := CompareRuns(manifest, baseline, candidate); err == nil {
		t.Fatal("comparison accepted an evaluation build outside its run binding")
	}
}

func TestCanonicalEvidenceCommitmentPreservesV01RecordIdentity(t *testing.T) {
	evidence := CanonicalEvidence{
		RecordID: "record-golden", SourceCommitment: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		AuthorityClass: "personal_evidence", Current: true,
		ContentHash: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Missingness: []string{"zeta", "alpha"}, ResourceStates: []string{"resource-z", "resource-a"},
		LibraryFingerprint: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
	}
	commitment, err := CanonicalEvidenceCommitment(evidence)
	if err != nil {
		t.Fatal(err)
	}
	const goldenV01 = "sha256:4ecdb937182a25b03e9d0d5035e94148fbf675e04ed2651026647c56cc56c267"
	if commitment != goldenV01 {
		t.Fatalf("v0.1 canonical evidence commitment changed: got %s want %s", commitment, goldenV01)
	}
	evidence.LibraryFingerprint = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	second, err := CanonicalEvidenceCommitment(evidence)
	if err != nil || second != goldenV01 {
		t.Fatalf("library binding leaked into v0.1 record identity: got %s err=%v", second, err)
	}
}

type fakeEvidencePort struct {
	results   map[string]CompactSearchResult
	evidence  map[string]CanonicalEvidence
	getErrors map[string]error
	getCalls  int
}

func (port *fakeEvidencePort) SearchCompact(_ context.Context, query string) (CompactSearchResult, error) {
	return port.results[query], nil
}
func (port *fakeEvidencePort) GetCanonicalEvidence(_ context.Context, id string) (CanonicalEvidence, error) {
	port.getCalls++
	if err := port.getErrors[id]; err != nil {
		return CanonicalEvidence{}, err
	}
	evidence, ok := port.evidence[id]
	if !ok {
		return CanonicalEvidence{}, fmt.Errorf("missing %s", id)
	}
	return evidence, nil
}

func syntheticOwnerManifest(t *testing.T) (OwnerManifest, *fakeEvidencePort) {
	t.Helper()
	port := &fakeEvidencePort{results: map[string]CompactSearchResult{}, evidence: map[string]CanonicalEvidence{}, getErrors: map[string]error{}}
	manifest := OwnerManifest{SchemaVersion: ManifestSchemaVersion, LibraryFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Baseline: RunBinding{BuildFingerprint: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", TreeFingerprint: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", ConfigurationFingerprint: "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"}, ReviewerFingerprint: "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", LabelsFrozenBeforeRun: true}
	for index := 1; index <= 20; index++ {
		caseID, query := fmt.Sprintf("case-%02d", index), fmt.Sprintf("synthetic query %02d", index)
		kind := CaseAnswerable
		if index > 12 {
			kind = CaseNoAnswer
		}
		item := OwnerCase{CaseID: caseID, Kind: kind, Query: query}
		if kind == CaseAnswerable {
			id := fmt.Sprintf("record-%02d", index)
			evidence := CanonicalEvidence{RecordID: id, SourceCommitment: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", AuthorityClass: "personal_evidence", Current: true, ContentHash: fmt.Sprintf("sha256:%064x", index), LibraryFingerprint: manifest.LibraryFingerprint}
			commitment, err := CanonicalEvidenceCommitment(evidence)
			if err != nil {
				t.Fatal(err)
			}
			item.ExpectedCanonicalCommitments = []string{commitment}
			port.evidence[id] = evidence
			port.results[query] = CompactSearchResult{Citations: []CompactCitation{{RecordID: id}}, LibraryFingerprint: manifest.LibraryFingerprint}
		} else {
			port.results[query] = CompactSearchResult{LibraryFingerprint: manifest.LibraryFingerprint}
		}
		manifest.Cases = append(manifest.Cases, item)
	}
	return manifest, port
}
