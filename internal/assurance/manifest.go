package assurance

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

const (
	WP46ManifestSchema = "mindline-proof-manifest/v1"
	WP46ManifestID     = "wp46-stable-control-v1"
	WP46ManifestSHA256 = "e7264dda3b6ed3978f6ac880d49be4ac798ce930af433728fbc6e35fe3a152a9"
	WP46GroupCount     = 44
)

//go:embed manifests/wp46-stable-control-v1.json
var embeddedWP46Manifest []byte

var placeholderPattern = regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*)\}`)

// RequiredWP46ContractTests is part of the signed proof-controller contract.
// Removing a test from the source manifest must not weaken the embedded runner.
var RequiredWP46ContractTests = []string{
	"TestWP46_SettingsDefaultsAndExactRoundTrip",
	"TestWP46_SettingsCASRejectsABAAndConflict",
	"TestWP46_SettingsCrashMatrixAndExplicitRecovery",
	"TestWP46_SettingsSurviveThirtyDaysAndRestarts",
	"TestWP46_RunSelectionCASBackupRecoveryAndNoLatest",
	"TestWP46_LegacyV03EvidenceRemainsByteIdentical",
	"TestWP46_BaselineCompatibilityInvocationCannotOpenBrowser",
	"TestWP46_ReceiptBindsExecutableSourceConfigurationManifestAndLauncher",
	"TestWP46_ProofAttemptAuthorityCrashMatrixAndAbandonedRecovery",
	"TestWP46_ProofAttemptsArePhysicallyIsolatedOnSameCommit",
	"TestWP46_FinalClosureRevalidatesFrozenTreeAndArtifacts",
	"TestWP46_ProofRunnerOuterBuildAndEveryGroupInvocationAreExact",
	"TestWP46_ControllerBootstrapCrashMatrixAndSameCommitRetry",
	"TestWP46_MissingGateAllowsOnlySafeReadOnlyState",
	"TestWP46_ProcessLeaseSurvivesElapsedTimeAndRevokesOnEvents",
	"TestWP46_FixedPortCollisionPrecedesMutationAndNeverOpensBrowser",
	"TestWP46_OperatorChannelVerifierRejectsUntrustedInputs",
	"TestWP46_OperatorLauncherOwnsWriterAndChildCannotInheritIt",
	"TestWP46_UnrelatedSameUIDProcessCannotInjectOperatorConfirmation",
	"TestWP46_PairingFrameExpiryReplayConcurrencyAndEOF",
	"TestWP46_HTTPBoundaryMatrix",
	"TestWP46_BrowserCapabilityStorageContract",
	"TestWP46_SettingsHydrationDirtyConflictAdoptionAndRecovery",
	"TestWP46_LocalV04ProofExistsBeforePopulatedRootRollback",
	"TestWP46_ProviderReconnectNeverReplaysMutation",
	"TestWP46_ApprovalBudgetCancellationReadbackReplayRegression",
	"TestWP46_RollbackRollForwardLifecycle",
	"TestWP46_AccessibilityContract",
}

type WP46Manifest struct {
	SchemaVersion            string                  `json:"schema_version"`
	ID                       string                  `json:"id"`
	WorkPackage              string                  `json:"work_package"`
	ShapeSHA256              string                  `json:"shape_sha256"`
	SpecSHA256               string                  `json:"spec_sha256"`
	RollbackBaselineCommit   string                  `json:"rollback_baseline_commit"`
	Execution                manifestExecution       `json:"execution"`
	ProofController          manifestProofController `json:"proof_controller"`
	Bindings                 []manifestBinding       `json:"bindings"`
	AttemptStateMachine      json.RawMessage         `json:"attempt_state_machine"`
	ToolIdentities           []json.RawMessage       `json:"tool_identities"`
	RequiredContractTests    []string                `json:"required_contract_tests"`
	ReceiptCheckMap          []manifestReceiptCheck  `json:"receipt_check_map"`
	PreclosureRequiredGroups []string                `json:"preclosure_required_groups"`
	FinalRevalidationSet     json.RawMessage         `json:"final_revalidation_set"`
	Groups                   []ManifestGroup         `json:"groups"`
	AcceptanceMap            []json.RawMessage       `json:"acceptance_map"`
	EvidenceRecord           json.RawMessage         `json:"evidence_record"`
}

type manifestExecution struct {
	WorkingDirectory string          `json:"working_directory"`
	ArtifactRoot     string          `json:"artifact_root"`
	ArtifactRootMode string          `json:"artifact_root_mode"`
	Shell            bool            `json:"shell"`
	AmbientNetwork   bool            `json:"ambient_network"`
	PhaseOrder       []string        `json:"phase_order"`
	DefaultRunPolicy string          `json:"default_run_policy"`
	FinallyPolicies  []string        `json:"finally_policies"`
	Bootstrap        json.RawMessage `json:"bootstrap"`
	DefectPolicy     string          `json:"defect_policy"`
}

type manifestProofController struct {
	SchemaVersion     string          `json:"schema_version"`
	Source            string          `json:"source"`
	OuterDriver       string          `json:"outer_driver"`
	Build             json.RawMessage `json:"build"`
	Version           manifestVersion `json:"version"`
	Invoke            manifestInvoke  `json:"invoke"`
	BootstrapEvidence json.RawMessage `json:"bootstrap_evidence"`
	IdentityBinding   []string        `json:"identity_binding"`
}

type manifestVersion struct {
	Tool        string   `json:"tool"`
	Argv        []string `json:"argv"`
	StdoutExact string   `json:"stdout_exact"`
}

type manifestInvoke struct {
	Tool             string   `json:"tool"`
	Argv             []string `json:"argv"`
	WorkingDirectory string   `json:"working_directory"`
	Shell            bool     `json:"shell"`
}

type manifestBinding struct {
	Name         string   `json:"name"`
	Source       string   `json:"source"`
	Base         string   `json:"base,omitempty"`
	PathElements []string `json:"path_elements,omitempty"`
	Validation   []string `json:"validation"`
	Generation   string   `json:"generation,omitempty"`
	Group        string   `json:"group,omitempty"`
	Export       string   `json:"export,omitempty"`
	Resolution   string   `json:"resolution,omitempty"`
}

type manifestReceiptCheck struct {
	ReceiptCheck string `json:"receipt_check"`
	Group        string `json:"group"`
}

type ManifestGroup struct {
	ID            string
	Phase         string
	RunPolicy     string
	DependsOn     []string
	AfterAttempts []string
	Tool          string
	Argv          []string
	Artifacts     []string
	Raw           map[string]json.RawMessage
}

var allowedManifestGroupFields = map[string]struct{}{
	"id": {}, "phase": {}, "run_policy": {}, "depends_on": {}, "after_attempts": {},
	"tool": {}, "argv": {}, "artifacts": {}, "predicate": {}, "working_directory": {},
	"commands": {}, "network": {}, "description": {}, "process": {}, "child_argv": {},
	"bundle": {}, "chromium": {}, "operator": {}, "steps": {}, "exports": {},
	"signal": {}, "target_export": {}, "command": {}, "process_observer": {},
	"snapshots": {}, "surface": {}, "controllers": {}, "post_stop_probe": {},
	"stop": {}, "binding": {}, "producer_group": {}, "on_failure": {}, "on_success": {},
	"failure_artifacts": {}, "success_artifacts": {}, "after_terminal": {},
	"environment": {}, "path": {},
}

func (group *ManifestGroup) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for key := range raw {
		if _, ok := allowedManifestGroupFields[key]; !ok {
			return fmt.Errorf("manifest group contains unknown field %q", key)
		}
	}
	decode := func(key string, target any) error {
		value, ok := raw[key]
		if !ok {
			return nil
		}
		if err := json.Unmarshal(value, target); err != nil {
			return fmt.Errorf("manifest group %s: %w", key, err)
		}
		return nil
	}
	if err := decode("id", &group.ID); err != nil {
		return err
	}
	if err := decode("phase", &group.Phase); err != nil {
		return err
	}
	if err := decode("run_policy", &group.RunPolicy); err != nil {
		return err
	}
	if err := decode("depends_on", &group.DependsOn); err != nil {
		return err
	}
	if err := decode("after_attempts", &group.AfterAttempts); err != nil {
		return err
	}
	if err := decode("tool", &group.Tool); err != nil {
		return err
	}
	if err := decode("argv", &group.Argv); err != nil {
		return err
	}
	if err := decode("artifacts", &group.Artifacts); err != nil {
		return err
	}
	group.Raw = raw
	return nil
}

// EmbeddedWP46Manifest returns a copy so callers cannot mutate runner authority.
func EmbeddedWP46Manifest() []byte {
	return append([]byte(nil), embeddedWP46Manifest...)
}

// LoadSignedWP46Manifest requires the source-controlled manifest to be byte-for-byte
// identical to the copy embedded into this runner binary.
func LoadSignedWP46Manifest(path string) (WP46Manifest, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return WP46Manifest{}, errors.New("source manifest must be a regular non-symlink file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return WP46Manifest{}, err
	}
	if !bytes.Equal(data, embeddedWP46Manifest) {
		return WP46Manifest{}, errors.New("source manifest differs from embedded manifest")
	}
	return ParseWP46Manifest(data)
}

func ParseWP46Manifest(data []byte) (WP46Manifest, error) {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return WP46Manifest{}, err
	}
	if err := requireEmbeddedManifestShape(data); err != nil {
		return WP46Manifest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest WP46Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return WP46Manifest{}, fmt.Errorf("strict manifest decode: %w", err)
	}
	if decoder.More() {
		return WP46Manifest{}, errors.New("manifest contains trailing JSON")
	}
	if err := validateWP46Manifest(manifest, data); err != nil {
		return WP46Manifest{}, err
	}
	return manifest, nil
}

func requireEmbeddedManifestShape(data []byte) error {
	var candidate, signed any
	if err := json.Unmarshal(data, &candidate); err != nil {
		return err
	}
	if err := json.Unmarshal(embeddedWP46Manifest, &signed); err != nil {
		return errors.New("embedded manifest is invalid")
	}
	var compare func(any, any, string) error
	compare = func(actual, expected any, path string) error {
		switch expectedValue := expected.(type) {
		case map[string]any:
			actualValue, ok := actual.(map[string]any)
			if !ok {
				return fmt.Errorf("manifest field %s has the wrong type", path)
			}
			for key := range actualValue {
				if _, exists := expectedValue[key]; !exists {
					return fmt.Errorf("manifest field %s contains unknown field %q", path, key)
				}
			}
			for key, expectedChild := range expectedValue {
				actualChild, exists := actualValue[key]
				if !exists {
					return fmt.Errorf("manifest field %s is missing required field %q", path, key)
				}
				if err := compare(actualChild, expectedChild, path+"."+key); err != nil {
					return err
				}
			}
		case []any:
			actualValue, ok := actual.([]any)
			if !ok || len(actualValue) != len(expectedValue) {
				return fmt.Errorf("manifest field %s has a changed array shape", path)
			}
			for index := range expectedValue {
				if err := compare(actualValue[index], expectedValue[index], fmt.Sprintf("%s[%d]", path, index)); err != nil {
					return err
				}
			}
		case string:
			if _, ok := actual.(string); !ok {
				return fmt.Errorf("manifest field %s has the wrong type", path)
			}
		case bool:
			if _, ok := actual.(bool); !ok {
				return fmt.Errorf("manifest field %s has the wrong type", path)
			}
		case float64:
			if _, ok := actual.(float64); !ok {
				return fmt.Errorf("manifest field %s has the wrong type", path)
			}
		case nil:
			if actual != nil {
				return fmt.Errorf("manifest field %s has the wrong type", path)
			}
		}
		return nil
	}
	return compare(candidate, signed, "manifest")
}

func validateWP46Manifest(manifest WP46Manifest, data []byte) error {
	digest := sha256.Sum256(data)
	if bytes.Equal(data, embeddedWP46Manifest) && hex.EncodeToString(digest[:]) != WP46ManifestSHA256 {
		return errors.New("embedded manifest fingerprint mismatch")
	}
	if manifest.SchemaVersion != WP46ManifestSchema || manifest.ID != WP46ManifestID || manifest.WorkPackage != "WP-46" {
		return errors.New("manifest identity mismatch")
	}
	if manifest.Execution.Shell || manifest.ProofController.Invoke.Shell {
		return errors.New("manifest shell execution is forbidden")
	}
	if manifest.ProofController.SchemaVersion != ProofRunnerVersion ||
		manifest.ProofController.Version.StdoutExact != ProofRunnerVersion+"\n" ||
		!equalStrings(manifest.ProofController.Version.Argv, []string{"--version"}) {
		return errors.New("proof controller identity mismatch")
	}
	bindings := make(map[string]struct{}, len(manifest.Bindings))
	for _, binding := range manifest.Bindings {
		if binding.Name == "" {
			return errors.New("manifest binding is missing a name")
		}
		if _, exists := bindings[binding.Name]; exists {
			return fmt.Errorf("duplicate manifest binding %q", binding.Name)
		}
		bindings[binding.Name] = struct{}{}
	}
	for _, match := range placeholderPattern.FindAllSubmatch(data, -1) {
		if _, exists := bindings[string(match[1])]; !exists {
			return fmt.Errorf("unrecognized manifest placeholder %q", match[1])
		}
	}
	if len(manifest.Groups) != WP46GroupCount {
		return fmt.Errorf("manifest requires %d groups", WP46GroupCount)
	}
	groups := make(map[string]ManifestGroup, len(manifest.Groups))
	phases := make(map[string]struct{}, len(manifest.Execution.PhaseOrder))
	for _, phase := range manifest.Execution.PhaseOrder {
		phases[phase] = struct{}{}
	}
	for _, group := range manifest.Groups {
		if group.ID == "" || group.Phase == "" || group.Tool == "" {
			return errors.New("manifest group identity is incomplete")
		}
		if _, exists := groups[group.ID]; exists {
			return fmt.Errorf("duplicate manifest group %q", group.ID)
		}
		if _, exists := phases[group.Phase]; !exists {
			return fmt.Errorf("manifest group %q has unknown phase", group.ID)
		}
		if group.Tool == "${MINDLINE_PROOF_RUNNER}" && !equalStrings(group.Argv, []string{"group", group.ID}) {
			return fmt.Errorf("runner-owned group %q has inexact argv", group.ID)
		}
		groups[group.ID] = group
	}
	for _, group := range manifest.Groups {
		for _, dependency := range append(append([]string{}, group.DependsOn...), group.AfterAttempts...) {
			if _, exists := groups[dependency]; !exists {
				return fmt.Errorf("manifest group %q references missing group %q", group.ID, dependency)
			}
		}
	}
	if err := validateExactSet(manifest.RequiredContractTests, RequiredWP46ContractTests, "contract tests"); err != nil {
		return err
	}
	allButCloser := make([]string, 0, len(manifest.Groups)-1)
	for _, group := range manifest.Groups {
		if group.ID != "close_attempt_and_finalize_evidence" {
			allButCloser = append(allButCloser, group.ID)
		}
	}
	if err := validateExactSet(manifest.PreclosureRequiredGroups, allButCloser, "preclosure groups"); err != nil {
		return err
	}
	for _, mapping := range manifest.ReceiptCheckMap {
		if _, exists := groups[mapping.Group]; !exists || strings.TrimSpace(mapping.ReceiptCheck) == "" {
			return errors.New("receipt check map references an invalid group")
		}
	}
	var generic any
	if err := json.Unmarshal(data, &generic); err != nil {
		return err
	}
	if containsEnabledShell(generic) {
		return errors.New("manifest enables shell execution")
	}
	return nil
}

func validateExactSet(actual, expected []string, label string) error {
	a := append([]string(nil), actual...)
	e := append([]string(nil), expected...)
	sort.Strings(a)
	sort.Strings(e)
	if !equalStrings(a, e) {
		return fmt.Errorf("manifest %s are missing, duplicated, or changed", label)
	}
	return nil
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

func containsEnabledShell(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "shell" {
				if enabled, ok := child.(bool); ok && enabled {
					return true
				}
			}
			if containsEnabledShell(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsEnabledShell(child) {
				return true
			}
		}
	}
	return false
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var visit func() error
	visit = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("JSON object key is not a string")
				}
				if _, exists := seen[key]; exists {
					return fmt.Errorf("duplicate JSON field %q", key)
				}
				seen[key] = struct{}{}
				if err := visit(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := visit(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return errors.New("unexpected JSON delimiter")
		}
	}
	if err := visit(); err != nil {
		return err
	}
	if token, err := decoder.Token(); err == nil || token != nil {
		return errors.New("manifest contains trailing JSON")
	}
	return nil
}
