package sbos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSBOSProductionCodeDoesNotOwnMethodSections(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "sbos", "engine.go"))
	if err != nil {
		t.Fatalf("read engine: %v", err)
	}
	for _, term := range []string{"PARA", "CODE", "BASB", "Snapshot", "Source Content", "Key Details", "Relevance", "Signals", "Related Sources", "Next Action"} {
		if strings.Contains(string(body), term) {
			t.Fatalf("SBOS production code must not contain method-shaped term %q", term)
		}
	}
}
