package recalleval

import (
	"context"
	"testing"
)

func TestSealOwnerManifestConvertsPrivateRecordIDsToCommitments(t *testing.T) {
	manifest, port := syntheticOwnerManifest(t)
	draft := DraftManifest{
		SchemaVersion:      DraftManifestSchemaVersion,
		LibraryFingerprint: manifest.LibraryFingerprint, Baseline: manifest.Baseline,
		ReviewerFingerprint: manifest.ReviewerFingerprint,
	}
	for _, item := range manifest.Cases {
		draftCase := DraftCase{CaseID: item.CaseID, Kind: item.Kind, Query: item.Query}
		if item.Kind == CaseAnswerable {
			draftCase.ExpectedRecordIDs = []string{"record-" + item.CaseID[len("case-"):]}
		}
		draft.Cases = append(draft.Cases, draftCase)
	}
	sealed, err := SealOwnerManifest(context.Background(), draft, port)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.Fingerprint == "" || len(sealed.Cases) != 20 {
		t.Fatalf("sealed manifest = %+v", sealed)
	}
	sealed.Cases[0].Query = "tampered"
	if err := sealed.Validate(); err == nil {
		t.Fatal("tampered sealed owner manifest was accepted")
	}
}

func TestSealOwnerManifestRejectsChangedLibrary(t *testing.T) {
	manifest, port := syntheticOwnerManifest(t)
	draft := DraftManifest{SchemaVersion: DraftManifestSchemaVersion, LibraryFingerprint: manifest.LibraryFingerprint, Baseline: manifest.Baseline, ReviewerFingerprint: manifest.ReviewerFingerprint}
	for _, item := range manifest.Cases {
		entry := DraftCase{CaseID: item.CaseID, Kind: item.Kind, Query: item.Query}
		if item.Kind == CaseAnswerable {
			entry.ExpectedRecordIDs = []string{"record-" + item.CaseID[len("case-"):]}
		}
		draft.Cases = append(draft.Cases, entry)
	}
	evidence := port.evidence["record-01"]
	evidence.LibraryFingerprint = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	port.evidence["record-01"] = evidence
	if _, err := SealOwnerManifest(context.Background(), draft, port); err == nil {
		t.Fatal("manifest sealing accepted evidence from a changed library")
	}
}

func TestSealOwnerManifestRejectsDuplicateExpectedRecordsBeforeHydration(t *testing.T) {
	manifest, port := syntheticOwnerManifest(t)
	draft := DraftManifest{
		SchemaVersion: DraftManifestSchemaVersion, LibraryFingerprint: manifest.LibraryFingerprint,
		Baseline: manifest.Baseline, ReviewerFingerprint: manifest.ReviewerFingerprint,
	}
	for _, item := range manifest.Cases {
		entry := DraftCase{CaseID: item.CaseID, Kind: item.Kind, Query: item.Query}
		if item.Kind == CaseAnswerable {
			id := "record-" + item.CaseID[len("case-"):]
			entry.ExpectedRecordIDs = []string{id}
		}
		draft.Cases = append(draft.Cases, entry)
	}
	draft.Cases[0].ExpectedRecordIDs = append(draft.Cases[0].ExpectedRecordIDs, draft.Cases[0].ExpectedRecordIDs[0])
	if _, err := SealOwnerManifest(context.Background(), draft, port); err == nil {
		t.Fatal("duplicate expected record was accepted")
	}
	if port.getCalls != 0 {
		t.Fatalf("invalid draft hydrated %d canonical records", port.getCalls)
	}
}

func TestOwnerManifestRejectsDuplicateExpectedCommitments(t *testing.T) {
	manifest, _ := syntheticOwnerManifest(t)
	manifest.Cases[0].ExpectedCanonicalCommitments = append(
		manifest.Cases[0].ExpectedCanonicalCommitments,
		manifest.Cases[0].ExpectedCanonicalCommitments[0],
	)
	if err := manifest.Validate(); err == nil {
		t.Fatal("duplicate expected commitment was accepted")
	}
	if _, err := OwnerManifestFingerprint(manifest); err == nil {
		t.Fatal("duplicate expected commitment received a fingerprint")
	}
}
