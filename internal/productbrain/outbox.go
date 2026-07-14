package productbrain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/synergyai-os/Mindline/internal/routing"
)

const (
	OutboxSchema                     = "productbrain-outbox/v0.1"
	OutboxSummarySchema              = "productbrain-outbox-summary/v0.1"
	ExpectedCreatedBy                = "mindline:agent-operator"
	legacyDeliveredOutboxFingerprint = "4dabd8cc6b0c67f3b19173b0a80c425c2ee4ec3ab8b1fe80ea16959baf1f5020"
)

type Outbox struct {
	SchemaVersion      string                  `json:"schema_version"`
	Fingerprint        string                  `json:"fingerprint"`
	RoutingFingerprint string                  `json:"routing_fingerprint"`
	ProfileFingerprint string                  `json:"profile_fingerprint"`
	ProfileSnapshot    DeliveryProfileSnapshot `json:"delivery_profile_snapshot"`
	ReviewContext      ReviewContext           `json:"review_context"`
	Operations         []OutboxOperation       `json:"operations"`
	PrivacyFindings    []PrivacyFinding        `json:"privacy_findings"`
	OperatorJudged     bool                    `json:"operator_judged"`
	HeldOut            bool                    `json:"held_out"`
	Generalizable      bool                    `json:"generalizable"`
	AutonomyClaim      bool                    `json:"autonomy_claim"`
}

type DeliveryProfileSnapshot struct {
	ProfileID             string                 `json:"profile_id"`
	TransportKind         string                 `json:"transport_kind,omitempty"`
	TransportAPIPath      string                 `json:"transport_api_path,omitempty"`
	ExpectedOrigin        string                 `json:"expected_origin"`
	ExpectedWorkspaceID   string                 `json:"expected_workspace_id"`
	ExpectedWorkspaceSlug string                 `json:"expected_workspace_slug"`
	ExpectedKeyID         string                 `json:"expected_key_id"`
	DraftOnly             bool                   `json:"draft_only"`
	RoleMappings          map[string]RoleMapping `json:"role_mappings"`
	RelationMappings      map[string]string      `json:"relation_mappings"`
	ReviewPolicy          *DeliveryReviewPolicy  `json:"review_policy,omitempty"`
}

type OutboxOperation struct {
	OperationID        string             `json:"operation_id"`
	Kind               string             `json:"kind"`
	Dependencies       []string           `json:"dependencies"`
	PayloadFingerprint string             `json:"payload_fingerprint"`
	Entry              *EntryOperation    `json:"entry,omitempty"`
	Relation           *RelationOperation `json:"relation,omitempty"`
}

type EntryOperation struct {
	CollectionSlug string         `json:"collection_slug"`
	EntryID        string         `json:"entry_id"`
	Name           string         `json:"name"`
	Data           map[string]any `json:"data"`
	SourceRef      string         `json:"source_ref"`
	SourceExcerpt  string         `json:"source_excerpt"`
	CreatedBy      string         `json:"created_by"`
	ForceDraft     bool           `json:"force_draft"`
}

type RelationOperation struct {
	RelationIdentity string         `json:"relation_identity"`
	FromEntryID      string         `json:"from_entry_id"`
	ToEntryID        string         `json:"to_entry_id"`
	Type             string         `json:"type"`
	Metadata         map[string]any `json:"metadata"`
	IfMissing        bool           `json:"if_missing"`
}

type ReviewContext struct {
	Captures        []CaptureReview  `json:"captures"`
	DepthOneSources []DepthOneReview `json:"depth_one_sources"`
	PendingActions  []string         `json:"pending_actions"`
}
type CaptureReview struct {
	CaptureRef              string                     `json:"capture_ref"`
	CanonicalURL            string                     `json:"canonical_url"`
	CanonicalURLID          string                     `json:"canonical_url_id"`
	DuplicateOf             string                     `json:"duplicate_of,omitempty"`
	EnrichmentState         string                     `json:"enrichment_state"`
	PublicMetadata          *routing.PublicMetadata    `json:"public_metadata,omitempty"`
	PublicExcerpts          []routing.PublicExcerpt    `json:"public_excerpts,omitempty"`
	Missingness             []string                   `json:"missingness"`
	SemanticAssessment      routing.SemanticAssessment `json:"semantic_assessment"`
	LensResults             []routing.LensResult       `json:"lens_results"`
	Disposition             string                     `json:"disposition"`
	DispositionRationale    string                     `json:"disposition_rationale"`
	SemanticNodes           []routing.SemanticNode     `json:"semantic_nodes"`
	SemanticEdges           []routing.SemanticEdge     `json:"semantic_edges"`
	DestinationOperationIDs []string                   `json:"destination_operation_ids"`
}
type DepthOneReview struct {
	CanonicalURL       string                     `json:"canonical_url"`
	ParentCanonicalURL string                     `json:"parent_canonical_url"`
	EnrichmentState    string                     `json:"enrichment_state,omitempty"`
	PublicMetadata     *routing.PublicMetadata    `json:"public_metadata,omitempty"`
	PublicExcerpts     []routing.PublicExcerpt    `json:"public_excerpts,omitempty"`
	Missingness        []string                   `json:"missingness,omitempty"`
	SemanticAssessment routing.SemanticAssessment `json:"semantic_assessment"`
	LensResults        []routing.LensResult       `json:"lens_results"`
	Disposition        string                     `json:"disposition"`
}

type OutboxSummary struct {
	SchemaVersion          string `json:"schema_version"`
	Fingerprint            string `json:"fingerprint"`
	OutboxFingerprint      string `json:"outbox_fingerprint"`
	OperationCount         int    `json:"operation_count"`
	EntryOperationCount    int    `json:"entry_operation_count"`
	RelationOperationCount int    `json:"relation_operation_count"`
	PrivacyFindingCount    int    `json:"privacy_finding_count"`
	DraftOnly              bool   `json:"draft_only"`
	OperatorJudged         bool   `json:"operator_judged"`
	HeldOut                bool   `json:"held_out"`
	Generalizable          bool   `json:"generalizable"`
}

func CompileOutbox(route routing.Result, profile DeliveryProfile) (Outbox, OutboxSummary, error) {
	if err := ValidateDeliveryProfile(profile); err != nil {
		return Outbox{}, OutboxSummary{}, err
	}
	outbox := Outbox{SchemaVersion: OutboxSchema, RoutingFingerprint: route.Decisions.Fingerprint, ProfileFingerprint: hashValue(profile), OperatorJudged: true, HeldOut: false, Generalizable: false, AutonomyClaim: false}
	outbox.ProfileSnapshot = DeliveryProfileSnapshot{ProfileID: profile.ProfileID, TransportKind: profile.Transport.Kind, TransportAPIPath: profile.Transport.APIPath, ExpectedOrigin: profile.Transport.BaseURL, ExpectedWorkspaceID: profile.Workspace.ExpectedID, ExpectedWorkspaceSlug: profile.Workspace.ExpectedSlug, ExpectedKeyID: profile.Credential.ExpectedKeyID, DraftOnly: profile.DraftOnly, RoleMappings: cloneRoleMappings(profile.RoleMappings), RelationMappings: cloneStringMap(profile.RelationMappings), ReviewPolicy: cloneReviewPolicy(profile.ReviewPolicy)}
	nodeEntryIDs := map[string]string{}
	nodeOperationIDs := map[string]string{}
	operationIDsByCanonical := map[string][]string{}
	for _, source := range route.Decisions.Sources {
		if source.Disposition != "promote" {
			continue
		}
		for _, node := range source.SemanticNodes {
			mapping, ok := profile.RoleMappings[node.Role]
			if !ok {
				return Outbox{}, OutboxSummary{}, errors.New("unsupported_destination_mapping")
			}
			entryID := deterministicEntryID(mapping.IDPrefix, source.CanonicalURL, node.SemanticNodeID, mapping.CollectionSlug)
			entry := EntryOperation{CollectionSlug: mapping.CollectionSlug, EntryID: entryID, Name: node.Name, Data: mapNodeData(node, source), SourceRef: source.CanonicalURL, SourceExcerpt: firstExcerpt(source), CreatedBy: ExpectedCreatedBy, ForceDraft: true}
			opID := "op-entry-" + hashText(entryID+"|"+source.CanonicalURL+"|"+node.SemanticNodeID)
			op := OutboxOperation{OperationID: opID, Kind: "entry", Dependencies: []string{}, Entry: &entry}
			op.PayloadFingerprint = outboxOperationFingerprint(op)
			outbox.Operations = append(outbox.Operations, op)
			nodeEntryIDs[source.CanonicalURLID+"|"+node.SemanticNodeID] = entryID
			nodeOperationIDs[source.CanonicalURLID+"|"+node.SemanticNodeID] = opID
			operationIDsByCanonical[source.CanonicalURLID] = append(operationIDsByCanonical[source.CanonicalURLID], opID)
		}
	}
	for _, source := range route.Decisions.Sources {
		if source.Disposition != "promote" {
			continue
		}
		for _, edge := range source.SemanticEdges {
			mapped, ok := profile.RelationMappings[edge.Type]
			if !ok {
				return Outbox{}, OutboxSummary{}, errors.New("unsupported_destination_mapping")
			}
			fromKey := source.CanonicalURLID + "|" + edge.From
			toKey := source.CanonicalURLID + "|" + edge.To
			fromID, toID := nodeEntryIDs[fromKey], nodeEntryIDs[toKey]
			if fromID == "" || toID == "" {
				return Outbox{}, OutboxSummary{}, errors.New("invalid semantic edge mapping")
			}
			identity := hashText("mindline/relation/v0.1|" + fromID + "|" + mapped + "|" + toID)
			metadata := map[string]any{"evidence_refs": append([]string{}, edge.EvidenceRefs...), "lens_refs": edgeLensRefs(source, edge), "rationale": edge.Rationale, "initiator_type": "agent_operator", "judgment_method": "operator_agent_review", "credential_key_id": profile.Credential.ExpectedKeyID}
			relation := RelationOperation{RelationIdentity: identity, FromEntryID: fromID, ToEntryID: toID, Type: mapped, Metadata: metadata, IfMissing: true}
			opID := "op-relation-" + identity
			deps := []string{nodeOperationIDs[fromKey], nodeOperationIDs[toKey]}
			sort.Strings(deps)
			op := OutboxOperation{OperationID: opID, Kind: "relation", Dependencies: deps, Relation: &relation}
			op.PayloadFingerprint = outboxOperationFingerprint(op)
			outbox.Operations = append(outbox.Operations, op)
			operationIDsByCanonical[source.CanonicalURLID] = append(operationIDsByCanonical[source.CanonicalURLID], opID)
		}
	}
	if err := rejectIdentityCollisions(outbox.Operations); err != nil {
		return Outbox{}, OutboxSummary{}, err
	}
	outbox.ReviewContext = buildReviewContext(route, operationIDsByCanonical, profile, outbox.Operations)
	outbox.PrivacyFindings = ScanPublicArtifact(outbox, "")
	if len(outbox.PrivacyFindings) > 0 {
		return Outbox{}, OutboxSummary{}, errors.New("unsafe_outbound_value")
	}
	outbox.Fingerprint = hashValue(outbox)
	if err := ValidateOutbox(outbox); err != nil {
		return Outbox{}, OutboxSummary{}, err
	}
	summary := OutboxSummary{SchemaVersion: OutboxSummarySchema, OutboxFingerprint: outbox.Fingerprint, OperationCount: len(outbox.Operations), PrivacyFindingCount: len(outbox.PrivacyFindings), DraftOnly: true, OperatorJudged: true, HeldOut: false, Generalizable: false}
	for _, op := range outbox.Operations {
		if op.Kind == "entry" {
			summary.EntryOperationCount++
		} else if op.Kind == "relation" {
			summary.RelationOperationCount++
		}
	}
	summary.Fingerprint = hashValue(summary)
	return outbox, summary, nil
}

func deterministicEntryID(prefix, canonicalURL, nodeID, collection string) string {
	sum := sha256.Sum256([]byte("mindline/v0.1|" + canonicalURL + "|" + nodeID + "|" + collection))
	number := new(big.Int).SetBytes(sum[:10])
	return prefix + "-" + number.String()
}
func mapNodeData(node routing.SemanticNode, source routing.RoutedSource) map[string]any {
	data := map[string]any{"description": node.Description}
	switch node.Role {
	case "external_entity":
		data["url"] = source.CanonicalURL
		mapStringAttribute(data, "category", node.Attributes, "entity_category", map[string]string{"competitor": "competitor", "complementary": "complementary", "complementary_tool": "complementary", "adjacent_product": "complementary", "platform": "platform", "ecosystem": "ecosystem"})
		mapStringAttribute(data, "relationshipToPb", node.Attributes, "market_relationship", map[string]string{"direct_competitor": "direct_competitor", "indirect_competitor": "indirect_competitor", "adjacent": "complementary", "complementary": "complementary", "neutral": "neutral"})
		mapStringAttribute(data, "icpOverlap", node.Attributes, "audience_overlap", map[string]string{"high": "high", "medium": "medium", "low": "low", "none": "none"})
		copyStringAttribute(data, "keyDifferentiator", node.Attributes, "key_differentiator")
		copyStringAttribute(data, "whatWeLearn", node.Attributes, "learning")
	case "evidence_backed_finding":
		data["source"] = source.CanonicalURL
		data["evidenceStrength"] = confidenceLabel(node.Confidence)
	case "unresolved_tension":
		mapStringAttribute(data, "type", node.Attributes, "tension_kind", map[string]string{"bug": "bug", "improvement": "improvement", "tech_debt": "tech-debt", "process": "process", "governance_tradeoff": "process", "ux": "ux", "other": "other"})
		mapStringAttribute(data, "severity", node.Attributes, "severity", map[string]string{"low": "low", "medium": "medium", "high": "high", "critical": "critical"})
		mapStringAttribute(data, "priority", node.Attributes, "priority", map[string]string{"low": "low", "medium": "medium", "high": "high", "critical": "critical", "investigate": "medium"})
		copyStringAttribute(data, "affectedArea", node.Attributes, "affected_area")
		mapStringAttribute(data, "status", node.Attributes, "resolution_state", map[string]string{"draft": "draft", "unresolved": "active", "active": "active", "resolved": "resolved"})
	}
	return data
}

func mapStringAttribute(target map[string]any, targetKey string, source map[string]any, sourceKey string, values map[string]string) {
	raw, ok := source[sourceKey].(string)
	if !ok {
		return
	}
	if mapped := values[raw]; mapped != "" {
		target[targetKey] = mapped
	}
}
func copyStringAttribute(target map[string]any, targetKey string, source map[string]any, sourceKey string) {
	if value, ok := source[sourceKey].(string); ok && value != "" {
		target[targetKey] = value
	}
}
func firstExcerpt(source routing.RoutedSource) string {
	if len(source.PublicExcerpts) == 0 {
		return source.SemanticAssessment.Summary
	}
	return source.PublicExcerpts[0].Text
}
func confidenceLabel(v float64) string {
	if v >= .8 {
		return "strong"
	}
	if v >= .5 {
		return "moderate"
	}
	return "weak"
}
func edgeLensRefs(source routing.RoutedSource, edge routing.SemanticEdge) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range source.SemanticNodes {
		if n.SemanticNodeID == edge.From || n.SemanticNodeID == edge.To {
			for _, r := range n.LensRefs {
				if !seen[r] {
					seen[r] = true
					out = append(out, r)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}
func buildReviewContext(route routing.Result, operations map[string][]string, profile DeliveryProfile, outboxOperations []OutboxOperation) ReviewContext {
	context := ReviewContext{PendingActions: productBrainPendingActions(profile, outboxOperations)}
	routed := map[string]routing.RoutedSource{}
	canonicalByID := map[string]routing.CanonicalURL{}
	for _, s := range route.Decisions.Sources {
		routed[s.CanonicalURLID] = s
	}
	for _, u := range route.Graph.CanonicalURLs {
		canonicalByID[u.CanonicalURLID] = u
	}
	firstCapture := map[string]string{}
	for i, record := range route.Graph.SourceRecords {
		ref := fmt.Sprintf("capture-%03d", i+1)
		occ := findOccurrence(route.Graph.URLOccurrences, record.URLOccurrenceIDs[0])
		source := routed[occ.CanonicalURLID]
		metadata := source.PublicMetadata
		review := CaptureReview{CaptureRef: ref, CanonicalURL: source.CanonicalURL, CanonicalURLID: source.CanonicalURLID, EnrichmentState: source.EnrichmentState, PublicMetadata: &metadata, PublicExcerpts: append([]routing.PublicExcerpt{}, source.PublicExcerpts...), Missingness: append([]string{}, source.Missingness...), SemanticAssessment: source.SemanticAssessment, LensResults: append([]routing.LensResult{}, source.LensResults...), Disposition: source.Disposition, DispositionRationale: source.DispositionRationale, SemanticNodes: append([]routing.SemanticNode{}, source.SemanticNodes...), SemanticEdges: append([]routing.SemanticEdge{}, source.SemanticEdges...), DestinationOperationIDs: append([]string{}, operations[source.CanonicalURLID]...)}
		if first := firstCapture[source.CanonicalURLID]; first != "" {
			review.DuplicateOf = first
		} else {
			firstCapture[source.CanonicalURLID] = ref
		}
		context.Captures = append(context.Captures, review)
	}
	for _, u := range route.Graph.CanonicalURLs {
		if u.Depth != 1 {
			continue
		}
		s := routed[u.CanonicalURLID]
		parent := canonicalByID[u.ParentCanonicalURLID]
		metadata := s.PublicMetadata
		context.DepthOneSources = append(context.DepthOneSources, DepthOneReview{CanonicalURL: s.CanonicalURL, ParentCanonicalURL: parent.CanonicalURL, EnrichmentState: s.EnrichmentState, PublicMetadata: &metadata, PublicExcerpts: append([]routing.PublicExcerpt{}, s.PublicExcerpts...), Missingness: append([]string{}, s.Missingness...), SemanticAssessment: s.SemanticAssessment, LensResults: append([]routing.LensResult{}, s.LensResults...), Disposition: s.Disposition})
	}
	return context
}

func productBrainPendingActions(profile DeliveryProfile, operations []OutboxOperation) []string {
	entryCount, relationCount := 0, 0
	for _, operation := range operations {
		switch operation.Kind {
		case "entry":
			entryCount++
		case "relation":
			relationCount++
		}
	}
	actions := []string{fmt.Sprintf("Review %d Product Brain draft %s and %d proposed %s; accept or reject the routing judgments", entryCount, plural(entryCount, "entry", "entries"), relationCount, plural(relationCount, "relation", "relations"))}
	if profile.ReviewPolicy == nil {
		actions = append(actions, "Complete the Product Brain credential lifecycle required by the selected delivery profile", "Confirm private runtime retention or cleanup after review")
		return actions
	}
	if profile.ReviewPolicy.CredentialLifecycle == "retire_after_review" {
		actions = append(actions, "Retire the temporary Product Brain key after review")
	} else {
		actions = append(actions, "Keep the Product Brain credential active under the selected delivery profile")
	}
	if profile.ReviewPolicy.PrivateRuntimeLifecycle == "cleanup_after_review" {
		if profile.ReviewPolicy.CredentialLifecycle == "retire_after_review" {
			actions = append(actions, "Confirm owner-validated private runtime cleanup after key retirement")
		} else {
			actions = append(actions, "Confirm owner-validated private runtime cleanup after review")
		}
	} else {
		actions = append(actions, "Retain the owner-validated private runtime evidence after review")
	}
	return actions
}

func legacyProductBrainPendingActions() []string {
	return []string{
		"Review the three Product Brain drafts and routing judgments",
		"Retire the temporary Product Brain key after review",
		"Confirm owner-validated private runtime cleanup after key retirement",
	}
}

func validateProductBrainPendingActions(outbox Outbox) error {
	profile := DeliveryProfileFromSnapshot(outbox.ProfileSnapshot)
	expected := productBrainPendingActions(profile, outbox.Operations)
	if equalStrings(outbox.ReviewContext.PendingActions, expected) {
		return nil
	}
	if outbox.Fingerprint == legacyDeliveredOutboxFingerprint && outbox.ProfileSnapshot.ReviewPolicy == nil && equalStrings(outbox.ReviewContext.PendingActions, legacyProductBrainPendingActions()) {
		return nil
	}
	return errors.New("review actions do not match delivery profile and operation counts")
}

func plural(count int, singular, multiple string) string {
	if count == 1 {
		return singular
	}
	return multiple
}
func findOccurrence(values []routing.URLOccurrence, id string) routing.URLOccurrence {
	for _, v := range values {
		if v.URLOccurrenceID == id {
			return v
		}
	}
	return routing.URLOccurrence{}
}
func rejectIdentityCollisions(ops []OutboxOperation) error {
	seen := map[string]string{}
	for _, op := range ops {
		if op.Entry == nil {
			continue
		}
		canonical := hashValue(op.Entry)
		if prior, ok := seen[op.Entry.EntryID]; ok && prior != canonical {
			return errors.New("entry_identity_collision")
		}
		seen[op.Entry.EntryID] = canonical
	}
	return nil
}
func cloneRoleMappings(v map[string]RoleMapping) map[string]RoleMapping {
	out := map[string]RoleMapping{}
	for k, x := range v {
		out[k] = x
	}
	return out
}
func cloneStringMap(v map[string]string) map[string]string {
	out := map[string]string{}
	for k, x := range v {
		out[k] = x
	}
	return out
}
func cloneReviewPolicy(v *DeliveryReviewPolicy) *DeliveryReviewPolicy {
	if v == nil {
		return nil
	}
	clone := *v
	return &clone
}
func hashText(v string) string { sum := sha256.Sum256([]byte(v)); return hex.EncodeToString(sum[:]) }
func hashValue(v any) string {
	data, _ := json.Marshal(v)
	var raw any
	_ = json.Unmarshal(data, &raw)
	stripFingerprint(raw)
	canonical, _ := json.Marshal(raw)
	return hashText(string(canonical))
}
func stripFingerprint(v any) {
	switch x := v.(type) {
	case map[string]any:
		delete(x, "fingerprint")
		for _, value := range x {
			stripFingerprint(value)
		}
	case []any:
		for _, value := range x {
			stripFingerprint(value)
		}
	}
}

func LoadOutbox(dir string) (Outbox, error) {
	data, err := osReadFile(filepath.Join(dir, "outbox.json"))
	if err != nil {
		return Outbox{}, err
	}
	return DecodeOutbox(data)
}

// DecodeOutbox is the single strict JSON boundary used by runtime delivery and
// eval proof. Unknown fields cannot hide outside the fingerprint-bound contract.
func DecodeOutbox(data []byte) (Outbox, error) {
	var outbox Outbox
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&outbox); err != nil {
		return Outbox{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Outbox{}, errors.New("invalid trailing outbox data")
	}
	if err := ValidateOutbox(outbox); err != nil {
		return Outbox{}, err
	}
	return outbox, nil
}

func ValidateOutbox(outbox Outbox) error {
	if outbox.SchemaVersion != OutboxSchema || outbox.Fingerprint == "" || outbox.Fingerprint != hashValue(outbox) {
		return errors.New("outbox fingerprint mismatch")
	}
	if outbox.RoutingFingerprint == "" || outbox.ProfileFingerprint == "" || validateProfileSnapshot(outbox.ProfileSnapshot, outbox.Fingerprint) != nil || len(outbox.ReviewContext.Captures) == 0 || len(outbox.Operations) == 0 || !outbox.OperatorJudged || outbox.HeldOut || outbox.Generalizable || outbox.AutonomyClaim {
		return errors.New("invalid outbox authority")
	}
	profile := DeliveryProfileFromSnapshot(outbox.ProfileSnapshot)
	if err := ValidateDeliveryProfile(profile); err != nil || outbox.ProfileFingerprint != hashValue(profile) {
		return errors.New("outbox profile snapshot mismatch")
	}
	operationIDs := map[string]OutboxOperation{}
	entryOperationIDs := map[string]bool{}
	entryOperationByEntryID := map[string]string{}
	for _, operation := range outbox.Operations {
		if operation.OperationID == "" || operation.PayloadFingerprint == "" || operation.PayloadFingerprint != outboxOperationFingerprint(operation) {
			return errors.New("invalid outbox operation fingerprint")
		}
		if _, exists := operationIDs[operation.OperationID]; exists {
			return errors.New("duplicate outbox operation")
		}
		operationIDs[operation.OperationID] = operation
		switch operation.Kind {
		case "entry":
			if operation.Entry == nil || operation.Relation != nil || len(operation.Dependencies) != 0 || operation.Entry.CollectionSlug == "" || operation.Entry.EntryID == "" || operation.Entry.Name == "" || operation.Entry.CreatedBy != ExpectedCreatedBy || !operation.Entry.ForceDraft {
				return errors.New("invalid entry operation")
			}
			entryOperationIDs[operation.OperationID] = true
			if prior := entryOperationByEntryID[operation.Entry.EntryID]; prior != "" {
				return errors.New("duplicate destination entry identity")
			}
			entryOperationByEntryID[operation.Entry.EntryID] = operation.OperationID
		case "relation":
			if operation.Entry != nil || operation.Relation == nil || len(operation.Dependencies) != 2 || operation.Relation.RelationIdentity == "" || operation.Relation.FromEntryID == "" || operation.Relation.ToEntryID == "" || operation.Relation.Type == "" || !operation.Relation.IfMissing || !validRelationMetadata(operation.Relation.Metadata, outbox.ProfileSnapshot.ExpectedKeyID) {
				return errors.New("invalid relation operation")
			}
			expectedIdentity := hashText("mindline/relation/v0.1|" + operation.Relation.FromEntryID + "|" + operation.Relation.Type + "|" + operation.Relation.ToEntryID)
			if operation.Relation.RelationIdentity != expectedIdentity || operation.OperationID != "op-relation-"+expectedIdentity {
				return errors.New("invalid relation identity")
			}
		default:
			return errors.New("invalid outbox operation kind")
		}
	}
	for _, operation := range outbox.Operations {
		seenDependencies := map[string]bool{}
		for _, dependency := range operation.Dependencies {
			if dependency == "" || seenDependencies[dependency] || !entryOperationIDs[dependency] {
				return errors.New("invalid outbox dependency")
			}
			seenDependencies[dependency] = true
		}
		if operation.Kind == "relation" {
			expected := []string{entryOperationByEntryID[operation.Relation.FromEntryID], entryOperationByEntryID[operation.Relation.ToEntryID]}
			sort.Strings(expected)
			actual := append([]string{}, operation.Dependencies...)
			sort.Strings(actual)
			if expected[0] == "" || expected[1] == "" || !equalStrings(expected, actual) {
				return errors.New("relation dependency mismatch")
			}
		}
	}
	if err := validateReviewContext(outbox); err != nil {
		return err
	}
	if findings := ScanPublicArtifact(outbox, ""); len(findings) > 0 {
		return errors.New("unsafe outbound outbox")
	}
	return nil
}

func validateProfileSnapshot(snapshot DeliveryProfileSnapshot, outboxFingerprint string) error {
	if snapshot.ProfileID == "" || snapshot.ExpectedOrigin != ProductionGatewayOrigin || snapshot.ExpectedWorkspaceID == "" || snapshot.ExpectedWorkspaceSlug == "" || snapshot.ExpectedKeyID == "" || !snapshot.DraftOnly {
		return errors.New("invalid delivery profile snapshot")
	}
	transportExplicit := snapshot.TransportKind == "aki" && snapshot.TransportAPIPath == "/api/aki"
	legacyTransportOmitted := snapshot.TransportKind == "" && snapshot.TransportAPIPath == "" && outboxFingerprint == legacyDeliveredOutboxFingerprint
	if !transportExplicit && !legacyTransportOmitted {
		return errors.New("unsupported delivery profile transport")
	}
	allowedRoles := map[string]bool{"external_entity": true, "evidence_backed_finding": true, "unresolved_tension": true}
	if len(snapshot.RoleMappings) != len(allowedRoles) {
		return errors.New("unsupported delivery profile role mappings")
	}
	seenCollections := map[string]bool{}
	for role, mapping := range snapshot.RoleMappings {
		if !allowedRoles[role] || strings.TrimSpace(mapping.CollectionSlug) == "" || strings.TrimSpace(mapping.IDPrefix) == "" || seenCollections[mapping.CollectionSlug] {
			return errors.New("unsupported delivery profile role mappings")
		}
		seenCollections[mapping.CollectionSlug] = true
	}
	if len(snapshot.RelationMappings) != 1 || snapshot.RelationMappings["related_to"] != "related_to" {
		return errors.New("unsupported delivery profile relation mappings")
	}
	if snapshot.ReviewPolicy != nil {
		if snapshot.ReviewPolicy.CredentialLifecycle != "persistent" && snapshot.ReviewPolicy.CredentialLifecycle != "retire_after_review" {
			return errors.New("unsupported Product Brain credential review lifecycle")
		}
		if snapshot.ReviewPolicy.PrivateRuntimeLifecycle != "retain" && snapshot.ReviewPolicy.PrivateRuntimeLifecycle != "cleanup_after_review" {
			return errors.New("unsupported Product Brain private runtime review lifecycle")
		}
	}
	return nil
}

func validRelationMetadata(metadata map[string]any, expectedKeyID string) bool {
	expectedKeys := map[string]bool{"evidence_refs": true, "lens_refs": true, "rationale": true, "initiator_type": true, "judgment_method": true, "credential_key_id": true}
	if len(metadata) != len(expectedKeys) {
		return false
	}
	for key := range metadata {
		if !expectedKeys[key] {
			return false
		}
	}
	if metadata["initiator_type"] != "agent_operator" || metadata["judgment_method"] != "operator_agent_review" || metadata["credential_key_id"] != expectedKeyID || strings.TrimSpace(fmt.Sprint(metadata["rationale"])) == "" {
		return false
	}
	return validStringList(metadata["evidence_refs"]) && validStringList(metadata["lens_refs"])
}

func validStringList(value any) bool {
	values, ok := value.([]string)
	if !ok {
		// JSON round trips through map[string]any produce []any.
		raw, rawOK := value.([]any)
		if !rawOK {
			return false
		}
		values = make([]string, 0, len(raw))
		for _, item := range raw {
			text, itemOK := item.(string)
			if !itemOK {
				return false
			}
			values = append(values, text)
		}
	}
	if len(values) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func validateReviewContext(outbox Outbox) error {
	if err := validateProductBrainPendingActions(outbox); err != nil {
		return err
	}
	operations := map[string]OutboxOperation{}
	for _, operation := range outbox.Operations {
		operations[operation.OperationID] = operation
	}
	firstCapture := map[string]string{}
	canonicalAuthority := map[string]string{}
	coveredOperations := map[string]bool{}
	var expectedLensIDs []string
	for index, capture := range outbox.ReviewContext.Captures {
		expectedRef := fmt.Sprintf("capture-%03d", index+1)
		if capture.CaptureRef != expectedRef || capture.CanonicalURL == "" || capture.CanonicalURLID == "" || !validEnrichmentState(capture.EnrichmentState) || !validSemanticAssessment(capture.SemanticAssessment) || !validDisposition(capture.Disposition) || capture.DispositionRationale == "" {
			return errors.New("invalid review capture authority")
		}
		lensIDs, ok := validLensResults(capture.LensResults)
		if !ok || expectedLensIDs != nil && !equalStrings(expectedLensIDs, lensIDs) {
			return errors.New("incomplete review lens matrix")
		}
		if expectedLensIDs == nil {
			expectedLensIDs = lensIDs
		}
		first := firstCapture[capture.CanonicalURLID]
		if first == "" {
			if capture.DuplicateOf != "" {
				return errors.New("invalid duplicate review reference")
			}
			firstCapture[capture.CanonicalURLID] = capture.CaptureRef
		} else if capture.DuplicateOf != first {
			return errors.New("invalid duplicate review reference")
		}
		normalized := capture
		normalized.CaptureRef = ""
		normalized.DuplicateOf = ""
		fingerprint := hashValue(normalized)
		if prior := canonicalAuthority[capture.CanonicalURLID]; prior != "" && prior != fingerprint {
			return errors.New("conflicting duplicate review authority")
		}
		canonicalAuthority[capture.CanonicalURLID] = fingerprint
		if err := validateCaptureSemantics(capture, expectedLensIDs); err != nil {
			return err
		}
		if capture.Disposition == "promote" {
			expectedOperations, err := expectedCaptureOperations(capture, outbox.ProfileSnapshot)
			if err != nil || len(expectedOperations) == 0 || len(expectedOperations) != len(capture.DestinationOperationIDs) {
				return errors.New("incomplete promoted review destination mapping")
			}
			for operationIndex, expected := range expectedOperations {
				operationID := capture.DestinationOperationIDs[operationIndex]
				actual, exists := operations[operationID]
				if !exists || operationID != expected.OperationID {
					return errors.New("invalid review destination operation reference")
				}
				// Immutable v0.1 review contexts created before public excerpts were
				// embedded cannot reconstruct the excerpt, but they must still retain
				// a non-empty public excerpt on the exact derived entry operation.
				if expected.Entry != nil && len(capture.PublicExcerpts) == 0 {
					if actual.Entry == nil || strings.TrimSpace(actual.Entry.SourceExcerpt) == "" {
						return errors.New("review destination source excerpt mismatch")
					}
					expected.Entry.SourceExcerpt = actual.Entry.SourceExcerpt
					expected.PayloadFingerprint = outboxOperationFingerprint(expected)
				}
				if !canonicalOperationEqual(actual, expected) {
					return errors.New("review semantic destination mapping mismatch")
				}
				coveredOperations[operationID] = true
			}
		} else if len(capture.DestinationOperationIDs) != 0 {
			return errors.New("non-promoted review has destination operations")
		}
	}
	depthURLs := map[string]bool{}
	for _, source := range outbox.ReviewContext.DepthOneSources {
		if source.CanonicalURL == "" || source.ParentCanonicalURL == "" || !validDepthOneEnrichmentAuthority(source) || !validSemanticAssessment(source.SemanticAssessment) || !validDisposition(source.Disposition) || depthURLs[source.CanonicalURL] {
			return errors.New("invalid depth-one review authority")
		}
		lensIDs, ok := validLensResults(source.LensResults)
		if !ok || !equalStrings(expectedLensIDs, lensIDs) {
			return errors.New("incomplete review lens matrix")
		}
		depthURLs[source.CanonicalURL] = true
	}
	if len(coveredOperations) != len(operations) {
		return errors.New("incomplete review destination operation closure")
	}
	return nil
}

func validSemanticAssessment(assessment routing.SemanticAssessment) bool {
	roles := map[string]bool{"external_entity": true, "evidence_backed_finding": true, "unresolved_tension": true, "reference_resource": true, "action": true, "unknown": true}
	if !roles[assessment.PrimaryRole] || assessment.Confidence < 0 || assessment.Confidence > 1 {
		return false
	}
	if assessment.PrimaryRole == "unknown" {
		return assessment.Confidence == 0 && strings.TrimSpace(assessment.Summary) == "" && len(assessment.EvidenceRefs) == 0 && validNonemptyStringSlice(assessment.Missingness)
	}
	return strings.TrimSpace(assessment.Summary) != "" && validNonemptyStringSlice(assessment.EvidenceRefs)
}

func validLensResults(results []routing.LensResult) ([]string, bool) {
	if len(results) == 0 {
		return nil, false
	}
	seen := map[string]bool{}
	ids := make([]string, 0, len(results))
	for _, result := range results {
		if result.LensID == "" || result.Rationale == "" || result.Confidence < 0 || result.Confidence > 1 || seen[result.LensID] {
			return nil, false
		}
		switch result.Result {
		case "matched", "not_matched":
			if !validNonemptyStringSlice(result.EvidenceRefs) {
				return nil, false
			}
		case "unknown":
			if !validNonemptyStringSlice(result.Missingness) {
				return nil, false
			}
		default:
			return nil, false
		}
		seen[result.LensID] = true
		ids = append(ids, result.LensID)
	}
	return ids, true
}

func validateCaptureSemantics(capture CaptureReview, lensIDs []string) error {
	lenses := map[string]bool{}
	for _, lensID := range lensIDs {
		lenses[lensID] = true
	}
	if capture.Disposition == "promote" && capture.EnrichmentState != "complete" || len(capture.SemanticNodes) > 3 {
		return errors.New("invalid promoted review semantics")
	}
	roles := map[string]bool{"external_entity": true, "evidence_backed_finding": true, "unresolved_tension": true, "reference_resource": true, "action": true}
	nodes := map[string]routing.SemanticNode{}
	for _, node := range capture.SemanticNodes {
		if node.SemanticNodeID == "" || !roles[node.Role] || node.Name == "" || node.Description == "" || node.Confidence < 0 || node.Confidence > 1 || nodes[node.SemanticNodeID].SemanticNodeID != "" || !validNonemptyStringSlice(node.EvidenceRefs) {
			return errors.New("invalid review semantic node")
		}
		seenLensRefs := map[string]bool{}
		for _, lensID := range node.LensRefs {
			if !lenses[lensID] || seenLensRefs[lensID] {
				return errors.New("invalid semantic node lens reference")
			}
			seenLensRefs[lensID] = true
		}
		nodes[node.SemanticNodeID] = node
	}
	seenEdges := map[string]bool{}
	for _, edge := range capture.SemanticEdges {
		identity := edge.From + "|" + edge.Type + "|" + edge.To
		if edge.From == "" || edge.To == "" || edge.Type != "related_to" || edge.Rationale == "" || nodes[edge.From].SemanticNodeID == "" || nodes[edge.To].SemanticNodeID == "" || seenEdges[identity] || !validNonemptyStringSlice(edge.EvidenceRefs) {
			return errors.New("invalid review semantic edge")
		}
		seenEdges[identity] = true
	}
	return nil
}

func validEnrichmentState(state string) bool {
	switch state {
	case "complete", "partial", "inaccessible", "failed", "not_attempted":
		return true
	default:
		return false
	}
}

func validDepthOneEnrichmentAuthority(source DepthOneReview) bool {
	if validEnrichmentState(source.EnrichmentState) {
		return true
	}
	if source.EnrichmentState != "" {
		return false
	}
	// Immutable early v0.1 outboxes did not retain this field for depth-one
	// review sources. Their only accepted interpretations are fully implied by
	// the embedded semantic evidence: unknown/unevidenced means inaccessible;
	// evidenced/non-unknown means complete. Mixed evidence cannot use the shim.
	if source.SemanticAssessment.PrimaryRole == "unknown" {
		for _, result := range source.LensResults {
			if result.Result != "unknown" {
				return false
			}
		}
		return true
	}
	for _, result := range source.LensResults {
		if result.Result == "unknown" {
			return false
		}
	}
	return true
}

func validDisposition(disposition string) bool {
	switch disposition {
	case "promote", "hold", "monitor", "archive", "clarify":
		return true
	default:
		return false
	}
}

func validNonemptyStringSlice(values []string) bool {
	if len(values) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func expectedCaptureOperations(capture CaptureReview, snapshot DeliveryProfileSnapshot) ([]OutboxOperation, error) {
	metadata := routing.PublicMetadata{}
	if capture.PublicMetadata != nil {
		metadata = *capture.PublicMetadata
	}
	source := routing.RoutedSource{
		CanonicalURLID: capture.CanonicalURLID, CanonicalURL: capture.CanonicalURL,
		EnrichmentState: capture.EnrichmentState, PublicMetadata: metadata,
		PublicExcerpts: append([]routing.PublicExcerpt{}, capture.PublicExcerpts...),
		Missingness:    append([]string{}, capture.Missingness...), LensResults: append([]routing.LensResult{}, capture.LensResults...),
		SemanticAssessment: capture.SemanticAssessment, Disposition: capture.Disposition,
		DispositionRationale: capture.DispositionRationale, SemanticNodes: append([]routing.SemanticNode{}, capture.SemanticNodes...),
		SemanticEdges: append([]routing.SemanticEdge{}, capture.SemanticEdges...),
	}
	nodeEntryIDs := map[string]string{}
	nodeOperationIDs := map[string]string{}
	operations := make([]OutboxOperation, 0, len(source.SemanticNodes)+len(source.SemanticEdges))
	for _, node := range source.SemanticNodes {
		mapping, ok := snapshot.RoleMappings[node.Role]
		if !ok {
			return nil, errors.New("unsupported destination role mapping")
		}
		entryID := deterministicEntryID(mapping.IDPrefix, source.CanonicalURL, node.SemanticNodeID, mapping.CollectionSlug)
		entry := EntryOperation{CollectionSlug: mapping.CollectionSlug, EntryID: entryID, Name: node.Name, Data: mapNodeData(node, source), SourceRef: source.CanonicalURL, SourceExcerpt: firstExcerpt(source), CreatedBy: ExpectedCreatedBy, ForceDraft: true}
		opID := "op-entry-" + hashText(entryID+"|"+source.CanonicalURL+"|"+node.SemanticNodeID)
		operation := OutboxOperation{OperationID: opID, Kind: "entry", Dependencies: []string{}, Entry: &entry}
		operation.PayloadFingerprint = outboxOperationFingerprint(operation)
		operations = append(operations, operation)
		nodeEntryIDs[node.SemanticNodeID] = entryID
		nodeOperationIDs[node.SemanticNodeID] = opID
	}
	for _, edge := range source.SemanticEdges {
		mapped, ok := snapshot.RelationMappings[edge.Type]
		if !ok || nodeEntryIDs[edge.From] == "" || nodeEntryIDs[edge.To] == "" {
			return nil, errors.New("unsupported destination relation mapping")
		}
		fromID, toID := nodeEntryIDs[edge.From], nodeEntryIDs[edge.To]
		identity := hashText("mindline/relation/v0.1|" + fromID + "|" + mapped + "|" + toID)
		relation := RelationOperation{RelationIdentity: identity, FromEntryID: fromID, ToEntryID: toID, Type: mapped, Metadata: map[string]any{"evidence_refs": append([]string{}, edge.EvidenceRefs...), "lens_refs": edgeLensRefs(source, edge), "rationale": edge.Rationale, "initiator_type": "agent_operator", "judgment_method": "operator_agent_review", "credential_key_id": snapshot.ExpectedKeyID}, IfMissing: true}
		dependencies := []string{nodeOperationIDs[edge.From], nodeOperationIDs[edge.To]}
		sort.Strings(dependencies)
		operation := OutboxOperation{OperationID: "op-relation-" + identity, Kind: "relation", Dependencies: dependencies, Relation: &relation}
		operation.PayloadFingerprint = outboxOperationFingerprint(operation)
		operations = append(operations, operation)
	}
	return operations, nil
}

func canonicalOperationEqual(left, right OutboxOperation) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func outboxOperationFingerprint(operation OutboxOperation) string {
	operation.PayloadFingerprint = ""
	return hashValue(operation)
}

var osReadFile = func(path string) ([]byte, error) { return os.ReadFile(path) }
