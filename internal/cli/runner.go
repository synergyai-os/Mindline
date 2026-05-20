package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/synergyai-os/Mindline/internal/sbos"
)

const (
	ExitOK            = 0
	ExitUsage         = 1
	ExitProcess       = 2
	ExitArtifactWrite = 3
)

const usage = "usage: mindline process <candidate.json> [--out <dir>]\n"

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
	fs FileSystem
}

type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	MkdirAll(path string, perm fs.FileMode) error
	Stat(path string) (fs.FileInfo, error)
	CanWriteDir(path string) error
	WriteFile(path string, data []byte) error
	Getwd() (string, error)
}

type ResultEnvelope struct {
	State         string             `json:"state"`
	RecordID      string             `json:"record_id"`
	ArtifactCount int                `json:"artifact_count"`
	Artifacts     []ArtifactEnvelope `json:"artifacts"`
	AuthorityIDs  []string           `json:"authority_ids"`
}

type ArtifactEnvelope struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
	Body string `json:"body"`
}

func NewRunner(fileSystem FileSystem) Runner {
	return Runner{fs: fileSystem}
}

func (r Runner) Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "process" {
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
		State:         string(result.State),
		RecordID:      result.RecordID,
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
