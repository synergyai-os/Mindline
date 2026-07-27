package recalleval

import (
	"context"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/localservice"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

type fakeAgentClient struct {
	compact personalmemory.CompactContextPacket
	legacy  personalmemory.ContextPacket
	get     personalmemory.HydratedCapture
}

func (client fakeAgentClient) Search(context.Context, localservice.SearchInput) (personalmemory.ContextPacket, error) {
	return client.legacy, nil
}
func (client fakeAgentClient) SearchCompact(context.Context, localservice.SearchInput) (personalmemory.CompactContextPacket, error) {
	return client.compact, nil
}
func (client fakeAgentClient) Get(context.Context, string) (personalmemory.HydratedCapture, error) {
	return client.get, nil
}

func TestLocalServicePortUsesCompactResponseAndExplicitCanonicalGet(t *testing.T) {
	record := personalmemory.CaptureRecord{
		RecordID: "record-1", SourceRef: "slack://fixture/1",
		AuthorityClass: personalmemory.AuthorityClass,
		ContentHash:    strings.Repeat("a", 64), Missingness: []string{"capture_gap"},
	}
	client := fakeAgentClient{
		compact: personalmemory.CompactContextPacket{
			SchemaVersion: personalmemory.CompactPacketSchemaVersion,
			Citations:     []personalmemory.CompactCitation{{RecordID: record.RecordID}},
		},
		get: personalmemory.HydratedCapture{
			VersionState: "current", Record: record,
			Resources: []personalmemory.ResourceContext{{
				ResourceID: "resource-1", State: "partial", AccessClass: "public",
				ContentHash: strings.Repeat("b", 64), Missingness: []string{"resource_gap"},
			}},
		},
	}
	port := LocalServicePort{Client: client, Mode: ServiceModeCompact}
	search, err := port.SearchCompact(context.Background(), "saved lesson")
	if err != nil || len(search.Citations) != 1 || search.UnselectedHydratedContent {
		t.Fatalf("compact search = %+v err=%v", search, err)
	}
	evidence, err := port.GetCanonicalEvidence(context.Background(), record.RecordID)
	if err != nil || !evidence.Current || evidence.RecordID != record.RecordID ||
		evidence.ContentHash != "sha256:"+record.ContentHash ||
		len(evidence.ResourceStates) != 1 {
		t.Fatalf("canonical evidence = %+v err=%v", evidence, err)
	}
	if _, err := CanonicalEvidenceCommitment(evidence); err != nil {
		t.Fatal(err)
	}
}

func TestCompareRunsAllowsDifferentBaselineAndCandidateTrees(t *testing.T) {
	manifest, port := syntheticOwnerManifest(t)
	baseline, err := Run(context.Background(), manifest, manifest.Baseline, port, port)
	if err != nil {
		t.Fatal(err)
	}
	candidateBinding := manifest.Baseline
	candidateBinding.BuildFingerprint = "sha256:" + strings.Repeat("f", 64)
	candidateBinding.TreeFingerprint = "sha256:" + strings.Repeat("1", 64)
	candidate, err := Run(context.Background(), manifest, candidateBinding, port, port)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := CompareRuns(manifest, baseline, candidate); err != nil || !result.Passed {
		t.Fatalf("different successor tree was not comparable: %+v err=%v", result, err)
	}
}
