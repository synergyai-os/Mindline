package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

func TestPersonalMemoryCLIImportsSearchesAndSurvivesNewRunner(t *testing.T) {
	root := filepath.Join(t.TempDir(), "personal-memory")
	batch := acquisitionslack.NativeBatch{
		SchemaVersion: acquisitionslack.NativeBatchSchema,
		WorkspaceID:   "T-cli", ChannelID: "D-cli",
		LowerInclusive: "1784902473.012269",
		UpperInclusive: "1784988423.680879",
		Watermark:      "1784988423.680879",
		IncludeThreads: true, IncludeReplies: true,
		PaginationExhausted: true, ThreadPaginationExhausted: true,
		DeclaredSourceRecords: 2,
		Messages: []acquisitionslack.NativeMessage{
			{NativeMessageID: "1784902473.012269", Timestamp: "1784902473.012269", AuthorName: "Randy", Text: "https://example.com/company-brain"},
			{NativeMessageID: "1784988423.680879", Timestamp: "1784988423.680879", AuthorName: "Randy", Text: "https://example.com/context-engineering"},
		},
	}
	input, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	runner := NewRunnerWithInput(NewOSFileSystem(), bytes.NewReader(input))
	if code := runner.Run([]string{"memory", "import-slack", "-", "--root", root}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("import failed: code=%d stderr=%s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	restarted := NewRunner(NewOSFileSystem())
	if code := restarted.Run([]string{"memory", "search", "company", "brain", "--root", root}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("search failed: code=%d stderr=%s", code, stderr.String())
	}
	var packet personalmemory.ContextPacket
	if err := json.Unmarshal(stdout.Bytes(), &packet); err != nil {
		t.Fatal(err)
	}
	if len(packet.Citations) != 1 || packet.Citations[0].AuthorityClass != personalmemory.AuthorityClass {
		t.Fatalf("unexpected agent context packet: %+v", packet)
	}
	if !strings.Contains(packet.Records[0].RawText, "company-brain") {
		t.Fatalf("full source context is missing: %+v", packet.Records[0])
	}
	enrichment := personalmemory.EnrichmentBatch{
		SchemaVersion: personalmemory.EnrichmentBatchSchemaVersion,
		Resources: []acquisition.ImportedEvidence{{
			CanonicalItemID: "activation-item",
			CanonicalURL:    packet.Records[0].URLs[0],
			State:           "complete",
			RetrievedAt:     "2026-07-26T10:00:00Z",
			AccessClass:     "public",
			Metadata:        acquisition.ImportedMetadata{Title: "Company memory architecture"},
			Excerpts: []acquisition.ImportedExcerpt{{
				ExcerptID: "excerpt-1",
				Text:      "Durable evidence should remain separate from organizational authority.",
				Locator:   "post",
			}},
		}},
		Contents: []personalmemory.ExtractedContent{{
			CanonicalURL: packet.Records[0].URLs[0],
			MediaType:    "text/plain",
			Completeness: "full",
			Text:         "The complete retained context says durable evidence remains separate from organizational authority and must survive restart.",
		}},
	}
	enrichmentInput, err := json.Marshal(enrichment)
	if err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	enrichmentRunner := NewRunnerWithInput(NewOSFileSystem(), bytes.NewReader(enrichmentInput))
	if code := enrichmentRunner.Run([]string{"memory", "enrich", "-", "--root", root}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("enrichment failed: code=%d stderr=%s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := restarted.Run([]string{"memory", "search", "organizational", "authority", "--root", root}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("enriched search failed: code=%d stderr=%s", code, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &packet); err != nil || len(packet.Resources) != 1 {
		t.Fatalf("enriched context packet is missing: %+v err=%v", packet, err)
	}
	if len(packet.Citations) != 1 ||
		len(packet.Citations[0].EvidenceRefs) == 0 ||
		packet.Citations[0].EvidenceRefs[0].ResourceID == "" ||
		packet.Citations[0].EvidenceRefs[0].ResourceHash == "" {
		t.Fatalf("search citation is not bound to retained resource evidence: %+v", packet.Citations)
	}
	recordID := packet.Citations[0].RecordID
	stdout.Reset()
	stderr.Reset()
	if code := restarted.Run([]string{"memory", "get", recordID, "--root", root}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("hydrated get failed: code=%d stderr=%s", code, stderr.String())
	}
	var hydrated personalmemory.HydratedCapture
	if err := json.Unmarshal(stdout.Bytes(), &hydrated); err != nil ||
		len(hydrated.Contents) != 1 ||
		!strings.Contains(hydrated.Contents[0].Text, "complete retained context") {
		t.Fatalf("hydrated capture did not return durable full content: %+v err=%v", hydrated, err)
	}
	lenses := make([]personalmemory.Lens, 12)
	for index := range lenses {
		lenses[index] = personalmemory.Lens{
			ID: "lens-" + string(rune('a'+index)), Name: "Lens", Query: "company",
		}
	}
	lensInput, err := json.Marshal(personalmemory.LensBatch{
		SchemaVersion: personalmemory.LensBatchSchemaVersion,
		Lenses:        lenses,
	})
	if err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	lensRunner := NewRunnerWithInput(NewOSFileSystem(), bytes.NewReader(lensInput))
	if code := lensRunner.Run([]string{"memory", "lenses", "-", "--root", root}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("lens review failed: code=%d stderr=%s", code, stderr.String())
	}
	var review personalmemory.LensReviewPacket
	if err := json.Unmarshal(stdout.Bytes(), &review); err != nil ||
		review.LensCount != 12 ||
		!review.RetentionUnchanged ||
		review.RetainedBefore != 2 ||
		review.RetainedAfter != 2 {
		t.Fatalf("lens review changed retention or constrained lens count: %+v err=%v", review, err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := restarted.Run([]string{"memory", "status", "--root", root}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("status failed: code=%d stderr=%s", code, stderr.String())
	}
	var status personalmemory.Status
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil || status.RecordCount != 2 {
		t.Fatalf("unexpected durable status: %+v err=%v", status, err)
	}
}

func TestPersonalMemoryCLIRejectsDuplicateJSONKeys(t *testing.T) {
	root := filepath.Join(t.TempDir(), "personal-memory")
	payload := []byte(`{"schema_version":"mindline-personal-enrichment-batch/v0.1","schema_version":"mindline-personal-enrichment-batch/v0.1","resources":[]}`)
	var stdout, stderr bytes.Buffer
	runner := NewRunnerWithInput(NewOSFileSystem(), bytes.NewReader(payload))
	if code := runner.Run([]string{"memory", "enrich", "-", "--root", root}, &stdout, &stderr); code != ExitUsage {
		t.Fatalf("duplicate JSON keys were accepted: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}
