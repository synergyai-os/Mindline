package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	SchemaPipelineInput = "pipeline-input/v0.1"
	RunModeDryRun       = "dry_run"
	MethodBASBPARACODE  = "basb-para-code"
	DestinationTolaria  = "tolaria"
)

type SourceKind string

const (
	SourceSlackExport    SourceKind = "slack_export"
	SourceCandidate      SourceKind = "candidate"
	SourceCandidateBatch SourceKind = "candidate_batch"
)

type Input struct {
	SchemaVersion string      `json:"schema_version"`
	RunMode       string      `json:"run_mode"`
	Source        SourceInput `json:"source"`
	Method        IDRef       `json:"method"`
	Destination   IDRef       `json:"destination"`
	AuthorityIDs  []string    `json:"authority_ids"`
	BundleRoot    string      `json:"-"`
	InputPath     string      `json:"-"`
}

type SourceInput struct {
	Kind  SourceKind `json:"kind"`
	Path  string     `json:"path,omitempty"`
	Paths []string   `json:"paths,omitempty"`
}

type IDRef struct {
	ID string `json:"id"`
}

type ParseOptions struct {
	AllowedFutureWorkPackageID string
}

var authorityIDPattern = regexp.MustCompile(`^(DEC|STD|FEAT|WP)-[0-9]+$`)

var allowedAuthorityIDs = map[string]bool{
	"DEC-15": true,
	"DEC-6":  true,
	"DEC-12": true,
	"DEC-13": true,
}

func ParseInputFile(path string, opts ParseOptions) (Input, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Input{}, err
	}
	return ParseInputBytes(data, path, opts)
}

func ParseInputBytes(data []byte, path string, opts ParseOptions) (Input, error) {
	var input Input
	if err := json.Unmarshal(data, &input); err != nil {
		return Input{}, fmt.Errorf("decode pipeline input: %w", err)
	}
	input.InputPath = path
	input.BundleRoot = bundleRoot(path)
	if err := input.Validate(opts); err != nil {
		return Input{}, err
	}
	return input, nil
}

func (i Input) Validate(opts ParseOptions) error {
	if i.SchemaVersion != SchemaPipelineInput {
		return fmt.Errorf("schema_version must be %q", SchemaPipelineInput)
	}
	if i.RunMode != RunModeDryRun {
		return fmt.Errorf("unsupported run_mode: %s", i.RunMode)
	}
	if !oneOf(string(i.Source.Kind), string(SourceSlackExport), string(SourceCandidate), string(SourceCandidateBatch)) {
		return fmt.Errorf("unsupported source kind: %s", i.Source.Kind)
	}
	if i.Source.Kind == SourceCandidateBatch {
		if len(i.Source.Paths) == 0 {
			return fmt.Errorf("source.paths are required for candidate_batch")
		}
	} else if strings.TrimSpace(i.Source.Path) == "" {
		return fmt.Errorf("source.path is required for %s", i.Source.Kind)
	}
	if i.Method.ID != MethodBASBPARACODE {
		return fmt.Errorf("unsupported method: %s", i.Method.ID)
	}
	if i.Destination.ID != DestinationTolaria {
		return fmt.Errorf("unsupported destination: %s", i.Destination.ID)
	}
	return ValidateAuthorityIDs(i.AuthorityIDs, opts)
}

func ValidateAuthorityIDs(ids []string, opts ParseOptions) error {
	if len(ids) == 0 {
		return fmt.Errorf("authority_ids are required")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return fmt.Errorf("authority_id must not be empty")
		}
		if !authorityIDPattern.MatchString(id) {
			return fmt.Errorf("malformed authority_id: %s", id)
		}
		if seen[id] {
			return fmt.Errorf("duplicate authority_id: %s", id)
		}
		seen[id] = true
		if id == "WP-6" && opts.AllowedFutureWorkPackageID != "WP-6" {
			return fmt.Errorf("dropped or unauthorized authority_id: WP-6")
		}
		if !allowedAuthorityIDs[id] && id != opts.AllowedFutureWorkPackageID {
			return fmt.Errorf("unknown authority_id: %s", id)
		}
	}
	return nil
}

func (i Input) SourcePaths() []string {
	if i.Source.Kind == SourceCandidateBatch {
		return append([]string(nil), i.Source.Paths...)
	}
	return []string{i.Source.Path}
}

func (i Input) ResolveBundlePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	target := path
	if !filepath.IsAbs(target) {
		target = filepath.Join(i.BundleRoot, target)
	}
	realRoot, err := filepath.EvalSymlinks(i.BundleRoot)
	if err != nil {
		return "", err
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", err
	}
	realRoot, err = filepath.Abs(realRoot)
	if err != nil {
		return "", err
	}
	realTarget, err = filepath.Abs(realTarget)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(realRoot, realTarget)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path is outside pipeline bundle root")
	}
	return realTarget, nil
}

func bundleRoot(path string) string {
	dir := filepath.Dir(path)
	if filepath.Base(dir) == "inputs" {
		return filepath.Dir(dir)
	}
	return dir
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
