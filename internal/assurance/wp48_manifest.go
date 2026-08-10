package assurance

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"syscall"
)

const (
	WP48ManifestSchema = "mindline-proof-manifest/wp48-v1"
	WP48ManifestID     = "wp48-complete-recall-v1"
)

//go:embed manifests/wp48-complete-recall-v1.json
var embeddedWP48Manifest []byte

// WP48Manifest is intentionally separate from WP-46's executable controller.
// It freezes the source-controlled reusable proof contract without changing
// existing WP-46 parsing or runtime behavior.
type WP48Manifest struct {
	SchemaVersion string      `json:"schema_version"`
	ID            string      `json:"id"`
	WorkPackage   string      `json:"work_package"`
	Groups        []WP48Group `json:"groups"`
}

type WP48Group struct {
	ID        string   `json:"id"`
	Phase     string   `json:"phase"`
	DependsOn []string `json:"depends_on"`
	Tool      string   `json:"tool"`
	Argv      []string `json:"argv"`
	Shell     bool     `json:"shell"`
}

var requiredWP48Groups = []WP48Group{
	{ID: "wp48_ingestion_fixture", Phase: "pre_live", Tool: "go", Argv: []string{"test", "./internal/ingestioncontroller", "./internal/adapters/slack", "./internal/recallproof", "./internal/recallfixture"}, Shell: false},
	{ID: "wp48_resource_fetch_privacy", Phase: "pre_live", DependsOn: []string{"wp48_ingestion_fixture"}, Tool: "go", Argv: []string{"test", "./internal/resourcequeue", "./internal/resourcefetch", "./internal/resourcepipeline", "./internal/recallproof", "./internal/recallfixture"}, Shell: false},
	{ID: "wp48_resourcepipeline", Phase: "pre_live", DependsOn: []string{"wp48_resource_fetch_privacy"}, Tool: "go", Argv: []string{"test", "./internal/resourcepipeline", "./internal/resourcequeue"}, Shell: false},
	{ID: "wp48_compact_feedback", Phase: "pre_live", DependsOn: []string{"wp48_ingestion_fixture"}, Tool: "go", Argv: []string{"test", "./internal/personalmemory", "./internal/localservice", "./internal/agentstate"}, Shell: false},
	{ID: "wp48_cli", Phase: "pre_live", DependsOn: []string{"wp48_compact_feedback"}, Tool: "go", Argv: []string{"test", "./internal/cli"}, Shell: false},
	{ID: "wp48_agentstate", Phase: "pre_live", DependsOn: []string{"wp48_compact_feedback"}, Tool: "go", Argv: []string{"test", "./internal/agentstate"}, Shell: false},
	{ID: "wp48_founderreview", Phase: "pre_live", DependsOn: []string{"wp48_agentstate"}, Tool: "go", Argv: []string{"test", "./internal/founderreview"}, Shell: false},
	{ID: "wp48_recallfixture", Phase: "pre_live", DependsOn: []string{"wp48_ingestion_fixture"}, Tool: "go", Argv: []string{"test", "./internal/recallfixture", "./internal/recallproof", "./internal/recalleval"}, Shell: false},
	{ID: "wp48_lifecycle_rollback", Phase: "pre_live", DependsOn: []string{"wp48_resourcepipeline", "wp48_cli", "wp48_founderreview", "wp48_recallfixture"}, Tool: "go", Argv: []string{"test", "./internal/assurance", "./internal/recallproof", "./internal/recalleval", "./internal/founderreview", "./internal/localservice", "./cmd/mindline-recall-eval"}, Shell: false},
	{ID: "wp48_race", Phase: "pre_live", DependsOn: []string{"wp48_lifecycle_rollback"}, Tool: "go", Argv: []string{"test", "-race", "./internal/ingestioncontroller", "./internal/resourcefetch", "./internal/resourcequeue", "./internal/resourcepipeline", "./internal/personalmemory", "./internal/localservice", "./internal/agentstate", "./internal/recallfixture", "./internal/recalleval", "./internal/recallproof", "./internal/founderreview"}, Shell: false},
	{ID: "wp48_vet", Phase: "pre_live", DependsOn: []string{"wp48_race"}, Tool: "go", Argv: []string{"vet", "./internal/ingestioncontroller", "./internal/resourcefetch", "./internal/resourcequeue", "./internal/resourcepipeline", "./internal/personalmemory", "./internal/localservice", "./internal/agentstate", "./internal/cli", "./internal/assurance", "./internal/recallfixture", "./internal/recalleval", "./internal/recallproof", "./internal/founderreview"}, Shell: false},
	{ID: "wp48_build", Phase: "pre_live", DependsOn: []string{"wp48_vet"}, Tool: "go", Argv: []string{"build", "-trimpath", "-o", "/tmp/mindline-wp48-recall-proof", "./cmd/mindline-recall-proof"}, Shell: false},
	{ID: "wp48_diff", Phase: "pre_live", DependsOn: []string{"wp48_build"}, Tool: "git", Argv: []string{"diff", "--check"}, Shell: false},
	{ID: "wp48_private_scanner", Phase: "pre_live", DependsOn: []string{"wp48_diff"}, Tool: "go", Argv: []string{"run", "./cmd/mindline-secret-check", "--root", ".", "--self-test"}, Shell: false},
	{ID: "wp48_eval_proof", Phase: "post_live", DependsOn: []string{"wp48_private_scanner"}, Tool: "go", Argv: []string{"test", "./internal/recalleval", "./cmd/mindline-recall-eval", "./cmd/mindline-recall-proof"}, Shell: false},
}

func EmbeddedWP48Manifest() []byte { return append([]byte(nil), embeddedWP48Manifest...) }

func LoadSignedWP48Manifest(path string) (WP48Manifest, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return WP48Manifest{}, errors.New("WP-48 manifest must be a regular non-symlink file")
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return WP48Manifest{}, err
	}
	defer file.Close()
	if info.Size() != int64(len(embeddedWP48Manifest)) {
		return WP48Manifest{}, errors.New("WP-48 source manifest differs from embedded manifest")
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(len(embeddedWP48Manifest))+1))
	if err != nil || len(data) != len(embeddedWP48Manifest) {
		return WP48Manifest{}, errors.New("WP-48 source manifest differs from embedded manifest")
	}
	if !bytes.Equal(data, embeddedWP48Manifest) {
		return WP48Manifest{}, errors.New("WP-48 source manifest differs from embedded manifest")
	}
	return ParseWP48Manifest(data)
}

func ParseWP48Manifest(data []byte) (WP48Manifest, error) {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return WP48Manifest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest WP48Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return WP48Manifest{}, fmt.Errorf("strict WP-48 manifest decode: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return WP48Manifest{}, errors.New("WP-48 manifest contains trailing JSON")
	}
	if err := validateWP48Manifest(manifest); err != nil {
		return WP48Manifest{}, err
	}
	return manifest, nil
}

func validateWP48Manifest(manifest WP48Manifest) error {
	if manifest.SchemaVersion != WP48ManifestSchema || manifest.ID != WP48ManifestID || manifest.WorkPackage != "WP-48" {
		return errors.New("invalid WP-48 manifest identity")
	}
	if len(manifest.Groups) != len(requiredWP48Groups) {
		return errors.New("WP-48 manifest omits or adds required proof groups")
	}
	byID := map[string]WP48Group{}
	for _, group := range manifest.Groups {
		if group.ID == "" || group.Phase == "" || group.Tool == "" || len(group.Argv) == 0 || group.Shell {
			return errors.New("WP-48 proof commands must be non-empty and shell-free")
		}
		if _, exists := byID[group.ID]; exists {
			return fmt.Errorf("duplicate WP-48 proof group: %s", group.ID)
		}
		byID[group.ID] = group
	}
	for _, required := range requiredWP48Groups {
		actual, exists := byID[required.ID]
		if !exists {
			return fmt.Errorf("missing required WP-48 proof group: %s", required.ID)
		}
		if !equalWP48Group(actual, required) {
			return fmt.Errorf("WP-48 proof group command drift: %s", required.ID)
		}
		for _, dependency := range actual.DependsOn {
			if _, exists := byID[dependency]; !exists {
				return fmt.Errorf("WP-48 group %s depends on unknown group", actual.ID)
			}
		}
	}
	return nil
}

func equalWP48Group(left, right WP48Group) bool {
	if left.ID != right.ID || left.Phase != right.Phase || left.Tool != right.Tool || left.Shell != right.Shell || len(left.Argv) != len(right.Argv) || len(left.DependsOn) != len(right.DependsOn) {
		return false
	}
	for index := range left.Argv {
		if left.Argv[index] != right.Argv[index] {
			return false
		}
	}
	leftDeps, rightDeps := append([]string(nil), left.DependsOn...), append([]string(nil), right.DependsOn...)
	sort.Strings(leftDeps)
	sort.Strings(rightDeps)
	for index := range leftDeps {
		if leftDeps[index] != rightDeps[index] {
			return false
		}
	}
	return true
}

func WP48ManifestFingerprint() string {
	digest := sha256.Sum256(embeddedWP48Manifest)
	return hex.EncodeToString(digest[:])
}
