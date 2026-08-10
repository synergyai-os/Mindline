package agentstate

import (
	"context"
	"path/filepath"
	"testing"
)

func TestDurableFingerprintIncludesScopedState(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	store, err := Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
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
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("scoped state did not change durable fingerprint")
	}
}
