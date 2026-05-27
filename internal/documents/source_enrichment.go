package documents

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	SourceEnrichmentSummarySchemaVersion        = "source-enrichment-summary/v0.1"
	LocalSourceEnrichmentArtifactsSchemaVersion = "local-source-enrichment-artifacts/v0.1"
	SourceEnrichmentDirName                     = "source-enrichment"
)

type SourceEnrichmentState string

const (
	SourceEnrichmentStateEnriched               SourceEnrichmentState = "enriched"
	SourceEnrichmentStateNeedsManualProcessing  SourceEnrichmentState = "needs_manual_processing"
	SourceEnrichmentStateUnsupportedSource      SourceEnrichmentState = "unsupported_source"
	SourceEnrichmentStateBlockedPrivateOrSecret SourceEnrichmentState = "blocked_private_or_secret"
	SourceEnrichmentStateBlockedByPolicy        SourceEnrichmentState = "blocked_by_policy"
	SourceEnrichmentStateNoURL                  SourceEnrichmentState = "no_url"
)

type SourceEnrichmentRetrievalMode string

const (
	SourceEnrichmentRetrievalLocalArtifact SourceEnrichmentRetrievalMode = "local_artifact"
	SourceEnrichmentRetrievalNone          SourceEnrichmentRetrievalMode = "none"
)

type SourceEnrichmentGuardrails struct {
	HostedInferenceCalls   int `json:"hosted_inference_calls"`
	HostedTelemetryExports int `json:"hosted_telemetry_exports"`
	DestinationWrites      int `json:"destination_writes"`
	ProductBrainWrites     int `json:"product_brain_writes"`
	TolariaWrites          int `json:"tolaria_writes"`
}

type LocalSourceEnrichmentArtifactManifest struct {
	SchemaVersion string                          `json:"schema_version"`
	Artifacts     []LocalSourceEnrichmentArtifact `json:"artifacts"`
}

type LocalSourceEnrichmentArtifact struct {
	URL         string `json:"url"`
	Kind        string `json:"kind,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Excerpt     string `json:"excerpt,omitempty"`
	SourceName  string `json:"source_name,omitempty"`
	CapturedAt  string `json:"captured_at,omitempty"`
}

type SourceEnrichmentSummary struct {
	SchemaVersion            string                          `json:"schema_version"`
	CorpusID                 string                          `json:"corpus_id"`
	InputManifestPath        string                          `json:"input_manifest_path"`
	OutputManifestPath       string                          `json:"output_manifest_path"`
	SourceCount              int                             `json:"source_count"`
	URLCount                 int                             `json:"url_count"`
	AccountedURLCount        int                             `json:"accounted_url_count"`
	EnrichedURLCount         int                             `json:"enriched_url_count"`
	NeedsManualURLCount      int                             `json:"needs_manual_url_count"`
	UnsupportedURLCount      int                             `json:"unsupported_url_count"`
	BlockedURLCount          int                             `json:"blocked_url_count"`
	NoURLSourceCount         int                             `json:"no_url_source_count"`
	URLAccountingCoverage    float64                         `json:"url_accounting_coverage"`
	EnrichedArtifactCoverage float64                         `json:"enriched_artifact_coverage"`
	StateCounts              map[SourceEnrichmentState]int   `json:"state_counts"`
	Guardrails               SourceEnrichmentGuardrails      `json:"guardrails"`
	ReportPath               string                          `json:"report_path"`
	Sources                  []SourceEnrichmentSourceSummary `json:"sources"`
}

type SourceEnrichmentSourceSummary struct {
	SourceID         string                        `json:"source_id"`
	SourceKind       string                        `json:"source_kind"`
	InputPath        string                        `json:"input_path"`
	OutputPath       string                        `json:"output_path"`
	State            SourceEnrichmentState         `json:"state"`
	ReasonCodes      []string                      `json:"reason_codes,omitempty"`
	URLCount         int                           `json:"url_count"`
	EnrichedURLCount int                           `json:"enriched_url_count"`
	BlockedURLCount  int                           `json:"blocked_url_count"`
	ArtifactPath     string                        `json:"artifact_path"`
	URLStates        map[SourceEnrichmentState]int `json:"url_state_counts,omitempty"`
}

type SourceEnrichmentSourceArtifact struct {
	SchemaVersion string                `json:"schema_version"`
	SourceID      string                `json:"source_id"`
	SourceKind    string                `json:"source_kind"`
	InputPath     string                `json:"input_path"`
	OutputPath    string                `json:"output_path"`
	State         SourceEnrichmentState `json:"state"`
	ReasonCodes   []string              `json:"reason_codes,omitempty"`
	URLs          []SourceEnrichmentURL `json:"urls"`
}

type SourceEnrichmentURL struct {
	RawURL        string                        `json:"raw_url"`
	NormalizedURL string                        `json:"normalized_url"`
	Kind          string                        `json:"kind"`
	State         SourceEnrichmentState         `json:"state"`
	RetrievalMode SourceEnrichmentRetrievalMode `json:"retrieval_mode"`
	ReasonCodes   []string                      `json:"reason_codes,omitempty"`
	Title         string                        `json:"title,omitempty"`
	Description   string                        `json:"description,omitempty"`
	Excerpt       string                        `json:"excerpt,omitempty"`
	SourceName    string                        `json:"source_name,omitempty"`
	CapturedAt    string                        `json:"captured_at,omitempty"`
	ContentHash   string                        `json:"content_hash,omitempty"`
}

type localArtifactIndex struct {
	byURL map[string]LocalSourceEnrichmentArtifact
}

type sourceEnrichmentURLMatch struct {
	rawURL      string
	sourceToken string
}

var sourceEnrichmentURLPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s<>"')\]|]+`)

const redactedSourceEnrichmentURL = "[redacted blocked url]"

func BuildSourceEnrichment(manifestPath, artifactManifestPath, outDir string) (SourceEnrichmentSummary, error) {
	if strings.TrimSpace(outDir) == "" {
		return SourceEnrichmentSummary{}, fmt.Errorf("missing required --out")
	}
	if strings.TrimSpace(artifactManifestPath) == "" {
		return SourceEnrichmentSummary{}, fmt.Errorf("missing required --artifacts")
	}
	root, err := filepath.Abs(outDir)
	if err != nil {
		return SourceEnrichmentSummary{}, err
	}
	if err := rejectSymlinkAncestors(root); err != nil {
		return SourceEnrichmentSummary{}, err
	}
	if err := rejectIfSymlink(root); err != nil {
		return SourceEnrichmentSummary{}, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return SourceEnrichmentSummary{}, ArtifactWriteError{Err: err}
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return SourceEnrichmentSummary{}, err
	}
	manifest, manifestRoot, err := readSourceEnrichmentManifest(manifestPath)
	if err != nil {
		return SourceEnrichmentSummary{}, err
	}
	artifactIndex, err := readLocalSourceEnrichmentArtifacts(artifactManifestPath)
	if err != nil {
		return SourceEnrichmentSummary{}, err
	}
	summary := SourceEnrichmentSummary{
		SchemaVersion:      SourceEnrichmentSummarySchemaVersion,
		CorpusID:           manifest.CorpusID,
		InputManifestPath:  filepath.ToSlash(manifestPath),
		OutputManifestPath: "corpus-pressure-manifest.json",
		StateCounts:        map[SourceEnrichmentState]int{},
		ReportPath:         filepath.ToSlash(filepath.Join(SourceEnrichmentDirName, "enrichment-report.md")),
		Guardrails:         SourceEnrichmentGuardrails{},
	}
	outputManifest := CorpusPressureManifest{
		SchemaVersion: CorpusPressureManifestSchemaVersion,
		CorpusID:      manifest.CorpusID,
	}
	var sourceArtifacts []SourceEnrichmentSourceArtifact
	for _, source := range manifest.Sources {
		sourceSummary, sourceArtifact, sourceContent, err := enrichManifestSource(manifestRoot, realRoot, source, artifactIndex)
		if err != nil {
			return SourceEnrichmentSummary{}, err
		}
		if err := writeFile(realRoot, sourceSummary.OutputPath, []byte(sourceContent)); err != nil {
			return SourceEnrichmentSummary{}, ArtifactWriteError{Err: err}
		}
		if err := writeJSON(realRoot, sourceSummary.ArtifactPath, sourceArtifact); err != nil {
			return SourceEnrichmentSummary{}, ArtifactWriteError{Err: err}
		}
		outputManifest.Sources = append(outputManifest.Sources, CorpusPressureManifestSource{
			SourceID:   source.SourceID,
			SourceKind: source.SourceKind,
			Path:       sourceSummary.OutputPath,
		})
		summary.Sources = append(summary.Sources, sourceSummary)
		sourceArtifacts = append(sourceArtifacts, sourceArtifact)
		addSourceEnrichmentSummaryCounts(&summary, sourceSummary)
	}
	summary.SourceCount = len(summary.Sources)
	if summary.URLCount > 0 {
		summary.URLAccountingCoverage = float64(summary.AccountedURLCount) / float64(summary.URLCount)
	}
	if summary.EnrichedURLCount > 0 {
		summary.EnrichedArtifactCoverage = 1
	}
	if err := writeJSON(realRoot, "corpus-pressure-manifest.json", outputManifest); err != nil {
		return SourceEnrichmentSummary{}, ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, filepath.ToSlash(filepath.Join(SourceEnrichmentDirName, "enrichment-summary.json")), summary); err != nil {
		return SourceEnrichmentSummary{}, ArtifactWriteError{Err: err}
	}
	if err := writeFile(realRoot, filepath.ToSlash(filepath.Join(SourceEnrichmentDirName, "enrichment-report.md")), []byte(sourceEnrichmentReport(summary, sourceArtifacts))); err != nil {
		return SourceEnrichmentSummary{}, ArtifactWriteError{Err: err}
	}
	return summary, nil
}

func readSourceEnrichmentManifest(path string) (CorpusPressureManifest, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CorpusPressureManifest{}, "", err
	}
	var manifest CorpusPressureManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return CorpusPressureManifest{}, "", err
	}
	if manifest.SchemaVersion != CorpusPressureManifestSchemaVersion {
		return CorpusPressureManifest{}, "", fmt.Errorf("unsupported corpus pressure manifest schema version: %s", manifest.SchemaVersion)
	}
	if strings.TrimSpace(manifest.CorpusID) == "" || sanitizeID(manifest.CorpusID) != manifest.CorpusID {
		return CorpusPressureManifest{}, "", fmt.Errorf("unsafe corpus id: %s", manifest.CorpusID)
	}
	if len(manifest.Sources) == 0 {
		return CorpusPressureManifest{}, "", fmt.Errorf("corpus pressure manifest requires sources")
	}
	seen := map[string]bool{}
	for _, source := range manifest.Sources {
		if strings.TrimSpace(source.SourceID) == "" || sanitizeID(source.SourceID) != source.SourceID {
			return CorpusPressureManifest{}, "", fmt.Errorf("unsafe source id: %s", source.SourceID)
		}
		if seen[source.SourceID] {
			return CorpusPressureManifest{}, "", fmt.Errorf("duplicate source id: %s", source.SourceID)
		}
		seen[source.SourceID] = true
		if source.SourceKind != SourceKindMarkdown {
			return CorpusPressureManifest{}, "", fmt.Errorf("unsupported source kind: %s", source.SourceKind)
		}
	}
	return manifest, filepath.Dir(path), nil
}

func readLocalSourceEnrichmentArtifacts(path string) (localArtifactIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return localArtifactIndex{}, err
	}
	var manifest LocalSourceEnrichmentArtifactManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return localArtifactIndex{}, err
	}
	if manifest.SchemaVersion != LocalSourceEnrichmentArtifactsSchemaVersion {
		return localArtifactIndex{}, fmt.Errorf("unsupported local source enrichment artifacts schema version: %s", manifest.SchemaVersion)
	}
	index := localArtifactIndex{byURL: map[string]LocalSourceEnrichmentArtifact{}}
	for _, artifact := range manifest.Artifacts {
		normalized, _, ok := classifySourceEnrichmentURL(artifact.URL)
		if !ok {
			continue
		}
		if _, exists := index.byURL[normalized]; !exists {
			index.byURL[normalized] = artifact
		}
	}
	return index, nil
}

func enrichManifestSource(manifestRoot, outRoot string, source CorpusPressureManifestSource, artifacts localArtifactIndex) (SourceEnrichmentSourceSummary, SourceEnrichmentSourceArtifact, string, error) {
	sourcePath, err := containedManifestPath(manifestRoot, source.Path)
	if err != nil {
		return SourceEnrichmentSourceSummary{}, SourceEnrichmentSourceArtifact{}, "", fmt.Errorf("source path %s: %w", source.SourceID, err)
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return SourceEnrichmentSourceSummary{}, SourceEnrichmentSourceArtifact{}, "", err
	}
	body := string(data)
	outputBody := body
	urlMatches := extractSourceEnrichmentURLs(body)
	outputPath := filepath.ToSlash(filepath.Join("sources", sanitizeID(source.SourceID), "source.md"))
	artifactPath := filepath.ToSlash(filepath.Join(SourceEnrichmentDirName, "sources", sanitizeID(source.SourceID)+".json"))
	sourceArtifact := SourceEnrichmentSourceArtifact{
		SchemaVersion: SourceEnrichmentSummarySchemaVersion,
		SourceID:      source.SourceID,
		SourceKind:    source.SourceKind,
		InputPath:     filepath.ToSlash(source.Path),
		OutputPath:    outputPath,
		URLs:          []SourceEnrichmentURL{},
	}
	sourceSummary := SourceEnrichmentSourceSummary{
		SourceID:     source.SourceID,
		SourceKind:   source.SourceKind,
		InputPath:    filepath.ToSlash(source.Path),
		OutputPath:   outputPath,
		ArtifactPath: artifactPath,
		URLStates:    map[SourceEnrichmentState]int{},
	}
	if len(urlMatches) == 0 {
		sourceSummary.State = SourceEnrichmentStateNoURL
		sourceArtifact.State = SourceEnrichmentStateNoURL
		sourceArtifact.ReasonCodes = []string{"no_url"}
		sourceSummary.ReasonCodes = []string{"no_url"}
		return sourceSummary, sourceArtifact, body, nil
	}
	for _, urlMatch := range urlMatches {
		enrichedURL := enrichSourceURL(urlMatch, artifacts)
		sourceArtifact.URLs = append(sourceArtifact.URLs, enrichedURL)
		if sourceEnrichmentBlockedState(enrichedURL.State) {
			outputBody = strings.ReplaceAll(outputBody, urlMatch.sourceToken, redactedSourceEnrichmentURL)
		}
		sourceSummary.URLCount++
		sourceSummary.URLStates[enrichedURL.State]++
		switch enrichedURL.State {
		case SourceEnrichmentStateEnriched:
			sourceSummary.EnrichedURLCount++
		case SourceEnrichmentStateBlockedByPolicy, SourceEnrichmentStateBlockedPrivateOrSecret:
			sourceSummary.BlockedURLCount++
		}
		sourceSummary.ReasonCodes = appendUniqueStrings(sourceSummary.ReasonCodes, enrichedURL.ReasonCodes...)
	}
	sourceSummary.State = sourceEnrichmentSourceState(sourceArtifact.URLs)
	sourceArtifact.State = sourceSummary.State
	sourceArtifact.ReasonCodes = sourceSummary.ReasonCodes
	return sourceSummary, sourceArtifact, appendSourceEnrichmentMarkdown(outputBody, sourceArtifact), ensureSourceEnrichmentOutputPath(outRoot, outputPath)
}

func ensureSourceEnrichmentOutputPath(root, rel string) error {
	if filepath.IsAbs(rel) || strings.Contains(filepath.ToSlash(rel), "../") {
		return fmt.Errorf("output path escaped output directory")
	}
	target := filepath.Join(root, filepath.FromSlash(rel))
	if !isInside(root, target) {
		return fmt.Errorf("output path escaped output directory")
	}
	return nil
}

func enrichSourceURL(urlMatch sourceEnrichmentURLMatch, artifacts localArtifactIndex) SourceEnrichmentURL {
	if sourceEnrichmentUnsafe(urlMatch.sourceToken) {
		return SourceEnrichmentURL{
			RawURL:        redactedSourceEnrichmentURL,
			NormalizedURL: redactedSourceEnrichmentURL,
			Kind:          sourceEnrichmentURLKindFromRaw(urlMatch.rawURL),
			State:         SourceEnrichmentStateBlockedPrivateOrSecret,
			RetrievalMode: SourceEnrichmentRetrievalNone,
			ReasonCodes:   []string{"unsafe_or_private_url"},
		}
	}
	normalized, kind, policyOK := classifySourceEnrichmentURL(urlMatch.rawURL)
	record := SourceEnrichmentURL{
		RawURL:        urlMatch.rawURL,
		NormalizedURL: normalized,
		Kind:          kind,
		RetrievalMode: SourceEnrichmentRetrievalNone,
	}
	if !policyOK {
		record.RawURL = redactedSourceEnrichmentURL
		record.NormalizedURL = redactedSourceEnrichmentURL
		record.State = SourceEnrichmentStateBlockedByPolicy
		record.ReasonCodes = []string{"url_policy_blocked"}
		return record
	}
	if kind == "unknown" {
		record.State = SourceEnrichmentStateUnsupportedSource
		record.ReasonCodes = []string{"unsupported_source"}
		return record
	}
	artifact, ok := artifacts.byURL[normalized]
	if !ok {
		record.State = SourceEnrichmentStateNeedsManualProcessing
		record.ReasonCodes = []string{"missing_local_artifact"}
		return record
	}
	record.RetrievalMode = SourceEnrichmentRetrievalLocalArtifact
	if sourceEnrichmentUnsafeArtifact(artifact) {
		record.State = SourceEnrichmentStateBlockedPrivateOrSecret
		record.ReasonCodes = []string{"unsafe_artifact_payload"}
		return record
	}
	record.State = SourceEnrichmentStateEnriched
	record.ReasonCodes = []string{"local_artifact_matched"}
	record.Title = strings.TrimSpace(artifact.Title)
	record.Description = strings.TrimSpace(artifact.Description)
	record.Excerpt = strings.TrimSpace(artifact.Excerpt)
	record.SourceName = strings.TrimSpace(artifact.SourceName)
	record.CapturedAt = strings.TrimSpace(artifact.CapturedAt)
	record.ContentHash = "sha256:" + contentHash(strings.Join([]string{record.NormalizedURL, record.Title, record.Description, record.Excerpt, record.SourceName}, "\n"))
	return record
}

func extractSourceEnrichmentURLs(value string) []sourceEnrichmentURLMatch {
	seen := map[string]bool{}
	var out []sourceEnrichmentURLMatch
	for _, loc := range sourceEnrichmentURLPattern.FindAllStringIndex(value, -1) {
		match := value[loc[0]:loc[1]]
		clean := strings.TrimRight(match, ".,;:")
		token := clean
		if loc[0] > 0 && value[loc[0]-1] == '<' {
			if closeOffset := strings.IndexByte(value[loc[1]:], '>'); closeOffset >= 0 {
				label := value[loc[1] : loc[1]+closeOffset]
				if strings.HasPrefix(label, "|") {
					token = value[loc[0]-1 : loc[1]+closeOffset+1]
				}
			}
		}
		if seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, sourceEnrichmentURLMatch{
			rawURL:      clean,
			sourceToken: token,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].rawURL == out[j].rawURL {
			return out[i].sourceToken < out[j].sourceToken
		}
		return out[i].rawURL < out[j].rawURL
	})
	return out
}

func classifySourceEnrichmentURL(rawURL string) (string, string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "unknown", false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "unknown", false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" || sourceEnrichmentBlockedHost(host) {
		return "", sourceEnrichmentURLKind(parsed), false
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	normalized := strings.TrimRight(parsed.String(), "/")
	return normalized, sourceEnrichmentURLKind(parsed), true
}

func sourceEnrichmentURLKind(parsed *url.URL) string {
	host := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(parsed.Path)
	switch {
	case strings.Contains(host, "youtube.com") || strings.Contains(host, "youtu.be"):
		return "youtube_url"
	case strings.Contains(host, "linkedin.com"):
		return "linkedin_url"
	case strings.HasSuffix(path, ".pdf"):
		return "pdf_url"
	case sourceEnrichmentUnsupportedExtension(path):
		return "unknown"
	case parsed.Scheme == "http" || parsed.Scheme == "https":
		return "web_url"
	default:
		return "unknown"
	}
}

func sourceEnrichmentURLKindFromRaw(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "unknown"
	}
	return sourceEnrichmentURLKind(parsed)
}

func sourceEnrichmentUnsupportedExtension(path string) bool {
	for _, suffix := range []string{".zip", ".exe", ".dmg", ".pkg"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

func sourceEnrichmentBlockedState(state SourceEnrichmentState) bool {
	return state == SourceEnrichmentStateBlockedByPolicy || state == SourceEnrichmentStateBlockedPrivateOrSecret
}

func sourceEnrichmentBlockedHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || host == "169.254.169.254"
}

func sourceEnrichmentUnsafe(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "slack-file-private://") ||
		strings.Contains(lower, "files-pri") ||
		strings.Contains(lower, "workspace.slack.com/archives") ||
		strings.Contains(lower, "xoxb-") ||
		strings.Contains(lower, "sk_live_") ||
		strings.Contains(lower, "sk-proj-") ||
		containsUnsafeMarker(value) ||
		containsGovernanceID(value)
}

func sourceEnrichmentUnsafeArtifact(artifact LocalSourceEnrichmentArtifact) bool {
	return sourceEnrichmentUnsafe(strings.Join([]string{artifact.URL, artifact.Title, artifact.Description, artifact.Excerpt, artifact.SourceName}, "\n"))
}

func sourceEnrichmentSourceState(urls []SourceEnrichmentURL) SourceEnrichmentState {
	if len(urls) == 0 {
		return SourceEnrichmentStateNoURL
	}
	hasEnriched := false
	hasManual := false
	hasUnsupported := false
	hasBlockedPrivate := false
	hasBlockedPolicy := false
	for _, item := range urls {
		switch item.State {
		case SourceEnrichmentStateEnriched:
			hasEnriched = true
		case SourceEnrichmentStateNeedsManualProcessing:
			hasManual = true
		case SourceEnrichmentStateUnsupportedSource:
			hasUnsupported = true
		case SourceEnrichmentStateBlockedPrivateOrSecret:
			hasBlockedPrivate = true
		case SourceEnrichmentStateBlockedByPolicy:
			hasBlockedPolicy = true
		}
	}
	switch {
	case hasEnriched:
		return SourceEnrichmentStateEnriched
	case hasManual:
		return SourceEnrichmentStateNeedsManualProcessing
	case hasUnsupported:
		return SourceEnrichmentStateUnsupportedSource
	case hasBlockedPrivate:
		return SourceEnrichmentStateBlockedPrivateOrSecret
	case hasBlockedPolicy:
		return SourceEnrichmentStateBlockedByPolicy
	default:
		return SourceEnrichmentStateNeedsManualProcessing
	}
}

func addSourceEnrichmentSummaryCounts(summary *SourceEnrichmentSummary, source SourceEnrichmentSourceSummary) {
	summary.StateCounts[source.State]++
	if source.State == SourceEnrichmentStateNoURL {
		summary.NoURLSourceCount++
	}
	summary.URLCount += source.URLCount
	for state, count := range source.URLStates {
		summary.AccountedURLCount += count
		switch state {
		case SourceEnrichmentStateEnriched:
			summary.EnrichedURLCount += count
		case SourceEnrichmentStateNeedsManualProcessing:
			summary.NeedsManualURLCount += count
		case SourceEnrichmentStateUnsupportedSource:
			summary.UnsupportedURLCount += count
		case SourceEnrichmentStateBlockedByPolicy, SourceEnrichmentStateBlockedPrivateOrSecret:
			summary.BlockedURLCount += count
		}
	}
}

func appendUniqueStrings(values []string, additions ...string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additions {
		if strings.TrimSpace(value) == "" || seen[value] {
			continue
		}
		values = append(values, value)
		seen[value] = true
	}
	sort.Strings(values)
	return values
}
