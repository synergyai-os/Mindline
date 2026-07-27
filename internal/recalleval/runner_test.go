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
	port.evidence["record-01"] = CanonicalEvidence{RecordID: "record-01", SourceCommitment: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", AuthorityClass: "personal_evidence", Current: false, ContentHash: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
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

type fakeEvidencePort struct {
	results  map[string]CompactSearchResult
	evidence map[string]CanonicalEvidence
	getCalls int
}

func (port *fakeEvidencePort) SearchCompact(_ context.Context, query string) (CompactSearchResult, error) {
	return port.results[query], nil
}
func (port *fakeEvidencePort) GetCanonicalEvidence(_ context.Context, id string) (CanonicalEvidence, error) {
	port.getCalls++
	evidence, ok := port.evidence[id]
	if !ok {
		return CanonicalEvidence{}, fmt.Errorf("missing %s", id)
	}
	return evidence, nil
}

func syntheticOwnerManifest(t *testing.T) (OwnerManifest, *fakeEvidencePort) {
	t.Helper()
	port := &fakeEvidencePort{results: map[string]CompactSearchResult{}, evidence: map[string]CanonicalEvidence{}}
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
			evidence := CanonicalEvidence{RecordID: id, SourceCommitment: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", AuthorityClass: "personal_evidence", Current: true, ContentHash: fmt.Sprintf("sha256:%064x", index)}
			commitment, err := CanonicalEvidenceCommitment(evidence)
			if err != nil {
				t.Fatal(err)
			}
			item.ExpectedCanonicalCommitments = []string{commitment}
			port.evidence[id] = evidence
			port.results[query] = CompactSearchResult{Citations: []CompactCitation{{RecordID: id}}}
		}
		manifest.Cases = append(manifest.Cases, item)
	}
	return manifest, port
}
