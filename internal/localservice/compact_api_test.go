package localservice

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

func TestCapabilitiesCompactSearchAndStatusPrivacy(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "mindline-compact-service-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
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

	capabilities, err := client.Capabilities(context.Background())
	if err != nil || capabilities.SchemaVersion != CapabilitiesSchemaVersion ||
		!capabilities.FeedbackRetryToken ||
		capabilities.CompactAbstentionPolicy != personalmemory.DefaultCompactAbstentionPolicy() ||
		!hasCapability(capabilities.SearchFormats, "mindline-agent-context-packet/v0.3") {
		t.Fatalf("capabilities=%+v err=%v", capabilities, err)
	}
	packet, err := client.SearchCompact(context.Background(), SearchInput{
		Query: "what is this and how", Limit: 3,
	})
	if err != nil || packet.SchemaVersion != "mindline-agent-context-packet/v0.3" ||
		packet.AnswerState != "abstained" {
		t.Fatalf("compact packet=%+v err=%v", packet, err)
	}
	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "database_path") ||
		strings.Contains(string(data), config.StatePath) {
		t.Fatalf("public status exposed runtime path: %s", data)
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

func hasCapability(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
