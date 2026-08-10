package recalleval

import (
	"context"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/localservice"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

type fakeAgentClient struct {
	compact            personalmemory.CompactContextPacket
	legacy             personalmemory.ContextPacket
	get                personalmemory.HydratedCapture
	statusFingerprints []string
	statusCalls        int
}

func (client *fakeAgentClient) Status(context.Context) (localservice.Status, error) {
	index := client.statusCalls
	if index >= len(client.statusFingerprints) {
		index = len(client.statusFingerprints) - 1
	}
	client.statusCalls++
	if index < 0 {
		return localservice.Status{}, nil
	}
	return localservice.Status{Memory: personalmemory.Status{Fingerprint: client.statusFingerprints[index]}}, nil
}
func (client *fakeAgentClient) Search(context.Context, localservice.SearchInput) (personalmemory.ContextPacket, error) {
	return client.legacy, nil
}
func (client *fakeAgentClient) SearchCompact(context.Context, localservice.SearchInput) (personalmemory.CompactContextPacket, error) {
	return client.compact, nil
}
func (client *fakeAgentClient) Get(context.Context, string) (personalmemory.HydratedCapture, error) {
	return client.get, nil
}

func TestLocalServicePortUsesCompactResponseAndExplicitCanonicalGet(t *testing.T) {
	record := personalmemory.CaptureRecord{
		RecordID: "record-1", SourceRef: "slack://fixture/1",
		AuthorityClass: personalmemory.AuthorityClass,
		ContentHash:    strings.Repeat("a", 64), Missingness: []string{"capture_gap"},
	}
	client := fakeAgentClient{
		statusFingerprints: []string{strings.Repeat("c", 64), strings.Repeat("c", 64)},
		compact: personalmemory.CompactContextPacket{
			SchemaVersion:      personalmemory.CompactPacketSchemaVersion,
			LibraryFingerprint: strings.Repeat("c", 64),
			Citations:          []personalmemory.CompactCitation{{RecordID: record.RecordID}},
		},
		get: personalmemory.HydratedCapture{
			VersionState: "current", Record: record,
			Resources: []personalmemory.ResourceContext{{
				ResourceID: "resource-1", State: "partial", AccessClass: "public",
				ContentHash: strings.Repeat("b", 64), Missingness: []string{"resource_gap"},
			}},
		},
	}
	port := LocalServicePort{Client: &client, Mode: ServiceModeCompact}
	search, err := port.SearchCompact(context.Background(), "saved lesson")
	if err != nil || len(search.Citations) != 1 || search.UnselectedHydratedContent || search.LibraryFingerprint != "sha256:"+strings.Repeat("c", 64) {
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

func TestLocalServicePortSupportsPR47GetAndRejectsHydrationRace(t *testing.T) {
	fingerprint := strings.Repeat("a", 64)
	record := personalmemory.CaptureRecord{RecordID: "record-legacy", SourceRef: "slack://fixture/legacy", AuthorityClass: personalmemory.AuthorityClass, ContentHash: strings.Repeat("b", 64)}
	client := fakeAgentClient{
		legacy:             personalmemory.ContextPacket{SchemaVersion: personalmemory.ContextPacketSchemaVersion, LibraryFingerprint: fingerprint, Citations: []personalmemory.Citation{{RecordID: record.RecordID}}},
		get:                personalmemory.HydratedCapture{SchemaVersion: personalmemory.HydratedCaptureSchemaVersion, RecordID: record.RecordID, VersionState: "current", Record: record},
		statusFingerprints: []string{fingerprint, fingerprint},
	}
	port := LocalServicePort{Client: &client, Mode: ServiceModeLegacy}
	packet, err := port.SearchCompact(context.Background(), "legacy saved knowledge")
	if err != nil || packet.LibraryFingerprint != "sha256:"+fingerprint {
		t.Fatalf("legacy search binding=%+v err=%v", packet, err)
	}
	evidence, err := port.GetCanonicalEvidence(context.Background(), record.RecordID)
	if err != nil || evidence.LibraryFingerprint != "sha256:"+fingerprint {
		t.Fatalf("PR47-shaped hydration=%+v err=%v", evidence, err)
	}
	client.statusCalls = 0
	client.statusFingerprints = []string{fingerprint, strings.Repeat("c", 64)}
	if _, err := port.GetCanonicalEvidence(context.Background(), record.RecordID); err == nil {
		t.Fatal("hydration accepted a library mutation between status checks")
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
