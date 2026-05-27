package slack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/documents"
)

func TestBuildCorpusIntakeWritesPressureManifestAndAccountsForMessages(t *testing.T) {
	out := t.TempDir()
	summary, err := BuildCorpusIntake(corpusIntakePayload(), out)
	if err != nil {
		t.Fatalf("BuildCorpusIntake: %v", err)
	}
	if summary.InputCount != 3 || summary.ProcessedCount != 1 || summary.SkippedCount != 1 || summary.BlockedCount != 1 {
		t.Fatalf("bad counts: %#v", summary)
	}
	if summary.DestinationWrites != 0 || summary.ProductBrainWrites != 0 || summary.TolariaWrites != 0 {
		t.Fatalf("expected read-only guardrails: %#v", summary)
	}
	if summary.Items[0].SlackTS != "1710000000.000001" || summary.Items[0].State != CorpusIntakeItemSkipped {
		t.Fatalf("expected old-to-new item accounting, got %#v", summary.Items)
	}
	manifestPath := filepath.Join(out, "corpus-pressure-manifest.json")
	data, err := os.ReadFile(manifestPath)
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
	if manifest.Sources[0].SourceKind != documents.SourceKindMarkdown {
		t.Fatalf("expected markdown source, got %#v", manifest.Sources[0])
	}
	if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(manifest.Sources[0].Path))); err != nil {
		t.Fatalf("missing source markdown: %v", err)
	}
}

func TestBuildCorpusIntakeDoesNotLeakSecretOrPrivateFileURLToReports(t *testing.T) {
	out := t.TempDir()
	summary, err := BuildCorpusIntake(corpusIntakeUnsafePayload(), out)
	if err != nil {
		t.Fatalf("BuildCorpusIntake: %v", err)
	}
	if summary.ProcessedCount != 1 || summary.BlockedCount != 1 {
		t.Fatalf("bad unsafe counts: %#v", summary)
	}
	combined := readAllFiles(t, out)
	for _, forbidden := range []string{"sk_live_secret", "xoxb-secret-token", "files-pri"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("forbidden value leaked %q in output:\n%s", forbidden, combined)
		}
	}
	if !strings.Contains(combined, privateSlackFileSentinel) {
		t.Fatalf("expected private file sentinel in local source corpus")
	}
}

func TestBuildCorpusIntakeSuppressesManifestWhenNoSourcesAreEligible(t *testing.T) {
	out := t.TempDir()
	summary, err := BuildCorpusIntake(Payload{
		Source: Source{Workspace: "synthetic", ChannelID: "DTEST", ChannelName: "self-dm", AdapterID: "slack"},
		Messages: []Message{
			{TS: "1710000000.000001", User: "U123", AuthorName: "Randy", Text: ""},
			{TS: "1710000001.000001", User: "U123", AuthorName: "Randy", Text: "api_key=sk_live_secret"},
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
	report := string(mustReadFile(t, filepath.Join(out, CorpusIntakeDirName, "intake-report.md")))
	if !strings.Contains(report, "not emitted because there are no processed sources") {
		t.Fatalf("expected no-manifest report, got:\n%s", report)
	}
}

func TestBuildCorpusIntakeUsesMissingPermalinkSentinelInSource(t *testing.T) {
	out := t.TempDir()
	summary, err := BuildCorpusIntake(Payload{
		Source: Source{Workspace: "synthetic", ChannelID: "DTEST", ChannelName: "self-dm", AdapterID: "slack"},
		Messages: []Message{
			{TS: "1710000000.000001", User: "U123", AuthorName: "Randy", Text: "Save this"},
		},
	}, out)
	if err != nil {
		t.Fatalf("BuildCorpusIntake: %v", err)
	}
	if summary.ProcessedCount != 1 {
		t.Fatalf("expected processed source: %#v", summary)
	}
	source := string(mustReadFile(t, filepath.Join(out, filepath.FromSlash(summary.Items[0].SourcePath))))
	if !strings.Contains(source, "slack://missing-permalink/DTEST/1710000000000001") {
		t.Fatalf("missing permalink sentinel not written to source:\n%s", source)
	}
}

func TestBuildCorpusIntakeRejectsNestedSourceSymlinkEscape(t *testing.T) {
	out := t.TempDir()
	outside := t.TempDir()
	sourceID := corpusIntakeSourceID(Source{Workspace: "synthetic", ChannelID: "DTEST"}, "1710000000.000001")
	if err := os.MkdirAll(filepath.Join(out, "sources"), 0o755); err != nil {
		t.Fatalf("mkdir sources: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(out, "sources", sourceID)); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	summary, err := BuildCorpusIntake(Payload{
		Source: Source{Workspace: "synthetic", ChannelID: "DTEST", ChannelName: "self-dm", AdapterID: "slack"},
		Messages: []Message{
			{TS: "1710000000.000001", User: "U123", AuthorName: "Randy", Text: "private note"},
		},
	}, out)
	if err != nil {
		t.Fatalf("BuildCorpusIntake should account for per-source write failure: %v", err)
	}
	if summary.BlockedCount != 1 || summary.Items[0].ReasonCode != CorpusIntakeReasonArtifactWrite || summary.ManifestPath != "" {
		t.Fatalf("expected blocked artifact write and no manifest: %#v", summary)
	}
	if _, err := os.Stat(filepath.Join(outside, "source.md")); !os.IsNotExist(err) {
		t.Fatalf("source escaped to symlink target, stat err=%v", err)
	}
}

func TestBuildCorpusIntakeRejectsOutputRootSymlinkEscape(t *testing.T) {
	parent := t.TempDir()
	outside := t.TempDir()
	out := filepath.Join(parent, "intake")
	if err := os.Symlink(outside, out); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	_, err := BuildCorpusIntake(Payload{
		Source: Source{Workspace: "synthetic", ChannelID: "DTEST", ChannelName: "self-dm", AdapterID: "slack"},
		Messages: []Message{
			{TS: "1710000000.000001", User: "U123", AuthorName: "Randy", Text: "private note"},
		},
	}, out)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "sources")); !os.IsNotExist(err) {
		t.Fatalf("source artifacts escaped to symlink root, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, CorpusIntakeDirName)); !os.IsNotExist(err) {
		t.Fatalf("summary artifacts escaped to symlink root, stat err=%v", err)
	}
}

func TestBuildCorpusIntakeRejectsSummarySymlinkEscape(t *testing.T) {
	out := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(out, CorpusIntakeDirName)); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	_, err := BuildCorpusIntake(corpusIntakePayload(), out)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
}

func corpusIntakePayload() Payload {
	return Payload{
		Source: Source{Workspace: "synthetic", ChannelID: "DTEST", ChannelName: "self-dm", AdapterID: "slack"},
		Messages: []Message{
			{TS: "1710000002.000001", User: "U123", AuthorName: "Randy", Text: "password=sk_live_secret"},
			{TS: "1710000001.000001", User: "U123", AuthorName: "Randy", Text: "Save https://example.com/research", Permalink: "https://synthetic.slack.com/archives/DTEST/p1710000001000001"},
			{TS: "1710000000.000001", User: "U123", AuthorName: "Randy", Text: ""},
		},
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func corpusIntakeUnsafePayload() Payload {
	return Payload{
		Source: Source{Workspace: "synthetic", ChannelID: "DTEST", ChannelName: "self-dm", AdapterID: "slack"},
		Messages: []Message{
			{TS: "1710000000.000001", User: "U123", AuthorName: "Randy", Text: "token xoxb-secret-token"},
			{TS: "1710000001.000001", User: "U123", AuthorName: "Randy", Text: "Read https://files.slack.com/files-pri/T/F123/private.pdf"},
		},
	}
}

func readAllFiles(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteByte('\n')
		return nil
	}); err != nil {
		t.Fatalf("walk output: %v", err)
	}
	return b.String()
}
