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
	if lenses, err := client.ListScopedLenses(context.Background(), ""); err != nil || len(lenses) != 1 {
		t.Fatalf("all scoped lenses=%+v err=%v", lenses, err)
	}
	if _, err := client.SearchScoped(context.Background(), ScopedSearchInput{
		Query: "product brain citations", ScopeID: "project", LensID: "delivery",
	}); err == nil {
		t.Fatal("partial scoped tuple was accepted")
	}
	packet, err := client.SearchScoped(context.Background(), ScopedSearchInput{
		Query: "product brain citations", ScopeID: "project", LensID: "delivery",
		AgentID: "agent-a", Limit: 3,
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
	afterGet, err := client.Status(context.Background())
	if err != nil || beforeGet.State.RetrievalRunCount != afterGet.State.RetrievalRunCount ||
		beforeGet.State.JudgmentCount != afterGet.State.JudgmentCount {
		t.Fatalf("scoped get mutated state before=%+v after=%+v err=%v", beforeGet.State, afterGet.State, err)
	}
	if _, err := client.GetScoped(context.Background(), ScopedGetInput{
		RunID: "wrong-run", ScopeID: "project", LensID: "delivery", AgentID: "agent-a",
		RecordID: citation.RecordID,
	}); err == nil {
		t.Fatal("wrong run hydrated a scoped capture")
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
