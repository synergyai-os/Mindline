package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	slackadapter "github.com/synergyai-os/Mindline/internal/adapters/slack"
	"github.com/synergyai-os/Mindline/internal/destinations"
	tolariadestination "github.com/synergyai-os/Mindline/internal/destinations/tolaria"
	"github.com/synergyai-os/Mindline/internal/pipeline/artifacts"
	"github.com/synergyai-os/Mindline/internal/pipeline/methods"
	"github.com/synergyai-os/Mindline/internal/pipeline/processors"
	"github.com/synergyai-os/Mindline/internal/sbos"
)

type RunOptions struct {
	ProtectedRoots       []string
	DestinationAvailable func(adapterID string) bool
}

type Summary = artifacts.Output

func Run(inputPath, outDir string, opts RunOptions) (Summary, error) {
	runner := Runner{DestinationAvailable: opts.DestinationAvailable, ProtectedRoots: opts.ProtectedRoots}
	return runner.Run(inputPath, outDir)
}

type Runner struct {
	DestinationAvailable func(adapterID string) bool
	ProtectedRoots       []string
}

func (r Runner) Run(inputPath, outDir string) (Summary, error) {
	input, err := ParseInputFile(inputPath, ParseOptions{})
	if err != nil {
		return Summary{}, err
	}
	if available := r.DestinationAvailable; available != nil && !available(input.Destination.ID) {
		return Summary{}, fmt.Errorf("WP-5 destination dry-run support is required before pipeline delivery")
	}
	if r.DestinationAvailable == nil && input.Destination.ID != DestinationTolaria {
		return Summary{}, fmt.Errorf("WP-5 destination dry-run support is required before pipeline delivery")
	}
	profile, err := methods.Load(input.Method.ID)
	if err != nil {
		return Summary{}, err
	}
	candidates, err := loadCandidates(input)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{
		SchemaVersion: "pipeline-summary/v0.1",
		RunMode:       input.RunMode,
		MethodID:      profile.MethodID,
		DestinationID: input.Destination.ID,
		AuthorityIDs:  append([]string(nil), input.AuthorityIDs...),
	}
	engine := sbos.NewEngine()
	for _, candidate := range candidates {
		item, blocked, err := runCandidate(engine, candidate, profile, input.AuthorityIDs)
		if err != nil {
			return Summary{}, err
		}
		if blocked {
			summary.BlockedCount++
		}
		summary.Items = append(summary.Items, item)
	}
	summary.ItemCount = len(summary.Items)
	prepareSummaryPaths(&summary)
	if err := artifacts.Write(outDir, summary, r.ProtectedRoots); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

func prepareSummaryPaths(summary *Summary) {
	summary.Paths.Summary = "pipeline-summary.json"
	for i := range summary.Items {
		slug := sanitize(summary.Items[i].CandidateID)
		summary.Items[i].ResultPath = filepath.ToSlash(filepath.Join("results", slug+".json"))
		summary.Items[i].ProcessorPlanPath = filepath.ToSlash(filepath.Join("processors", slug+".json"))
		summary.Items[i].DestinationPath = filepath.ToSlash(filepath.Join("destinations", slug, "destination-summary.json"))
	}
}

func loadCandidates(input Input) ([]sbos.Candidate, error) {
	switch input.Source.Kind {
	case SourceCandidate:
		path, err := input.ResolveBundlePath(input.Source.Path)
		if err != nil {
			return nil, err
		}
		candidate, err := readCandidate(path)
		if err != nil {
			return nil, err
		}
		return []sbos.Candidate{candidate}, nil
	case SourceCandidateBatch:
		var candidates []sbos.Candidate
		for _, sourcePath := range input.Source.Paths {
			path, err := input.ResolveBundlePath(sourcePath)
			if err != nil {
				return nil, err
			}
			candidate, err := readCandidate(path)
			if err != nil {
				return nil, err
			}
			candidates = append(candidates, candidate)
		}
		return candidates, nil
	case SourceSlackExport:
		path, err := input.ResolveBundlePath(input.Source.Path)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var payload slackadapter.Payload
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, err
		}
		result, err := slackadapter.Normalize(payload)
		if err != nil {
			return nil, err
		}
		return result.Candidates, nil
	default:
		return nil, fmt.Errorf("unsupported source kind: %s", input.Source.Kind)
	}
}

func readCandidate(path string) (sbos.Candidate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return sbos.Candidate{}, err
	}
	var candidate sbos.Candidate
	if err := json.Unmarshal(data, &candidate); err != nil {
		return sbos.Candidate{}, err
	}
	if err := sbos.ValidateCandidate(candidate); err != nil {
		return sbos.Candidate{}, err
	}
	return candidate, nil
}

func runCandidate(engine *sbos.Engine, candidate sbos.Candidate, profile methods.Profile, authorityIDs []string) (artifacts.Item, bool, error) {
	candidateBytes, err := json.Marshal(candidate)
	if err != nil {
		return artifacts.Item{}, false, err
	}
	result, err := engine.ProcessCandidate(candidateBytes)
	if err != nil {
		return artifacts.Item{}, false, err
	}
	processorPlan := processors.Plan(processors.Input{
		Text:              candidate.Content.Text,
		URLs:              processorURLs(candidate.Content.URLs),
		PrivateProvenance: result.Safety.PrivateProvenance || result.Safety.RedactionRequired,
		SecretLike:        result.Safety.SecretLike,
	}, profile, authorityIDs)
	blocked := len(processorPlan.Blockers) > 0
	resultEnvelope := pipelineResult(candidate, result, profile, blocked)
	destinationSummary, operationFiles, previewFiles, err := destinationArtifacts(candidate, resultEnvelope, result, authorityIDs, blocked, processorPlan.Blockers)
	if err != nil {
		return artifacts.Item{}, false, err
	}
	return artifacts.Item{
		CandidateID:        candidate.CandidateID,
		State:              string(result.State),
		Result:             resultEnvelope,
		ProcessorPlan:      processorPlan,
		DestinationSummary: destinationSummary,
		OperationFiles:     operationFiles,
		PreviewFiles:       previewFiles,
	}, blocked, nil
}

func pipelineResult(candidate sbos.Candidate, result sbos.ProcessResult, profile methods.Profile, blocked bool) map[string]any {
	artifactsOut := []map[string]string{}
	for _, artifact := range result.Artifacts {
		body := artifact.Body
		if artifact.Kind == sbos.ArtifactDryRunPublish && !blocked {
			body = renderMethodMarkdown(candidate, profile)
		}
		artifactsOut = append(artifactsOut, map[string]string{"kind": string(artifact.Kind), "body": body})
	}
	return map[string]any{
		"schema_version":       "pipeline-result/v0.1",
		"state":                string(result.State),
		"record_id":            result.RecordID,
		"source_candidate_id":  result.SourceCandidateID,
		"idempotency_key":      safeDiagnostic(result.IdempotencyKey, result.Safety),
		"artifacts":            artifactsOut,
		"private_provenance":   result.Safety.PrivateProvenance,
		"redaction_required":   result.Safety.RedactionRequired,
		"secret_like":          result.Safety.SecretLike,
		"method_profile_id":    profile.MethodID,
		"destination_adapter":  DestinationTolaria,
		"processor_blocked":    blocked,
		"source_adapter":       candidate.AdapterID,
		"source_content_units": len(candidate.Content.URLs),
	}
}

func destinationArtifacts(candidate sbos.Candidate, resultEnvelope map[string]any, result sbos.ProcessResult, authorityIDs []string, blocked bool, blockers []string) (map[string]any, []artifacts.OperationFile, []artifacts.PreviewFile, error) {
	slug := sanitize(candidate.CandidateID)
	if blocked {
		operation := destinations.Operation{
			SchemaVersion:        destinations.OperationSchemaVersion,
			OperationID:          destinations.GenerateOperationID(DestinationTolaria, candidate.CandidateID, destinations.OperationBlocked),
			DestinationAdapterID: DestinationTolaria,
			SourceCandidateID:    safeDiagnostic(candidate.CandidateID, result.Safety),
			SourceRecordID:       safeDiagnostic(result.RecordID, result.Safety),
			IdempotencyKey:       safeDiagnostic(result.IdempotencyKey, result.Safety),
			OperationType:        destinations.OperationBlocked,
			WriteMode:            destinations.WriteModeDryRun,
			VisibilityLane:       destinations.VisibilityBlocked,
			Title:                "Pipeline blocked",
			Metadata:             map[string]any{"state": string(result.State)},
			Blockers:             blockers,
			AuthorityIDs:         append([]string(nil), authorityIDs...),
		}
		operationPath := filepath.ToSlash(filepath.Join("destinations", slug, "operations", operation.OperationID+".json"))
		return destinationSummary([]destinations.Operation{operation}, operationPath, ""), []artifacts.OperationFile{{Path: operationPath, Body: operation}}, nil, nil
	}
	destinationInput := destinations.InputResult{
		State:             string(result.State),
		RecordID:          result.RecordID,
		SourceCandidateID: result.SourceCandidateID,
		IdempotencyKey:    result.IdempotencyKey,
		AuthorityIDs:      append([]string(nil), authorityIDs...),
		Safety: destinations.InputSafety{
			PrivateProvenance: result.Safety.PrivateProvenance,
			RedactionRequired: result.Safety.RedactionRequired,
			SecretLike:        result.Safety.SecretLike,
		},
	}
	for _, raw := range resultEnvelope["artifacts"].([]map[string]string) {
		destinationInput.Artifacts = append(destinationInput.Artifacts, destinations.InputArtifact{Kind: raw["kind"], Body: raw["body"]})
	}
	operations, err := tolariadestination.Plan(destinationInput)
	if err != nil {
		return nil, nil, nil, err
	}
	var operationFiles []artifacts.OperationFile
	var previewFiles []artifacts.PreviewFile
	for _, operation := range operations {
		operationPath := filepath.ToSlash(filepath.Join("destinations", slug, "operations", operation.OperationID+".json"))
		operationFiles = append(operationFiles, artifacts.OperationFile{Path: operationPath, Body: operation})
		if operation.Body != "" && operation.OperationType == destinations.OperationCreateNote {
			previewPath := filepath.ToSlash(filepath.Join("destinations", slug, "previews", operation.OperationID+".md"))
			previewFiles = append(previewFiles, artifacts.PreviewFile{Path: previewPath, Body: operation.Body})
		}
	}
	previewPath := ""
	if len(previewFiles) > 0 {
		previewPath = previewFiles[0].Path
	}
	operationPath := ""
	if len(operationFiles) > 0 {
		operationPath = operationFiles[0].Path
	}
	return destinationSummary(operations, operationPath, previewPath), operationFiles, previewFiles, nil
}

func destinationSummary(operations []destinations.Operation, operationPath, previewPath string) map[string]any {
	items := make([]map[string]any, 0, len(operations))
	blocked := 0
	for _, operation := range operations {
		isBlocked := operation.OperationType == destinations.OperationBlocked || operation.OperationType == destinations.OperationSkip
		if isBlocked {
			blocked++
		}
		items = append(items, map[string]any{
			"operation_id":        operation.OperationID,
			"operation_type":      string(operation.OperationType),
			"visibility_lane":     string(operation.VisibilityLane),
			"operation_json_path": operationPath,
			"preview_path":        previewPath,
			"blocked":             isBlocked,
		})
	}
	return map[string]any{
		"destination_adapter_id": DestinationTolaria,
		"write_mode":             RunModeDryRun,
		"operation_count":        len(operations),
		"blocked_count":          blocked,
		"operations":             items,
	}
}

func processorURLs(urls []string) []processors.URLUnit {
	out := make([]processors.URLUnit, 0, len(urls))
	for _, value := range urls {
		out = append(out, processors.URLUnit{URL: value, Kind: processors.DetectURLKind(value)})
	}
	return out
}

func renderMethodMarkdown(candidate sbos.Candidate, profile methods.Profile) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("type: " + candidate.Classification.Type + "\n")
	b.WriteString("status: dry_run\n")
	b.WriteString("domain: " + candidate.Classification.Domain + "\n")
	b.WriteString("method: " + profile.MethodID + "\n")
	b.WriteString("para: " + profile.Organize.DefaultModel + "\n")
	b.WriteString("topics:\n")
	for _, topic := range candidate.Classification.Topics {
		b.WriteString("  - " + topic + "\n")
	}
	b.WriteString("---\n\n")
	b.WriteString("# " + titleOrFallback(candidate) + "\n\n")
	b.WriteString("## Snapshot\n\n")
	b.WriteString(candidate.Content.Text + "\n\n")
	b.WriteString("## Source Content\n\n")
	b.WriteString("- Captured from: " + candidate.AdapterID + "\n")
	if candidate.Provenance.Permalink.Visibility == "public" {
		b.WriteString("- Source: " + candidate.Provenance.Permalink.Value + "\n")
	}
	b.WriteString("\n## Key Details\n\n")
	b.WriteString("- " + candidate.Content.Text + "\n\n")
	b.WriteString("## Relevance\n\n")
	b.WriteString("Classified under " + candidate.Classification.Domain + " with " + candidate.Classification.Confidence + " confidence.\n\n")
	b.WriteString("## Signals\n\n")
	for _, topic := range candidate.Classification.Topics {
		b.WriteString("- " + topic + "\n")
	}
	b.WriteString("\n## Related Sources\n\n")
	for _, url := range candidate.Content.URLs {
		b.WriteString("- " + url + "\n")
	}
	b.WriteString("\n## Next Action\n\n")
	b.WriteString("No immediate action. Keep as processed source reference.\n")
	return b.String()
}

func titleOrFallback(candidate sbos.Candidate) string {
	if strings.TrimSpace(candidate.Content.SourceTitle) != "" {
		return candidate.Content.SourceTitle
	}
	return candidate.CandidateID
}

func safeDiagnostic(value string, safety sbos.Safety) string {
	if safety.PrivateProvenance || safety.RedactionRequired || safety.SecretLike {
		return "fingerprint:" + destinations.StableFingerprint(value)
	}
	return value
}

func sanitize(value string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	cleaned := strings.Trim(b.String(), "-")
	if cleaned == "" {
		return "source"
	}
	return cleaned
}
