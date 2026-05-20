package destinations

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DestinationInputSchemaVersion = "destination-input/v0.1"

type ParseOptions struct {
	BaseDir          string
	AllowFixtureDirs []string
}

type InputResult struct {
	State             string
	RecordID          string
	SourceCandidateID string
	IdempotencyKey    string
	AuthorityIDs      []string
	Artifacts         []InputArtifact
	Safety            InputSafety
}

type InputArtifact struct {
	Kind string
	Body string
}

type InputSafety struct {
	PrivateProvenance bool `json:"private_provenance"`
	RedactionRequired bool `json:"redaction_required"`
	SecretLike        bool `json:"secret_like"`
}

type destinationInputEnvelope struct {
	SchemaVersion string                 `json:"schema_version"`
	Result        destinationInputResult `json:"result"`
}

type destinationInputResult struct {
	State             string                     `json:"state"`
	RecordID          string                     `json:"record_id"`
	SourceCandidateID string                     `json:"source_candidate_id"`
	IdempotencyKey    string                     `json:"idempotency_key"`
	AuthorityIDs      []string                   `json:"authority_ids"`
	Artifacts         []destinationInputArtifact `json:"artifacts"`
	Safety            InputSafety                `json:"safety"`
	AllowNoArtifacts  bool                       `json:"-"`
}

type destinationInputArtifact struct {
	Kind string `json:"kind"`
	Body string `json:"body"`
	Path string `json:"path"`
}

type processResultEnvelope struct {
	State        string                     `json:"state"`
	RecordID     string                     `json:"record_id"`
	Artifacts    []destinationInputArtifact `json:"artifacts"`
	AuthorityIDs []string                   `json:"authority_ids"`
}

func ParseDestinationInput(input []byte, opts ParseOptions) (InputResult, error) {
	var envelope destinationInputEnvelope
	if err := json.Unmarshal(input, &envelope); err != nil {
		return InputResult{}, fmt.Errorf("decode destination input: %w", err)
	}
	if envelope.SchemaVersion == "" {
		return parseProcessResultEnvelope(input, opts)
	}
	if envelope.SchemaVersion != DestinationInputSchemaVersion {
		return InputResult{}, fmt.Errorf("schema_version must be %q", DestinationInputSchemaVersion)
	}
	return buildInputResult(envelope.Result, opts)
}

func parseProcessResultEnvelope(input []byte, opts ParseOptions) (InputResult, error) {
	var envelope processResultEnvelope
	if err := json.Unmarshal(input, &envelope); err != nil {
		return InputResult{}, fmt.Errorf("decode process result: %w", err)
	}
	result := destinationInputResult{
		State:             envelope.State,
		RecordID:          envelope.RecordID,
		SourceCandidateID: envelope.RecordID,
		IdempotencyKey:    "record:" + envelope.RecordID,
		AuthorityIDs:      envelope.AuthorityIDs,
		Artifacts:         envelope.Artifacts,
		AllowNoArtifacts:  true,
	}
	return buildInputResult(result, opts)
}

func buildInputResult(result destinationInputResult, opts ParseOptions) (InputResult, error) {
	if strings.TrimSpace(result.State) == "" {
		return InputResult{}, fmt.Errorf("state is required")
	}
	if strings.TrimSpace(result.RecordID) == "" {
		return InputResult{}, fmt.Errorf("record_id is required")
	}
	if strings.TrimSpace(result.SourceCandidateID) == "" {
		return InputResult{}, fmt.Errorf("source_candidate_id is required")
	}
	if strings.TrimSpace(result.IdempotencyKey) == "" {
		return InputResult{}, fmt.Errorf("idempotency_key is required")
	}
	if len(result.AuthorityIDs) == 0 {
		return InputResult{}, fmt.Errorf("authority_ids are required")
	}
	if len(result.Artifacts) == 0 && !result.AllowNoArtifacts {
		return InputResult{}, fmt.Errorf("artifact is required")
	}

	artifacts := make([]InputArtifact, 0, len(result.Artifacts))
	for index, artifact := range result.Artifacts {
		if strings.TrimSpace(artifact.Kind) == "" {
			return InputResult{}, fmt.Errorf("artifact %d kind is required", index)
		}
		body := artifact.Body
		if body == "" && artifact.Path != "" {
			resolvedBody, err := readAllowedArtifact(artifact.Path, opts)
			if err != nil {
				return InputResult{}, fmt.Errorf("artifact %d path: %w", index, err)
			}
			body = resolvedBody
		}
		if body == "" {
			return InputResult{}, fmt.Errorf("artifact %d body or path is required", index)
		}
		artifacts = append(artifacts, InputArtifact{Kind: artifact.Kind, Body: body})
	}

	return InputResult{
		State:             result.State,
		RecordID:          result.RecordID,
		SourceCandidateID: result.SourceCandidateID,
		IdempotencyKey:    result.IdempotencyKey,
		AuthorityIDs:      append([]string(nil), result.AuthorityIDs...),
		Artifacts:         artifacts,
		Safety:            result.Safety,
	}, nil
}

func readAllowedArtifact(path string, opts ParseOptions) (string, error) {
	if strings.TrimSpace(opts.BaseDir) == "" {
		return "", fmt.Errorf("base_dir is required for artifact path references")
	}
	roots, err := canonicalRoots(opts)
	if err != nil {
		return "", err
	}
	target := path
	if !filepath.IsAbs(target) {
		target = filepath.Join(opts.BaseDir, target)
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", err
	}
	realTarget, err = filepath.Abs(realTarget)
	if err != nil {
		return "", err
	}
	if !isUnderAnyRoot(realTarget, roots) {
		return "", fmt.Errorf("artifact path is outside allowed roots")
	}
	data, err := os.ReadFile(realTarget)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func canonicalRoots(opts ParseOptions) ([]string, error) {
	rootInputs := append([]string{opts.BaseDir}, opts.AllowFixtureDirs...)
	roots := make([]string, 0, len(rootInputs))
	for _, root := range rootInputs {
		if strings.TrimSpace(root) == "" {
			continue
		}
		realRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			return nil, err
		}
		realRoot, err = filepath.Abs(realRoot)
		if err != nil {
			return nil, err
		}
		roots = append(roots, realRoot)
	}
	return roots, nil
}

func isUnderAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			continue
		}
		if rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel)) {
			return true
		}
	}
	return false
}
