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
