package documents

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	LinkEnrichmentLoopSummarySchemaVersion = "link-enrichment-loop-summary/v0.1"
	LinkArtifactRequestsSchemaVersion      = "local-link-artifact-requests/v0.1"
	LinkEnrichmentComparisonSchemaVersion  = "link-enrichment-comparison/v0.1"
	LinkEnrichmentDirName                  = "link-enrichment"
)

type LinkRequestState string

const (
	LinkRequestStateRequestable            LinkRequestState = "requestable"
	LinkRequestStateAlreadyArtifacted      LinkRequestState = "already_artifacted"
	LinkRequestStateUnsupported            LinkRequestState = "unsupported"
	LinkRequestStateBlockedPrivateOrSecret LinkRequestState = "blocked_private_or_secret"
	LinkRequestStateBlockedByPolicy        LinkRequestState = "blocked_by_policy"
)

type LinkEnrichmentVerdict string

const (
	LinkEnrichmentVerdictImproved  LinkEnrichmentVerdict = "improved"
	LinkEnrichmentVerdictUnchanged LinkEnrichmentVerdict = "unchanged"
	LinkEnrichmentVerdictBlocked   LinkEnrichmentVerdict = "blocked"
)

type LinkEnrichmentLoopOptions struct {
	SemanticOptions          SemanticOptions
	CommandConfigFingerprint string
}

type LinkEnrichmentLoopSummary struct {
	SchemaVersion        string                          `json:"schema_version"`
	CorpusID             string                          `json:"corpus_id"`
	InputPath            string                          `json:"input_path"`
	ResolvedManifestPath string                          `json:"resolved_manifest_path"`
	ArtifactsPath        string                          `json:"artifacts_path"`
	RequestSummary       LinkArtifactRequestSummary      `json:"request_summary"`
	Comparison           LinkEnrichmentComparisonSummary `json:"comparison"`
	Guardrails           LinkEnrichmentGuardrails        `json:"guardrails"`
	ArtifactPaths        map[string]string               `json:"artifact_paths"`
}

type LinkEnrichmentGuardrails struct {
	HostedInferenceCalls   int `json:"hosted_inference_calls"`
	HostedTelemetryExports int `json:"hosted_telemetry_exports"`
	NetworkFetches         int `json:"network_fetches"`
	BrowserCalls           int `json:"browser_calls"`
	SlackAPICalls          int `json:"slack_api_calls"`
	DestinationWrites      int `json:"destination_writes"`
	ProductBrainWrites     int `json:"product_brain_writes"`
	TolariaWrites          int `json:"tolaria_writes"`
}

type LinkArtifactRequestPack struct {
	SchemaVersion string                     `json:"schema_version"`
	CorpusID      string                     `json:"corpus_id"`
	Summary       LinkArtifactRequestSummary `json:"summary"`
	Requests      []LinkArtifactRequest      `json:"requests"`
}

type LinkArtifactRequestSummary struct {
	SourceCount             int                      `json:"source_count"`
	URLMentionCount         int                      `json:"url_mention_count"`
	UniqueURLCount          int                      `json:"unique_url_count"`
	AccountedURLCount       int                      `json:"accounted_url_count"`
	RequestableCount        int                      `json:"requestable_count"`
	AlreadyArtifactedCount  int                      `json:"already_artifacted_count"`
	UnsupportedCount        int                      `json:"unsupported_count"`
	BlockedPrivateCount     int                      `json:"blocked_private_or_secret_count"`
	BlockedPolicyCount      int                      `json:"blocked_by_policy_count"`
	SuppliedArtifactCount   int                      `json:"supplied_artifact_count"`
	MatchedArtifactCount    int                      `json:"matched_artifact_count"`
	StaleArtifactCount      int                      `json:"stale_artifact_count"`
	URLAccountingCoverage   float64                  `json:"url_accounting_coverage"`
	ArtifactMatchCoverage   float64                  `json:"artifact_match_coverage"`
	StateCounts             map[LinkRequestState]int `json:"state_counts"`
	RequestedArtifactFields []string                 `json:"requested_artifact_fields"`
	NonGeneralizableRuntime bool                     `json:"non_generalizable_runtime"`
}

type LinkArtifactRequest struct {
	RequestID               string           `json:"request_id"`
	SourceID                string           `json:"source_id"`
	SourceKind              string           `json:"source_kind"`
	SourceLabel             string           `json:"source_label"`
	RawURL                  string           `json:"raw_url,omitempty"`
	NormalizedURL           string           `json:"normalized_url"`
	Kind                    string           `json:"kind"`
	State                   LinkRequestState `json:"state"`
	ReasonCodes             []string         `json:"reason_codes,omitempty"`
	RequestedArtifactFields []string         `json:"requested_artifact_fields"`
	SafeForTopLevelReport   bool             `json:"safe_for_top_level_report"`
}

type LinkEnrichmentComparisonSummary struct {
	SchemaVersion                 string                                  `json:"schema_version"`
	CorpusID                      string                                  `json:"corpus_id"`
	Verdict                       LinkEnrichmentVerdict                   `json:"verdict"`
	ReasonCodes                   []string                                `json:"reason_codes,omitempty"`
	Comparable                    bool                                    `json:"comparable"`
	ComparableBasis               []string                                `json:"comparable_basis,omitempty"`
	BaselineCorpusFingerprint     string                                  `json:"baseline_corpus_fingerprint"`
	EnrichedCorpusFingerprint     string                                  `json:"enriched_corpus_fingerprint"`
	BaselineSourceSetFingerprint  string                                  `json:"baseline_source_set_fingerprint"`
	EnrichedSourceSetFingerprint  string                                  `json:"enriched_source_set_fingerprint"`
	BaselineConfigFingerprint     string                                  `json:"baseline_config_fingerprint"`
	EnrichedConfigFingerprint     string                                  `json:"enriched_config_fingerprint"`
	BaselineMissingnessCounts     map[SourceMeaningPreviewMissingness]int `json:"baseline_missingness_counts"`
	EnrichedMissingnessCounts     map[SourceMeaningPreviewMissingness]int `json:"enriched_missingness_counts"`
	MissingnessDeltas             map[SourceMeaningPreviewMissingness]int `json:"missingness_deltas"`
	BaselineRoutingCounts         map[SourceMeaningPreviewRoutingHint]int `json:"baseline_routing_counts"`
	EnrichedRoutingCounts         map[SourceMeaningPreviewRoutingHint]int `json:"enriched_routing_counts"`
	RoutingDeltas                 map[SourceMeaningPreviewRoutingHint]int `json:"routing_deltas"`
	BaselineCandidateKinds        map[SemanticCandidateKind]int           `json:"baseline_candidate_kind_counts"`
	EnrichedCandidateKinds        map[SemanticCandidateKind]int           `json:"enriched_candidate_kind_counts"`
	BaselineEvidenceCoverage      float64                                 `json:"baseline_evidence_coverage_ratio"`
	EnrichedEvidenceCoverage      float64                                 `json:"enriched_evidence_coverage_ratio"`
	BaselineReviewBurdenRatio     float64                                 `json:"baseline_review_burden_ratio"`
	EnrichedReviewBurdenRatio     float64                                 `json:"enriched_review_burden_ratio"`
	MissingLinkReductionRatio     float64                                 `json:"missing_link_enrichment_reduction_ratio"`
	NeedsEnrichmentReductionRatio float64                                 `json:"needs_enrichment_reduction_ratio"`
	RequestSummary                LinkArtifactRequestSummary              `json:"request_summary"`
	Guardrails                    LinkEnrichmentGuardrails                `json:"guardrails"`
}

func BuildLinkEnrichmentLoop(inputPath, artifactsPath, outDir string, options LinkEnrichmentLoopOptions) (LinkEnrichmentLoopSummary, error) {
	if strings.TrimSpace(outDir) == "" {
		return LinkEnrichmentLoopSummary{}, fmt.Errorf("missing required --out")
	}
	if strings.TrimSpace(artifactsPath) == "" {
		return LinkEnrichmentLoopSummary{}, fmt.Errorf("missing required --artifacts")
	}
	root, err := filepath.Abs(outDir)
	if err != nil {
		return LinkEnrichmentLoopSummary{}, err
	}
	if err := rejectSymlinkAncestors(root); err != nil {
		return LinkEnrichmentLoopSummary{}, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return LinkEnrichmentLoopSummary{}, ArtifactWriteError{Err: err}
	}
	manifestPath, err := resolveLinkEnrichmentManifestPath(inputPath, root)
	if err != nil {
		return LinkEnrichmentLoopSummary{}, err
	}
	manifest, manifestRoot, err := readSourceEnrichmentManifest(manifestPath)
	if err != nil {
		return LinkEnrichmentLoopSummary{}, err
	}
	artifactIndex, err := readLocalSourceEnrichmentArtifacts(artifactsPath)
	if err != nil {
		return LinkEnrichmentLoopSummary{}, err
	}
	requestPack, err := BuildLinkArtifactRequestPack(manifest, manifestRoot, artifactIndex, linkArtifactNormalizedURLs(artifactsPath))
	if err != nil {
		return LinkEnrichmentLoopSummary{}, err
	}
	requestPack.Summary.NonGeneralizableRuntime = linkEnrichmentNonGeneralizableRuntime(inputPath, manifestPath, artifactsPath)
	if err := writeJSON(root, filepath.ToSlash(filepath.Join(LinkEnrichmentDirName, "requests", "link-artifact-requests.json")), requestPack); err != nil {
		return LinkEnrichmentLoopSummary{}, ArtifactWriteError{Err: err}
	}
	if err := writeFile(root, filepath.ToSlash(filepath.Join(LinkEnrichmentDirName, "requests", "link-artifact-requests.md")), []byte(linkArtifactRequestReport(requestPack))); err != nil {
		return LinkEnrichmentLoopSummary{}, ArtifactWriteError{Err: err}
	}

	pressureOptions := CorpusPressureOptions{
		SemanticOptions:          options.SemanticOptions,
		CommandConfigFingerprint: options.CommandConfigFingerprint,
	}
	baselinePressure, _, err := BuildCorpusPressure(manifestPath, filepath.Join(root, "baseline-pressure"), pressureOptions)
	if err != nil {
		return LinkEnrichmentLoopSummary{}, err
	}
	baselineMeaning, _, err := BuildSourceMeaningPreview(filepath.Join(root, "baseline-pressure"), filepath.Join(root, "baseline-meaning"))
	if err != nil {
		return LinkEnrichmentLoopSummary{}, err
	}
	enrichment, err := BuildSourceEnrichment(manifestPath, artifactsPath, filepath.Join(root, "enrichment"))
	if err != nil {
		return LinkEnrichmentLoopSummary{}, err
	}
	enrichedPressure, _, err := BuildCorpusPressure(filepath.Join(root, "enrichment", "corpus-pressure-manifest.json"), filepath.Join(root, "enriched-pressure"), pressureOptions)
	if err != nil {
		return LinkEnrichmentLoopSummary{}, err
	}
	enrichedMeaning, _, err := BuildSourceMeaningPreview(filepath.Join(root, "enriched-pressure"), filepath.Join(root, "enriched-meaning"))
	if err != nil {
		return LinkEnrichmentLoopSummary{}, err
	}
	comparison := buildLinkEnrichmentComparison(manifest.CorpusID, requestPack.Summary, baselinePressure, enrichedPressure, baselineMeaning, enrichedMeaning, enrichment)
	if err := writeJSON(root, filepath.ToSlash(filepath.Join(LinkEnrichmentDirName, "comparison", "comparison-summary.json")), comparison); err != nil {
		return LinkEnrichmentLoopSummary{}, ArtifactWriteError{Err: err}
	}
	if err := writeFile(root, filepath.ToSlash(filepath.Join(LinkEnrichmentDirName, "comparison", "comparison-report.md")), []byte(linkEnrichmentComparisonReport(comparison))); err != nil {
		return LinkEnrichmentLoopSummary{}, ArtifactWriteError{Err: err}
	}
	summary := LinkEnrichmentLoopSummary{
		SchemaVersion:        LinkEnrichmentLoopSummarySchemaVersion,
		CorpusID:             manifest.CorpusID,
		InputPath:            filepath.ToSlash(inputPath),
		ResolvedManifestPath: filepath.ToSlash(manifestPath),
		ArtifactsPath:        filepath.ToSlash(artifactsPath),
		RequestSummary:       requestPack.Summary,
		Comparison:           comparison,
		Guardrails:           comparison.Guardrails,
		ArtifactPaths: map[string]string{
			"requests_json":     filepath.ToSlash(filepath.Join(LinkEnrichmentDirName, "requests", "link-artifact-requests.json")),
			"requests_report":   filepath.ToSlash(filepath.Join(LinkEnrichmentDirName, "requests", "link-artifact-requests.md")),
			"comparison_json":   filepath.ToSlash(filepath.Join(LinkEnrichmentDirName, "comparison", "comparison-summary.json")),
			"comparison_report": filepath.ToSlash(filepath.Join(LinkEnrichmentDirName, "comparison", "comparison-report.md")),
			"baseline_meaning":  "baseline-meaning",
			"enriched_meaning":  "enriched-meaning",
		},
	}
	if err := writeJSON(root, filepath.ToSlash(filepath.Join(LinkEnrichmentDirName, "loop-summary.json")), summary); err != nil {
		return LinkEnrichmentLoopSummary{}, ArtifactWriteError{Err: err}
	}
	return summary, nil
}

func linkEnrichmentNonGeneralizableRuntime(paths ...string) bool {
	for _, path := range paths {
		cleaned := filepath.Clean(strings.TrimSpace(path))
		if strings.HasPrefix(cleaned, "/private/tmp/") || strings.HasPrefix(cleaned, "/tmp/") {
			return true
		}
	}
	return false
}

func resolveLinkEnrichmentManifestPath(inputPath, outRoot string) (string, error) {
	if strings.TrimSpace(inputPath) == "" {
		return "", fmt.Errorf("missing link enrichment input")
	}
	info, err := os.Stat(inputPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		candidate := filepath.Join(inputPath, "corpus-pressure-manifest.json")
		if _, err := os.Stat(candidate); err != nil {
			generated, generateErr := generateLinkEnrichmentManifestFromPressure(inputPath, outRoot)
			if generateErr != nil {
				return "", fmt.Errorf("read corpus pressure manifest from input dir: %w", err)
			}
			return generated, nil
		}
		return candidate, nil
	}
	return inputPath, nil
}

func generateLinkEnrichmentManifestFromPressure(inputPath, outRoot string) (string, error) {
	pressureRoot, pressureSummary, err := readSourceMeaningPressureSummary(inputPath)
	if err != nil {
		return "", err
	}
	manifest := CorpusPressureManifest{
		SchemaVersion: CorpusPressureManifestSchemaVersion,
		CorpusID:      pressureSummary.CorpusID,
	}
	for _, source := range pressureSummary.Sources {
		if strings.TrimSpace(source.SourcePath) == "" {
			continue
		}
		sourcePath, err := containedManifestPath(pressureRoot, source.SourcePath)
		if err != nil {
			return "", err
		}
		sourceData, err := os.ReadFile(sourcePath)
		if err != nil {
			return "", fmt.Errorf("read corpus pressure source snapshot: %w", err)
		}
		sourceRel := filepath.ToSlash(filepath.Join(LinkEnrichmentDirName, "generated-input", "sources", sanitizeID(source.SourceID), "source.md"))
		if err := writeFile(outRoot, sourceRel, sourceData); err != nil {
			return "", ArtifactWriteError{Err: err}
		}
		manifest.Sources = append(manifest.Sources, CorpusPressureManifestSource{
			SourceID:   source.SourceID,
			SourceKind: source.SourceKind,
			Path:       filepath.ToSlash(filepath.Join("sources", sanitizeID(source.SourceID), "source.md")),
		})
	}
	if len(manifest.Sources) == 0 {
		return "", fmt.Errorf("cannot generate link enrichment manifest: no source paths in corpus pressure summary")
	}
	generatedRel := filepath.ToSlash(filepath.Join(LinkEnrichmentDirName, "generated-input", "corpus-pressure-manifest.json"))
	if err := writeJSON(outRoot, generatedRel, manifest); err != nil {
		return "", ArtifactWriteError{Err: err}
	}
	return filepath.Join(outRoot, filepath.FromSlash(generatedRel)), nil
}

func BuildLinkArtifactRequestPack(manifest CorpusPressureManifest, manifestRoot string, artifacts localArtifactIndex, suppliedArtifactURLs map[string]bool) (LinkArtifactRequestPack, error) {
	pack := LinkArtifactRequestPack{
		SchemaVersion: LinkArtifactRequestsSchemaVersion,
		CorpusID:      manifest.CorpusID,
		Summary: LinkArtifactRequestSummary{
			SourceCount:             len(manifest.Sources),
			StateCounts:             map[LinkRequestState]int{},
			RequestedArtifactFields: []string{"title", "source_name", "description", "excerpt", "captured_at"},
		},
	}
	matched := map[string]bool{}
	seenUnique := map[string]bool{}
	for _, source := range manifest.Sources {
		sourcePath, err := containedManifestPath(manifestRoot, source.Path)
		if err != nil {
			return LinkArtifactRequestPack{}, err
		}
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return LinkArtifactRequestPack{}, err
		}
		for _, urlMatch := range extractSourceEnrichmentURLs(string(data)) {
			request := buildLinkArtifactRequest(source, urlMatch, artifacts)
			pack.Requests = append(pack.Requests, request)
			pack.Summary.URLMentionCount++
			pack.Summary.AccountedURLCount++
			pack.Summary.StateCounts[request.State]++
			if request.NormalizedURL != "" && request.NormalizedURL != redactedSourceEnrichmentURL {
				seenUnique[request.NormalizedURL] = true
			}
			if request.State == LinkRequestStateAlreadyArtifacted {
				matched[request.NormalizedURL] = true
			}
		}
	}
	sort.Slice(pack.Requests, func(i, j int) bool {
		if pack.Requests[i].SourceID == pack.Requests[j].SourceID {
			return pack.Requests[i].NormalizedURL < pack.Requests[j].NormalizedURL
		}
		return pack.Requests[i].SourceID < pack.Requests[j].SourceID
	})
	pack.Summary.UniqueURLCount = len(seenUnique)
	pack.Summary.RequestableCount = pack.Summary.StateCounts[LinkRequestStateRequestable]
	pack.Summary.AlreadyArtifactedCount = pack.Summary.StateCounts[LinkRequestStateAlreadyArtifacted]
	pack.Summary.UnsupportedCount = pack.Summary.StateCounts[LinkRequestStateUnsupported]
	pack.Summary.BlockedPrivateCount = pack.Summary.StateCounts[LinkRequestStateBlockedPrivateOrSecret]
	pack.Summary.BlockedPolicyCount = pack.Summary.StateCounts[LinkRequestStateBlockedByPolicy]
	pack.Summary.SuppliedArtifactCount = len(suppliedArtifactURLs)
	pack.Summary.MatchedArtifactCount = len(matched)
	for supplied := range suppliedArtifactURLs {
		if !matched[supplied] {
			pack.Summary.StaleArtifactCount++
		}
	}
	if pack.Summary.URLMentionCount > 0 {
		pack.Summary.URLAccountingCoverage = float64(pack.Summary.AccountedURLCount) / float64(pack.Summary.URLMentionCount)
	}
	if pack.Summary.SuppliedArtifactCount > 0 {
		pack.Summary.ArtifactMatchCoverage = float64(pack.Summary.MatchedArtifactCount) / float64(pack.Summary.SuppliedArtifactCount)
	}
	return pack, nil
}

func buildLinkArtifactRequest(source CorpusPressureManifestSource, urlMatch sourceEnrichmentURLMatch, artifacts localArtifactIndex) LinkArtifactRequest {
	enriched := enrichSourceURL(urlMatch, artifacts)
	request := LinkArtifactRequest{
		RequestID:               "lreq-" + contentHash(strings.Join([]string{source.SourceID, enriched.NormalizedURL, urlMatch.rawURL}, "\n"))[:16],
		SourceID:                source.SourceID,
		SourceKind:              source.SourceKind,
		SourceLabel:             filepath.ToSlash(source.Path),
		NormalizedURL:           enriched.NormalizedURL,
		Kind:                    enriched.Kind,
		ReasonCodes:             enriched.ReasonCodes,
		RequestedArtifactFields: []string{"title", "source_name", "description", "excerpt", "captured_at"},
		SafeForTopLevelReport:   !sourceEnrichmentUnsafe(urlMatch.sourceToken) && enriched.RawURL != redactedSourceEnrichmentURL,
	}
	if request.SafeForTopLevelReport {
		request.RawURL = enriched.RawURL
	}
	switch enriched.State {
	case SourceEnrichmentStateEnriched:
		request.State = LinkRequestStateAlreadyArtifacted
	case SourceEnrichmentStateNeedsManualProcessing:
		request.State = LinkRequestStateRequestable
	case SourceEnrichmentStateUnsupportedSource:
		request.State = LinkRequestStateUnsupported
	case SourceEnrichmentStateBlockedPrivateOrSecret:
		request.State = LinkRequestStateBlockedPrivateOrSecret
	case SourceEnrichmentStateBlockedByPolicy:
		request.State = LinkRequestStateBlockedByPolicy
	default:
		request.State = LinkRequestStateRequestable
	}
	return request
}

func linkArtifactNormalizedURLs(path string) map[string]bool {
	out := map[string]bool{}
	manifest, err := readLocalSourceEnrichmentArtifactsRaw(path)
	if err != nil {
		return out
	}
	for _, artifact := range manifest.Artifacts {
		normalized, _, ok := classifySourceEnrichmentURL(artifact.URL)
		if ok {
			out[normalized] = true
		}
	}
	return out
}

func readLocalSourceEnrichmentArtifactsRaw(path string) (LocalSourceEnrichmentArtifactManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LocalSourceEnrichmentArtifactManifest{}, err
	}
	var manifest LocalSourceEnrichmentArtifactManifest
	if err := jsonUnmarshal(data, &manifest); err != nil {
		return LocalSourceEnrichmentArtifactManifest{}, err
	}
	if manifest.SchemaVersion != LocalSourceEnrichmentArtifactsSchemaVersion {
		return LocalSourceEnrichmentArtifactManifest{}, fmt.Errorf("unsupported local source enrichment artifacts schema version: %s", manifest.SchemaVersion)
	}
	return manifest, nil
}

func buildLinkEnrichmentComparison(corpusID string, requests LinkArtifactRequestSummary, baselinePressure, enrichedPressure CorpusPressureSummary, baselineMeaning, enrichedMeaning SourceMeaningPreviewSummary, enrichment SourceEnrichmentSummary) LinkEnrichmentComparisonSummary {
	baselineSourceSetFingerprint := linkEnrichmentSourceSetFingerprint(baselinePressure.Sources)
	enrichedSourceSetFingerprint := linkEnrichmentSourceSetFingerprint(enrichedPressure.Sources)
	comparable := baselinePressure.CorpusID == enrichedPressure.CorpusID &&
		baselinePressure.CommandConfigFingerprint == enrichedPressure.CommandConfigFingerprint &&
		baselineSourceSetFingerprint == enrichedSourceSetFingerprint
	guardrails := LinkEnrichmentGuardrails{
		HostedInferenceCalls:   baselinePressure.Guardrails.HostedInferenceCalls + enrichedPressure.Guardrails.HostedInferenceCalls + enrichment.Guardrails.HostedInferenceCalls,
		HostedTelemetryExports: baselinePressure.Guardrails.HostedTelemetryExports + enrichedPressure.Guardrails.HostedTelemetryExports + enrichment.Guardrails.HostedTelemetryExports,
		DestinationWrites:      baselinePressure.Guardrails.DestinationWrites + enrichedPressure.Guardrails.DestinationWrites + enrichment.Guardrails.DestinationWrites,
		ProductBrainWrites:     enrichment.Guardrails.ProductBrainWrites,
		TolariaWrites:          enrichment.Guardrails.TolariaWrites,
	}
	comparison := LinkEnrichmentComparisonSummary{
		SchemaVersion:                LinkEnrichmentComparisonSchemaVersion,
		CorpusID:                     corpusID,
		Comparable:                   comparable,
		BaselineCorpusFingerprint:    baselinePressure.CorpusFingerprint,
		EnrichedCorpusFingerprint:    enrichedPressure.CorpusFingerprint,
		BaselineSourceSetFingerprint: baselineSourceSetFingerprint,
		EnrichedSourceSetFingerprint: enrichedSourceSetFingerprint,
		BaselineConfigFingerprint:    baselinePressure.CommandConfigFingerprint,
		EnrichedConfigFingerprint:    enrichedPressure.CommandConfigFingerprint,
		BaselineMissingnessCounts:    baselineMeaning.MissingnessCounts,
		EnrichedMissingnessCounts:    enrichedMeaning.MissingnessCounts,
		MissingnessDeltas:            sourceMeaningMissingnessDeltas(baselineMeaning.MissingnessCounts, enrichedMeaning.MissingnessCounts),
		BaselineRoutingCounts:        baselineMeaning.RoutingHintCounts,
		EnrichedRoutingCounts:        enrichedMeaning.RoutingHintCounts,
		RoutingDeltas:                sourceMeaningRoutingDeltas(baselineMeaning.RoutingHintCounts, enrichedMeaning.RoutingHintCounts),
		BaselineCandidateKinds:       baselineMeaning.CandidateKindCounts,
		EnrichedCandidateKinds:       enrichedMeaning.CandidateKindCounts,
		BaselineEvidenceCoverage:     baselineMeaning.EvidenceCoverageRatio,
		EnrichedEvidenceCoverage:     enrichedMeaning.EvidenceCoverageRatio,
		BaselineReviewBurdenRatio:    baselinePressure.ReviewBurdenRatio,
		EnrichedReviewBurdenRatio:    enrichedPressure.ReviewBurdenRatio,
		RequestSummary:               requests,
		Guardrails:                   guardrails,
	}
	comparison.MissingLinkReductionRatio = reductionRatio(baselineMeaning.MissingnessCounts[SourceMeaningMissingnessMissingLinkEnrichment], enrichedMeaning.MissingnessCounts[SourceMeaningMissingnessMissingLinkEnrichment])
	comparison.NeedsEnrichmentReductionRatio = reductionRatio(baselineMeaning.RoutingHintCounts[SourceMeaningRoutingNeedsEnrichment], enrichedMeaning.RoutingHintCounts[SourceMeaningRoutingNeedsEnrichment])
	if comparison.Comparable {
		comparison.ComparableBasis = []string{"same_corpus_id", "same_source_set", "same_command_config", "local_enrichment_transform"}
	}
	switch {
	case !comparison.Comparable:
		comparison.Verdict = LinkEnrichmentVerdictBlocked
		comparison.ReasonCodes = []string{"not_comparable"}
	case guardrails.NetworkFetches != 0 || guardrails.BrowserCalls != 0 || guardrails.SlackAPICalls != 0 || guardrails.HostedInferenceCalls != 0 || guardrails.HostedTelemetryExports != 0 || guardrails.DestinationWrites != 0 || guardrails.ProductBrainWrites != 0 || guardrails.TolariaWrites != 0:
		comparison.Verdict = LinkEnrichmentVerdictBlocked
		comparison.ReasonCodes = []string{"guardrail_violation"}
	case comparison.MissingLinkReductionRatio > 0 || comparison.NeedsEnrichmentReductionRatio > 0:
		comparison.Verdict = LinkEnrichmentVerdictImproved
		comparison.ReasonCodes = []string{"missingness_reduced"}
	default:
		comparison.Verdict = LinkEnrichmentVerdictUnchanged
		comparison.ReasonCodes = []string{"no_missingness_movement"}
	}
	return comparison
}

func linkEnrichmentSourceSetFingerprint(sources []CorpusPressureSourceResult) string {
	rows := make([]string, 0, len(sources))
	for _, source := range sources {
		rows = append(rows, strings.Join([]string{source.SourceID, source.SourceKind, string(source.State)}, "\t"))
	}
	sort.Strings(rows)
	return "source-set-" + contentHash(strings.Join(rows, "\n"))
}

func sourceMeaningMissingnessDeltas(before, after map[SourceMeaningPreviewMissingness]int) map[SourceMeaningPreviewMissingness]int {
	out := map[SourceMeaningPreviewMissingness]int{}
	for key, value := range before {
		out[key] = after[key] - value
	}
	for key, value := range after {
		if _, ok := out[key]; !ok {
			out[key] = value
		}
	}
	return out
}

func sourceMeaningRoutingDeltas(before, after map[SourceMeaningPreviewRoutingHint]int) map[SourceMeaningPreviewRoutingHint]int {
	out := map[SourceMeaningPreviewRoutingHint]int{}
	for key, value := range before {
		out[key] = after[key] - value
	}
	for key, value := range after {
		if _, ok := out[key]; !ok {
			out[key] = value
		}
	}
	return out
}

func reductionRatio(before, after int) float64 {
	if before <= 0 {
		return 0
	}
	reduced := before - after
	if reduced <= 0 {
		return 0
	}
	return float64(reduced) / float64(before)
}
