package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	slackadapter "github.com/synergyai-os/Mindline/internal/adapters/slack"
	"github.com/synergyai-os/Mindline/internal/destinations"
	tolariadestination "github.com/synergyai-os/Mindline/internal/destinations/tolaria"
	"github.com/synergyai-os/Mindline/internal/documents"
	"github.com/synergyai-os/Mindline/internal/pipeline"
	"github.com/synergyai-os/Mindline/internal/productbrain"
	"github.com/synergyai-os/Mindline/internal/sbos"
)

const (
	ExitOK            = 0
	ExitUsage         = 1
	ExitProcess       = 2
	ExitArtifactWrite = 3
)

const usage = "usage: mindline process <candidate.json> [--out <dir>]\nusage: mindline slack normalize <slack-export.json> [--out <dir>]\nusage: mindline destination dry-run <sbos-result.json> --adapter tolaria --out <dir>\nusage: mindline pipeline dry-run <pipeline-input.json> --method basb-para-code --destination tolaria --out <dir>\nusage: mindline product-brain propose <run-dir> --profile <profile.json> --out <dir>\nusage: mindline documents decompose <markdown-path-or-dir> --out <dir>\nusage: mindline documents structure <markdown-path-or-dir> --out <dir>\nusage: mindline documents semantics <structure-run-dir-or-markdown-path-or-markdown-dir> --out <dir> [--classifier deterministic|llm --llm-provider openai --llm-model <model>]\nusage: mindline documents accept <semantic-run-dir> --answer-key <answer-key.json> --out <dir>\nusage: mindline documents calibrate <semantic-acceptance-dir-or-parent> --out <dir> [--threshold 0.98] [--held-out] [--source-root <dir> --source <relative.md>]\nusage: mindline documents calibrate-next <semantic-calibration-dir-or-parent>\nusage: mindline documents judge <semantic-run-dir> --out <dir> [--source-root <dir> --source <relative.md>]\nusage: mindline documents judge-next <semantic-judgment-dir-or-parent>\nusage: mindline documents judge-record <semantic-judgment-dir-or-parent> --candidate <candidate-id> --choice accept|reject|unclear|duplicate|wrong-kind [--note <text>] [--reviewer <id>]\n"

const protectedRootsEnv = "MINDLINE_PROTECTED_ROOTS"
const defaultTolariaProtectedRoot = "/Users/randyhereman/Young Human Club Dropbox/02. Areas/PKM - Tolaria"

var cliAuthorityIDs = []string{
	"DEC-4",
	"DEC-3",
	"DEC-2",
	"DEC-1",
	"FEAT-1",
	"STD-1",
	"STD-7",
	"STD-10",
	"STD-11",
	"STD-12",
	"FEAT-4",
	"WP-1",
}

type Runner struct {
	fs             FileSystem
	protectedRoots []string
}

type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	MkdirAll(path string, perm fs.FileMode) error
	Stat(path string) (fs.FileInfo, error)
	CanWriteDir(path string) error
	WriteFile(path string, data []byte) error
	Getwd() (string, error)
	RealPath(path string) (string, error)
	IsSymlink(path string) (bool, error)
}

type ResultEnvelope struct {
	State             string                   `json:"state"`
	RecordID          string                   `json:"record_id"`
	SourceCandidateID string                   `json:"source_candidate_id"`
	IdempotencyKey    string                   `json:"idempotency_key"`
	Safety            destinations.InputSafety `json:"safety"`
	ArtifactCount     int                      `json:"artifact_count"`
	Artifacts         []ArtifactEnvelope       `json:"artifacts"`
	AuthorityIDs      []string                 `json:"authority_ids"`
}

type ArtifactEnvelope struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
	Body string `json:"body"`
}

type SlackNormalizeEnvelope struct {
	AdapterID      string                        `json:"adapter_id"`
	CandidateCount int                           `json:"candidate_count"`
	Candidates     []SlackNormalizeCandidateItem `json:"candidates"`
	Checkpoint     slackadapter.Checkpoint       `json:"checkpoint"`
	AuthorityIDs   []string                      `json:"authority_ids"`
}

type DestinationDryRunSummary struct {
	DestinationAdapterID string                         `json:"destination_adapter_id"`
	WriteMode            string                         `json:"write_mode"`
	OperationCount       int                            `json:"operation_count"`
	BlockedCount         int                            `json:"blocked_count"`
	Operations           []DestinationDryRunSummaryItem `json:"operations"`
	AuthorityIDs         []string                       `json:"authority_ids"`
}

type DestinationDryRunSummaryItem struct {
	OperationID       string `json:"operation_id"`
	OperationType     string `json:"operation_type"`
	VisibilityLane    string `json:"visibility_lane"`
	OperationJSONPath string `json:"operation_json_path"`
	PreviewPath       string `json:"preview_path"`
	Blocked           bool   `json:"blocked"`
}

type SlackNormalizeCandidateItem struct {
	Path      string          `json:"path"`
	Candidate *sbos.Candidate `json:"candidate,omitempty"`
}

func NewRunner(fileSystem FileSystem) Runner {
	return NewRunnerWithProtectedRoots(fileSystem, configuredProtectedRoots())
}

func NewRunnerWithProtectedRoots(fileSystem FileSystem, protectedRoots []string) Runner {
	return Runner{fs: fileSystem, protectedRoots: append([]string(nil), protectedRoots...)}
}

func configuredProtectedRoots() []string {
	raw := strings.TrimSpace(os.Getenv(protectedRootsEnv))
	if raw == "" {
		return []string{defaultTolariaProtectedRoot}
	}
	parts := strings.Split(raw, string(os.PathListSeparator))
	roots := make([]string, 0, len(parts))
	for _, part := range parts {
		if root := strings.TrimSpace(part); root != "" {
			roots = append(roots, root)
		}
	}
	if len(roots) == 0 {
		return []string{defaultTolariaProtectedRoot}
	}
	return roots
}

func (r Runner) Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	if args[0] == "slack" {
		return r.runSlack(args[1:], stdout, stderr)
	}
	if args[0] == "destination" {
		return r.runDestination(args[1:], stdout, stderr)
	}
	if args[0] == "pipeline" {
		return r.runPipeline(args[1:], stdout, stderr)
	}
	if args[0] == "product-brain" {
		return r.runProductBrain(args[1:], stdout, stderr)
	}
	if args[0] == "documents" {
		return r.runDocuments(args[1:], stdout, stderr)
	}
	if args[0] != "process" {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}

	candidatePath, outDir, parseError := parseProcessArgs(args[1:])
	if parseError == parseErrorInvalidOut {
		fmt.Fprint(stderr, "invalid --out: empty value\n")
		return ExitUsage
	}
	if parseError != parseErrorNone {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}

	input, err := r.fs.ReadFile(candidatePath)
	if err != nil {
		fmt.Fprintf(stderr, "read candidate: %v\n", err)
		return ExitUsage
	}

	result, err := sbos.NewEngine().ProcessCandidate(input)
	if err != nil {
		fmt.Fprintf(stderr, "process candidate: %v\n", err)
		return ExitProcess
	}

	if outDir != "" {
		if err := r.validateOutDir(outDir); err != nil {
			fmt.Fprintf(stderr, "invalid --out: %v\n", err)
			return ExitUsage
		}
	}

	envelope := ResultEnvelope{
		State:             string(result.State),
		RecordID:          result.RecordID,
		SourceCandidateID: result.SourceCandidateID,
		IdempotencyKey:    result.IdempotencyKey,
		Safety: destinations.InputSafety{
			PrivateProvenance: result.Safety.PrivateProvenance,
			RedactionRequired: result.Safety.RedactionRequired,
			SecretLike:        result.Safety.SecretLike,
		},
		ArtifactCount: len(result.Artifacts),
		AuthorityIDs:  authorityIDs(),
	}

	for _, artifact := range result.Artifacts {
		item := ArtifactEnvelope{Kind: string(artifact.Kind), Body: artifact.Body}
		if outDir != "" {
			path, err := r.writeArtifact(outDir, result.RecordID, artifact)
			if err != nil {
				fmt.Fprintf(stderr, "write artifact: %v\n", err)
				return ExitArtifactWrite
			}
			item.Path = path
			item.Body = ""
		}
		envelope.Artifacts = append(envelope.Artifacts, item)
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(envelope); err != nil {
		fmt.Fprintf(stderr, "write stdout: %v\n", err)
		return ExitUsage
	}
	return ExitOK
}

func (r Runner) runDocuments(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "structure" {
		return r.runDocumentsStructure(args, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "semantics" {
		return r.runDocumentsSemantics(args, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "accept" {
		return r.runDocumentsAccept(args, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "calibrate" {
		return r.runDocumentsCalibrate(args, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "calibrate-next" {
		return r.runDocumentsCalibrateNext(args, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "judge" {
		return r.runDocumentsJudge(args, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "judge-next" {
		return r.runDocumentsJudgeNext(args, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "judge-record" {
		return r.runDocumentsJudgeRecord(args, stdout, stderr)
	}
	inputPath, outDir, parseError := parseDocumentsArgs(args, "decompose")
	if parseError != parseErrorNone {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	summary, err := documents.DecomposePath(inputPath, outDir)
	if err != nil {
		if documents.IsArtifactWriteError(err) {
			fmt.Fprintf(stderr, "write document segments: %v\n", err)
			return ExitArtifactWrite
		}
		fmt.Fprintf(stderr, "decompose documents: %v\n", err)
		return ExitProcess
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		fmt.Fprintf(stderr, "write stdout: %v\n", err)
		return ExitUsage
	}
	return ExitOK
}

func (r Runner) runDocumentsJudge(args []string, stdout, stderr io.Writer) int {
	inputPath, outDir, options, parseError := parseDocumentsJudgeArgs(args)
	if parseError != parseErrorNone {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	summary, err := documents.JudgeSemanticCandidates(inputPath, outDir, options)
	if err != nil {
		if documents.IsArtifactWriteError(err) {
			fmt.Fprintf(stderr, "write semantic judgments: %v\n", err)
			return ExitArtifactWrite
		}
		fmt.Fprintf(stderr, "judge semantic candidates: %v\n", err)
		return ExitProcess
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		fmt.Fprintf(stderr, "write stdout: %v\n", err)
		return ExitUsage
	}
	return ExitOK
}

func (r Runner) runDocumentsJudgeNext(args []string, stdout, stderr io.Writer) int {
	inputPath, parseError := parseDocumentsJudgeNextArgs(args)
	if parseError != parseErrorNone {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	page, err := documents.NextSemanticJudgmentPage(inputPath)
	if err != nil {
		if documents.IsArtifactWriteError(err) {
			fmt.Fprintf(stderr, "write semantic judgment cursor: %v\n", err)
			return ExitArtifactWrite
		}
		fmt.Fprintf(stderr, "page semantic judgments: %v\n", err)
		return ExitProcess
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(page); err != nil {
		fmt.Fprintf(stderr, "write stdout: %v\n", err)
		return ExitUsage
	}
	return ExitOK
}

func (r Runner) runDocumentsJudgeRecord(args []string, stdout, stderr io.Writer) int {
	inputPath, record, parseError := parseDocumentsJudgeRecordArgs(args)
	if parseError != parseErrorNone {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	summary, err := documents.RecordSemanticJudgment(inputPath, record)
	if err != nil {
		if documents.IsArtifactWriteError(err) {
			fmt.Fprintf(stderr, "write semantic judgment: %v\n", err)
			return ExitArtifactWrite
		}
		fmt.Fprintf(stderr, "record semantic judgment: %v\n", err)
		return ExitProcess
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		fmt.Fprintf(stderr, "write stdout: %v\n", err)
		return ExitUsage
	}
	return ExitOK
}

func (r Runner) runDocumentsAccept(args []string, stdout, stderr io.Writer) int {
	inputPath, answerKeyPath, outDir, parseError := parseDocumentsAcceptArgs(args)
	if parseError != parseErrorNone {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	summary, err := documents.AcceptSemantic(inputPath, answerKeyPath, outDir)
	if err != nil {
		if documents.IsArtifactWriteError(err) {
			fmt.Fprintf(stderr, "write semantic acceptance: %v\n", err)
			return ExitArtifactWrite
		}
		fmt.Fprintf(stderr, "accept semantic candidates: %v\n", err)
		return ExitProcess
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		fmt.Fprintf(stderr, "write stdout: %v\n", err)
		return ExitUsage
	}
	return ExitOK
}

func (r Runner) runDocumentsCalibrate(args []string, stdout, stderr io.Writer) int {
	inputPath, outDir, options, parseError := parseDocumentsCalibrateArgs(args)
	if parseError != parseErrorNone {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	summary, err := documents.CalibrateSemanticAcceptance(inputPath, outDir, options)
	if err != nil {
		if documents.IsArtifactWriteError(err) {
			fmt.Fprintf(stderr, "write semantic calibration: %v\n", err)
			return ExitArtifactWrite
		}
		fmt.Fprintf(stderr, "calibrate semantic acceptance: %v\n", err)
		return ExitProcess
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		fmt.Fprintf(stderr, "write stdout: %v\n", err)
		return ExitUsage
	}
	return ExitOK
}

func (r Runner) runDocumentsCalibrateNext(args []string, stdout, stderr io.Writer) int {
	inputPath, parseError := parseDocumentsCalibrateNextArgs(args)
	if parseError != parseErrorNone {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	page, err := documents.NextSemanticCalibrationReviewPage(inputPath)
	if err != nil {
		if documents.IsArtifactWriteError(err) {
			fmt.Fprintf(stderr, "write semantic calibration cursor: %v\n", err)
			return ExitArtifactWrite
		}
		fmt.Fprintf(stderr, "page semantic calibration: %v\n", err)
		return ExitProcess
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(page); err != nil {
		fmt.Fprintf(stderr, "write stdout: %v\n", err)
		return ExitUsage
	}
	return ExitOK
}

func (r Runner) runDocumentsSemantics(args []string, stdout, stderr io.Writer) int {
	inputPath, outDir, options, parseError, configError := r.parseDocumentsSemanticsArgs(args)
	if configError != "" {
		fmt.Fprintln(stderr, configError)
		return ExitUsage
	}
	if parseError != parseErrorNone {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	summary, err := documents.SemanticPathWithOptions(inputPath, outDir, options)
	if err != nil {
		if documents.IsArtifactWriteError(err) {
			fmt.Fprintf(stderr, "write semantic candidates: %v\n", err)
			return ExitArtifactWrite
		}
		fmt.Fprintf(stderr, "generate semantic candidates: %v\n", err)
		return ExitProcess
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		fmt.Fprintf(stderr, "write stdout: %v\n", err)
		return ExitUsage
	}
	return ExitOK
}

func (r Runner) runDocumentsStructure(args []string, stdout, stderr io.Writer) int {
	inputPath, outDir, parseError := parseDocumentsArgs(args, "structure")
	if parseError != parseErrorNone {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	summary, err := documents.StructurePath(inputPath, outDir)
	if err != nil {
		if documents.IsArtifactWriteError(err) {
			fmt.Fprintf(stderr, "write document structure: %v\n", err)
			return ExitArtifactWrite
		}
		fmt.Fprintf(stderr, "structure documents: %v\n", err)
		return ExitProcess
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		fmt.Fprintf(stderr, "write stdout: %v\n", err)
		return ExitUsage
	}
	return ExitOK
}

func (r Runner) runProductBrain(args []string, stdout, stderr io.Writer) int {
	runDir, profilePath, outDir, parseError := parseProductBrainProposeArgs(args)
	if parseError != parseErrorNone {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	summary, err := productbrain.Propose(runDir, profilePath, outDir)
	if err != nil {
		if strings.Contains(err.Error(), "write") || strings.Contains(err.Error(), "output") || strings.Contains(err.Error(), "private or secret") {
			fmt.Fprintf(stderr, "write Product Brain proposals: %v\n", err)
			return ExitArtifactWrite
		}
		fmt.Fprintf(stderr, "propose Product Brain writes: %v\n", err)
		return ExitProcess
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		fmt.Fprintf(stderr, "write stdout: %v\n", err)
		return ExitUsage
	}
	return ExitOK
}

func (r Runner) runPipeline(args []string, stdout, stderr io.Writer) int {
	inputPath, methodID, destinationID, outDir, parseError := parsePipelineDryRunArgs(args)
	if parseError != parseErrorNone {
		if parseError == parseErrorInvalidOut {
			fmt.Fprint(stderr, "missing required --out\n")
			return ExitProcess
		}
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	if methodID != pipeline.MethodBASBPARACODE {
		fmt.Fprintf(stderr, "unsupported method: %s\n", methodID)
		return ExitProcess
	}
	if destinationID != pipeline.DestinationTolaria {
		fmt.Fprintf(stderr, "unsupported destination: %s\n", destinationID)
		return ExitProcess
	}
	input, err := pipeline.ParseInputFile(inputPath, pipeline.ParseOptions{})
	if err != nil {
		fmt.Fprintf(stderr, "parse pipeline input: %v\n", err)
		return ExitProcess
	}
	if input.Method.ID != methodID {
		fmt.Fprintf(stderr, "method mismatch: input=%s cli=%s\n", input.Method.ID, methodID)
		return ExitProcess
	}
	if input.Destination.ID != destinationID {
		fmt.Fprintf(stderr, "destination mismatch: input=%s cli=%s\n", input.Destination.ID, destinationID)
		return ExitProcess
	}
	summary, err := pipeline.Run(inputPath, outDir, pipeline.RunOptions{ProtectedRoots: r.protectedRoots})
	if err != nil {
		if strings.Contains(err.Error(), "protected Tolaria vault") || strings.Contains(err.Error(), "sentinel") || strings.Contains(err.Error(), "output") {
			fmt.Fprintf(stderr, "write pipeline artifacts: %v\n", err)
			return ExitArtifactWrite
		}
		fmt.Fprintf(stderr, "run pipeline: %v\n", err)
		return ExitProcess
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		fmt.Fprintf(stderr, "write stdout: %v\n", err)
		return ExitUsage
	}
	return ExitOK
}

func (r Runner) runDestination(args []string, stdout, stderr io.Writer) int {
	inputPath, adapterID, outDir, parseError := parseDestinationDryRunArgs(args)
	if parseError != parseErrorNone {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	if adapterID != tolariadestination.AdapterID {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	if err := r.validateDestinationOutDir(outDir); err != nil {
		fmt.Fprintf(stderr, "invalid --out: %v\n", err)
		return ExitUsage
	}

	input, err := r.fs.ReadFile(inputPath)
	if err != nil {
		fmt.Fprintf(stderr, "read destination input: %v\n", err)
		return ExitUsage
	}
	inputRealPath, err := r.fs.RealPath(inputPath)
	if err != nil {
		fmt.Fprintf(stderr, "resolve destination input: %v\n", err)
		return ExitUsage
	}
	result, err := destinations.ParseDestinationInput(input, destinations.ParseOptions{BaseDir: filepath.Dir(inputRealPath)})
	if err != nil {
		fmt.Fprintf(stderr, "parse destination input: %v\n", err)
		return ExitProcess
	}
	operations, err := tolariadestination.Plan(result)
	if err != nil {
		fmt.Fprintf(stderr, "plan destination dry-run: %v\n", err)
		return ExitProcess
	}

	summary := DestinationDryRunSummary{
		DestinationAdapterID: tolariadestination.AdapterID,
		WriteMode:            string(destinations.WriteModeDryRun),
		OperationCount:       len(operations),
		AuthorityIDs:         result.AuthorityIDs,
	}
	for _, operation := range operations {
		operationJSONPath, err := r.writeDestinationOperation(outDir, operation)
		if err != nil {
			fmt.Fprintf(stderr, "write destination operation: %v\n", err)
			return ExitArtifactWrite
		}
		previewPath := ""
		if destinationOperationHasPreview(operation) {
			previewPath, err = r.writeDestinationPreview(outDir, operation)
			if err != nil {
				fmt.Fprintf(stderr, "write destination preview: %v\n", err)
				return ExitArtifactWrite
			}
		}
		blocked := operation.OperationType == destinations.OperationBlocked || operation.OperationType == destinations.OperationSkip
		if blocked {
			summary.BlockedCount++
		}
		summary.Operations = append(summary.Operations, DestinationDryRunSummaryItem{
			OperationID:       operation.OperationID,
			OperationType:     string(operation.OperationType),
			VisibilityLane:    string(operation.VisibilityLane),
			OperationJSONPath: operationJSONPath,
			PreviewPath:       previewPath,
			Blocked:           blocked,
		})
	}
	if err := r.writeDestinationSummary(outDir, summary); err != nil {
		fmt.Fprintf(stderr, "write destination summary: %v\n", err)
		return ExitArtifactWrite
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		fmt.Fprintf(stderr, "write stdout: %v\n", err)
		return ExitUsage
	}
	return ExitOK
}

func (r Runner) runSlack(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "normalize" {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}

	inputPath, outDir, parseError := parseProcessArgs(args[1:])
	if parseError == parseErrorInvalidOut {
		fmt.Fprint(stderr, "invalid --out: empty value\n")
		return ExitUsage
	}
	if parseError != parseErrorNone {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}

	input, err := r.fs.ReadFile(inputPath)
	if err != nil {
		fmt.Fprintf(stderr, "read Slack export: %v\n", err)
		return ExitUsage
	}

	var payload slackadapter.Payload
	if err := json.Unmarshal(input, &payload); err != nil {
		fmt.Fprintf(stderr, "normalize Slack export: %v\n", err)
		return ExitProcess
	}
	result, err := slackadapter.Normalize(payload)
	if err != nil {
		fmt.Fprintf(stderr, "normalize Slack export: %v\n", err)
		return ExitProcess
	}

	if outDir != "" {
		if err := r.validateOutDir(outDir); err != nil {
			fmt.Fprintf(stderr, "invalid --out: %v\n", err)
			return ExitUsage
		}
	}

	envelope := SlackNormalizeEnvelope{
		AdapterID:      result.AdapterID,
		CandidateCount: len(result.Candidates),
		Checkpoint:     result.Checkpoint,
		AuthorityIDs:   result.AuthorityIDs,
	}
	for _, candidate := range result.Candidates {
		candidate := candidate
		item := SlackNormalizeCandidateItem{Candidate: &candidate}
		if outDir != "" {
			path, err := r.writeCandidate(outDir, candidate)
			if err != nil {
				fmt.Fprintf(stderr, "write Slack candidate: %v\n", err)
				return ExitArtifactWrite
			}
			item.Path = path
			item.Candidate = nil
		}
		envelope.Candidates = append(envelope.Candidates, item)
	}
	if outDir != "" {
		if err := r.writeCheckpoint(outDir, result.Checkpoint); err != nil {
			fmt.Fprintf(stderr, "write Slack checkpoint: %v\n", err)
			return ExitArtifactWrite
		}
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(envelope); err != nil {
		fmt.Fprintf(stderr, "write stdout: %v\n", err)
		return ExitUsage
	}
	return ExitOK
}

type parseError string

const (
	parseErrorNone       parseError = ""
	parseErrorUsage      parseError = "usage"
	parseErrorInvalidOut parseError = "invalid_out"
)

func parseProcessArgs(args []string) (candidatePath string, outDir string, err parseError) {
	if len(args) != 1 && len(args) != 3 {
		return "", "", parseErrorUsage
	}
	candidatePath = args[0]
	if strings.TrimSpace(candidatePath) == "" {
		return "", "", parseErrorUsage
	}
	if len(args) == 1 {
		return candidatePath, "", parseErrorNone
	}
	if args[1] != "--out" {
		return "", "", parseErrorUsage
	}
	if strings.TrimSpace(args[2]) == "" {
		return "", "", parseErrorInvalidOut
	}
	return candidatePath, args[2], parseErrorNone
}

func parseDestinationDryRunArgs(args []string) (inputPath string, adapterID string, outDir string, err parseError) {
	if len(args) != 6 {
		return "", "", "", parseErrorUsage
	}
	if args[0] != "dry-run" || strings.TrimSpace(args[1]) == "" {
		return "", "", "", parseErrorUsage
	}
	inputPath = args[1]
	for i := 2; i < len(args); i += 2 {
		if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
			return "", "", "", parseErrorUsage
		}
		switch args[i] {
		case "--adapter":
			adapterID = args[i+1]
		case "--out":
			outDir = args[i+1]
		default:
			return "", "", "", parseErrorUsage
		}
	}
	if adapterID == "" || outDir == "" {
		return "", "", "", parseErrorUsage
	}
	return inputPath, adapterID, outDir, parseErrorNone
}

func parsePipelineDryRunArgs(args []string) (inputPath string, methodID string, destinationID string, outDir string, err parseError) {
	if len(args) < 2 || args[0] != "dry-run" || strings.TrimSpace(args[1]) == "" {
		return "", "", "", "", parseErrorUsage
	}
	inputPath = args[1]
	if len(args) == 2 {
		return "", "", "", "", parseErrorInvalidOut
	}
	if len(args) != 8 {
		return "", "", "", "", parseErrorUsage
	}
	for i := 2; i < len(args); i += 2 {
		if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
			if args[i] == "--out" {
				return "", "", "", "", parseErrorInvalidOut
			}
			return "", "", "", "", parseErrorUsage
		}
		switch args[i] {
		case "--method":
			methodID = args[i+1]
		case "--destination":
			destinationID = args[i+1]
		case "--out":
			outDir = args[i+1]
		default:
			return "", "", "", "", parseErrorUsage
		}
	}
	if outDir == "" {
		return "", "", "", "", parseErrorInvalidOut
	}
	if methodID == "" || destinationID == "" {
		return "", "", "", "", parseErrorUsage
	}
	return inputPath, methodID, destinationID, outDir, parseErrorNone
}

func parseProductBrainProposeArgs(args []string) (runDir string, profilePath string, outDir string, err parseError) {
	if len(args) != 6 || args[0] != "propose" || strings.TrimSpace(args[1]) == "" {
		return "", "", "", parseErrorUsage
	}
	runDir = args[1]
	for i := 2; i < len(args); i += 2 {
		if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
			return "", "", "", parseErrorUsage
		}
		switch args[i] {
		case "--profile":
			profilePath = args[i+1]
		case "--out":
			outDir = args[i+1]
		default:
			return "", "", "", parseErrorUsage
		}
	}
	if profilePath == "" || outDir == "" {
		return "", "", "", parseErrorUsage
	}
	return runDir, profilePath, outDir, parseErrorNone
}

func parseDocumentsArgs(args []string, command string) (inputPath string, outDir string, err parseError) {
	if len(args) != 4 || args[0] != command || strings.TrimSpace(args[1]) == "" {
		return "", "", parseErrorUsage
	}
	inputPath = args[1]
	if args[2] != "--out" || strings.TrimSpace(args[3]) == "" {
		return "", "", parseErrorUsage
	}
	return inputPath, args[3], parseErrorNone
}

func (r Runner) parseDocumentsSemanticsArgs(args []string) (inputPath string, outDir string, options documents.SemanticOptions, err parseError, configError string) {
	options.Classifier = documents.SemanticClassifierDeterministic
	if len(args) < 4 || args[0] != "semantics" || strings.TrimSpace(args[1]) == "" {
		return "", "", options, parseErrorUsage, ""
	}
	inputPath = args[1]
	for i := 2; i < len(args); {
		switch args[i] {
		case "--out":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", "", options, parseErrorUsage, ""
			}
			outDir = args[i+1]
			i += 2
		case "--classifier":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", "", options, parseErrorUsage, ""
			}
			classifier := documents.SemanticClassifier(args[i+1])
			if classifier != documents.SemanticClassifierDeterministic && classifier != documents.SemanticClassifierLLM {
				return "", "", options, parseErrorUsage, ""
			}
			options.Classifier = classifier
			i += 2
		case "--llm-provider":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", "", options, parseErrorUsage, ""
			}
			options.LLMProvider = strings.TrimSpace(args[i+1])
			i += 2
		case "--llm-model":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", "", options, parseErrorUsage, ""
			}
			options.LLMModel = strings.TrimSpace(args[i+1])
			i += 2
		case "--profile", "--destination":
			return "", "", options, parseErrorUsage, ""
		default:
			return "", "", options, parseErrorUsage, ""
		}
	}
	if outDir == "" {
		return "", "", options, parseErrorUsage, ""
	}
	options = r.resolveSemanticLLMEnv(options)
	if options.Classifier == documents.SemanticClassifierLLM {
		if options.LLMProvider == "" {
			return "", "", options, parseErrorNone, "missing LLM provider"
		}
		if options.LLMProvider != "openai" {
			return "", "", options, parseErrorNone, fmt.Sprintf("unsupported LLM provider: %s", options.LLMProvider)
		}
		if options.LLMModel == "" {
			return "", "", options, parseErrorNone, "missing OpenAI model"
		}
		if options.LLMAPIKey == "" {
			return "", "", options, parseErrorNone, "missing OpenAI API key"
		}
	}
	return inputPath, outDir, options, parseErrorNone, ""
}

func (r Runner) resolveSemanticLLMEnv(options documents.SemanticOptions) documents.SemanticOptions {
	values := map[string]string{}
	if wd, err := r.fs.Getwd(); err == nil {
		if data, err := r.fs.ReadFile(filepath.Join(wd, ".env.local")); err == nil {
			for key, value := range parseDotEnv(string(data)) {
				values[key] = value
			}
		}
	}
	for _, key := range []string{"MINDLINE_LLM_PROVIDER", "OPENAI_API_KEY", "OPENAI_MODEL"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			values[key] = value
		}
	}
	if options.LLMProvider == "" {
		options.LLMProvider = values["MINDLINE_LLM_PROVIDER"]
	}
	if options.LLMModel == "" {
		options.LLMModel = values["OPENAI_MODEL"]
	}
	options.LLMAPIKey = values["OPENAI_API_KEY"]
	return options
}

func parseDotEnv(raw string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key != "" {
			out[key] = value
		}
	}
	return out
}

func parseDocumentsAcceptArgs(args []string) (inputPath string, answerKeyPath string, outDir string, err parseError) {
	if len(args) != 6 || args[0] != "accept" || strings.TrimSpace(args[1]) == "" {
		return "", "", "", parseErrorUsage
	}
	inputPath = args[1]
	for i := 2; i < len(args); i += 2 {
		if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
			return "", "", "", parseErrorUsage
		}
		switch args[i] {
		case "--answer-key":
			answerKeyPath = args[i+1]
		case "--out":
			outDir = args[i+1]
		default:
			return "", "", "", parseErrorUsage
		}
	}
	if answerKeyPath == "" || outDir == "" {
		return "", "", "", parseErrorUsage
	}
	return inputPath, answerKeyPath, outDir, parseErrorNone
}

func parseDocumentsCalibrateArgs(args []string) (inputPath string, outDir string, options documents.SemanticCalibrationOptions, err parseError) {
	if len(args) < 4 || args[0] != "calibrate" || strings.TrimSpace(args[1]) == "" {
		return "", "", options, parseErrorUsage
	}
	inputPath = args[1]
	for i := 2; i < len(args); {
		switch args[i] {
		case "--out":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", "", options, parseErrorUsage
			}
			outDir = args[i+1]
			i += 2
		case "--threshold":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", "", options, parseErrorUsage
			}
			threshold, parseErr := strconv.ParseFloat(args[i+1], 64)
			if parseErr != nil || math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold <= 0 {
				return "", "", options, parseErrorUsage
			}
			options.Threshold = threshold
			i += 2
		case "--held-out":
			options.HeldOut = true
			i++
		case "--source-root":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", "", options, parseErrorUsage
			}
			options.SourceRoot = args[i+1]
			i += 2
		case "--source":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", "", options, parseErrorUsage
			}
			options.SourcePath = args[i+1]
			i += 2
		default:
			return "", "", options, parseErrorUsage
		}
	}
	if outDir == "" {
		return "", "", options, parseErrorUsage
	}
	if (strings.TrimSpace(options.SourceRoot) == "") != (strings.TrimSpace(options.SourcePath) == "") {
		return "", "", options, parseErrorUsage
	}
	return inputPath, outDir, options, parseErrorNone
}

func parseDocumentsCalibrateNextArgs(args []string) (inputPath string, err parseError) {
	if len(args) != 2 || args[0] != "calibrate-next" || strings.TrimSpace(args[1]) == "" {
		return "", parseErrorUsage
	}
	return args[1], parseErrorNone
}

func parseDocumentsJudgeArgs(args []string) (inputPath string, outDir string, options documents.SemanticJudgmentOptions, err parseError) {
	if len(args) < 4 || args[0] != "judge" || strings.TrimSpace(args[1]) == "" {
		return "", "", options, parseErrorUsage
	}
	inputPath = args[1]
	for i := 2; i < len(args); {
		switch args[i] {
		case "--out":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", "", options, parseErrorUsage
			}
			outDir = args[i+1]
			i += 2
		case "--source-root":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", "", options, parseErrorUsage
			}
			options.SourceRoot = args[i+1]
			i += 2
		case "--source":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", "", options, parseErrorUsage
			}
			options.SourcePath = args[i+1]
			i += 2
		default:
			return "", "", options, parseErrorUsage
		}
	}
	if outDir == "" {
		return "", "", options, parseErrorUsage
	}
	if (strings.TrimSpace(options.SourceRoot) == "") != (strings.TrimSpace(options.SourcePath) == "") {
		return "", "", options, parseErrorUsage
	}
	return inputPath, outDir, options, parseErrorNone
}

func parseDocumentsJudgeNextArgs(args []string) (inputPath string, err parseError) {
	if len(args) != 2 || args[0] != "judge-next" || strings.TrimSpace(args[1]) == "" {
		return "", parseErrorUsage
	}
	return args[1], parseErrorNone
}

func parseDocumentsJudgeRecordArgs(args []string) (inputPath string, record documents.SemanticJudgmentRecordInput, err parseError) {
	if len(args) < 6 || args[0] != "judge-record" || strings.TrimSpace(args[1]) == "" {
		return "", record, parseErrorUsage
	}
	inputPath = args[1]
	for i := 2; i < len(args); {
		if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
			return "", record, parseErrorUsage
		}
		switch args[i] {
		case "--candidate":
			record.CandidateID = args[i+1]
		case "--choice":
			record.Choice = documents.SemanticJudgmentChoice(args[i+1])
		case "--note":
			record.Note = args[i+1]
		case "--reviewer":
			record.ReviewerID = args[i+1]
		default:
			return "", record, parseErrorUsage
		}
		i += 2
	}
	if strings.TrimSpace(record.CandidateID) == "" || strings.TrimSpace(string(record.Choice)) == "" {
		return "", record, parseErrorUsage
	}
	return inputPath, record, parseErrorNone
}

func (r Runner) validateOutDir(outDir string) error {
	info, err := r.fs.Stat(outDir)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", outDir)
		}
		return r.fs.CanWriteDir(outDir)
	}
	if err := r.fs.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	info, err = r.fs.Stat(outDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", outDir)
	}
	return r.fs.CanWriteDir(outDir)
}

func (r Runner) validateDestinationOutDir(outDir string) error {
	realOut, err := r.fs.RealPath(outDir)
	if err != nil {
		return err
	}
	if err := r.rejectProtectedPath(realOut, outDir); err != nil {
		return err
	}
	if err := r.validateOutDir(outDir); err != nil {
		return err
	}
	realOut, err = r.fs.RealPath(outDir)
	if err != nil {
		return err
	}
	return r.rejectProtectedPath(realOut, outDir)
}

func (r Runner) rejectProtectedPath(realPath, displayPath string) error {
	for _, protectedRoot := range r.protectedRoots {
		if strings.TrimSpace(protectedRoot) == "" {
			continue
		}
		realRoot, err := r.fs.RealPath(protectedRoot)
		if err != nil {
			continue
		}
		if isSameOrInside(realRoot, realPath) {
			return fmt.Errorf("protected output root: %s", displayPath)
		}
	}
	return nil
}

func (r Runner) writeArtifact(outDir string, recordID string, artifact sbos.Artifact) (string, error) {
	target := filepath.Join(outDir, artifactFilename(recordID, artifact.Kind))
	if !isInside(outDir, target) {
		return "", fmt.Errorf("artifact path escaped output directory")
	}
	if err := r.fs.WriteFile(target, []byte(artifact.Body)); err != nil {
		return "", err
	}
	return displayPath(r.fs, target), nil
}

func (r Runner) writeCandidate(outDir string, candidate sbos.Candidate) (string, error) {
	target := filepath.Join(outDir, sanitizeFilenameBase(candidate.CandidateID)+".json")
	if !isInside(outDir, target) {
		return "", fmt.Errorf("candidate path escaped output directory")
	}
	data, err := json.MarshalIndent(candidate, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := r.fs.WriteFile(target, data); err != nil {
		return "", err
	}
	return displayPath(r.fs, target), nil
}

func (r Runner) writeCheckpoint(outDir string, checkpoint slackadapter.Checkpoint) error {
	target := filepath.Join(outDir, "slack-checkpoint.json")
	if !isInside(outDir, target) {
		return fmt.Errorf("checkpoint path escaped output directory")
	}
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return r.fs.WriteFile(target, data)
}

func (r Runner) writeDestinationOperation(outDir string, operation destinations.Operation) (string, error) {
	if err := r.ensureOutputChildDir(outDir, "operations"); err != nil {
		return "", err
	}
	target := filepath.Join(outDir, "operations", sanitizeFilenameBase(operation.OperationID)+".json")
	if !isInside(outDir, target) {
		return "", fmt.Errorf("operation path escaped output directory")
	}
	if err := r.rejectUnsafeOutputFile(outDir, target); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(operation, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := r.fs.WriteFile(target, data); err != nil {
		return "", err
	}
	return displayPath(r.fs, target), nil
}

func (r Runner) writeDestinationPreview(outDir string, operation destinations.Operation) (string, error) {
	if err := r.ensureOutputChildDir(outDir, "previews"); err != nil {
		return "", err
	}
	target := filepath.Join(outDir, "previews", sanitizeFilenameBase(operation.OperationID)+".md")
	if !isInside(outDir, target) {
		return "", fmt.Errorf("preview path escaped output directory")
	}
	if err := r.rejectUnsafeOutputFile(outDir, target); err != nil {
		return "", err
	}
	if err := r.fs.WriteFile(target, []byte(operation.Body)); err != nil {
		return "", err
	}
	return displayPath(r.fs, target), nil
}

func (r Runner) writeDestinationSummary(outDir string, summary DestinationDryRunSummary) error {
	target := filepath.Join(outDir, "destination-summary.json")
	if !isInside(outDir, target) {
		return fmt.Errorf("summary path escaped output directory")
	}
	if err := r.rejectUnsafeOutputFile(outDir, target); err != nil {
		return err
	}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return r.fs.WriteFile(target, data)
}

func (r Runner) ensureOutputChildDir(outDir, child string) error {
	targetDir := filepath.Join(outDir, child)
	if !isInside(outDir, targetDir) {
		return fmt.Errorf("%s path escaped output directory", child)
	}
	if err := r.fs.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	realOut, err := r.fs.RealPath(outDir)
	if err != nil {
		return err
	}
	realTarget, err := r.fs.RealPath(targetDir)
	if err != nil {
		return err
	}
	if !isSameOrInside(realOut, realTarget) || realOut == realTarget {
		return fmt.Errorf("%s path escaped output directory", child)
	}
	for _, protectedRoot := range r.protectedRoots {
		if strings.TrimSpace(protectedRoot) == "" {
			continue
		}
		realRoot, err := r.fs.RealPath(protectedRoot)
		if err != nil {
			continue
		}
		if isSameOrInside(realRoot, realTarget) {
			return fmt.Errorf("protected output root: %s", targetDir)
		}
	}
	return nil
}

func (r Runner) rejectUnsafeOutputFile(outDir, target string) error {
	isSymlink, err := r.fs.IsSymlink(target)
	if err != nil {
		return err
	}
	if isSymlink {
		return fmt.Errorf("file path escaped output directory")
	}
	realOut, err := r.fs.RealPath(outDir)
	if err != nil {
		return err
	}
	realTarget, err := r.fs.RealPath(target)
	if err != nil {
		return err
	}
	if !isSameOrInside(realOut, realTarget) || realOut == realTarget {
		return fmt.Errorf("file path escaped output directory")
	}
	return r.rejectProtectedPath(realTarget, target)
}

func destinationOperationHasPreview(operation destinations.Operation) bool {
	return operation.Body != "" && (operation.OperationType == destinations.OperationCreateNote || operation.OperationType == destinations.OperationAttentionPreview)
}

func artifactFilename(recordID string, kind sbos.ArtifactKind) string {
	base := sanitizeFilenameBase(recordID)
	switch kind {
	case sbos.ArtifactAttentionPreview:
		return base + "-attention.md"
	case sbos.ArtifactDryRunPublish:
		return base + "-publish.md"
	default:
		return base + "-artifact.md"
	}
}

func sanitizeFilenameBase(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	cleaned := strings.Trim(b.String(), "._-")
	for strings.Contains(cleaned, "..") {
		cleaned = strings.ReplaceAll(cleaned, "..", ".")
	}
	cleaned = strings.Trim(cleaned, "._-")
	if cleaned == "" {
		return "candidate"
	}
	return cleaned
}

func isInside(outDir, target string) bool {
	cleanOut := filepath.Clean(outDir)
	cleanTarget := filepath.Clean(target)
	rel, err := filepath.Rel(cleanOut, cleanTarget)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel)
}

func isSameOrInside(root, target string) bool {
	cleanRoot := filepath.Clean(root)
	cleanTarget := filepath.Clean(target)
	rel, err := filepath.Rel(cleanRoot, cleanTarget)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel))
}

func displayPath(fileSystem FileSystem, target string) string {
	cwd, err := fileSystem.Getwd()
	if err != nil {
		return filepath.Clean(target)
	}
	rel, err := filepath.Rel(cwd, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.Clean(target)
	}
	return filepath.Clean(rel)
}

func authorityIDs() []string {
	ids := make([]string, len(cliAuthorityIDs))
	copy(ids, cliAuthorityIDs)
	return ids
}
