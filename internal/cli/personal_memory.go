package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
	slackadapter "github.com/synergyai-os/Mindline/internal/adapters/slack"
	"github.com/synergyai-os/Mindline/internal/founderreview"
	"github.com/synergyai-os/Mindline/internal/ingestioncontroller"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

const maximumNativeBatchBytes = 32 << 20

func (r Runner) runPersonalMemory(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	switch args[0] {
	case "resources-run":
		return r.runResourceCommand(args[1:], stdout, stderr, "run")
	case "resources-continue":
		return r.runResourceCommand(args[1:], stdout, stderr, "continue")
	case "resources-reconcile":
		return r.runResourceCommand(args[1:], stdout, stderr, "reconcile")
	case "resources-retry":
		return r.runResourceCommand(args[1:], stdout, stderr, "retry")
	case "resources-status":
		return r.runResourceCommand(args[1:], stdout, stderr, "status")
	case "resources-proof":
		return r.runResourceCommand(args[1:], stdout, stderr, "proof")
	case "resources-rebuild-proof":
		return r.runResourceCommand(args[1:], stdout, stderr, "rebuild")
	case "founder-review":
		return r.runFounderReview(args[1:], stdout, stderr)
	case "ingest-slack-run":
		return r.runIngestionApply(args[1:], stdout, stderr)
	case "ingest-slack-run-status":
		return r.runIngestionStatus(args[1:], stdout, stderr, false)
	case "ingest-slack-run-proof":
		return r.runIngestionStatus(args[1:], stdout, stderr, true)
	case "import-slack":
		return r.runPersonalMemoryImportSlack(args[1:], stdout, stderr)
	case "enrich":
		return r.runPersonalMemoryEnrich(args[1:], stdout, stderr)
	case "search":
		return r.runPersonalMemorySearch(args[1:], stdout, stderr)
	case "lenses":
		return r.runPersonalMemoryLenses(args[1:], stdout, stderr)
	case "get":
		return r.runPersonalMemoryGet(args[1:], stdout, stderr)
	case "status":
		return r.runPersonalMemoryStatus(args[1:], stdout, stderr)
	default:
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
}

type founderReviewInput struct {
	ProofRunID                 string                `json:"proof_run_id"`
	StructuralProofFingerprint string                `json:"structural_proof_fingerprint"`
	CitedRecordsFingerprint    string                `json:"cited_records_fingerprint"`
	Verdict                    founderreview.Verdict `json:"verdict"`
	RetryToken                 string                `json:"retry_token"`
}

func (r Runner) runFounderReview(args []string, stdout, stderr io.Writer) int {
	positionals, root, _, err := parsePersonalMemoryArgs(args, false)
	if err != nil || len(positionals) != 1 || positionals[0] != "-" {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	root, err = resolvePersonalMemoryRoot(root)
	if err != nil {
		fmt.Fprintln(stderr, "open founder review: unavailable")
		return ExitProcess
	}
	input, err := r.readPersonalMemoryInput("-")
	if err != nil || int64(len(input)) > founderreview.MaxRecordBytes {
		fmt.Fprintln(stderr, "read founder review: invalid closed schema")
		return ExitUsage
	}
	var request founderReviewInput
	if err := privateio.DecodeJSONStrict(input, &request); err != nil {
		fmt.Fprintln(stderr, "read founder review: invalid closed schema")
		return ExitUsage
	}
	repository, err := founderreview.NewRepository(
		filepath.Join(filepath.Dir(root), "founder-review-runtime"),
		founderreview.Options{},
	)
	if err != nil {
		fmt.Fprintln(stderr, "open founder review: unavailable")
		return ExitProcess
	}
	record, err := repository.Create(context.Background(), founderreview.Request{
		ProofRunID:                 request.ProofRunID,
		StructuralProofFingerprint: request.StructuralProofFingerprint,
		CitedRecordsFingerprint:    request.CitedRecordsFingerprint,
		Verdict:                    request.Verdict, RetryToken: request.RetryToken,
	})
	if err != nil {
		fmt.Fprintln(stderr, "record founder review: unavailable")
		return ExitProcess
	}
	return encodePersonalMemoryJSON(stdout, stderr, record.Receipt())
}

type ingestionProof struct {
	SchemaVersion           string `json:"schema_version"`
	State                   string `json:"state"`
	DeliveredCount          int    `json:"delivered_count"`
	UniqueNativeCount       int    `json:"unique_native_count"`
	CanonicalDeclaredCount  int    `json:"canonical_declared_count"`
	StructuralExcludedCount int    `json:"structural_excluded_count"`
	UserAuthoredExcluded    int    `json:"user_authored_excluded_count"`
	OwnedCount              int    `json:"owned_count"`
	RetainedCount           int    `json:"retained_count"`
	WithheldCount           int    `json:"withheld_count"`
	OverlapCount            int    `json:"overlap_count"`
	GapCount                int    `json:"gap_count"`
	ThreadCount             int    `json:"thread_count"`
	AggregateCommitment     string `json:"aggregate_commitment"`
	CanonicalFingerprint    string `json:"canonical_fingerprint"`
}

func (r Runner) runIngestionStatus(args []string, stdout, stderr io.Writer, proof bool) int {
	positionals, root, _, err := parsePersonalMemoryArgs(args, false)
	if err != nil || len(positionals) != 0 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	root, err = resolvePersonalMemoryRoot(root)
	if err != nil {
		fmt.Fprintln(stderr, "open ingestion ledger: unavailable")
		return ExitProcess
	}
	store, err := ingestioncontroller.NewLedgerStore(filepath.Join(filepath.Dir(root), "ingestion-ledger"))
	if err != nil {
		fmt.Fprintln(stderr, "open ingestion ledger: unavailable")
		return ExitProcess
	}
	ledger, err := store.Load()
	if err != nil {
		fmt.Fprintln(stderr, "read ingestion ledger: unavailable")
		return ExitProcess
	}
	_ = proof
	return encodePersonalMemoryJSON(stdout, stderr, projectIngestionProof(ledger))
}

func (r Runner) runIngestionApply(args []string, stdout, stderr io.Writer) int {
	positionals, root, _, err := parsePersonalMemoryArgs(args, false)
	if err != nil || len(positionals) != 1 || positionals[0] != "-" {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	root, err = resolvePersonalMemoryRoot(root)
	if err != nil {
		fmt.Fprintln(stderr, "open personal evidence library: stable root unavailable")
		return ExitProcess
	}
	envelope, err := ingestioncontroller.DecodeEnvelope(r.nativeInput)
	if err != nil {
		fmt.Fprintln(stderr, "read ingestion envelope: invalid closed framing")
		return ExitUsage
	}
	repository, err := personalmemory.NewFileRepository(root, nil)
	if err != nil {
		fmt.Fprintln(stderr, "open personal evidence library: unavailable")
		return ExitProcess
	}
	ledger, err := ingestioncontroller.NewLedgerStore(filepath.Join(filepath.Dir(root), "ingestion-ledger"))
	if err != nil {
		fmt.Fprintln(stderr, "open ingestion ledger: unavailable")
		return ExitProcess
	}
	result, err := (ingestioncontroller.Controller{Repository: repository, Ledger: ledger}).Apply(envelope)
	if err != nil {
		fmt.Fprintln(stderr, "apply ingestion envelope: incomplete")
		return ExitProcess
	}
	return encodePersonalMemoryJSON(stdout, stderr, projectIngestionProof(result))
}

func projectIngestionProof(ledger ingestioncontroller.Ledger) ingestionProof {
	return ingestionProof{
		SchemaVersion: ledger.SchemaVersion, State: ledger.State,
		DeliveredCount: ledger.DeliveredCount, UniqueNativeCount: ledger.OwnedCount,
		CanonicalDeclaredCount:  ledger.CanonicalDeclaredCount,
		StructuralExcludedCount: ledger.StructuralExcludedCount,
		UserAuthoredExcluded:    0, OwnedCount: ledger.OwnedCount,
		RetainedCount: ledger.RetainedCount, WithheldCount: ledger.WithheldCount,
		OverlapCount: ledger.OverlapCount, GapCount: ledger.GapCount,
		ThreadCount: ledger.ThreadCount, AggregateCommitment: ledger.AggregateCommitment,
		CanonicalFingerprint: ledger.CanonicalAfterFingerprint,
	}
}

func (r Runner) runPersonalMemoryImportSlack(args []string, stdout, stderr io.Writer) int {
	positionals, root, _, err := parsePersonalMemoryArgs(args, false)
	if err != nil || len(positionals) != 1 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	root, err = resolvePersonalMemoryRoot(root)
	if err != nil {
		fmt.Fprintln(stderr, "open personal evidence library: stable root unavailable")
		return ExitProcess
	}
	var input []byte
	if positionals[0] == "-" {
		input, err = io.ReadAll(io.LimitReader(r.nativeInput, maximumNativeBatchBytes+1))
		if err == nil && len(input) > maximumNativeBatchBytes {
			err = fmt.Errorf("native batch exceeds size limit")
		}
	} else {
		input, err = r.fs.ReadFileBounded(positionals[0], maximumNativeBatchBytes)
	}
	if err != nil {
		fmt.Fprintln(stderr, "read native Slack batch: source unavailable")
		return ExitUsage
	}
	var batch acquisitionslack.NativeBatch
	if err := privateio.DecodeJSONStrict(input, &batch); err != nil {
		fmt.Fprintln(stderr, "read native Slack batch: invalid closed schema")
		return ExitUsage
	}
	repository, err := personalmemory.NewFileRepository(root, nil)
	if err != nil {
		fmt.Fprintf(stderr, "open personal evidence library: %v\n", err)
		return ExitProcess
	}
	captureBatch, err := slackadapter.CaptureBatchFromNative(batch)
	if err != nil {
		fmt.Fprintf(stderr, "import personal evidence: %v\n", err)
		return ExitProcess
	}
	receipt, err := repository.Import(captureBatch)
	if err != nil {
		fmt.Fprintf(stderr, "import personal evidence: %v\n", err)
		return ExitProcess
	}
	return encodePersonalMemoryJSON(stdout, stderr, receipt)
}

func (r Runner) runPersonalMemoryEnrich(args []string, stdout, stderr io.Writer) int {
	positionals, root, _, err := parsePersonalMemoryArgs(args, false)
	if err != nil || len(positionals) != 1 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	root, err = resolvePersonalMemoryRoot(root)
	if err != nil {
		fmt.Fprintln(stderr, "open personal evidence library: stable root unavailable")
		return ExitProcess
	}
	input, err := r.readPersonalMemoryInput(positionals[0])
	if err != nil {
		fmt.Fprintln(stderr, "read personal enrichment batch: source unavailable")
		return ExitUsage
	}
	var batch personalmemory.EnrichmentBatch
	if err := privateio.DecodeJSONStrict(input, &batch); err != nil ||
		batch.SchemaVersion != personalmemory.EnrichmentBatchSchemaVersion {
		fmt.Fprintln(stderr, "read personal enrichment batch: invalid closed schema")
		return ExitUsage
	}
	repository, err := personalmemory.NewFileRepository(root, nil)
	if err != nil {
		fmt.Fprintf(stderr, "open personal evidence library: %v\n", err)
		return ExitProcess
	}
	receipt, err := repository.MergeEnrichment(batch)
	if err != nil {
		fmt.Fprintf(stderr, "merge personal evidence enrichment: %v\n", err)
		return ExitProcess
	}
	return encodePersonalMemoryJSON(stdout, stderr, receipt)
}

func (r Runner) runPersonalMemorySearch(args []string, stdout, stderr io.Writer) int {
	positionals, root, limit, err := parsePersonalMemoryArgs(args, true)
	if err != nil || len(positionals) == 0 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	root, err = resolvePersonalMemoryRoot(root)
	if err != nil {
		fmt.Fprintln(stderr, "open personal evidence library: stable root unavailable")
		return ExitProcess
	}
	repository, err := personalmemory.NewFileRepository(root, nil)
	if err != nil {
		fmt.Fprintf(stderr, "open personal evidence library: %v\n", err)
		return ExitProcess
	}
	packet, err := personalmemory.NewLexicalRetriever(repository).Search(personalmemory.SearchRequest{
		Query: strings.Join(positionals, " "), Limit: limit,
	})
	if err != nil {
		fmt.Fprintf(stderr, "search personal evidence: %v\n", err)
		return ExitProcess
	}
	return encodePersonalMemoryJSON(stdout, stderr, packet)
}

func (r Runner) runPersonalMemoryLenses(args []string, stdout, stderr io.Writer) int {
	positionals, root, limit, err := parsePersonalMemoryArgs(args, true)
	if err != nil || len(positionals) != 1 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	root, err = resolvePersonalMemoryRoot(root)
	if err != nil {
		fmt.Fprintln(stderr, "open personal evidence library: stable root unavailable")
		return ExitProcess
	}
	input, err := r.readPersonalMemoryInput(positionals[0])
	if err != nil {
		fmt.Fprintln(stderr, "read personal lens batch: source unavailable")
		return ExitUsage
	}
	var batch personalmemory.LensBatch
	if err := privateio.DecodeJSONStrict(input, &batch); err != nil {
		fmt.Fprintln(stderr, "read personal lens batch: invalid closed schema")
		return ExitUsage
	}
	repository, err := personalmemory.NewFileRepository(root, nil)
	if err != nil {
		fmt.Fprintf(stderr, "open personal evidence library: %v\n", err)
		return ExitProcess
	}
	packet, err := personalmemory.NewLexicalRetriever(repository).ReviewLenses(batch, limit)
	if err != nil {
		fmt.Fprintf(stderr, "review personal evidence lenses: %v\n", err)
		return ExitProcess
	}
	return encodePersonalMemoryJSON(stdout, stderr, packet)
}

func (r Runner) runPersonalMemoryGet(args []string, stdout, stderr io.Writer) int {
	positionals, root, _, err := parsePersonalMemoryArgs(args, false)
	if err != nil || len(positionals) != 1 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	root, err = resolvePersonalMemoryRoot(root)
	if err != nil {
		fmt.Fprintln(stderr, "open personal evidence library: stable root unavailable")
		return ExitProcess
	}
	repository, err := personalmemory.NewFileRepository(root, nil)
	if err != nil {
		fmt.Fprintf(stderr, "open personal evidence library: %v\n", err)
		return ExitProcess
	}
	record, err := personalmemory.NewLexicalRetriever(repository).Get(positionals[0])
	if err != nil {
		fmt.Fprintf(stderr, "get personal evidence: %v\n", err)
		return ExitProcess
	}
	return encodePersonalMemoryJSON(stdout, stderr, record)
}

func (r Runner) runPersonalMemoryStatus(args []string, stdout, stderr io.Writer) int {
	positionals, root, _, err := parsePersonalMemoryArgs(args, false)
	if err != nil || len(positionals) != 0 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	root, err = resolvePersonalMemoryRoot(root)
	if err != nil {
		fmt.Fprintln(stderr, "open personal evidence library: stable root unavailable")
		return ExitProcess
	}
	repository, err := personalmemory.NewFileRepository(root, nil)
	if err != nil {
		fmt.Fprintf(stderr, "open personal evidence library: %v\n", err)
		return ExitProcess
	}
	status, err := repository.Status()
	if err != nil {
		fmt.Fprintf(stderr, "read personal evidence status: %v\n", err)
		return ExitProcess
	}
	return encodePersonalMemoryJSON(stdout, stderr, status)
}

func parsePersonalMemoryArgs(args []string, allowLimit bool) ([]string, string, int, error) {
	positionals := []string{}
	root := ""
	limit := 10
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--root":
			index++
			if index >= len(args) || strings.TrimSpace(args[index]) == "" {
				return nil, "", 0, fmt.Errorf("missing root")
			}
			root = strings.TrimSpace(args[index])
		case "--limit":
			if !allowLimit {
				return nil, "", 0, fmt.Errorf("limit is not supported")
			}
			index++
			if index >= len(args) {
				return nil, "", 0, fmt.Errorf("missing limit")
			}
			parsed, err := strconv.Atoi(args[index])
			if err != nil || parsed < 1 || parsed > 100 {
				return nil, "", 0, fmt.Errorf("invalid limit")
			}
			limit = parsed
		default:
			if strings.HasPrefix(args[index], "--") {
				return nil, "", 0, fmt.Errorf("unknown option")
			}
			positionals = append(positionals, args[index])
		}
	}
	return positionals, root, limit, nil
}

func resolvePersonalMemoryRoot(root string) (string, error) {
	if strings.TrimSpace(root) != "" {
		return root, nil
	}
	controlRoot, err := privateio.DefaultControlPlaneRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(controlRoot, "personal-memory"), nil
}

func (r Runner) readPersonalMemoryInput(path string) ([]byte, error) {
	if path == "-" {
		input, err := io.ReadAll(io.LimitReader(r.nativeInput, maximumNativeBatchBytes+1))
		if err != nil || len(input) > maximumNativeBatchBytes {
			return nil, fmt.Errorf("personal memory input exceeds size limit")
		}
		return input, nil
	}
	return r.fs.ReadFileBounded(path, maximumNativeBatchBytes)
}

func encodePersonalMemoryJSON(stdout, stderr io.Writer, value any) int {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(stderr, "write personal evidence response: %v\n", err)
		return ExitUsage
	}
	return ExitOK
}
