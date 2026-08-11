package agentstate

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestDurableFingerprintIncludesScopedState(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	store, err := Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	before, err := DurableFingerprint(path)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	if _, err := store.PutScope(context.Background(), Scope{
		ID: "fingerprint-scope", Name: "Fingerprint", Purpose: "prove scoped durability",
	}); err != nil {
		store.Close()
		t.Fatal(err)
	}
	after, err := DurableFingerprint(path)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("scoped state did not change durable fingerprint")
	}
	if _, err := store.PutScopedLens(context.Background(), ScopedLens{
		ScopeID: "fingerprint-scope", ID: "product", Name: "Product", Query: "strategy",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutAgentActor(context.Background(), AgentActor{
		ID: "agent-a", Name: "Agent A",
	}); err != nil {
		t.Fatal(err)
	}
	beforeConnection, err := DurableFingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("7", 64)
	if _, err := store.BindProjectConnection(context.Background(), digest, ScopedContext{
		ScopeID: "fingerprint-scope", LensID: "product", AgentID: "agent-a",
	}); err != nil {
		t.Fatal(err)
	}
	afterConnection, err := DurableFingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	if beforeConnection == afterConnection {
		t.Fatal("project connection did not change durable fingerprint")
	}
	if _, err := store.ArchiveProjectConnection(context.Background(), digest); err != nil {
		t.Fatal(err)
	}
	afterArchive, err := DurableFingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	if afterConnection == afterArchive {
		t.Fatal("project connection tombstone did not change durable fingerprint")
	}
}
