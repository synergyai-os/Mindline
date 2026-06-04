package gmail

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/documents"
)

func TestNormalizeAcceptsConnectorResponsesWrapper(t *testing.T) {
	result, err := Normalize(Payload{
		Source: Source{Account: "private-account", Mailbox: "all-mail", AdapterID: "gmail"},
		Responses: []Message{
			{ID: "m2", ThreadID: "t2", FromAlt: "sender@example.com", Subject: "Second", Body: "Body 2", EmailTS: "2026-06-04T08:00:00Z"},
			{ID: "m1", ThreadID: "t1", FromAlt: "sender@example.com", Subject: "First", Body: "Body 1", EmailTS: "2026-06-04T07:00:00Z"},
		},
	})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if result.AdapterID != "gmail" || len(result.Candidates) != 2 {
		t.Fatalf("bad result: %#v", result)
	}
	if !strings.HasPrefix(result.Candidates[0].ExternalID, "gmail-message-") || result.Checkpoint.FirstEmailTS != "2026-06-04T07:00:00Z" {
		t.Fatalf("expected old-to-new ordering, got %#v", result)
	}
	if strings.Contains(result.Candidates[0].ExternalID, "m1") {
		t.Fatalf("expected hashed private-safe external id, got %q", result.Candidates[0].ExternalID)
	}
	if !strings.HasPrefix(result.Candidates[0].CandidateID, "gmail-") || strings.Contains(result.Candidates[0].CandidateID, "m1") {
		t.Fatalf("expected hashed private-safe candidate id, got %q", result.Candidates[0].CandidateID)
	}
	if !result.Candidates[0].Safety.PrivateProvenance {
		t.Fatalf("expected Gmail provenance to be private")
	}
}

func TestBuildCorpusIntakeWritesPressureManifestAndBlocksSecrets(t *testing.T) {
	out := t.TempDir()
	secret := "sk-proj-" + strings.Repeat("a", 48)
	summary, err := BuildCorpusIntake(Payload{
		Source: Source{Account: "private-account", Mailbox: "inbox", AdapterID: "gmail"},
		Messages: []Message{
			{ID: "m1", FromAlt: "sender@example.com", Subject: "Research", Body: "Save https://example.com/research", EmailTS: "2026-06-04T07:00:00Z"},
			{ID: "m2", FromAlt: "sender@example.com", Subject: "", Body: "", EmailTS: "2026-06-04T08:00:00Z"},
			{ID: "m3", FromAlt: "sender@example.com", Subject: "Secret", Body: "token " + secret, EmailTS: "2026-06-04T09:00:00Z"},
		},
	}, out)
	if err != nil {
		t.Fatalf("BuildCorpusIntake: %v", err)
	}
	if summary.InputCount != 3 || summary.ProcessedCount != 1 || summary.SkippedCount != 1 || summary.BlockedCount != 1 {
		t.Fatalf("bad counts: %#v", summary)
	}
	if summary.DestinationWrites != 0 || summary.ProductBrainWrites != 0 || summary.TolariaWrites != 0 {
		t.Fatalf("expected read-only guardrails: %#v", summary)
	}
	data, err := os.ReadFile(filepath.Join(out, "corpus-pressure-manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest documents.CorpusPressureManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.SchemaVersion != documents.CorpusPressureManifestSchemaVersion || len(manifest.Sources) != 1 {
		t.Fatalf("bad manifest: %#v", manifest)
	}
	combined := readAllFiles(t, out)
	for _, forbidden := range []string{secret, "sender@example.com", "private-account"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("forbidden value leaked %q in output:\n%s", forbidden, combined)
		}
	}
	if strings.Contains(combined, "token ") {
		t.Fatalf("secret-like blocked source body leaked in output:\n%s", combined)
	}
}

func TestBuildCorpusIntakeSuppressesManifestWhenNoSourcesAreEligible(t *testing.T) {
	out := t.TempDir()
	summary, err := BuildCorpusIntake(Payload{
		Source: Source{Account: "private-account", Mailbox: "all-mail", AdapterID: "gmail"},
		Messages: []Message{
			{ID: "m1", Body: "", EmailTS: "2026-06-04T07:00:00Z"},
			{ID: "m2", Body: "api_key=sk_live_secret", EmailTS: "2026-06-04T08:00:00Z"},
		},
	}, out)
	if err != nil {
		t.Fatalf("BuildCorpusIntake: %v", err)
	}
	if summary.ProcessedCount != 0 || summary.ManifestPath != "" {
		t.Fatalf("expected no processed sources and no manifest: %#v", summary)
	}
	if _, err := os.Stat(filepath.Join(out, "corpus-pressure-manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("expected no pressure manifest, stat err=%v", err)
	}
}

func readAllFiles(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatalf("walk files: %v", err)
	}
	return b.String()
}
