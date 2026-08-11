package localservice

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/agentstate"
)

func TestProjectConnectionsStayInsideExplicitRootsAndSurviveRestart(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "mindline-connection-roots-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	configA, clientA, stopA := startProjectConnectionService(t, filepath.Join(root, "a"), "agent-a")
	_, clientB, stopB := startProjectConnectionService(t, filepath.Join(root, "b"), "agent-b")
	digest := strings.Repeat("e", 64)
	if _, err := clientA.BindProjectConnection(context.Background(), ProjectConnectionInput{
		Digest: digest, ScopeID: "project", LensID: "product", AgentID: "agent-a",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := clientB.BindProjectConnection(context.Background(), ProjectConnectionInput{
		Digest: digest, ScopeID: "project", LensID: "product", AgentID: "agent-b",
	}); err != nil {
		t.Fatal(err)
	}
	resolutionA, errA := clientA.ResolveProjectConnection(context.Background(), digest)
	resolutionB, errB := clientB.ResolveProjectConnection(context.Background(), digest)
	if errA != nil || errB != nil || resolutionA.AgentID != "agent-a" || resolutionB.AgentID != "agent-b" {
		t.Fatalf("root A=%+v err=%v root B=%+v err=%v", resolutionA, errA, resolutionB, errB)
	}
	stopA()
	restarted, err := NewServer(configA, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- restarted.Serve() }()
	restartedClient := NewClient(configA.SocketPath)
	waitForService(t, restartedClient)
	resolutionA, err = restartedClient.ResolveProjectConnection(context.Background(), digest)
	if err != nil || resolutionA.AgentID != "agent-a" {
		t.Fatalf("restarted resolution=%+v err=%v", resolutionA, err)
	}
	if _, err := restartedClient.ArchiveProjectConnection(context.Background(), digest); err != nil {
		t.Fatal(err)
	}
	if _, err := clientB.ResolveProjectConnection(context.Background(), digest); err != nil {
		t.Fatalf("archiving root A changed root B: %v", err)
	}
	closeProjectConnectionServer(t, restarted, result)
	stopB()
}

func startProjectConnectionService(
	t *testing.T, root, actorID string,
) (Config, *Client, func()) {
	t.Helper()
	config, err := ConfigFromRoots(filepath.Join(root, "runtime"), filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(config, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- server.Serve() }()
	client := NewClient(config.SocketPath)
	waitForService(t, client)
	ctx := context.Background()
	if _, err := client.PutScope(ctx, agentstate.Scope{ID: "project", Name: "Project", Purpose: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutScopedLens(ctx, agentstate.ScopedLens{
		ScopeID: "project", ID: "product", Name: "Product", Query: "strategy",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutActor(ctx, agentstate.AgentActor{ID: actorID, Name: actorID}); err != nil {
		t.Fatal(err)
	}
	return config, client, func() { closeProjectConnectionServer(t, server, result) }
}

func closeProjectConnectionServer(t *testing.T, server *Server, result <-chan error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}
