package localservice

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

func TestScopedRecallV04RoutesFailClosedAndKeepLegacyCompactUnchanged(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "mindline-scoped-service-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	config, err := ConfigFromRoots(filepath.Join(root, "runtime"), filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	seedSemanticIndexMemory(t, config.MemoryRoot)
	seedUnrelatedExistingRecord(t, config.MemoryRoot)
	server, err := NewServer(config, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- server.Serve() }()
	client := NewClient(config.SocketPath)
	waitForService(t, client)

	capabilities, err := client.Capabilities(context.Background())
	if err != nil || !hasCapability(capabilities.Features, ScopedRecallCapability) ||
		!hasCapability(capabilities.Features, DiscoveryCapability) ||
		capabilities.ScopedSearchEndpoint != "/v1/scoped/search/compact" ||
		capabilities.ScopedHydrationEndpoint != ScopedHydrationEndpoint ||
		capabilities.RecommendedAgentRoute != RecommendedAgentRoute {
		t.Fatalf("scoped capabilities=%+v err=%v", capabilities, err)
	}
	legacy, err := client.SearchCompact(context.Background(), SearchInput{
		Query: "product brain citations", Limit: 3,
	})
	if err != nil || legacy.SchemaVersion != personalmemory.CompactPacketSchemaVersion ||
		legacy.ScopeID != "" || legacy.AgentID != "" || legacy.AgentRecallApproved {
		t.Fatalf("legacy compact changed: packet=%+v err=%v", legacy, err)
	}

	if _, err := client.PutScope(context.Background(), agentstate.Scope{
		ID: "project", Name: "Project", Purpose: "local recall",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutScopedLens(context.Background(), agentstate.ScopedLens{
		ScopeID: "project", ID: "delivery", Name: "Delivery", Query: "citations",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutActor(context.Background(), agentstate.AgentActor{
		ID: "agent-a", Name: "Agent A",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutScope(context.Background(), agentstate.Scope{
		ID: "project-b", Name: "Project B", Purpose: "separate context",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutScopedLens(context.Background(), agentstate.ScopedLens{
		ScopeID: "project-b", ID: "delivery", Name: "Delivery B", Query: "citations",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutScopedLens(context.Background(), agentstate.ScopedLens{
		ScopeID: "project", ID: "delivery-b", Name: "Delivery alternate", Query: "citations",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutActor(context.Background(), agentstate.AgentActor{
		ID: "agent-b", Name: "Agent B",
	}); err != nil {
		t.Fatal(err)
	}
	if lenses, err := client.ListScopedLenses(context.Background(), ""); err != nil || len(lenses) != 3 {
		t.Fatalf("all scoped lenses=%+v err=%v", lenses, err)
	}
	if _, err := client.SearchScoped(context.Background(), ScopedSearchInput{
		Query: "product brain citations", ScopeID: "project", LensID: "delivery",
	}); err == nil {
		t.Fatal("partial scoped tuple was accepted")
	}
	packet, err := client.SearchScoped(context.Background(), ScopedSearchInput{
		Query: "product brain citations", ScopeID: "project", LensID: "delivery",
		AgentID: "agent-a", Limit: 1,
	})
	if err != nil || packet.SchemaVersion != personalmemory.ScopedCompactPacketSchemaVersion ||
		packet.ScopeID != "project" || packet.LensID != "delivery" || packet.AgentID != "agent-a" ||
		!packet.AgentRecallApproved || len(packet.Citations) == 0 {
		t.Fatalf("scoped packet=%+v err=%v", packet, err)
	}
	citation := packet.Citations[0]
	beforeGet, err := client.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	capture, err := client.GetScoped(context.Background(), ScopedGetInput{
		RunID: packet.RunID, ScopeID: "project", LensID: "delivery", AgentID: "agent-a",
		RecordID: citation.RecordID,
	})
	if err != nil || !capture.AgentRecallApproved || capture.RouteClass != "agent_scoped_governed" ||
		capture.RunID != packet.RunID || capture.RecordID != citation.RecordID {
		t.Fatalf("scoped capture=%+v err=%v", capture, err)
	}
	library, err := server.repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	cited := map[string]bool{}
	for _, item := range packet.Citations {
		cited[item.RecordID] = true
	}
	uncitedRecord := ""
	for _, item := range library.Records {
		if !cited[item.RecordID] {
			uncitedRecord = item.RecordID
			break
		}
	}
	if uncitedRecord == "" {
		t.Fatal("fixture has no existing uncited record")
	}
	negativeHydration := []struct {
		name  string
		input ScopedGetInput
	}{
		{name: "wrong run", input: ScopedGetInput{RunID: "wrong-run", ScopeID: "project", LensID: "delivery", AgentID: "agent-a", RecordID: citation.RecordID}},
		{name: "missing record", input: ScopedGetInput{RunID: packet.RunID, ScopeID: "project", LensID: "delivery", AgentID: "agent-a", RecordID: "missing-record"}},
		{name: "existing uncited record", input: ScopedGetInput{RunID: packet.RunID, ScopeID: "project", LensID: "delivery", AgentID: "agent-a", RecordID: uncitedRecord}},
		{name: "wrong scope", input: ScopedGetInput{RunID: packet.RunID, ScopeID: "project-b", LensID: "delivery", AgentID: "agent-a", RecordID: citation.RecordID}},
		{name: "wrong lens", input: ScopedGetInput{RunID: packet.RunID, ScopeID: "project", LensID: "delivery-b", AgentID: "agent-a", RecordID: citation.RecordID}},
		{name: "wrong agent", input: ScopedGetInput{RunID: packet.RunID, ScopeID: "project", LensID: "delivery", AgentID: "agent-b", RecordID: citation.RecordID}},
	}
	for _, item := range negativeHydration {
		if _, err := client.GetScoped(context.Background(), item.input); err == nil {
			t.Fatalf("%s hydrated a scoped capture", item.name)
		}
	}
	afterGet, err := client.Status(context.Background())
	if err != nil || beforeGet.State.RetrievalRunCount != afterGet.State.RetrievalRunCount ||
		beforeGet.State.JudgmentCount != afterGet.State.JudgmentCount {
		t.Fatalf("scoped get matrix mutated state before=%+v after=%+v err=%v", beforeGet.State, afterGet.State, err)
	}
	legacyCapture, err := client.Get(context.Background(), citation.RecordID)
	if err != nil || legacyCapture.AgentRecallApproved || legacyCapture.RouteClass != "legacy_agent_unscoped" {
		t.Fatalf("legacy capture=%+v err=%v", legacyCapture, err)
	}
	if _, err := client.ArchiveActor(context.Background(), "agent-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SearchScoped(context.Background(), ScopedSearchInput{
		Query: "product brain citations", ScopeID: "project", LensID: "delivery",
		AgentID: "agent-a", Limit: 3,
	}); err == nil {
		t.Fatal("archived actor was accepted")
	}
	if _, err := client.GetScoped(context.Background(), ScopedGetInput{
		RunID: packet.RunID, ScopeID: "project", LensID: "delivery", AgentID: "agent-a",
		RecordID: citation.RecordID,
	}); err == nil {
		t.Fatal("archived actor hydrated a scoped capture")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func seedUnrelatedExistingRecord(t *testing.T, root string) {
	t.Helper()
	repository, err := personalmemory.NewFileRepository(root, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	record, err := personalmemory.NewCaptureRecord(personalmemory.CaptureRecordInput{
		SourceAdapter: "slack", SourceScopeID: "self", SourceContainerID: "dm",
		ExternalID: "2", OccurredAt: "2026-08-08T08:01:00Z",
		SourceRef: "slack://self/dm/2", RawText: "orchards tides and ceramic glazing",
	})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := personalmemory.NewCaptureBatch(personalmemory.CaptureBatchInput{
		SourceIdentity: "slack-self", LowerInclusive: "2", UpperInclusive: "2",
		Watermark: "2", DeclaredRecords: 1, Records: []personalmemory.CaptureRecord{record},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Import(batch); err != nil {
		t.Fatal(err)
	}
}
