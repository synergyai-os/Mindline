package evalreadback

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/synergyai-os/Mindline/internal/documents"
	"github.com/synergyai-os/Mindline/internal/privateio"
	"github.com/synergyai-os/Mindline/internal/productbrain"
)

func Build(inputRoot, outRoot string, options Options) (Summary, error) {
	summary, err := BuildSummary(inputRoot, options)
	if err != nil {
		return Summary{}, err
	}
	if err := Write(outRoot, summary, options.ProtectedRoots); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

func BuildSummary(inputRoot string, options Options) (Summary, error) {
	model, err := buildModel(inputRoot)
	if err != nil {
		return Summary{}, err
	}
	summary := summarize(model)
	if strings.TrimSpace(options.BaselineRoot) != "" {
		baseline, err := buildBaselineModel(options.BaselineRoot)
		if err != nil {
			return Summary{}, fmt.Errorf("read baseline: %w", err)
		}
		comparison := compareModels(baseline, model)
		summary.BaselineRootLabel = baseline.rootLabel
		summary.BaselineArtifactRefs = prefixedArtifactRefs("baseline", artifactRefs(baseline.artifacts))
		summary.BaselineArtifacts = baseline.artifacts
		summary.Comparison = &comparison
		summary.ImprovementStatus = comparison.Status
		rebuildClaimGates(&summary)
	}
	return summary, nil
}

func ApplyBaseline(summary Summary, baselineRoot string) (Summary, error) {
	summary = RefreshSummary(summary)
	if strings.TrimSpace(baselineRoot) == "" {
		return summary, nil
	}
	baseline, err := buildBaselineModel(baselineRoot)
	if err != nil {
		return Summary{}, fmt.Errorf("read baseline: %w", err)
	}
	current := modelFromSummary(summary)
	comparison := compareModels(baseline, current)
	summary.BaselineRootLabel = baseline.rootLabel
	summary.BaselineArtifactRefs = prefixedArtifactRefs("baseline", artifactRefs(baseline.artifacts))
	summary.BaselineArtifacts = baseline.artifacts
	summary.Comparison = &comparison
	summary.ImprovementStatus = comparison.Status
	rebuildClaimGates(&summary)
	return summary, nil
}

func ApplyBaselineSummary(summary Summary, baselineSummary Summary) Summary {
	summary = RefreshSummary(summary)
	baselineSummary = RefreshSummary(baselineSummary)
	current := modelFromSummary(summary)
	baseline := modelFromSummary(baselineSummary)
	comparison := compareModels(baseline, current)
	summary.BaselineRootLabel = baseline.rootLabel
	summary.BaselineArtifactRefs = prefixedArtifactRefs("baseline", artifactRefs(baseline.artifacts))
	summary.BaselineArtifacts = baseline.artifacts
	summary.Comparison = &comparison
	summary.ImprovementStatus = comparison.Status
	rebuildClaimGates(&summary)
	return summary
}

func RefreshSummary(summary Summary) Summary {
	current := modelFromSummary(summary)
	refreshed := summarize(current)
	if len(summary.BaselineArtifacts) == 0 {
		if summary.Comparison != nil {
			refreshed.BaselineRootLabel = summary.BaselineRootLabel
			refreshed.BaselineArtifactRefs = append([]string{}, summary.BaselineArtifactRefs...)
			refreshed.BaselineArtifacts = append([]ArtifactEvidence{}, summary.BaselineArtifacts...)
			comparison := *summary.Comparison
			refreshed.Comparison = &comparison
			refreshed.ImprovementStatus = summary.ImprovementStatus
			rebuildClaimGates(&refreshed)
		}
		return refreshed
	}
	baseline := modelFromArtifacts(summary.BaselineRootLabel, summary.SampleStatus, summary.BaselineArtifacts)
	comparison := compareModels(baseline, current)
	refreshed.BaselineRootLabel = summary.BaselineRootLabel
	refreshed.BaselineArtifactRefs = prefixedArtifactRefs("baseline", artifactRefs(baseline.artifacts))
	refreshed.BaselineArtifacts = baseline.artifacts
	refreshed.Comparison = &comparison
	refreshed.ImprovementStatus = comparison.Status
	rebuildClaimGates(&refreshed)
	return refreshed
}

func modelFromSummary(summary Summary) readbackModel {
	model := modelFromArtifacts(summary.InputRootLabel, summary.SampleStatus, summary.Artifacts)
	if summary.ReplayBaseline.Status == "blocked" {
		model.flags["replay_baseline_blocked"] = true
		model.replayBaselineReasonCodes = append([]string{}, summary.ReplayBaseline.ReasonCodes...)
	}
	return model
}

func modelFromArtifacts(rootLabel, sampleStatus string, artifacts []ArtifactEvidence) readbackModel {
	model := readbackModel{
		rootLabel:     rootLabel,
		sampleStatus:  sampleStatus,
		metrics:       map[string]float64{},
		flags:         map[string]bool{},
		fingerprints:  map[string]string{},
		artifactTypes: map[string]bool{},
		artifacts:     append([]ArtifactEvidence{}, artifacts...),
	}
	for _, artifact := range model.artifacts {
		mergeModelEvidence(&model, artifact)
	}
	return model
}

func Write(outRoot string, summary Summary, protectedRoots []string) error {
	return writeSummary(outRoot, summary, protectedRoots)
}

func LoadSummary(path string) (Summary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Summary{}, err
	}
	if containsDeniedString(string(data)) {
		return Summary{}, errors.New("readback summary contains unsafe private or secret pattern")
	}
	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		return Summary{}, err
	}
	if summary.SchemaVersion != SummarySchemaVersion {
		return Summary{}, fmt.Errorf("unsupported readback summary schema: %s", summary.SchemaVersion)
	}
	return summary, nil
}

func ValidateOutputPath(root, candidate string, protectedRoots []string) error {
	return rejectSymlinkEscape(root, candidate, protectedRoots)
}

type readbackModel struct {
	root                      string
	rootLabel                 string
	sampleStatus              string
	artifacts                 []ArtifactEvidence
	guardrails                Guardrails
	metrics                   map[string]float64
	flags                     map[string]bool
	fingerprints              map[string]string
	artifactTypes             map[string]bool
	replayBaselineReasonCodes []string
	segmentSummaryCount       int
	segmentSummarySegments    int
}

func buildBaselineModel(inputRoot string) (readbackModel, error) {
	summary, _, err := LoadSummaryFromRoot(inputRoot)
	if err == nil {
		summary = RefreshSummary(summary)
		return modelFromSummary(summary), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return readbackModel{}, err
	}
	return buildModel(inputRoot)
}

func LoadSummaryFromRoot(input string) (Summary, string, error) {
	info, err := os.Stat(input)
	if err != nil {
		return Summary{}, "", err
	}
	candidates := []string{}
	if !info.IsDir() {
		if filepath.Base(input) == ReadbackSummaryFile {
			summary, err := LoadSummary(input)
			return summary, filepath.ToSlash(filepath.Base(input)), err
		}
		return Summary{}, "", os.ErrNotExist
	}
	for _, rel := range []string{
		ReadbackSummaryFile,
		filepath.Join(DirName, ReadbackSummaryFile),
		filepath.Join("readback", DirName, ReadbackSummaryFile),
		filepath.Join("eval-proof", "readback", DirName, ReadbackSummaryFile),
		filepath.Join("eval-loop-decision", "readback", DirName, ReadbackSummaryFile),
		filepath.Join("eval-loop-decision", DirName, "readback", DirName, ReadbackSummaryFile),
	} {
		candidates = append(candidates, filepath.Join(input, rel))
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			summary, err := LoadSummary(candidate)
			if err != nil {
				return Summary{}, "", err
			}
			rel, err := filepath.Rel(input, candidate)
			if err != nil {
				return summary, filepath.ToSlash(candidate), nil
			}
			return summary, filepath.ToSlash(rel), nil
		}
	}
	return Summary{}, "", os.ErrNotExist
}

func buildModel(inputRoot string) (readbackModel, error) {
	if strings.TrimSpace(inputRoot) == "" {
		return readbackModel{}, errors.New("missing input root")
	}
	root, err := filepath.Abs(inputRoot)
	if err != nil {
		return readbackModel{}, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return readbackModel{}, err
	}
	if !info.IsDir() {
		return readbackModel{}, fmt.Errorf("input root must be a directory: %s", inputRoot)
	}
	model := readbackModel{
		root:          root,
		rootLabel:     safeRootLabel(root),
		sampleStatus:  sampleStatusFor(root),
		metrics:       map[string]float64{},
		flags:         map[string]bool{},
		fingerprints:  map[string]string{},
		artifactTypes: map[string]bool{},
	}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if filepath.Ext(path) != ".json" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		ref := filepath.ToSlash(rel)
		artifactType := artifactTypeFor(root, ref)
		if artifactType == "" {
			return nil
		}
		artifact, err := readArtifact(root, path, ref, artifactType)
		if err != nil {
			return err
		}
		model.artifacts = append(model.artifacts, artifact)
		mergeModelEvidence(&model, artifact)
		return nil
	})
	if err != nil {
		return readbackModel{}, err
	}
	sort.Slice(model.artifacts, func(i, j int) bool { return model.artifacts[i].Ref < model.artifacts[j].Ref })
	if len(model.artifacts) == 0 {
		return readbackModel{}, errors.New("no supported eval/trace artifacts found")
	}
	return model, nil
}

func artifactTypeFor(root, ref string) string {
	if artifactType := artifactTypeForRef(ref); artifactType != "" {
		return artifactType
	}
	prefix := artifactRootPrefix(root)
	if prefix == "" {
		return ""
	}
	return artifactTypeForRef(filepath.ToSlash(filepath.Join(prefix, ref)))
}

func artifactRootPrefix(root string) string {
	base := filepath.Base(root)
	switch base {
	case "trace", "corpus-pressure", "corpus-graph", "corpus-pressure-loop", "corpus-acceptance", "autonomy-readiness", "link-enrichment", "value-proof", "source-meaning-packet", "corpus-concepts":
		return base
	case "comparison", "requests", "posthog":
		if filepath.Base(filepath.Dir(root)) == "link-enrichment" {
			return filepath.ToSlash(filepath.Join("link-enrichment", base))
		}
	}
	return ""
}

func artifactTypeForRef(ref string) string {
	switch {
	case strings.HasSuffix(ref, "trace/trace-summary.json"):
		return "generic_trace_summary"
	case strings.HasSuffix(ref, "corpus-pressure/pressure-summary.json"):
		return "corpus_pressure_summary"
	case strings.HasSuffix(ref, "corpus-pressure/eval-input.json"):
		return "corpus_pressure_eval_input"
	case strings.HasSuffix(ref, "corpus-pressure/trace-summary.json"):
		return "corpus_pressure_trace_summary"
	case strings.HasSuffix(ref, "corpus-graph/graph-summary.json"):
		return "corpus_graph_summary"
	case strings.HasSuffix(ref, "document-segments/segment-summary.json"):
		return "document_segment_summary"
	case strings.HasSuffix(ref, "semantic-candidates/semantic-summary.json"):
		return "semantic_candidate_summary"
	case strings.HasSuffix(ref, "corpus-pressure-loop/loop-summary.json"):
		return "corpus_pressure_loop_summary"
	case strings.HasSuffix(ref, "corpus-acceptance/benchmark-summary.json"):
		return "corpus_acceptance_benchmark"
	case strings.HasSuffix(ref, "autonomy-readiness/readiness-report.json"):
		return "autonomy_readiness_report"
	case strings.HasSuffix(ref, "link-enrichment/loop-summary.json"):
		return "link_enrichment_loop_summary"
	case strings.HasSuffix(ref, "link-enrichment/comparison/comparison-summary.json"):
		return "link_enrichment_comparison_summary"
	case strings.HasSuffix(ref, "link-enrichment/requests/link-artifact-requests.json"):
		return "link_artifact_requests"
	case strings.HasSuffix(ref, "link-enrichment/posthog/eval-projection.json"):
		return "link_enrichment_eval_projection"
	case strings.HasSuffix(ref, "value-proof/value-summary.json"):
		return "value_proof_summary"
	case strings.HasSuffix(ref, "source-meaning-packet/meaning-summary.json"):
		return "source_meaning_packet_summary"
	case strings.HasSuffix(ref, "corpus-concepts/concept-summary.json"):
		return "corpus_concept_summary"
	case strings.HasSuffix(ref, "corpus-concepts/review-records.json"):
		return "corpus_concept_review_records"
	case strings.HasSuffix(ref, "route-summary.json"):
		return "strategic_routing_summary"
	case strings.HasSuffix(ref, "outbox-summary.json"):
		return "productbrain_outbox_summary"
	case strings.HasSuffix(ref, "outbox.json"):
		return "productbrain_outbox"
	case strings.Contains(ref, "preflight-snapshots/") && strings.HasSuffix(ref, ".json"):
		return "productbrain_preflight_snapshot"
	case strings.HasSuffix(ref, "preflight.json"):
		return "productbrain_preflight"
	case strings.HasSuffix(ref, "delivery-history.json"):
		return "productbrain_delivery_history"
	case strings.HasSuffix(ref, "delivery-summary.json"):
		return "productbrain_delivery_summary"
	default:
		return ""
	}
}

func readArtifact(root, path, ref, artifactType string) (ArtifactEvidence, error) {
	if containsDeniedRefString(ref) {
		return ArtifactEvidence{
			Type: artifactType, Ref: sanitizedArtifactRef(ref), Status: "unsafe_or_leaky", ReasonCodes: []string{"unsafe_artifact_ref"},
		}, nil
	}
	if proofCriticalPrivateAuthority(artifactType) {
		if err := privateio.ValidateContained(root, path); err != nil {
			return ArtifactEvidence{
				Type: artifactType, Ref: ref, Status: "invalid_binding", ReasonCodes: []string{"unsafe_authority_permissions"},
			}, nil
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ArtifactEvidence{}, err
	}
	if containsDeniedString(string(data)) {
		return ArtifactEvidence{
			Type: artifactType, Ref: ref, Status: "unsafe_or_leaky", ReasonCodes: []string{"raw_private_or_secret_pattern"},
		}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return ArtifactEvidence{}, fmt.Errorf("decode %s: %w", ref, err)
	}
	artifact := ArtifactEvidence{
		Type:          artifactType,
		SchemaVersion: stringValue(raw["schema_version"]),
		Ref:           ref,
		Status:        "detected",
		Metrics:       map[string]float64{},
		Flags:         map[string]bool{},
		Fingerprints:  map[string]string{},
	}
	if !supportedSchema(artifact.Type, artifact.SchemaVersion) {
		artifact.Status = "unsupported_schema"
		artifact.ReasonCodes = []string{"unsupported_schema_version"}
		artifact.Metrics = nil
		artifact.Flags = nil
		artifact.Fingerprints = nil
		return artifact, nil
	}
	if artifactType == "corpus_concept_review_records" {
		if _, err := documents.ReadCorpusConceptReviewRecords(filepath.Dir(path)); err != nil {
			artifact.Status = "invalid_binding"
			artifact.ReasonCodes = []string{"stale_or_invalid_review_contract"}
			artifact.Metrics = nil
			artifact.Flags = nil
			artifact.Fingerprints = nil
			return artifact, nil
		}
	}
	if strings.HasPrefix(artifactType, "productbrain_") || artifactType == "strategic_routing_summary" {
		if !artifactFingerprintValid(raw) {
			artifact.Status = "invalid_binding"
			artifact.ReasonCodes = []string{"artifact_fingerprint_mismatch"}
			artifact.Metrics = nil
			artifact.Flags = nil
			artifact.Fingerprints = nil
			return artifact, nil
		}
	}
	var deliveryAuthority *deliveryHistoryAuthority
	var validatedOutbox *productbrain.Outbox
	var validatedPreflight *productbrain.PreflightArtifact
	if artifactType == "productbrain_delivery_history" {
		authority, valid := validateDeliveryHistory(path, raw)
		if !valid {
			artifact.Status = "invalid_binding"
			artifact.ReasonCodes = []string{"invalid_delivery_history"}
			artifact.Metrics = nil
			artifact.Flags = nil
			artifact.Fingerprints = nil
			return artifact, nil
		}
		deliveryAuthority = &authority
	}
	if artifactType == "productbrain_outbox" {
		outbox, err := productbrain.DecodeOutbox(data)
		if err != nil {
			artifact.Status = "invalid_binding"
			artifact.ReasonCodes = []string{"invalid_productbrain_outbox"}
			artifact.Metrics = nil
			artifact.Flags = nil
			artifact.Fingerprints = nil
			return artifact, nil
		}
		validatedOutbox = &outbox
	}
	if artifactType == "productbrain_preflight" || artifactType == "productbrain_preflight_snapshot" {
		preflight, err := productbrain.DecodePreflight(data)
		if err != nil {
			artifact.Status = "invalid_binding"
			artifact.ReasonCodes = []string{"invalid_productbrain_preflight"}
			artifact.Metrics = nil
			artifact.Flags = nil
			artifact.Fingerprints = nil
			return artifact, nil
		}
		validatedPreflight = &preflight
	}
	extractEvidence(raw, &artifact)
	if deliveryAuthority != nil {
		mergeDeliveryHistoryAuthority(&artifact, *deliveryAuthority)
	}
	if validatedOutbox != nil {
		extractProductBrainOutboxEvidence(raw, *validatedOutbox, &artifact)
	}
	if validatedPreflight != nil {
		artifact.collectionContracts = []map[string]string{collectionContractMap(*validatedPreflight)}
		artifact.productBrainPreflights = []productbrain.PreflightArtifact{*validatedPreflight}
	}
	return artifact, nil
}

func proofCriticalPrivateAuthority(artifactType string) bool {
	return artifactType == "strategic_routing_summary" || strings.HasPrefix(artifactType, "productbrain_")
}

func supportedSchema(artifactType, schemaVersion string) bool {
	expected := map[string]string{
		"generic_trace_summary":              "mindline-trace-summary/v0.1",
		"corpus_pressure_summary":            "corpus-pressure-summary/v0.1",
		"corpus_pressure_eval_input":         "corpus-pressure-eval-input/v0.1",
		"corpus_pressure_trace_summary":      "corpus-pressure-trace-summary/v0.1",
		"corpus_graph_summary":               "corpus-graph-summary/v0.1",
		"document_segment_summary":           "document-segment-summary/v0.1",
		"semantic_candidate_summary":         "semantic-candidate-summary/v0.1",
		"corpus_pressure_loop_summary":       "corpus-pressure-loop-summary/v0.1",
		"corpus_acceptance_benchmark":        "corpus-acceptance-summary/v0.1",
		"autonomy_readiness_report":          "autonomy-readiness-report/v0.1",
		"link_enrichment_loop_summary":       "link-enrichment-loop-summary/v0.1",
		"link_enrichment_comparison_summary": "link-enrichment-comparison/v0.1",
		"link_artifact_requests":             "local-link-artifact-requests/v0.1",
		"link_enrichment_eval_projection":    "mindline-link-enrichment-eval-projection/v0.1",
		"value_proof_summary":                "mindline-value-proof/v0.1",
		"source_meaning_packet_summary":      "source-meaning-packet/v0.1",
		"corpus_concept_summary":             "corpus-concepts/v0.2",
		"corpus_concept_review_records":      "corpus-concept-review-records/v0.2",
		"strategic_routing_summary":          "mindline-strategic-routing-summary/v0.1",
		"productbrain_outbox":                "productbrain-outbox/v0.1",
		"productbrain_outbox_summary":        "productbrain-outbox-summary/v0.1",
		"productbrain_preflight":             "productbrain-preflight/v0.1",
		"productbrain_preflight_snapshot":    "productbrain-preflight/v0.1",
		"productbrain_delivery_history":      "productbrain-delivery-history/v0.1",
		"productbrain_delivery_summary":      "productbrain-delivery-summary/v0.1",
	}
	return strings.TrimSpace(schemaVersion) == expected[artifactType]
}

func artifactFingerprintValid(raw map[string]any) bool {
	expected := stringValue(raw["fingerprint"])
	if expected == "" {
		return false
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return false
	}
	var clone any
	if err := json.Unmarshal(data, &clone); err != nil {
		return false
	}
	stripArtifactFingerprints(clone)
	canonical, err := json.Marshal(clone)
	if err != nil {
		return false
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]) == expected
}

func stripArtifactFingerprints(value any) {
	switch item := value.(type) {
	case map[string]any:
		delete(item, "fingerprint")
		for _, child := range item {
			stripArtifactFingerprints(child)
		}
	case []any:
		for _, child := range item {
			stripArtifactFingerprints(child)
		}
	}
}

type deliveryHistoryAuthority struct {
	Metrics              map[string]float64
	Flags                map[string]bool
	PreflightFingerprint string
	Operations           map[string]operationContract
	OperationRuns        []map[string]operationContract
	CollectionContracts  []map[string]string
	Preflights           []productbrain.PreflightArtifact
}

type operationContract struct {
	OperationID         string
	Kind                string
	EntryID             string
	CollectionSlug      string
	Name                string
	Data                map[string]any
	SourceRef           string
	SourceExcerpt       string
	CreatedBy           string
	RelationIdentity    string
	RelationFromEntryID string
	RelationToEntryID   string
	RelationType        string
	RelationMetadata    map[string]any
	RemoteObjectID      string
	EntryDocID          string
	ReadbackFingerprint string
}

func validateDeliveryHistory(path string, raw map[string]any) (deliveryHistoryAuthority, bool) {
	invalid := func() (deliveryHistoryAuthority, bool) { return deliveryHistoryAuthority{}, false }
	runs, ok := raw["runs"].([]any)
	if !ok || len(runs) == 0 {
		return invalid()
	}
	refs, ok := raw["run_refs"].([]any)
	if !ok || len(refs) != len(runs) {
		return invalid()
	}
	outbox := stringValue(raw["outbox_fingerprint"])
	profile := stringValue(raw["profile_fingerprint"])
	if outbox == "" || profile == "" {
		return invalid()
	}
	base := filepath.Dir(path)
	expectedRunRefs := map[string]bool{}
	expectedSnapshotRefs := map[string]bool{}
	authority := deliveryHistoryAuthority{Metrics: map[string]float64{}, Flags: map[string]bool{"preflight_lineage_verified": true}, Operations: map[string]operationContract{}}
	for index, value := range runs {
		run, ok := value.(map[string]any)
		if !ok {
			return invalid()
		}
		sequence, ok := numberValue(run["sequence"])
		if !ok || int(sequence) != index+1 || stringValue(run["schema_version"]) != "productbrain-delivery-run/v0.1" || !artifactFingerprintValid(run) {
			return invalid()
		}
		invocation := stringValue(run["invocation_id"])
		ref := stringValue(refs[index])
		expectedRef := fmt.Sprintf("delivery-runs/%06d-%s.json", index+1, invocation)
		if invocation == "" || ref != expectedRef {
			return invalid()
		}
		expectedRunRefs[ref] = true
		sealed, valid := readContainedJSONObject(base, ref)
		if !valid || !artifactFingerprintValid(sealed) || !canonicalJSONEqual(sealed, run) {
			return invalid()
		}
		preflightFingerprint := stringValue(run["preflight_fingerprint"])
		snapshotRef := stringValue(run["preflight_snapshot_ref"])
		if stringValue(run["outbox_fingerprint"]) != outbox || stringValue(run["profile_fingerprint"]) != profile || preflightFingerprint == "" || snapshotRef != "preflight-snapshots/"+preflightFingerprint+".json" {
			return invalid()
		}
		authority.PreflightFingerprint = preflightFingerprint
		expectedSnapshotRefs[snapshotRef] = true
		snapshot, valid := readContainedJSONObject(base, snapshotRef)
		snapshotData, marshalErr := json.Marshal(snapshot)
		preflight, decodeErr := productbrain.DecodePreflight(snapshotData)
		if !valid || marshalErr != nil || decodeErr != nil || preflight.Fingerprint != preflightFingerprint || preflight.OutboxFingerprint != outbox || preflight.ProfileFingerprint != profile {
			return invalid()
		}
		authority.CollectionContracts = append(authority.CollectionContracts, collectionContractMap(preflight))
		authority.Preflights = append(authority.Preflights, preflight)
		if mutations, ok := numberValue(run["preflight_mutation_calls"]); !ok || mutations != 0 {
			return invalid()
		}
		if !boolValueOrFalse(run["external_preconditions_repeated"]) && !safeFailedPreconditionRun(run) {
			return invalid()
		}
	}
	if !directoryExactlyReferences(filepath.Join(base, "delivery-runs"), expectedRunRefs) || !directoryExactlyReferences(filepath.Join(base, "preflight-snapshots"), expectedSnapshotRefs) {
		return invalid()
	}
	if !deriveDeliveryHistoryAuthority(runs, &authority) {
		return invalid()
	}
	return authority, true
}

func readContainedJSONObject(base, ref string) (map[string]any, bool) {
	if ref == "" || filepath.IsAbs(ref) || filepath.ToSlash(filepath.Clean(filepath.FromSlash(ref))) != ref || ref == "." || strings.HasPrefix(ref, "../") {
		return nil, false
	}
	path := filepath.Join(base, filepath.FromSlash(ref))
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, false
	}
	if err := privateio.ValidateContained(base, path); err != nil {
		return nil, false
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, false
	}
	data, err := io.ReadAll(file)
	closeErr := file.Close()
	if err != nil || closeErr != nil || containsDeniedString(string(data)) {
		return nil, false
	}
	var raw map[string]any
	if json.Unmarshal(data, &raw) != nil {
		return nil, false
	}
	return raw, true
}

func directoryExactlyReferences(dir string, expected map[string]bool) bool {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != len(expected) {
		return false
	}
	parent := filepath.Base(dir)
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !expected[filepath.ToSlash(filepath.Join(parent, entry.Name()))] {
			return false
		}
	}
	return true
}

func canonicalJSONEqual(left, right any) bool {
	a, errA := json.Marshal(left)
	b, errB := json.Marshal(right)
	return errA == nil && errB == nil && string(a) == string(b)
}

func boolValueOrFalse(value any) bool {
	result, _ := boolValue(value)
	return result
}

func deriveDeliveryHistoryAuthority(runs []any, authority *deliveryHistoryAuthority) bool {
	for _, key := range []string{"completed_run_count", "interrupted_run_count", "failed_run_count", "entries_acknowledged", "relations_acknowledged", "blocked", "mismatches", "destination_writes", "product_brain_writes"} {
		authority.Metrics[key] = 0
	}
	authority.Metrics["run_count"] = float64(len(runs))
	var expectedKinds map[string]string
	for _, value := range runs {
		run := value.(map[string]any)
		outcome := stringValue(run["outcome"])
		switch outcome {
		case "completed":
			authority.Metrics["completed_run_count"]++
		case "interrupted":
			authority.Metrics["interrupted_run_count"]++
		case "failed":
			authority.Metrics["failed_run_count"]++
		default:
			return false
		}
		authority.Metrics["destination_writes"] += intNumberAsFloat(run["entries_created_this_run"]) + intNumberAsFloat(run["relations_created_this_run"])
		authority.Metrics["product_brain_writes"] = authority.Metrics["destination_writes"]
		operations, ok := run["operations"].([]any)
		if !ok || len(operations) == 0 {
			return false
		}
		currentKinds := map[string]string{}
		currentContracts := map[string]operationContract{}
		remoteIDs := map[string]bool{}
		entryMutations, relationMutations := 0, 0
		for _, operationValue := range operations {
			op, ok := operationValue.(map[string]any)
			if !ok {
				return false
			}
			operationID, kind := stringValue(op["operation_id"]), stringValue(op["kind"])
			if operationID == "" || (kind != "entry" && kind != "relation") || currentKinds[operationID] != "" {
				return false
			}
			currentKinds[operationID] = kind
			acknowledged := boolValueOrFalse(op["acknowledged"])
			state := stringValue(op["state"])
			if !validDeliveryOperationState(op) || acknowledged != (state == "acknowledged") {
				return false
			}
			mutationObserved := boolValueOrFalse(op["mutation_observed"])
			mutationResponseReceived := boolValueOrFalse(op["mutation_response_received"])
			if mutationObserved || mutationResponseReceived {
				if kind == "entry" {
					entryMutations++
				} else {
					relationMutations++
				}
			}
			if stringValue(op["safe_category"]) == "readback_mismatch" {
				authority.Metrics["mismatches"]++
			}
			if state == "blocked" {
				authority.Metrics["blocked"]++
			}
			if !acknowledged {
				if stringValue(op["remote_object_id"]) != "" || stringValue(op["entry_doc_id"]) != "" || stringValue(op["readback_fingerprint"]) != "" || boolValueOrFalse(op["draft_verified"]) || boolValueOrFalse(op["actor_verified"]) || boolValueOrFalse(op["attribution_verified"]) {
					return false
				}
				continue
			}
			remoteID, readbackFingerprint := stringValue(op["remote_object_id"]), stringValue(op["readback_fingerprint"])
			if remoteID == "" || readbackFingerprint == "" || remoteIDs[remoteID] {
				return false
			}
			remoteIDs[remoteID] = true
			contract := operationContract{OperationID: operationID, Kind: kind, RemoteObjectID: remoteID, EntryDocID: stringValue(op["entry_doc_id"]), ReadbackFingerprint: readbackFingerprint}
			switch kind {
			case "entry":
				if contract.EntryDocID == "" {
					return false
				}
			}
			currentContracts[operationID] = contract
		}
		if float64(entryMutations) != intNumberAsFloat(run["entries_created_this_run"]) || float64(relationMutations) != intNumberAsFloat(run["relations_created_this_run"]) {
			return false
		}
		if outcome == "completed" && len(currentContracts) != len(operations) {
			return false
		}
		if expectedKinds == nil {
			expectedKinds = currentKinds
		} else if !stringMapEqual(expectedKinds, currentKinds) {
			return false
		}
		authority.OperationRuns = append(authority.OperationRuns, currentContracts)
	}
	latest := runs[len(runs)-1].(map[string]any)
	for index, value := range runs {
		if len(runs) == 1 || index < len(runs)-1 {
			run := value.(map[string]any)
			authority.Metrics["first_run_entry_mutations"] += intNumberAsFloat(run["entries_created_this_run"])
			authority.Metrics["first_run_relation_mutations"] += intNumberAsFloat(run["relations_created_this_run"])
		}
	}
	authority.Metrics["latest_run_entry_mutations"] = intNumberAsFloat(latest["entries_created_this_run"])
	authority.Metrics["latest_run_relation_mutations"] = intNumberAsFloat(latest["relations_created_this_run"])
	latestContracts := authority.OperationRuns[len(authority.OperationRuns)-1]
	latestOperations, _ := latest["operations"].([]any)
	draftOnly, entryActor, relationAttribution := true, true, true
	for _, value := range latestOperations {
		op := value.(map[string]any)
		if stringValue(op["kind"]) == "entry" {
			draftOnly = draftOnly && boolValueOrFalse(op["draft_verified"])
			entryActor = entryActor && boolValueOrFalse(op["actor_verified"])
		} else {
			relationAttribution = relationAttribution && boolValueOrFalse(op["attribution_verified"])
		}
	}
	for _, contract := range latestContracts {
		switch contract.Kind {
		case "entry":
			authority.Metrics["entries_acknowledged"]++
		case "relation":
			authority.Metrics["relations_acknowledged"]++
		default:
			return false
		}
		authority.Operations[contract.OperationID] = contract
	}
	authority.Metrics["expected_operation_count"] = float64(len(expectedKinds))
	authority.Flags["draft_only"] = draftOnly
	authority.Flags["entry_actor_verified"] = entryActor
	authority.Flags["relation_attribution_verified"] = relationAttribution
	authority.Flags["replay_zero_mutation"] = len(runs) >= 2 && stringValue(latest["outcome"]) == "completed" && authority.Metrics["latest_run_entry_mutations"] == 0 && authority.Metrics["latest_run_relation_mutations"] == 0 && len(latestContracts) == len(expectedKinds)
	return len(authority.Operations) == len(expectedKinds)
}

func validDeliveryOperationState(operation map[string]any) bool {
	state := stringValue(operation["state"])
	attempts, ok := numberValue(operation["attempts"])
	if !ok || attempts < 0 || attempts != float64(int(attempts)) {
		return false
	}
	acknowledged := boolValueOrFalse(operation["acknowledged"])
	mutationResponseReceived := boolValueOrFalse(operation["mutation_response_received"])
	mutationObserved := boolValueOrFalse(operation["mutation_observed"])
	safeCategory := stringValue(operation["safe_category"])
	switch state {
	case "pending":
		return attempts == 0 && !mutationResponseReceived && !acknowledged && !mutationObserved && safeCategory == ""
	case "sending":
		return attempts >= 1 && !mutationResponseReceived && !acknowledged && !mutationObserved && safeCategory == ""
	case "reconciling":
		return attempts >= 1 && !acknowledged && !mutationObserved && safeCategory == ""
	case "blocked":
		if attempts < 1 || acknowledged || !productbrain.ValidSafeDeliveryCategory(safeCategory) {
			return false
		}
		if mutationObserved != (safeCategory == "readback_mismatch") {
			return false
		}
		return !mutationResponseReceived || safeCategory == "readback_mismatch" || safeCategory == "ambiguous_outcome"
	case "acknowledged":
		return attempts >= 1 && acknowledged && safeCategory == ""
	default:
		return false
	}
}

func stringMapEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func intNumberAsFloat(value any) float64 {
	number, _ := numberValue(value)
	return number
}

func safeFailedPreconditionRun(run map[string]any) bool {
	if stringValue(run["outcome"]) != "failed" || intNumberAsFloat(run["entries_created_this_run"]) != 0 || intNumberAsFloat(run["relations_created_this_run"]) != 0 {
		return false
	}
	operations, ok := run["operations"].([]any)
	if !ok || len(operations) == 0 {
		return false
	}
	for _, value := range operations {
		operation, ok := value.(map[string]any)
		if !ok || stringValue(operation["state"]) != "pending" || intNumberAsFloat(operation["attempts"]) != 0 || boolValueOrFalse(operation["mutation_response_received"]) || boolValueOrFalse(operation["mutation_observed"]) || boolValueOrFalse(operation["acknowledged"]) || stringValue(operation["remote_object_id"]) != "" || stringValue(operation["entry_doc_id"]) != "" || stringValue(operation["readback_fingerprint"]) != "" {
			return false
		}
	}
	return true
}

func mergeDeliveryHistoryAuthority(artifact *ArtifactEvidence, authority deliveryHistoryAuthority) {
	for key, value := range authority.Metrics {
		artifact.Metrics[key] = value
	}
	for key, value := range authority.Flags {
		artifact.Flags[key] = value
	}
	artifact.Fingerprints["preflight_fingerprint"] = authority.PreflightFingerprint
	artifact.operationContracts = authority.Operations
	artifact.operationRuns = authority.OperationRuns
	artifact.collectionContracts = authority.CollectionContracts
	artifact.productBrainPreflights = authority.Preflights
}

func extractProductBrainOutboxEvidence(raw map[string]any, outbox productbrain.Outbox, artifact *ArtifactEvidence) {
	operations, _ := raw["operations"].([]any)
	artifact.operationContracts = map[string]operationContract{}
	artifact.Metrics["operation_count"] = float64(len(operations))
	for _, value := range operations {
		op, _ := value.(map[string]any)
		operationID, kind := stringValue(op["operation_id"]), stringValue(op["kind"])
		contract := operationContract{OperationID: operationID, Kind: kind}
		switch kind {
		case "entry":
			artifact.Metrics["entry_operation_count"]++
			entry, _ := op["entry"].(map[string]any)
			contract.EntryID = stringValue(entry["entry_id"])
			contract.CollectionSlug = stringValue(entry["collection_slug"])
			contract.Name = stringValue(entry["name"])
			contract.Data, _ = entry["data"].(map[string]any)
			contract.SourceRef = stringValue(entry["source_ref"])
			contract.SourceExcerpt = stringValue(entry["source_excerpt"])
			contract.CreatedBy = stringValue(entry["created_by"])
		case "relation":
			artifact.Metrics["relation_operation_count"]++
			relation, _ := op["relation"].(map[string]any)
			contract.RelationIdentity = stringValue(relation["relation_identity"])
			contract.RelationFromEntryID = stringValue(relation["from_entry_id"])
			contract.RelationToEntryID = stringValue(relation["to_entry_id"])
			contract.RelationType = stringValue(relation["type"])
			contract.RelationMetadata, _ = relation["metadata"].(map[string]any)
		}
		artifact.operationContracts[operationID] = contract
	}
	findings, _ := raw["privacy_findings"].([]any)
	artifact.Metrics["privacy_finding_count"] = float64(len(findings))
	artifact.Fingerprints["outbox_fingerprint"] = stringValue(raw["fingerprint"])
	artifact.Fingerprints["routing_fingerprint"] = stringValue(raw["routing_fingerprint"])
	artifact.Fingerprints["profile_fingerprint"] = stringValue(raw["profile_fingerprint"])
	if snapshot, ok := raw["delivery_profile_snapshot"].(map[string]any); ok {
		artifact.Flags["draft_only"] = boolValueOrFalse(snapshot["draft_only"])
	}
	uniquePrimary := map[string]bool{}
	duplicateCount := 0
	lensIDs := map[string]bool{}
	for _, capture := range outbox.ReviewContext.Captures {
		uniquePrimary[capture.CanonicalURLID] = true
		if capture.DuplicateOf != "" {
			duplicateCount++
		}
		for _, lens := range capture.LensResults {
			lensIDs[lens.LensID] = true
		}
	}
	artifact.Metrics["review_capture_count"] = float64(len(outbox.ReviewContext.Captures))
	artifact.Metrics["review_primary_canonical_count"] = float64(len(uniquePrimary))
	artifact.Metrics["review_depth_one_count"] = float64(len(outbox.ReviewContext.DepthOneSources))
	artifact.Metrics["review_duplicate_count"] = float64(duplicateCount)
	artifact.Metrics["review_lens_count"] = float64(len(lensIDs))
	artifact.Metrics["review_lens_result_count"] = float64((len(uniquePrimary) + len(outbox.ReviewContext.DepthOneSources)) * len(lensIDs))
	collectionSet := map[string]string{}
	for _, operation := range outbox.Operations {
		if operation.Entry != nil {
			collectionSet[operation.Entry.CollectionSlug] = ""
		}
	}
	artifact.collectionContracts = []map[string]string{collectionSet}
	artifact.productBrainOutbox = &outbox
}

func collectionContractMap(artifact productbrain.PreflightArtifact) map[string]string {
	result := map[string]string{}
	for _, contract := range artifact.CollectionContracts {
		result[contract.Slug] = contract.Fingerprint
	}
	return result
}

func extractEvidence(raw map[string]any, artifact *ArtifactEvidence) {
	if projection, ok := raw["eval_projection"].(map[string]any); ok {
		artifact.SampleStatus = stringValue(projection["sample_status"])
		if value, present := boolValue(projection["held_out"]); present {
			artifact.Flags["held_out"] = value
			if !value {
				artifact.Flags["declared_not_held_out"] = true
			}
		}
		if value, present := boolValue(projection["generalizable"]); present {
			artifact.Flags["generalizable"] = value
			if !value {
				artifact.Flags["declared_non_generalizable"] = true
			}
		}
	}
	if artifact.Type == "productbrain_preflight" {
		artifact.Fingerprints["preflight_fingerprint"] = stringValue(raw["fingerprint"])
	}
	for _, key := range []string{
		"source_count", "candidate_count", "semantic_candidate_count", "observation_count", "evidence_ready_atom_count",
		"processed_source_count", "semantic_observation_count", "document_segment_count", "segment_count",
		"reference_candidate_count", "one_candidate_source_count", "reference_only_source_count",
		"accounted_source_count", "atom_count", "evidence_or_blocker_atom_count", "relation_count",
		"review_burden_count", "missing_link_reduction_ratio", "needs_enrichment_reduction_ratio",
		"missing_link_enrichment_reduction_ratio",
		"url_accounting_coverage", "artifact_match_coverage", "model_error_count",
		"human_review_required_count",
		"eval_counted_count", "evidence_ready_count", "eval_counted_human_review_required_count",
		"eval_counted_model_error_count",
		"review_group_count", "ready_group_count", "needs_review_group_count", "blocked_group_count",
		"proposal_count", "evidence_reference_count", "evidence_or_blocker_group_count",
		"generated_review_group_count", "generated_ready_group_count", "generated_needs_review_group_count",
		"generated_blocked_group_count", "generated_proposal_count", "generated_evidence_reference_count",
		"generated_evidence_or_blocker_group_count", "generated_review_burden_count",
		"concept_count", "generated_concept_count", "cross_source_concept_count", "local_concept_count",
		"needs_review_concept_count", "blocked_concept_count", "concept_review_burden_count",
		"concept_review_count", "cleanup_triage_count", "enrichment_backlog_count", "blocked_diagnostic_count",
		"cross_source_evidence_reference_count", "cross_source_kind_pair_count", "max_concept_count",
		"omitted_concept_count",
		"scale_skipped_source_count", "max_processed_sources", "max_source_bytes", "max_source_segments", "max_source_candidates",
		"max_graph_pair_comparisons", "max_graph_relations", "max_packet_review_groups",
		"graph_pair_comparison_count", "graph_pair_comparison_limit", "graph_relation_candidate_limit",
		"pair_comparison_count", "pair_comparison_limit", "relation_candidate_limit",
		"max_review_group_count", "omitted_atom_count",
		"threshold", "accuracy", "eval_count",
		"input_record_count", "url_occurrence_count", "primary_canonical_url_count", "depth_one_url_count",
		"canonical_source_count", "duplicate_occurrence_count", "lens_count", "required_lens_result_count",
		"lens_result_count", "validation_failure_count", "local_private_handling_findings", "outbound_privacy_findings",
		"operation_count", "entry_operation_count", "relation_operation_count", "privacy_finding_count", "mutation_calls",
		"run_count", "completed_run_count", "interrupted_run_count", "failed_run_count", "expected_operation_count", "entries_acknowledged",
		"relations_acknowledged", "blocked", "mismatches", "first_run_entry_mutations", "first_run_relation_mutations",
		"latest_run_entry_mutations", "latest_run_relation_mutations", "destination_writes", "product_brain_writes",
	} {
		if value, ok := numberValue(raw[key]); ok {
			artifact.Metrics[key] = value
		}
	}
	for _, key := range []string{"processed_source_ratio", "source_accounting_ratio", "evidence_ready_atom_ratio", "evidence_or_blocker_ratio", "review_burden_ratio", "candidate_per_processed_source_ratio", "observation_per_segment_ratio", "reference_candidate_ratio", "atom_compression_ratio", "relation_review_compression_ratio", "evidence_or_blocker_group_ratio", "generated_atom_compression_ratio", "generated_relation_review_compression_ratio", "generated_evidence_or_blocker_group_ratio", "generated_review_burden_ratio", "concept_review_burden_ratio", "atom_coverage_ratio", "cross_source_atom_ratio"} {
		if value, ok := numberValue(raw[key]); ok {
			artifact.Metrics[key] = value
		}
	}
	extractSemanticReadinessEvidence(raw, artifact)
	if status := stringValue(raw["scale_status"]); status != "" {
		artifact.Flags[status] = true
		if status == "scale_partial" {
			artifact.ReasonCodes = appendUnique(artifact.ReasonCodes, "scale_partial")
		}
	}
	if reasons, ok := raw["scale_reason_codes"].([]any); ok {
		for _, item := range reasons {
			if reason := stringValue(item); reason != "" {
				artifact.ReasonCodes = appendUnique(artifact.ReasonCodes, reason)
			}
		}
	}
	if budget, ok := raw["scale_budget"].(map[string]any); ok {
		extractEvidence(budget, artifact)
	}
	for _, key := range []string{"ready_for_50_file_pressure", "held_out", "non_generalizable_runtime", "comparable", "dec64_eligible", "no_human_eligible", "suite_valid"} {
		if value, ok := boolValue(raw[key]); ok {
			artifact.Flags[key] = value
		}
	}
	for _, key := range []string{"operator_judged", "draft_only", "preflight_lineage_verified", "replay_zero_mutation", "entry_actor_verified", "relation_attribution_verified", "autonomy_claim", "generalizable"} {
		if value, ok := boolValue(raw[key]); ok {
			artifact.Flags[key] = value
		}
	}
	if value, ok := boolValue(raw["held_out"]); ok && !value {
		artifact.Flags["declared_not_held_out"] = true
	}
	if value, ok := boolValue(raw["generalizable"]); ok && !value {
		artifact.Flags["declared_non_generalizable"] = true
	}
	if value, ok := boolValue(raw["autonomy_claim"]); ok && !value {
		artifact.Flags["declared_no_autonomy_claim"] = true
	}
	if stringValue(raw["verdict"]) == "pass" && artifact.Type == "productbrain_preflight" {
		artifact.Flags["preflight_pass"] = true
	}
	if stringValue(raw["threshold_status"]) == "eligible" {
		artifact.Flags["threshold_eligible"] = true
	}
	for _, key := range []string{"corpus_fingerprint", "command_config_fingerprint", "replay_fingerprint", "graph_replay_fingerprint", "outbox_fingerprint", "profile_fingerprint", "source_graph_fingerprint", "route_decisions_fingerprint", "lens_profile_fingerprint"} {
		if value := stringValue(raw[key]); value != "" {
			artifact.Fingerprints[key] = value
		}
	}
	if value := stringValue(raw["review_contract_fingerprint"]); value != "" {
		artifact.Fingerprints["review_contract_fingerprint"] = value
	}
	for _, key := range []string{"baseline_corpus_fingerprint", "enriched_corpus_fingerprint", "baseline_config_fingerprint", "enriched_config_fingerprint"} {
		if value := stringValue(raw[key]); value != "" {
			artifact.Fingerprints[key] = value
		}
	}
	if value := stringValue(raw["baseline_corpus_fingerprint"]); value != "" {
		artifact.Fingerprints["corpus_fingerprint"] = value
	}
	if value := stringValue(raw["baseline_config_fingerprint"]); value != "" {
		artifact.Fingerprints["command_config_fingerprint"] = value
	}
	if value := stringValue(raw["pressure_replay_fingerprint"]); value != "" {
		artifact.Fingerprints["pressure_replay_fingerprint"] = value
	}
	if guardrails, ok := raw["guardrails"].(map[string]any); ok {
		extractGuardrails(guardrails, artifact)
	}
	if safetyCounters, ok := raw["safety_counters"].(map[string]any); ok {
		extractSafetyCounters(safetyCounters, artifact)
	}
	if comparison, ok := raw["comparison"].(map[string]any); ok {
		extractEvidence(comparison, artifact)
	}
	if requestSummary, ok := raw["request_summary"].(map[string]any); ok {
		extractEvidence(requestSummary, artifact)
	}
	if summary, ok := raw["summary"].(map[string]any); ok {
		extractEvidence(summary, artifact)
	}
	if counts, ok := raw["counts"].(map[string]any); ok {
		extractEvidence(counts, artifact)
	}
	extractCorpusConceptReviewProgressEvidence(raw, artifact)
	if events, ok := raw["events"].([]any); ok {
		for _, event := range events {
			item, ok := event.(map[string]any)
			if !ok {
				continue
			}
			props, ok := item["properties"].(map[string]any)
			if !ok {
				props, ok = item["Properties"].(map[string]any)
			}
			if !ok {
				continue
			}
			for _, key := range []string{"$ai_evaluation_result", "non_generalizable_runtime", "metadata_only"} {
				if value, ok := boolValue(props[key]); ok {
					artifact.Flags[key] = value
				}
			}
			for _, key := range []string{
				"missing_link_reduction_ratio", "needs_enrichment_reduction_ratio",
				"missing_link_enrichment_reduction_ratio",
				"url_accounting_coverage", "artifact_match_coverage",
				"safety_network_fetches", "safety_hosted_telemetry_exports", "safety_hosted_inference_calls",
				"safety_browser_calls", "safety_slack_api_calls",
				"safety_destination_writes", "safety_product_brain_writes", "safety_tolaria_writes", "safety_auto_accepts",
				"safety_committed_private_artifacts",
			} {
				if value, ok := numberValue(props[key]); ok {
					artifact.Metrics[key] = value
				}
			}
			if value, ok := numberValue(props["safety_no_human_claims"]); ok {
				artifact.Metrics["safety_no_human_claims"] = value
			} else if value, ok := boolValue(props["safety_no_human_claims"]); ok {
				artifact.Flags["safety_no_human_claims"] = value
			}
		}
	}
}

func extractCorpusConceptReviewProgressEvidence(raw map[string]any, artifact *ArtifactEvidence) {
	progressByKind := map[string]map[string]float64{}
	hasProgress := false
	if progress, ok := raw["review_work_kind_progress"].(map[string]any); ok {
		hasProgress = true
		for kind, item := range progress {
			bucket, ok := item.(map[string]any)
			if !ok {
				continue
			}
			metrics := progressByKind[kind]
			if metrics == nil {
				metrics = map[string]float64{}
				progressByKind[kind] = metrics
			}
			for _, key := range []string{"total_count", "reviewed_count", "remaining_count"} {
				if value, ok := numberValue(bucket[key]); ok {
					metrics[key] = value
				}
			}
			if choices, ok := bucket["choice_counts"].(map[string]any); ok {
				for choice, rawCount := range choices {
					if value, ok := numberValue(rawCount); ok {
						metrics["choice_"+choice+"_count"] = value
					}
				}
			}
		}
	}
	if records, ok := raw["records"].([]any); ok && !hasProgress {
		seen := map[string]bool{}
		for _, item := range records {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			conceptID := stringValue(record["concept_id"])
			if conceptID == "" || seen[conceptID] {
				continue
			}
			seen[conceptID] = true
			kind := stringValue(record["review_work_kind"])
			if kind == "" {
				kind = "concept_review"
			}
			metrics := progressByKind[kind]
			if metrics == nil {
				metrics = map[string]float64{}
				progressByKind[kind] = metrics
			}
			metrics["reviewed_count"]++
			if choice := stringValue(record["choice"]); choice != "" {
				metrics["choice_"+choice+"_count"]++
			}
		}
	}
	for kind, metrics := range progressByKind {
		safeKind := strings.ReplaceAll(kind, "-", "_")
		for key, value := range metrics {
			artifact.Metrics[safeKind+"_"+key] = value
		}
	}
}

func extractGuardrails(guardrails map[string]any, artifact *ArtifactEvidence) {
	for _, key := range []string{"network_fetches", "hosted_telemetry_exports", "hosted_inference_calls", "browser_calls", "slack_api_calls", "destination_writes", "product_brain_writes", "tolaria_writes", "auto_accepts", "committed_private_artifacts"} {
		if value, ok := numberValue(guardrails[key]); ok {
			artifact.Metrics["guardrail_"+key] = value
		}
	}
	if value, ok := numberValue(guardrails["no_human_claims"]); ok {
		artifact.Metrics["guardrail_no_human_claims"] = value
	} else if value, ok := boolValue(guardrails["no_human_claims"]); ok {
		artifact.Flags["guardrail_no_human_claims"] = value
	}
}

func extractSemanticReadinessEvidence(raw map[string]any, artifact *ArtifactEvidence) {
	if status := stringValue(raw["semantic_readiness_status"]); status != "" {
		artifact.Flags["semantic_readiness_"+status] = true
	}
	if sources, ok := raw["sources"].([]any); ok {
		processed := 0
		oneCandidate := 0
		referenceOnly := 0
		referenceCandidates := 0
		for _, item := range sources {
			source, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if stringValue(source["state"]) != "processed" {
				continue
			}
			processed++
			candidates := intNumberValue(source["candidate_count"])
			if candidates == 1 {
				oneCandidate++
			}
			if counts, ok := source["candidate_kind_counts"].(map[string]any); ok {
				reference := intNumberValue(counts["reference_candidate"])
				referenceCandidates += reference
				if candidates > 0 && reference == candidates {
					referenceOnly++
				}
			}
		}
		if processed > 0 {
			artifact.Metrics["processed_source_count"] = float64(processed)
			artifact.Metrics["one_candidate_source_count"] = float64(oneCandidate)
			artifact.Metrics["reference_only_source_count"] = float64(referenceOnly)
		}
		if referenceCandidates > 0 {
			artifact.Metrics["reference_candidate_count"] = float64(referenceCandidates)
		}
	}
	if counts, ok := raw["candidate_kind_counts"].(map[string]any); ok {
		if reference := intNumberValue(counts["reference_candidate"]); reference > 0 {
			artifact.Metrics["reference_candidate_count"] = float64(reference)
		}
	}
}

func intNumberValue(value any) int {
	if number, ok := numberValue(value); ok {
		return int(number)
	}
	return 0
}

func extractSafetyCounters(safetyCounters map[string]any, artifact *ArtifactEvidence) {
	for _, key := range []string{"destination_writes", "auto_accepts", "no_human_claims", "committed_private_artifacts"} {
		if value, ok := numberValue(safetyCounters[key]); ok {
			artifact.Metrics["safety_"+key] = value
		}
	}
}

func mergeModelEvidence(model *readbackModel, artifact ArtifactEvidence) {
	model.artifactTypes[artifact.Type] = true
	if artifact.SampleStatus != "" {
		if model.flags["declared_sample_status"] && model.sampleStatus != artifact.SampleStatus {
			model.flags["conflicting_sample_status"] = true
		} else {
			model.sampleStatus = artifact.SampleStatus
			model.flags["declared_sample_status"] = true
		}
	}
	if artifact.Status == "invalid_binding" {
		model.flags["invalid_delivery_binding"] = true
	}
	if artifact.Status == "unsafe_or_leaky" {
		model.flags["unsafe_or_leaky"] = true
	}
	if artifact.Type == "document_segment_summary" {
		model.segmentSummaryCount++
		if value, ok := artifact.Metrics["segment_count"]; ok {
			model.segmentSummarySegments += int(value)
			model.metrics["document_segment_count_from_summaries"] = float64(model.segmentSummarySegments)
		}
	}
	if artifact.Type == "semantic_candidate_summary" {
		sourceCount, hasSourceCount := artifact.Metrics["source_count"]
		candidateCount, hasCandidateCount := artifact.Metrics["candidate_count"]
		referenceCandidateCount, hasReferenceCandidateCount := artifact.Metrics["reference_candidate_count"]
		if hasSourceCount {
			model.metrics["processed_source_count_from_summaries"] += sourceCount
			if hasCandidateCount && candidateCount == sourceCount {
				model.metrics["one_candidate_source_count_from_summaries"] += sourceCount
			}
			if hasCandidateCount && hasReferenceCandidateCount && referenceCandidateCount == candidateCount {
				model.metrics["reference_only_source_count_from_summaries"] += sourceCount
			}
		}
		if value, ok := artifact.Metrics["observation_count"]; ok {
			model.metrics["semantic_observation_count_from_summaries"] += value
		}
		if value, ok := artifact.Metrics["candidate_count"]; ok {
			model.metrics["semantic_candidate_count_from_summaries"] += value
		}
		if value, ok := artifact.Metrics["reference_candidate_count"]; ok {
			model.metrics["reference_candidate_count_from_summaries"] += value
		}
	}
	for key, value := range artifact.Metrics {
		if !mergeArtifactMetricIntoModel(artifact.Type, key) {
			continue
		}
		model.metrics[key] = value
		switch key {
		case "guardrail_network_fetches", "safety_network_fetches":
			model.guardrails.NetworkFetches = maxInt(model.guardrails.NetworkFetches, int(value))
		case "guardrail_hosted_telemetry_exports", "safety_hosted_telemetry_exports":
			model.guardrails.HostedTelemetryExports = maxInt(model.guardrails.HostedTelemetryExports, int(value))
		case "guardrail_hosted_inference_calls", "safety_hosted_inference_calls":
			model.guardrails.HostedInferenceCalls = maxInt(model.guardrails.HostedInferenceCalls, int(value))
		case "guardrail_browser_calls", "safety_browser_calls":
			model.guardrails.BrowserCalls = maxInt(model.guardrails.BrowserCalls, int(value))
		case "guardrail_slack_api_calls", "safety_slack_api_calls":
			model.guardrails.SlackAPICalls = maxInt(model.guardrails.SlackAPICalls, int(value))
		case "guardrail_destination_writes", "safety_destination_writes", "destination_writes":
			model.guardrails.DestinationWrites = maxInt(model.guardrails.DestinationWrites, int(value))
		case "guardrail_product_brain_writes", "safety_product_brain_writes", "product_brain_writes":
			model.guardrails.ProductBrainWrites = maxInt(model.guardrails.ProductBrainWrites, int(value))
		case "guardrail_tolaria_writes", "safety_tolaria_writes":
			model.guardrails.TolariaWrites = maxInt(model.guardrails.TolariaWrites, int(value))
		case "guardrail_auto_accepts", "safety_auto_accepts":
			model.guardrails.AutoAccepts = maxInt(model.guardrails.AutoAccepts, int(value))
		case "guardrail_no_human_claims", "safety_no_human_claims":
			if value > 0 {
				model.guardrails.NoHumanClaims = true
			}
		case "guardrail_committed_private_artifacts", "safety_committed_private_artifacts":
			model.guardrails.CommittedPrivateArtifacts = maxInt(model.guardrails.CommittedPrivateArtifacts, int(value))
		}
	}
	for key, value := range artifact.Flags {
		if key == "comparable" && !value {
			model.flags["artifact_not_comparable"] = true
		}
		if value {
			model.flags[key] = true
		} else if _, exists := model.flags[key]; !exists {
			model.flags[key] = false
		}
		if key == "guardrail_no_human_claims" && value {
			model.guardrails.NoHumanClaims = true
		}
	}
	for key, value := range artifact.Fingerprints {
		if existing, exists := model.fingerprints[key]; exists {
			if existing != value {
				model.flags["conflicting_"+key] = true
			}
		} else {
			model.fingerprints[key] = value
		}
	}
}

func mergeArtifactMetricIntoModel(artifactType, metric string) bool {
	if artifactType == "source_meaning_packet_summary" {
		switch metric {
		case "review_burden_count", "review_burden_ratio":
			return false
		}
	}
	return true
}

func summarize(model readbackModel) Summary {
	typeCounts := map[string]int{}
	for _, artifact := range model.artifacts {
		typeCounts[artifact.Type]++
	}
	refs := artifactRefs(model.artifacts)
	sampleStatus := model.sampleStatus
	if sampleStatus == "unknown" && model.flags["held_out"] {
		sampleStatus = "held_out"
	}
	sort.Strings(refs)
	summary := Summary{
		SchemaVersion:      SummarySchemaVersion,
		RunID:              stableID(model.rootLabel, refs),
		InputRootLabel:     model.rootLabel,
		ArtifactCount:      len(model.artifacts),
		ArtifactTypeCounts: typeCounts,
		SampleStatus:       sampleStatus,
		ImprovementStatus:  "not_evaluated",
		Guardrails:         model.guardrails,
		SafeArtifactRefs:   refs,
		Artifacts:          model.artifacts,
	}
	if model.flags["non_generalizable_runtime"] || model.flags["declared_non_generalizable"] || model.flags["declared_not_held_out"] || strings.Contains(sampleStatus, "private") || strings.Contains(sampleStatus, "curated") || sampleStatus == "temp_runtime" || sampleStatus == "unknown" {
		summary.GeneralizationStatus = "non_generalizable"
	} else {
		summary.GeneralizationStatus = "generalizable"
	}
	summary.SemanticReadiness = semanticReadinessForModel(model)
	summary.TopImprovementTarget = chooseTarget(model, summary.GeneralizationStatus)
	summary.RerunInstructions = []string{"rerun the source command after addressing " + summary.TopImprovementTarget.Code + ", then run eval readback with --baseline pointing to this run"}
	summary.ReplayBaseline = replayBaselineForSummary(summary)
	rebuildClaimGates(&summary)
	return summary
}

func replayBaselineForSummary(summary Summary) ReplayBaseline {
	artifactTypes := make([]string, 0, len(summary.ArtifactTypeCounts))
	for artifactType := range summary.ArtifactTypeCounts {
		artifactTypes = append(artifactTypes, artifactType)
	}
	sort.Strings(artifactTypes)
	baseline := ReplayBaseline{
		Status:           "ready",
		ArtifactTypes:    artifactTypes,
		SafeArtifactRefs: append([]string{}, summary.SafeArtifactRefs...),
		RerunInstruction: "rerun the same source command with the same corpus and command configuration, then use this readback output as --baseline for eval proof-gate or eval loop-decision",
	}
	corpus := map[string]bool{}
	config := map[string]bool{}
	for _, artifact := range summary.Artifacts {
		if artifact.Status == "unsupported_schema" {
			baseline.ReasonCodes = appendUnique(baseline.ReasonCodes, "unsupported_schema")
		}
		if artifact.Status == "unsafe_or_leaky" {
			baseline.ReasonCodes = appendUnique(baseline.ReasonCodes, "unsafe_or_leaky")
		}
		if value := artifact.Fingerprints["corpus_fingerprint"]; value != "" {
			corpus[value] = true
		}
		if value := artifact.Fingerprints["command_config_fingerprint"]; value != "" {
			config[value] = true
		}
	}
	switch len(corpus) {
	case 0:
		baseline.ReasonCodes = appendUnique(baseline.ReasonCodes, "missing_corpus_fingerprint")
	case 1:
		for value := range corpus {
			baseline.CorpusFingerprint = value
		}
	default:
		baseline.ReasonCodes = appendUnique(baseline.ReasonCodes, "conflicting_corpus_fingerprints")
	}
	switch len(config) {
	case 0:
		baseline.ReasonCodes = appendUnique(baseline.ReasonCodes, "missing_command_config_fingerprint")
	case 1:
		for value := range config {
			baseline.CommandConfigFingerprint = value
		}
	default:
		baseline.ReasonCodes = appendUnique(baseline.ReasonCodes, "conflicting_command_config_fingerprints")
	}
	hasSupportedArtifact := false
	for _, artifact := range summary.Artifacts {
		if artifact.Status == "detected" {
			hasSupportedArtifact = true
			break
		}
	}
	if !hasSupportedArtifact {
		baseline.ReasonCodes = appendUnique(baseline.ReasonCodes, "missing_supported_artifacts")
	}
	if !hasSideEffectEvidence(&summary) {
		baseline.ReasonCodes = appendUnique(baseline.ReasonCodes, "missing_side_effect_evidence")
	}
	if hasSideEffectCounter(&summary) {
		baseline.ReasonCodes = appendUnique(baseline.ReasonCodes, "side_effect_counter_nonzero")
	}
	if len(baseline.ReasonCodes) > 0 {
		baseline.Status = "blocked"
	}
	return baseline
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func rebuildClaimGates(summary *Summary) {
	gates := []ClaimGate{
		{Gate: "artifact_presence", Status: "pass", EvidenceRefs: proofArtifactRefs(summary), ClaimImpact: "readback has local evidence to inspect"},
	}
	unsafe := hasUnsafeArtifact(summary)
	unsupported := hasUnsupportedArtifact(summary)
	if unsafe {
		gates = append(gates, ClaimGate{Gate: "privacy_safe_readback", Status: "fail", ReasonCodes: []string{"unsafe_or_leaky"}, EvidenceRefs: unsafeArtifactRefs(summary), ClaimImpact: "blocks improvement and Chain proof claims until unsafe artifacts are removed or redacted"})
	} else {
		gates = append(gates, ClaimGate{Gate: "privacy_safe_readback", Status: "pass", ClaimImpact: "readback output did not detect unsafe supported artifacts"})
	}
	if summary.GeneralizationStatus == "generalizable" {
		gates = append(gates, ClaimGate{Gate: "generalization_claim", Status: "pass", ClaimImpact: "sample can support its bounded generalization claim"})
	} else {
		gates = append(gates, ClaimGate{Gate: "generalization_claim", Status: "blocked", ReasonCodes: []string{"sample_bound_or_non_held_out"}, EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "blocks broad product, DEC-64, or no-human claims"})
	}
	deliveryStatus, deliveryReasons := deliveryClaimStatus(summary)
	gates = append(gates, ClaimGate{Gate: "delivery_claim", Status: deliveryStatus, ReasonCodes: deliveryReasons, EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "authorizes only the bounded acknowledged Product Brain draft-delivery claim"})
	if summary.SemanticReadiness.Status == "blocked" {
		gates = append(gates, ClaimGate{Gate: "semantic_readiness", Status: "blocked", ReasonCodes: append([]string{"semantic_readiness_blocked"}, summary.SemanticReadiness.ReasonCodes...), EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "source intake may have succeeded, but semantic value is not proven"})
	} else if summary.SemanticReadiness.Status == "ready" {
		gates = append(gates, ClaimGate{Gate: "semantic_readiness", Status: "pass", EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "semantic-density counters did not detect reference-only collapse"})
	} else {
		gates = append(gates, ClaimGate{Gate: "semantic_readiness", Status: "not_evaluated", ReasonCodes: summary.SemanticReadiness.ReasonCodes, EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "semantic readiness was not evaluated for this artifact shape or sample size"})
	}
	improvementStatus := summary.ImprovementStatus
	if summary.SemanticReadiness.Status == "blocked" {
		gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "blocked", ReasonCodes: append([]string{"semantic_readiness_blocked"}, summary.SemanticReadiness.ReasonCodes...), EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "blocks improvement claim until semantic extraction value is proven"})
	} else if summary.SemanticReadiness.Status == "not_evaluated" && !stringListContains(summary.SemanticReadiness.ReasonCodes, "insufficient_processed_sources") {
		gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "blocked", ReasonCodes: append([]string{"semantic_readiness_not_evaluated"}, summary.SemanticReadiness.ReasonCodes...), EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "blocks improvement claim until semantic-density evidence is present or reconstructable"})
	} else if summaryHasFlag(summary, "scale_partial") {
		gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "blocked", ReasonCodes: []string{"scale_partial"}, EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "blocks improvement claim until scale capacity limits are resolved"})
	} else {
		switch improvementStatus {
		case "improved":
			switch {
			case unsafe:
				gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "blocked", ReasonCodes: []string{"unsafe_or_leaky"}, EvidenceRefs: unsafeArtifactRefs(summary), ClaimImpact: "blocks improvement claim until readback evidence is privacy-safe"})
			case unsupported:
				gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "blocked", ReasonCodes: []string{"unsupported_schema"}, EvidenceRefs: unsupportedArtifactRefs(summary), ClaimImpact: "blocks improvement claim until supported-looking artifacts use known schemas"})
			default:
				gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "pass", ClaimImpact: "current run improved against a comparable baseline"})
			}
		case "unchanged", "regressed":
			gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "fail", ReasonCodes: []string{improvementStatus}, ClaimImpact: "blocks improvement claim"})
		case "not_comparable":
			reasons := []string{"not_comparable"}
			if summary.Comparison != nil {
				reasons = append(reasons, summary.Comparison.ReasonCodes...)
			}
			gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "blocked", ReasonCodes: reasons, ClaimImpact: "blocks improvement claim"})
		default:
			gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "blocked", ReasonCodes: []string{"missing_baseline"}, ClaimImpact: "blocks improvement claim until comparable baseline is supplied"})
		}
	}
	scalePartial := summaryHasFlag(summary, "scale_partial")
	if hasDEC64ThresholdProof(summary) && hasSideEffectEvidence(summary) && !hasSideEffectCounter(summary) && !unsafe && !unsupported && !scalePartial {
		gates = append(gates, ClaimGate{Gate: "dec64_no_human_claim", Status: "pass", EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "held-out threshold proof supports bounded no-human readiness claim"})
	} else {
		gates = append(gates, ClaimGate{Gate: "dec64_no_human_claim", Status: "blocked", ReasonCodes: dec64BlockedReasonCodes(summary, unsafe, unsupported), ClaimImpact: "blocks no-human autonomy readiness claim"})
	}
	if hasSideEffectCounter(summary) {
		gates = append(gates, ClaimGate{Gate: "side_effect_claim", Status: "fail", ReasonCodes: []string{"guardrail_counter_nonzero"}, ClaimImpact: "blocks safety claim"})
	} else if !hasSideEffectEvidence(summary) {
		gates = append(gates, ClaimGate{Gate: "side_effect_claim", Status: "blocked", ReasonCodes: []string{"missing_side_effect_evidence"}, ClaimImpact: "blocks safety claim until artifacts expose guardrail counters"})
	} else {
		gates = append(gates, ClaimGate{Gate: "side_effect_claim", Status: "pass", ClaimImpact: "readback found no prohibited side-effect counters"})
	}
	if summary.TopImprovementTarget.Code != "" {
		gates = append(gates, ClaimGate{Gate: "next_target", Status: "pass", EvidenceRefs: summary.TopImprovementTarget.EvidenceRefs, ClaimImpact: "next improvement target is explicit"})
	}
	summary.ClaimGates = gates
}

func deliveryClaimStatus(summary *Summary) (string, []string) {
	requiredTypes := []string{"strategic_routing_summary", "productbrain_outbox", "productbrain_outbox_summary", "productbrain_preflight", "productbrain_delivery_history", "productbrain_delivery_summary"}
	reasons := []string{}
	for _, artifactType := range requiredTypes {
		if summary.ArtifactTypeCounts[artifactType] == 0 {
			reasons = append(reasons, "missing_"+artifactType)
		}
	}
	for _, artifact := range summary.Artifacts {
		if artifact.Status != "detected" && stringListContains(requiredTypes, artifact.Type) {
			reasons = append(reasons, artifact.Type+"_"+artifact.Status)
		}
	}
	routingArtifact, routeOK := singleDetectedArtifact(*summary, "strategic_routing_summary")
	outboxArtifact, outboxOK := singleDetectedArtifact(*summary, "productbrain_outbox")
	outboxSummary, outboxSummaryOK := singleDetectedArtifact(*summary, "productbrain_outbox_summary")
	preflightArtifact, preflightOK := singleDetectedArtifact(*summary, "productbrain_preflight")
	historyArtifact, historyOK := singleDetectedArtifact(*summary, "productbrain_delivery_history")
	deliverySummary, deliverySummaryOK := singleDetectedArtifact(*summary, "productbrain_delivery_summary")
	if !routeOK || !outboxOK || !outboxSummaryOK || !historyOK || !deliverySummaryOK {
		reasons = append(reasons, "duplicate_or_missing_delivery_authority")
	}
	if routeOK {
		input := artifactMetricValue(routingArtifact, "input_record_count")
		occurrences := artifactMetricValue(routingArtifact, "url_occurrence_count")
		primary := artifactMetricValue(routingArtifact, "primary_canonical_url_count")
		depthOne := artifactMetricValue(routingArtifact, "depth_one_url_count")
		canonical := artifactMetricValue(routingArtifact, "canonical_source_count")
		requiredLenses := artifactMetricValue(routingArtifact, "required_lens_result_count")
		actualLenses := artifactMetricValue(routingArtifact, "lens_result_count")
		lensCount := artifactMetricValue(routingArtifact, "lens_count")
		if !artifactHasMetrics(routingArtifact, "input_record_count", "url_occurrence_count", "primary_canonical_url_count", "depth_one_url_count", "canonical_source_count", "lens_count", "required_lens_result_count", "lens_result_count", "validation_failure_count", "outbound_privacy_findings") || input <= 0 || occurrences != input || canonical != primary+depthOne || lensCount <= 0 || requiredLenses != canonical*lensCount || actualLenses != requiredLenses || artifactMetricValue(routingArtifact, "validation_failure_count") != 0 || artifactMetricValue(routingArtifact, "outbound_privacy_findings") != 0 {
			reasons = append(reasons, "routing_accounting_mismatch")
		}
		if routingArtifact.SampleStatus != "private_curated_sample" || !routingArtifact.Flags["operator_judged"] || !routingArtifact.Flags["declared_not_held_out"] || !routingArtifact.Flags["declared_non_generalizable"] {
			reasons = append(reasons, "routing_claim_boundary_missing")
		}
	}
	entryOps, relationOps, operationCount := float64(0), float64(0), float64(0)
	if outboxOK && outboxSummaryOK {
		entryOps = artifactMetricValue(outboxArtifact, "entry_operation_count")
		relationOps = artifactMetricValue(outboxArtifact, "relation_operation_count")
		operationCount = artifactMetricValue(outboxArtifact, "operation_count")
		if !artifactHasMetrics(outboxArtifact, "operation_count", "entry_operation_count", "relation_operation_count", "privacy_finding_count") || operationCount <= 0 || operationCount != entryOps+relationOps || artifactMetricValue(outboxArtifact, "privacy_finding_count") != 0 || !artifactMetricsEqual(outboxArtifact, outboxSummary, "operation_count", "entry_operation_count", "relation_operation_count", "privacy_finding_count") {
			reasons = append(reasons, "outbox_accounting_mismatch")
		}
		if !outboxArtifact.Flags["draft_only"] || !outboxArtifact.Flags["operator_judged"] || !outboxArtifact.Flags["declared_not_held_out"] || !outboxArtifact.Flags["declared_non_generalizable"] || !outboxArtifact.Flags["declared_no_autonomy_claim"] {
			reasons = append(reasons, "outbox_claim_boundary_missing")
		}
		if routingArtifact.Fingerprints["route_decisions_fingerprint"] == "" || routingArtifact.Fingerprints["route_decisions_fingerprint"] != outboxArtifact.Fingerprints["routing_fingerprint"] {
			reasons = append(reasons, "routing_outbox_binding_mismatch")
		}
		if artifactMetricValue(outboxArtifact, "review_capture_count") != artifactMetricValue(routingArtifact, "input_record_count") || artifactMetricValue(outboxArtifact, "review_primary_canonical_count") != artifactMetricValue(routingArtifact, "primary_canonical_url_count") || artifactMetricValue(outboxArtifact, "review_depth_one_count") != artifactMetricValue(routingArtifact, "depth_one_url_count") || artifactMetricValue(outboxArtifact, "review_duplicate_count") != artifactMetricValue(routingArtifact, "duplicate_occurrence_count") || artifactMetricValue(outboxArtifact, "review_lens_count") != artifactMetricValue(routingArtifact, "lens_count") || artifactMetricValue(outboxArtifact, "review_lens_result_count") != artifactMetricValue(routingArtifact, "lens_result_count") {
			reasons = append(reasons, "routing_review_matrix_mismatch")
		}
		if !preflightOK || !productBrainPreflightsMatch(outboxArtifact, preflightArtifact) {
			reasons = append(reasons, "preflight_collection_binding_mismatch")
		}
	}
	if historyOK && deliverySummaryOK {
		if !deliveryOperationContractsMatch(outboxArtifact, historyArtifact) {
			reasons = append(reasons, "delivery_operation_binding_mismatch")
		}
		if !artifactHasMetrics(historyArtifact, "run_count", "completed_run_count", "interrupted_run_count", "failed_run_count", "expected_operation_count", "entries_acknowledged", "relations_acknowledged", "first_run_entry_mutations", "first_run_relation_mutations", "latest_run_entry_mutations", "latest_run_relation_mutations", "blocked", "mismatches", "destination_writes", "product_brain_writes") || artifactMetricValue(historyArtifact, "run_count") < 2 || artifactMetricValue(historyArtifact, "completed_run_count") < 2 || artifactMetricValue(historyArtifact, "completed_run_count")+artifactMetricValue(historyArtifact, "interrupted_run_count")+artifactMetricValue(historyArtifact, "failed_run_count") != artifactMetricValue(historyArtifact, "run_count") || artifactMetricValue(historyArtifact, "expected_operation_count") != operationCount || artifactMetricValue(historyArtifact, "entries_acknowledged") != entryOps || artifactMetricValue(historyArtifact, "relations_acknowledged") != relationOps || artifactMetricValue(historyArtifact, "first_run_entry_mutations") != entryOps || artifactMetricValue(historyArtifact, "first_run_relation_mutations") != relationOps || artifactMetricValue(historyArtifact, "destination_writes") != operationCount || artifactMetricValue(historyArtifact, "product_brain_writes") != operationCount || artifactMetricValue(historyArtifact, "latest_run_entry_mutations") != 0 || artifactMetricValue(historyArtifact, "latest_run_relation_mutations") != 0 || artifactMetricValue(historyArtifact, "blocked") != 0 || artifactMetricValue(historyArtifact, "mismatches") != 0 {
			reasons = append(reasons, "delivery_history_accounting_mismatch")
		}
		if !artifactMetricsEqual(historyArtifact, deliverySummary, "run_count", "completed_run_count", "interrupted_run_count", "failed_run_count", "expected_operation_count", "entries_acknowledged", "relations_acknowledged", "first_run_entry_mutations", "first_run_relation_mutations", "latest_run_entry_mutations", "latest_run_relation_mutations", "blocked", "mismatches", "destination_writes", "product_brain_writes") {
			reasons = append(reasons, "delivery_projection_mismatch")
		}
		for _, key := range []string{"preflight_lineage_verified", "replay_zero_mutation", "draft_only", "entry_actor_verified", "relation_attribution_verified"} {
			if !historyArtifact.Flags[key] || !deliverySummary.Flags[key] {
				reasons = append(reasons, key+"_not_proven")
			}
		}
		for _, key := range []string{"declared_not_held_out", "declared_non_generalizable", "declared_no_autonomy_claim"} {
			if !deliverySummary.Flags[key] {
				reasons = append(reasons, key+"_not_proven")
			}
		}
	}
	preflightDetected := false
	for _, artifact := range summary.Artifacts {
		if artifact.Type == "productbrain_preflight" && artifact.Status == "detected" {
			preflightDetected = true
			if !artifactHasMetrics(artifact, "mutation_calls") || artifactMetricValue(artifact, "mutation_calls") != 0 || !artifact.Flags["preflight_pass"] {
				reasons = append(reasons, "preflight_not_read_only_or_passing")
			}
		}
	}
	if !preflightDetected {
		reasons = append(reasons, "missing_passing_preflight")
	}
	for _, key := range []string{"outbox_fingerprint", "profile_fingerprint", "preflight_fingerprint"} {
		if !consistentArtifactFingerprint(summary, key) {
			reasons = append(reasons, "conflicting_"+key)
		}
	}
	if len(reasons) > 0 {
		sort.Strings(reasons)
		return "fail", reasons
	}
	return "pass", nil
}

func deliveryOperationContractsMatch(outbox, history ArtifactEvidence) bool {
	if len(outbox.operationContracts) == 0 || len(outbox.operationContracts) != len(history.operationContracts) {
		return false
	}
	if len(history.operationRuns) == 0 || !productBrainPreflightsMatch(outbox, history) {
		return false
	}
	for _, run := range history.operationRuns {
		for operationID, actual := range run {
			expected, ok := outbox.operationContracts[operationID]
			if !ok || !operationContractMatches(expected, actual) {
				return false
			}
		}
	}
	return true
}

func operationContractMatches(expected, actual operationContract) bool {
	if actual.OperationID != expected.OperationID || actual.Kind != expected.Kind || actual.RemoteObjectID == "" || actual.ReadbackFingerprint == "" {
		return false
	}
	switch expected.Kind {
	case "entry":
		return actual.RemoteObjectID == expected.EntryID && actual.EntryDocID != "" && actual.ReadbackFingerprint == expectedEntryReadbackFingerprint(expected, actual)
	case "relation":
		return expected.RelationIdentity != "" && expected.RelationType != "" && expected.RelationFromEntryID != "" && expected.RelationToEntryID != "" && actual.ReadbackFingerprint == expectedRelationReadbackFingerprint(expected, actual.RemoteObjectID)
	default:
		return false
	}
}

func collectionContractSetsMatch(outbox, authority ArtifactEvidence) bool {
	if len(outbox.collectionContracts) != 1 || len(outbox.collectionContracts[0]) == 0 || len(authority.collectionContracts) == 0 {
		return false
	}
	expected := outbox.collectionContracts[0]
	for _, contracts := range authority.collectionContracts {
		if len(contracts) != len(expected) {
			return false
		}
		for slug := range expected {
			if strings.TrimSpace(contracts[slug]) == "" {
				return false
			}
		}
	}
	return true
}

func productBrainPreflightsMatch(outbox, authority ArtifactEvidence) bool {
	if outbox.productBrainOutbox == nil || len(authority.productBrainPreflights) == 0 || !collectionContractSetsMatch(outbox, authority) {
		return false
	}
	profile := productbrain.DeliveryProfileFromSnapshot(outbox.productBrainOutbox.ProfileSnapshot)
	for _, preflight := range authority.productBrainPreflights {
		if err := productbrain.ValidatePreflight(preflight, *outbox.productBrainOutbox, profile); err != nil {
			return false
		}
	}
	return true
}

func expectedEntryReadbackFingerprint(expected, actual operationContract) string {
	readback := struct {
		Found          bool           `json:"found"`
		DocID          string         `json:"doc_id,omitempty"`
		EntryID        string         `json:"entry_id,omitempty"`
		CollectionSlug string         `json:"collection_slug,omitempty"`
		Name           string         `json:"name,omitempty"`
		Status         string         `json:"status,omitempty"`
		Data           map[string]any `json:"data,omitempty"`
		SourceRef      string         `json:"source_ref,omitempty"`
		SourceExcerpt  string         `json:"source_excerpt,omitempty"`
		CreatedBy      string         `json:"created_by,omitempty"`
	}{true, actual.EntryDocID, expected.EntryID, expected.CollectionSlug, expected.Name, "draft", expected.Data, expected.SourceRef, expected.SourceExcerpt, expected.CreatedBy}
	return canonicalFingerprint(readback)
}

func expectedRelationReadbackFingerprint(expected operationContract, remoteID string) string {
	return canonicalFingerprint(map[string]any{"relation_id": remoteID, "identity": expected.RelationIdentity, "metadata": expected.RelationMetadata})
}

func canonicalFingerprint(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	var canonicalValue any
	if json.Unmarshal(data, &canonicalValue) != nil {
		return ""
	}
	stripArtifactFingerprints(canonicalValue)
	canonical, err := json.Marshal(canonicalValue)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:])
}

func singleDetectedArtifact(summary Summary, artifactType string) (ArtifactEvidence, bool) {
	var found ArtifactEvidence
	count := 0
	for _, artifact := range summary.Artifacts {
		if artifact.Type == artifactType && artifact.Status == "detected" {
			found = artifact
			count++
		}
	}
	return found, count == 1
}

func artifactMetricValue(artifact ArtifactEvidence, key string) float64 {
	return artifact.Metrics[key]
}

func artifactMetricsEqual(left, right ArtifactEvidence, keys ...string) bool {
	for _, key := range keys {
		leftValue, leftOK := left.Metrics[key]
		rightValue, rightOK := right.Metrics[key]
		if !leftOK || !rightOK || leftValue != rightValue {
			return false
		}
	}
	return true
}

func artifactHasMetrics(artifact ArtifactEvidence, keys ...string) bool {
	for _, key := range keys {
		if _, ok := artifact.Metrics[key]; !ok {
			return false
		}
	}
	return true
}
func consistentArtifactFingerprint(summary *Summary, key string) bool {
	values := map[string]bool{}
	for _, artifact := range summary.Artifacts {
		if value := artifact.Fingerprints[key]; value != "" {
			values[value] = true
		}
	}
	return len(values) == 1
}

func summaryHasFlag(summary *Summary, flag string) bool {
	for _, artifact := range summary.Artifacts {
		if artifact.Flags[flag] {
			return true
		}
	}
	return false
}

func dec64BlockedReasonCodes(summary *Summary, unsafe bool, unsupported bool) []string {
	reasonCodes := []string{}
	if !hasDEC64ThresholdProof(summary) {
		reasonCodes = append(reasonCodes, "held_out_threshold_not_proven")
	}
	if !hasSideEffectEvidence(summary) {
		reasonCodes = append(reasonCodes, "missing_side_effect_evidence")
	}
	if hasSideEffectCounter(summary) {
		reasonCodes = append(reasonCodes, "guardrail_counter_nonzero")
	}
	if summaryHasFlag(summary, "scale_partial") {
		reasonCodes = append(reasonCodes, "scale_partial")
	}
	if unsafe {
		reasonCodes = append(reasonCodes, "unsafe_or_leaky")
	}
	if unsupported {
		reasonCodes = append(reasonCodes, "unsupported_schema")
	}
	if len(reasonCodes) == 0 {
		return []string{"held_out_threshold_not_proven"}
	}
	return reasonCodes
}

func hasUnsafeArtifact(summary *Summary) bool {
	for _, artifact := range summary.Artifacts {
		if artifact.Status == "unsafe_or_leaky" {
			return true
		}
	}
	for _, artifact := range summary.BaselineArtifacts {
		if artifact.Status == "unsafe_or_leaky" {
			return true
		}
	}
	return false
}

func hasUnsupportedArtifact(summary *Summary) bool {
	for _, artifact := range summary.Artifacts {
		if artifact.Status == "unsupported_schema" {
			return true
		}
	}
	for _, artifact := range summary.BaselineArtifacts {
		if artifact.Status == "unsupported_schema" {
			return true
		}
	}
	return false
}

func artifactRefs(artifacts []ArtifactEvidence) []string {
	refs := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		refs = append(refs, artifact.Ref)
	}
	sort.Strings(refs)
	return refs
}

func prefixedArtifactRefs(prefix string, refs []string) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, filepath.ToSlash(filepath.Join(prefix, ref)))
	}
	sort.Strings(out)
	return out
}

func proofArtifactRefs(summary *Summary) []string {
	refs := append([]string{}, summary.SafeArtifactRefs...)
	refs = append(refs, summary.BaselineArtifactRefs...)
	return firstRefs(refs)
}

func unsafeArtifactRefs(summary *Summary) []string {
	refs := []string{}
	for _, artifact := range summary.Artifacts {
		if artifact.Status == "unsafe_or_leaky" {
			refs = append(refs, artifact.Ref)
		}
	}
	for _, artifact := range summary.BaselineArtifacts {
		if artifact.Status == "unsafe_or_leaky" {
			refs = append(refs, filepath.ToSlash(filepath.Join("baseline", artifact.Ref)))
		}
	}
	return firstRefs(refs)
}

func unsupportedArtifactRefs(summary *Summary) []string {
	refs := []string{}
	for _, artifact := range summary.Artifacts {
		if artifact.Status == "unsupported_schema" {
			refs = append(refs, artifact.Ref)
		}
	}
	for _, artifact := range summary.BaselineArtifacts {
		if artifact.Status == "unsupported_schema" {
			refs = append(refs, filepath.ToSlash(filepath.Join("baseline", artifact.Ref)))
		}
	}
	return firstRefs(refs)
}

func hasSideEffectEvidence(summary *Summary) bool {
	present := map[string]bool{}
	autonomyPresent := map[string]bool{}
	hasAutonomyReport := false
	hasLinkEnrichmentSafetyArtifact := false
	for _, artifact := range summary.Artifacts {
		if artifact.Type == "autonomy_readiness_report" {
			hasAutonomyReport = true
		}
		if isLinkEnrichmentSafetyArtifact(artifact.Type) {
			hasLinkEnrichmentSafetyArtifact = true
		}
		for key := range artifact.Metrics {
			if name, ok := sideEffectMetricName(key); ok {
				present[name] = true
				if artifact.Type == "autonomy_readiness_report" {
					autonomyPresent[name] = true
				}
			}
		}
		for key := range artifact.Flags {
			if name, ok := sideEffectMetricName(key); ok {
				present[name] = true
				if artifact.Type == "autonomy_readiness_report" {
					autonomyPresent[name] = true
				}
			}
		}
	}
	hasAutonomySafetyEvidence := hasRequiredSideEffectEvidence(autonomyPresent, []string{"destination_writes", "auto_accepts", "no_human_claims", "committed_private_artifacts"})
	hasBaseEvidence := hasRequiredSideEffectEvidence(present, []string{
		"network_fetches", "hosted_telemetry_exports", "hosted_inference_calls", "browser_calls", "slack_api_calls",
		"destination_writes", "product_brain_writes", "tolaria_writes", "auto_accepts", "no_human_claims", "committed_private_artifacts",
	})
	if !hasBaseEvidence && !(hasAutonomyReport && hasAutonomySafetyEvidence) {
		return false
	}
	if hasAutonomyReport && !hasAutonomySafetyEvidence {
		return false
	}
	if hasLinkEnrichmentSafetyArtifact && !hasRequiredSideEffectEvidence(present, []string{"browser_calls", "slack_api_calls"}) {
		return false
	}
	return true
}

func hasRequiredSideEffectEvidence(present map[string]bool, required []string) bool {
	for _, key := range required {
		if !present[key] {
			return false
		}
	}
	return true
}

func isCorpusPressureSafetyArtifact(artifactType string) bool {
	switch artifactType {
	case "corpus_pressure_summary", "corpus_pressure_eval_input", "corpus_pressure_trace_summary", "corpus_pressure_loop_summary", "corpus_acceptance_benchmark":
		return true
	default:
		return false
	}
}

func isLinkEnrichmentSafetyArtifact(artifactType string) bool {
	switch artifactType {
	case "link_enrichment_loop_summary", "link_enrichment_comparison_summary", "link_enrichment_eval_projection":
		return true
	default:
		return false
	}
}

func sideEffectMetricName(metric string) (string, bool) {
	name := strings.TrimPrefix(metric, "guardrail_")
	if name == metric {
		name = strings.TrimPrefix(metric, "safety_")
	}
	if name == metric {
		return "", false
	}
	switch name {
	case "network_fetches", "hosted_telemetry_exports", "hosted_inference_calls", "browser_calls", "slack_api_calls", "destination_writes", "product_brain_writes", "tolaria_writes", "auto_accepts", "no_human_claims", "committed_private_artifacts":
		return name, true
	default:
		return "", false
	}
}

func hasSideEffectCounter(summary *Summary) bool {
	return summary.Guardrails.NetworkFetches > 0 ||
		summary.Guardrails.HostedTelemetryExports > 0 ||
		summary.Guardrails.HostedInferenceCalls > 0 ||
		summary.Guardrails.BrowserCalls > 0 ||
		summary.Guardrails.SlackAPICalls > 0 ||
		summary.Guardrails.DestinationWrites > 0 ||
		summary.Guardrails.ProductBrainWrites > 0 ||
		summary.Guardrails.TolariaWrites > 0 ||
		summary.Guardrails.AutoAccepts > 0 ||
		summary.Guardrails.NoHumanClaims ||
		summary.Guardrails.CommittedPrivateArtifacts > 0
}

func hasDEC64ThresholdProof(summary *Summary) bool {
	if summary.GeneralizationStatus != "generalizable" {
		return false
	}
	for _, artifact := range summary.Artifacts {
		if artifact.Type == "corpus_acceptance_benchmark" && artifact.Flags["held_out"] && artifact.Flags["suite_valid"] && (artifact.Flags["dec64_eligible"] || artifact.Flags["no_human_eligible"]) && hasThresholdProof(artifact) {
			return true
		}
		if artifact.Type == "autonomy_readiness_report" && artifact.Flags["held_out"] && artifact.Flags["threshold_eligible"] && hasThresholdProof(artifact) {
			return true
		}
	}
	return false
}

func hasThresholdProof(artifact ArtifactEvidence) bool {
	threshold, thresholdOK := artifact.Metrics["threshold"]
	accuracy, accuracyOK := artifact.Metrics["accuracy"]
	return thresholdOK && accuracyOK && threshold >= 0.98 && accuracy >= threshold
}

func compareModels(baseline, current readbackModel) ComparisonSummary {
	comparison := ComparisonSummary{
		SchemaVersion: ComparisonSchemaVersion,
		Status:        "not_comparable",
		BaselineLabel: baseline.rootLabel,
		CurrentLabel:  current.rootLabel,
		MetricDeltas:  map[string]float64{},
	}
	comparable, reasons := comparableModels(baseline, current)
	if !comparable {
		comparison.ReasonCodes = reasons
		return comparison
	}
	comparison.ReasonCodes = reasons
	improved, regressed := false, false
	for _, metric := range []string{
		"evidence_ready_atom_ratio", "evidence_or_blocker_ratio", "source_accounting_ratio", "processed_source_ratio",
		"missing_link_reduction_ratio", "missing_link_enrichment_reduction_ratio", "needs_enrichment_reduction_ratio",
		"url_accounting_coverage", "artifact_match_coverage", "evidence_ready_count",
		"semantic_observation_count", "semantic_candidate_count", "candidate_per_processed_source_ratio", "observation_per_segment_ratio",
		"atom_compression_ratio", "relation_review_compression_ratio", "evidence_or_blocker_group_ratio",
	} {
		before, bok := baseline.metrics[metric]
		after, aok := current.metrics[metric]
		if !bok || !aok {
			continue
		}
		delta := after - before
		comparison.MetricDeltas[metric] = delta
		if delta > 0 {
			improved = true
		}
		if delta < 0 {
			regressed = true
		}
	}
	for _, metric := range []string{
		"review_burden_ratio", "review_burden_count", "human_review_required_count", "model_error_count",
		"reference_candidate_ratio", "reference_candidate_count", "reference_only_source_count", "one_candidate_source_count",
	} {
		before, bok := baseline.metrics[metric]
		after, aok := current.metrics[metric]
		if !bok || !aok {
			continue
		}
		delta := before - after
		comparison.MetricDeltas[metric+"_reduction"] = delta
		if delta > 0 {
			improved = true
		}
		if delta < 0 {
			regressed = true
		}
	}
	for _, guardrail := range []struct {
		name   string
		before int
		after  int
	}{
		{name: "network_fetches", before: baseline.guardrails.NetworkFetches, after: current.guardrails.NetworkFetches},
		{name: "hosted_telemetry_exports", before: baseline.guardrails.HostedTelemetryExports, after: current.guardrails.HostedTelemetryExports},
		{name: "hosted_inference_calls", before: baseline.guardrails.HostedInferenceCalls, after: current.guardrails.HostedInferenceCalls},
		{name: "browser_calls", before: baseline.guardrails.BrowserCalls, after: current.guardrails.BrowserCalls},
		{name: "slack_api_calls", before: baseline.guardrails.SlackAPICalls, after: current.guardrails.SlackAPICalls},
		{name: "destination_writes", before: baseline.guardrails.DestinationWrites, after: current.guardrails.DestinationWrites},
		{name: "product_brain_writes", before: baseline.guardrails.ProductBrainWrites, after: current.guardrails.ProductBrainWrites},
		{name: "tolaria_writes", before: baseline.guardrails.TolariaWrites, after: current.guardrails.TolariaWrites},
		{name: "auto_accepts", before: baseline.guardrails.AutoAccepts, after: current.guardrails.AutoAccepts},
		{name: "no_human_claims", before: boolInt(baseline.guardrails.NoHumanClaims), after: boolInt(current.guardrails.NoHumanClaims)},
		{name: "committed_private_artifacts", before: baseline.guardrails.CommittedPrivateArtifacts, after: current.guardrails.CommittedPrivateArtifacts},
	} {
		delta := float64(guardrail.before - guardrail.after)
		if delta != 0 {
			comparison.MetricDeltas["guardrail_"+guardrail.name+"_reduction"] = delta
		}
		if guardrail.after > guardrail.before {
			regressed = true
			comparison.ReasonCodes = append(comparison.ReasonCodes, "guardrail_regression")
		}
	}
	if !baseline.artifactTypes["source_meaning_packet_summary"] && current.artifactTypes["source_meaning_packet_summary"] {
		if compression, ok := current.metrics["atom_compression_ratio"]; ok && compression > 0 {
			comparison.MetricDeltas["source_meaning_packet_added"] = 1
			improved = true
		}
	}
	switch {
	case regressed:
		comparison.Status = "regressed"
	case improved:
		comparison.Status = "improved"
	default:
		comparison.Status = "unchanged"
	}
	return comparison
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func boolMetric(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func maxInt(a, b int) int {
	if b > a {
		return b
	}
	return a
}

func comparableModels(a, b readbackModel) (bool, []string) {
	if a.flags["replay_baseline_blocked"] {
		return false, append([]string{"replay_baseline_blocked"}, a.replayBaselineReasonCodes...)
	}
	if a.flags["artifact_not_comparable"] || b.flags["artifact_not_comparable"] {
		return false, []string{"artifact_not_comparable"}
	}
	if a.flags["conflicting_corpus_fingerprint"] || b.flags["conflicting_corpus_fingerprint"] || a.flags["conflicting_command_config_fingerprint"] || b.flags["conflicting_command_config_fingerprint"] {
		return false, []string{"conflicting_fingerprints"}
	}
	av, aok := a.fingerprints["corpus_fingerprint"]
	bv, bok := b.fingerprints["corpus_fingerprint"]
	if aok != bok {
		return false, []string{"one_sided_fingerprint"}
	}
	if !aok {
		if sharesArtifactDomain(a, b) {
			return false, []string{"missing_fingerprints"}
		}
		return false, []string{"artifact_domain_mismatch"}
	}
	if av != bv {
		return false, []string{"fingerprint_mismatch"}
	}

	av, aok = a.fingerprints["command_config_fingerprint"]
	bv, bok = b.fingerprints["command_config_fingerprint"]
	if aok != bok {
		return false, []string{"one_sided_fingerprint"}
	}
	if aok && av != bv {
		return false, []string{"fingerprint_mismatch"}
	}
	return true, nil
}

func sharesArtifactDomain(a, b readbackModel) bool {
	for artifactType := range a.artifactTypes {
		if b.artifactTypes[artifactType] {
			return true
		}
	}
	return false
}

func chooseTarget(model readbackModel, generalization string) ImprovementTarget {
	refs := []string{}
	for _, artifact := range model.artifacts {
		refs = append(refs, artifact.Ref)
	}
	refs = firstRefs(refs)
	if model.flags["unsafe_or_leaky"] {
		return ImprovementTarget{Code: "unsafe_or_leaky", Rationale: "Readback detected a denied private or secret-looking pattern in a supported artifact.", EvidenceRefs: refs}
	}
	if model.flags["scale_partial"] {
		return ImprovementTarget{Code: "scale_capacity", Rationale: "The run completed only as bounded partial evidence because one or more configured source, graph, or packet scale budgets were reached.", EvidenceRefs: refs}
	}
	if readiness := semanticReadinessForModel(model); readiness.Status == "blocked" {
		return ImprovementTarget{Code: "needs_semantic_density", Rationale: "Source intake may have succeeded, but semantic value is not proven because extraction collapsed into shallow reference output.", EvidenceRefs: refs}
	}
	if generalization != "generalizable" {
		return ImprovementTarget{Code: "needs_held_out_labels", Rationale: "The run is sample-bound, private, temp, unknown, or explicitly non-generalizable.", EvidenceRefs: refs}
	}
	if value, ok := model.metrics["artifact_match_coverage"]; ok && value < 1 {
		return ImprovementTarget{Code: "needs_source_enrichment", Rationale: "Artifact coverage is incomplete, so source meaning is still missing enrichment.", EvidenceRefs: refs}
	}
	if value, ok := model.metrics["evidence_ready_atom_ratio"]; ok && value < 1 {
		return ImprovementTarget{Code: "needs_evidence_readiness", Rationale: "Not all eval-counted atoms are evidence-ready.", EvidenceRefs: refs}
	}
	if evidenceReady, ok := model.metrics["evidence_ready_count"]; ok {
		if evalCounted, evalOK := model.metrics["eval_counted_count"]; evalOK && evidenceReady < evalCounted {
			return ImprovementTarget{Code: "needs_evidence_readiness", Rationale: "Not all eval-counted atoms are evidence-ready.", EvidenceRefs: refs}
		}
	}
	return ImprovementTarget{Code: "ready_for_next_pressure_run", Rationale: "No higher-priority readback blocker was found; rerun the next pressure/eval slice with comparable baseline.", EvidenceRefs: refs}
}

func semanticReadinessForModel(model readbackModel) SemanticReadiness {
	readiness := SemanticReadiness{
		ProcessedSourceCount:       intMetric(model, "processed_source_count", "processed_source_count_from_summaries", "source_count"),
		DocumentSegmentCount:       intMetric(model, "document_segment_count_from_summaries", "document_segment_count", "segment_count"),
		SemanticObservationCount:   intMetric(model, "semantic_observation_count", "semantic_observation_count_from_summaries", "observation_count"),
		SemanticCandidateCount:     intMetric(model, "semantic_candidate_count", "semantic_candidate_count_from_summaries", "candidate_count"),
		ReferenceCandidateCount:    intMetric(model, "reference_candidate_count_from_summaries", "reference_candidate_count"),
		OneCandidateSourceCount:    intMetric(model, "one_candidate_source_count", "one_candidate_source_count_from_summaries"),
		ReferenceOnlySourceCount:   intMetric(model, "reference_only_source_count", "reference_only_source_count_from_summaries"),
		CandidatePerSourceRatio:    model.metrics["candidate_per_processed_source_ratio"],
		ObservationPerSegmentRatio: model.metrics["observation_per_segment_ratio"],
		ReferenceCandidateRatio:    model.metrics["reference_candidate_ratio"],
	}
	if readiness.ProcessedSourceCount > 0 && readiness.CandidatePerSourceRatio == 0 {
		readiness.CandidatePerSourceRatio = float64(readiness.SemanticCandidateCount) / float64(readiness.ProcessedSourceCount)
	}
	if readiness.DocumentSegmentCount > 0 && readiness.ObservationPerSegmentRatio == 0 {
		readiness.ObservationPerSegmentRatio = float64(readiness.SemanticObservationCount) / float64(readiness.DocumentSegmentCount)
	}
	if readiness.SemanticCandidateCount > 0 && readiness.ReferenceCandidateRatio == 0 {
		readiness.ReferenceCandidateRatio = float64(readiness.ReferenceCandidateCount) / float64(readiness.SemanticCandidateCount)
	}
	if readiness.ProcessedSourceCount < 10 {
		readiness.Status = "not_evaluated"
		readiness.ReasonCodes = []string{"insufficient_processed_sources"}
		return readiness
	}
	hasReferenceCandidateCount := metricPresent(model, "reference_candidate_count_from_summaries", "reference_candidate_count")
	hasOneCandidateSourceCount := metricPresent(model, "one_candidate_source_count", "one_candidate_source_count_from_summaries")
	hasDocumentSegmentCount := metricPresent(model, "document_segment_count_from_summaries", "document_segment_count", "segment_count")
	if readiness.SemanticCandidateCount == readiness.ProcessedSourceCount &&
		hasReferenceCandidateCount &&
		hasOneCandidateSourceCount &&
		readiness.ReferenceCandidateCount == readiness.SemanticCandidateCount &&
		readiness.OneCandidateSourceCount == readiness.ProcessedSourceCount {
		readiness.ReasonCodes = append(readiness.ReasonCodes, "reference_only_one_candidate_per_source")
	}
	if readiness.DocumentSegmentCount > 0 && readiness.DocumentSegmentCount >= readiness.SemanticObservationCount*2 && readiness.ObservationPerSegmentRatio < 0.25 {
		readiness.ReasonCodes = append(readiness.ReasonCodes, "low_observation_to_segment_density")
	}
	if len(readiness.ReasonCodes) > 0 {
		readiness.Status = "blocked"
		return readiness
	}
	referenceCollapseEvaluated := readiness.SemanticCandidateCount != readiness.ProcessedSourceCount || (hasReferenceCandidateCount && hasOneCandidateSourceCount)
	lowDensityEvaluated := hasDocumentSegmentCount
	if !referenceCollapseEvaluated || !lowDensityEvaluated {
		readiness.Status = "not_evaluated"
		readiness.ReasonCodes = []string{"missing_semantic_density_counters"}
		if !referenceCollapseEvaluated {
			readiness.ReasonCodes = append(readiness.ReasonCodes, "missing_reference_collapse_counters")
		}
		if !lowDensityEvaluated {
			readiness.ReasonCodes = append(readiness.ReasonCodes, "missing_document_segment_count")
		}
		return readiness
	}
	readiness.Status = "ready"
	return readiness
}

func intMetric(model readbackModel, keys ...string) int {
	for _, key := range keys {
		if value, ok := model.metrics[key]; ok {
			return int(value)
		}
	}
	return 0
}

func metricPresent(model readbackModel, keys ...string) bool {
	for _, key := range keys {
		if _, ok := model.metrics[key]; ok {
			return true
		}
	}
	return false
}

func stringListContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func writeSummary(outRoot string, summary Summary, protectedRoots []string) error {
	if strings.TrimSpace(outRoot) == "" {
		return errors.New("missing output root")
	}
	root, err := filepath.Abs(outRoot)
	if err != nil {
		return err
	}
	if err := rejectSymlinkEscape(root, root, protectedRoots); err != nil {
		return err
	}
	if err := privateio.PrepareDir(root); err != nil {
		return err
	}
	dir := filepath.Join(root, DirName)
	if err := rejectSymlinkEscape(root, dir, protectedRoots); err != nil {
		return err
	}
	if err := privateio.PrepareDir(dir); err != nil {
		return err
	}
	if err := rejectSymlinkEscape(root, dir, protectedRoots); err != nil {
		return err
	}
	summaryPath := filepath.Join(dir, ReadbackSummaryFile)
	if err := rejectSymlinkEscape(root, summaryPath, protectedRoots); err != nil {
		return err
	}
	if err := writeJSON(summaryPath, summary); err != nil {
		return err
	}
	comparisonPath := filepath.Join(dir, "comparison-summary.json")
	if err := rejectSymlinkEscape(root, comparisonPath, protectedRoots); err != nil {
		return err
	}
	if summary.Comparison != nil {
		if err := writeJSON(comparisonPath, summary.Comparison); err != nil {
			return err
		}
	} else if err := removeIfExists(comparisonPath); err != nil {
		return err
	}
	reportPath := filepath.Join(dir, "readback-report.md")
	if err := rejectSymlinkEscape(root, reportPath, protectedRoots); err != nil {
		return err
	}
	if err := privateio.WriteFile(reportPath, []byte(markdownReport(summary)), false); err != nil {
		return err
	}
	chainPath := filepath.Join(dir, "chain-capture-draft.md")
	if err := rejectSymlinkEscape(root, chainPath, protectedRoots); err != nil {
		return err
	}
	if err := privateio.WriteFile(chainPath, []byte(chainDraft(summary)), false); err != nil {
		return err
	}
	return nil
}

func rejectSymlinkEscape(root, dir string, protectedRoots []string) error {
	realRoot, err := resolveOutputPath(root)
	if err != nil {
		return err
	}
	realDir := dir
	if _, err := os.Lstat(dir); err == nil {
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return err
		}
		realDir = resolved
	} else if os.IsNotExist(err) {
		rel, relErr := filepath.Rel(root, dir)
		if relErr != nil {
			return relErr
		}
		realDir = filepath.Join(realRoot, rel)
	} else {
		return err
	}
	realDir, err = filepath.Abs(realDir)
	if err != nil {
		return err
	}
	if !isSameOrInside(realRoot, realDir) {
		return fmt.Errorf("eval readback output escapes output root")
	}
	for _, protectedRoot := range protectedRoots {
		if strings.TrimSpace(protectedRoot) == "" {
			continue
		}
		realProtected, err := filepath.EvalSymlinks(protectedRoot)
		if err != nil {
			continue
		}
		realProtected, err = filepath.Abs(realProtected)
		if err != nil {
			continue
		}
		if isSameOrInside(realProtected, realDir) {
			return fmt.Errorf("protected output root")
		}
	}
	return nil
}

func resolveOutputPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Abs(resolved)
	}
	current := abs
	missing := []string{}
	for {
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("resolve output path: %s", path)
		}
		missing = append([]string{filepath.Base(current)}, missing...)
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			parts := append([]string{resolved}, missing...)
			return filepath.Abs(filepath.Join(parts...))
		}
		current = parent
	}
}

func isSameOrInside(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func writeJSON(path string, value any) error {
	return privateio.WriteJSON(path, value)
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func markdownReport(summary Summary) string {
	var b strings.Builder
	b.WriteString("# Eval Readback\n\n")
	b.WriteString(fmt.Sprintf("- Artifacts: %d\n", summary.ArtifactCount))
	b.WriteString(fmt.Sprintf("- Sample status: %s\n", summary.SampleStatus))
	b.WriteString(fmt.Sprintf("- Generalization: %s\n", summary.GeneralizationStatus))
	b.WriteString(fmt.Sprintf("- Improvement: %s\n\n", summary.ImprovementStatus))
	if summary.Comparison != nil {
		b.WriteString("## Comparison\n\n")
		b.WriteString(fmt.Sprintf("- Status: %s\n", summary.Comparison.Status))
		if len(summary.Comparison.ReasonCodes) > 0 {
			b.WriteString(fmt.Sprintf("- Reasons: %s\n", strings.Join(summary.Comparison.ReasonCodes, ", ")))
		}
		if len(summary.Comparison.MetricDeltas) > 0 {
			b.WriteString("- Metric deltas:\n")
			keys := make([]string, 0, len(summary.Comparison.MetricDeltas))
			for key := range summary.Comparison.MetricDeltas {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				b.WriteString(fmt.Sprintf("  - `%s`: %.4f\n", key, summary.Comparison.MetricDeltas[key]))
			}
		}
		b.WriteString("\n")
	} else {
		b.WriteString("## Comparison\n\nNo baseline was supplied, so improvement is not evaluated and the improvement claim remains blocked.\n\n")
	}
	b.WriteString("## Replay Baseline\n\n")
	b.WriteString(fmt.Sprintf("- Status: %s\n", summary.ReplayBaseline.Status))
	if len(summary.ReplayBaseline.ReasonCodes) > 0 {
		b.WriteString(fmt.Sprintf("- Reasons: %s\n", strings.Join(summary.ReplayBaseline.ReasonCodes, ", ")))
	}
	if summary.ReplayBaseline.CorpusFingerprint != "" {
		b.WriteString(fmt.Sprintf("- Corpus fingerprint: `%s`\n", summary.ReplayBaseline.CorpusFingerprint))
	}
	if summary.ReplayBaseline.CommandConfigFingerprint != "" {
		b.WriteString(fmt.Sprintf("- Command config fingerprint: `%s`\n", summary.ReplayBaseline.CommandConfigFingerprint))
	}
	if summary.ReplayBaseline.RerunInstruction != "" {
		b.WriteString(fmt.Sprintf("- Rerun: %s\n", summary.ReplayBaseline.RerunInstruction))
	}
	b.WriteString("\n")
	b.WriteString("## Semantic Readiness\n\n")
	b.WriteString(fmt.Sprintf("- Status: %s\n", summary.SemanticReadiness.Status))
	if len(summary.SemanticReadiness.ReasonCodes) > 0 {
		b.WriteString(fmt.Sprintf("- Reasons: %s\n", strings.Join(summary.SemanticReadiness.ReasonCodes, ", ")))
	}
	b.WriteString(fmt.Sprintf("- Density: %d observations / %d segments; %d candidates / %d processed sources; %d reference candidates\n", summary.SemanticReadiness.SemanticObservationCount, summary.SemanticReadiness.DocumentSegmentCount, summary.SemanticReadiness.SemanticCandidateCount, summary.SemanticReadiness.ProcessedSourceCount, summary.SemanticReadiness.ReferenceCandidateCount))
	if summary.SemanticReadiness.Status == "blocked" {
		b.WriteString("- Source intake may have succeeded, but semantic value is not proven.\n")
	}
	b.WriteString("\n")
	b.WriteString("## Top improvement target\n\n")
	b.WriteString(fmt.Sprintf("`%s`: %s\n\n", summary.TopImprovementTarget.Code, summary.TopImprovementTarget.Rationale))
	b.WriteString("## Claim gates\n\n")
	for _, gate := range summary.ClaimGates {
		b.WriteString(fmt.Sprintf("- `%s`: %s — %s\n", gate.Gate, gate.Status, gate.ClaimImpact))
	}
	b.WriteString("\n## Safe artifact refs\n\n")
	for _, ref := range summary.SafeArtifactRefs {
		b.WriteString(fmt.Sprintf("- `%s`\n", ref))
	}
	return b.String()
}

func chainDraft(summary Summary) string {
	var b strings.Builder
	b.WriteString("Mindline eval readback result: ")
	b.WriteString(summary.ImprovementStatus)
	b.WriteString(". Generalization: ")
	b.WriteString(summary.GeneralizationStatus)
	b.WriteString(". Replay baseline: ")
	b.WriteString(summary.ReplayBaseline.Status)
	if len(summary.ReplayBaseline.ReasonCodes) > 0 {
		b.WriteString(" (")
		b.WriteString(strings.Join(summary.ReplayBaseline.ReasonCodes, ", "))
		b.WriteString(")")
	}
	b.WriteString(". Semantic readiness: ")
	b.WriteString(summary.SemanticReadiness.Status)
	if len(summary.SemanticReadiness.ReasonCodes) > 0 {
		b.WriteString(" (")
		b.WriteString(strings.Join(summary.SemanticReadiness.ReasonCodes, ", "))
		b.WriteString(")")
	}
	b.WriteString(". Blocked claims: ")
	blocked := []string{}
	for _, gate := range summary.ClaimGates {
		if gate.Status == "blocked" || gate.Status == "fail" {
			blocked = append(blocked, gate.Gate)
		}
	}
	b.WriteString(strings.Join(blocked, ", "))
	b.WriteString(". Next target: ")
	b.WriteString(summary.TopImprovementTarget.Code)
	b.WriteString(". Proof refs: ")
	b.WriteString(strings.Join(firstRefs(summary.SafeArtifactRefs), ", "))
	b.WriteString(".")
	return b.String()
}

func sampleStatusFor(root string) string {
	clean := filepath.ToSlash(root)
	switch {
	case strings.Contains(clean, "/private/tmp/"):
		return "private_runtime"
	case strings.Contains(clean, "/temp/") || strings.HasSuffix(clean, "/temp"):
		return "temp_runtime"
	case strings.Contains(clean, "/testdata/"):
		return "fixture"
	default:
		return "unknown"
	}
}

func safeRootLabel(root string) string {
	return stableID("root", []string{filepath.ToSlash(root)})
}

func stableID(prefix string, parts []string) string {
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(prefix + ":" + strings.Join(parts, "|")))
	return strings.Trim(prefix, "-_ ") + "-" + hex.EncodeToString(sum[:])[:12]
}

func firstRefs(refs []string) []string {
	out := append([]string(nil), refs...)
	sort.Strings(out)
	if len(out) > 3 {
		return out[:3]
	}
	return out
}

func containsDeniedString(value string) bool {
	lower := strings.ToLower(value)
	denied := []string{"/private/tmp/", "/users/", "young human club dropbox", "slack.com/archives", "xoxb-", "xoxp-", "api_key=", "bearer ", "openai_api_key", "posthog_api_key"}
	for _, item := range denied {
		if strings.Contains(lower, item) {
			return true
		}
	}
	return containsSecretLikeSKToken(lower)
}

func containsSecretLikeSKToken(value string) bool {
	for start := 0; start < len(value); {
		idx := strings.Index(value[start:], "sk-")
		if idx < 0 {
			return false
		}
		idx += start
		if idx > 0 && isASCIIAlphaNumeric(value[idx-1]) {
			start = idx + len("sk-")
			continue
		}
		end := idx + len("sk-")
		for end < len(value) && isSecretTokenChar(value[end]) {
			end++
		}
		if end-idx >= 16 {
			return true
		}
		start = idx + len("sk-")
	}
	return false
}

func isSecretTokenChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_'
}

func isASCIIAlphaNumeric(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
}

func ContainsDeniedString(value string) bool {
	return containsDeniedString(value)
}

func containsDeniedRefString(ref string) bool {
	lower := strings.ToLower(filepath.ToSlash(ref))
	if strings.HasPrefix(lower, "users/") || containsDeniedString("/"+lower) {
		return true
	}
	return false
}

func sanitizedArtifactRef(ref string) string {
	return stableID("artifact-ref", []string{filepath.ToSlash(ref)}) + ".json"
}

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	default:
		return 0, false
	}
}

func boolValue(value any) (bool, bool) {
	typed, ok := value.(bool)
	return typed, ok
}

func stringValue(value any) string {
	typed, _ := value.(string)
	return strings.TrimSpace(typed)
}
