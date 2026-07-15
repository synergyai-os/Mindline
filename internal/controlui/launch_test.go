package controlui

import (
	"context"
	"os"
	"strings"
	"testing"
)

type recordingOpener struct{ url string }

func (o *recordingOpener) Open(url string) error { o.url = url; return nil }

func TestLaunchUsesRandomLoopbackAndFragmentButSafeDisplayOmitsCapability(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opener := &recordingOpener{}
	authority, err := NewHumanAuthority()
	if err != nil {
		t.Fatal(err)
	}
	running, err := Launch(ctx, &fakeApplication{authority: authority}, root, opener)
	if err != nil {
		t.Fatal(err)
	}
	defer running.Shutdown(context.Background())
	if !strings.HasPrefix(opener.url, "http://127.0.0.1:") || !strings.Contains(opener.url, "#bootstrap=") {
		t.Fatalf("unexpected launch URL %q", opener.url)
	}
	if strings.Contains(running.SafeDisplayURL(), "bootstrap") || strings.Contains(running.SafeDisplayURL(), "#") {
		t.Fatalf("safe display URL leaks capability %q", running.SafeDisplayURL())
	}
}
