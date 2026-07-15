package processing

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/retrieval"
)

type EvidenceMatcher struct{}

func (EvidenceMatcher) Process(request Request) (Result, error) {
	if err := ValidateStrategy(request.Strategy); err != nil {
		return Result{}, err
	}
	if err := retrieval.ValidateArtifact(request.Retrieval); err != nil {
		return Result{}, err
	}
	if request.Retrieval.Origin == retrieval.OriginLiveRetrieval {
		return Result{}, retrieval.ErrLiveTransportDisabled
	}
	if request.Item.CanonicalItemID != request.Retrieval.CanonicalItemID || request.Item.CanonicalURL != request.Retrieval.CanonicalURL || request.Item.RetrievalStrategy != request.Retrieval.Strategy || request.Item.Format != request.Retrieval.Format {
		return Result{}, errors.New("processing input identity mismatch")
	}

	evidenceRefs := make([]string, 0, len(request.Retrieval.Excerpts))
	var evidenceText strings.Builder
	evidenceText.WriteString(request.Retrieval.Metadata.Title)
	evidenceText.WriteByte(' ')
	evidenceText.WriteString(request.Retrieval.Metadata.Author)
	for _, excerpt := range request.Retrieval.Excerpts {
		evidenceRefs = append(evidenceRefs, excerpt.ExcerptID)
		evidenceText.WriteByte(' ')
		evidenceText.WriteString(excerpt.Text)
	}
	tokens := tokenize(evidenceText.String())
	includeMatches := matchingTerms(tokens, request.Strategy.IncludeTerms)
	excludeMatches := matchingTerms(tokens, request.Strategy.ExcludeTerms)
	manual, reasons := manualOutcome(request.Retrieval)
	lenses := ContextLenses(request.Strategy)
	judgment := Judgment{}
	matchedLensIDs := []string{}
	for _, lens := range lenses {
		lensID := stableSlug(lens)
		if manual {
			judgment.LensResults = append(judgment.LensResults, LensResult{LensID: lensID, Result: "unknown", Confidence: 1, Rationale: "The source requires manual review before relevance can be judged.", Missingness: append([]string(nil), reasons...)})
			continue
		}
		lensTerms := tokenize(lens)
		matchesLens := overlap(tokens, lensTerms) || len(includeMatches) > 0
		if len(excludeMatches) > 0 {
			judgment.LensResults = append(judgment.LensResults, LensResult{LensID: lensID, Result: "not_matched", Confidence: 1, Rationale: "An explicit strategy exclusion term matched the public evidence.", EvidenceRefs: append([]string(nil), evidenceRefs...)})
		} else if matchesLens {
			matchedLensIDs = append(matchedLensIDs, lensID)
			judgment.LensResults = append(judgment.LensResults, LensResult{LensID: lensID, Result: "matched", Confidence: .8, Rationale: "Pinned strategy terms overlap the public evidence.", EvidenceRefs: append([]string(nil), evidenceRefs...)})
		} else {
			judgment.LensResults = append(judgment.LensResults, LensResult{LensID: lensID, Result: "not_matched", Confidence: .8, Rationale: "No pinned strategy term overlaps the public evidence.", EvidenceRefs: append([]string(nil), evidenceRefs...)})
		}
	}

	if manual {
		judgment.SemanticAssessment = SemanticAssessment{PrimaryRole: "unknown", Confidence: 1, Missingness: append([]string(nil), reasons...)}
		judgment.Disposition = "hold"
		judgment.DispositionRationale = "Manual review is required because retrieval is incomplete, private, authenticated, unsupported, or secret-like."
	} else if len(matchedLensIDs) == 0 || len(excludeMatches) > 0 {
		judgment.SemanticAssessment = SemanticAssessment{PrimaryRole: "reference_resource", Summary: boundedSummary(request.Retrieval), Confidence: .8, EvidenceRefs: firstEvidence(evidenceRefs)}
		judgment.Disposition = "hold"
		judgment.DispositionRationale = "The source has public evidence but does not satisfy the pinned relevance strategy."
	} else {
		role, roleComplete := proposeRole(tokens, request.Retrieval)
		judgment.SemanticAssessment = SemanticAssessment{PrimaryRole: role, Summary: boundedSummary(request.Retrieval), Confidence: .8, EvidenceRefs: firstEvidence(evidenceRefs)}
		if !roleComplete || role == "reference_resource" {
			judgment.Disposition = "hold"
			judgment.DispositionRationale = "Relevant evidence is present, but required role attributes are incomplete."
			reasons = append(reasons, "role_attributes_incomplete")
			manual = true
		} else {
			judgment.Disposition = "promote"
			judgment.DispositionRationale = "Public evidence matches the pinned strategy and supports the proposed semantic role."
			judgment.SemanticNodes = []SemanticNode{{
				SemanticNodeID: stableID("node-", request.Item.CanonicalItemID, role), Role: role, Name: nodeName(request.Retrieval),
				Description: boundedSummary(request.Retrieval), Confidence: .8, LensRefs: append([]string(nil), matchedLensIDs...), EvidenceRefs: firstEvidence(evidenceRefs),
				Attributes: map[string]any{"processor_version": EvidenceMatcherVersion},
			}}
		}
	}
	proposal := Proposal{
		SchemaVersion: ProposalSchema, CanonicalItemID: request.Item.CanonicalItemID, StrategyFingerprint: request.Strategy.Fingerprint,
		ProcessorVersion: EvidenceMatcherVersion, TokenizerVersion: StrategyTokenizerVersion, EvidenceOrigin: string(request.Retrieval.Origin),
		AllowedEvidenceRefs: append([]string(nil), evidenceRefs...), IncludeMatches: includeMatches, ExcludeMatches: excludeMatches,
		RequiresManualReview: manual, ReasonCodes: NormalizeTerms(reasons), Judgment: judgment,
	}
	proposal.Fingerprint = acquisition.Fingerprint(proposal)
	return Result{Proposal: proposal}, nil
}

func manualOutcome(artifact retrieval.Artifact) (bool, []string) {
	var reasons []string
	if artifact.SecretLike {
		reasons = append(reasons, "secret_like")
	}
	if artifact.Access != retrieval.AccessPublic {
		reasons = append(reasons, "access_"+string(artifact.Access))
	}
	if artifact.State != retrieval.StateComplete {
		reasons = append(reasons, "retrieval_"+string(artifact.State))
	}
	if len(artifact.Excerpts) == 0 {
		reasons = append(reasons, "public_evidence_missing")
	}
	return len(reasons) > 0, reasons
}

func proposeRole(tokens map[string]bool, artifact retrieval.Artifact) (string, bool) {
	if containsAny(tokens, "tension", "tradeoff", "conflict", "dilemma") {
		return "unresolved_tension", len(artifact.Excerpts) > 0
	}
	if containsAny(tokens, "evidence", "finding", "research", "study", "benchmark", "experiment") {
		return "evidence_backed_finding", len(artifact.Excerpts) > 0
	}
	if strings.TrimSpace(artifact.Metadata.Title) != "" && strings.TrimSpace(artifact.Metadata.Author) != "" {
		return "external_entity", len(artifact.Excerpts) > 0
	}
	return "reference_resource", false
}

func boundedSummary(artifact retrieval.Artifact) string {
	parts := []string{}
	if title := strings.TrimSpace(artifact.Metadata.Title); title != "" {
		parts = append(parts, title+".")
	}
	if len(artifact.Excerpts) > 0 {
		parts = append(parts, strings.TrimSpace(artifact.Excerpts[0].Text))
	}
	summary := strings.TrimSpace(strings.Join(parts, " "))
	if summary == "" {
		return "Public evidence requires operator review."
	}
	runes := []rune(summary)
	if len(runes) > 1000 {
		summary = string(runes[:1000])
	}
	return summary
}

func nodeName(artifact retrieval.Artifact) string {
	if title := strings.TrimSpace(artifact.Metadata.Title); title != "" {
		return title
	}
	return "Evidence-backed source"
}

func tokenize(value string) map[string]bool {
	words := strings.FieldsFunc(strings.ToLower(value), func(character rune) bool { return !unicode.IsLetter(character) && !unicode.IsDigit(character) })
	result := map[string]bool{}
	for _, word := range words {
		if utf8.RuneCountInString(word) < 2 || stopwords[word] {
			continue
		}
		result[word] = true
	}
	return result
}

var stopwords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true, "be": true, "by": true, "for": true, "from": true,
	"in": true, "is": true, "it": true, "of": true, "on": true, "or": true, "that": true, "the": true, "this": true, "to": true, "with": true,
}

func matchingTerms(tokens map[string]bool, terms []string) []string {
	var matches []string
	for _, term := range terms {
		termTokens := tokenize(term)
		if len(termTokens) > 0 && allPresent(tokens, termTokens) {
			matches = append(matches, term)
		}
	}
	sort.Strings(matches)
	return matches
}

func overlap(left, right map[string]bool) bool {
	for token := range right {
		if left[token] {
			return true
		}
	}
	return false
}

func allPresent(tokens, required map[string]bool) bool {
	for token := range required {
		if !tokens[token] {
			return false
		}
	}
	return true
}

func containsAny(tokens map[string]bool, values ...string) bool {
	for _, value := range values {
		if tokens[value] {
			return true
		}
	}
	return false
}

func firstEvidence(refs []string) []string {
	if len(refs) == 0 {
		return nil
	}
	return []string{refs[0]}
}

func stableSlug(value string) string {
	var builder strings.Builder
	lastDash := false
	for _, character := range strings.ToLower(value) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			builder.WriteRune(character)
			lastDash = false
		} else if !lastDash && builder.Len() > 0 {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return stableID("lens-", value)
	}
	if len(result) > 48 {
		result = result[:48]
	}
	return result
}

func stableID(prefix string, values ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return prefix + hex.EncodeToString(digest[:])[:20]
}
