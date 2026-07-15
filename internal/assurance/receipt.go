package assurance

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

const PreLiveReceiptSchema = "mindline-pre-live-gate-receipt/v0.2"

var RequiredChecks = []string{
	"go_test",
	"targeted_race",
	"go_vet",
	"git_diff_check",
	"govulncheck",
	"gosec",
	"gitleaks_clean_head",
	"gitleaks_history",
	"gitleaks_runtime_surface",
	"sentinel_surface_scan",
	"hardened_browser",
	"productbrain_crash_replay",
	"activation_journal_recovery",
	"legacy_compatibility",
}

type Check struct {
	Name                string `json:"name"`
	ToolVersion         string `json:"tool_version"`
	Outcome             string `json:"outcome"`
	EvidenceFingerprint string `json:"evidence_fingerprint"`
}

type Receipt struct {
	SchemaVersion            string  `json:"schema_version"`
	Fingerprint              string  `json:"fingerprint"`
	Commit                   string  `json:"commit"`
	ConfigurationFingerprint string  `json:"configuration_fingerprint"`
	SourceBindingFingerprint string  `json:"source_binding_fingerprint"`
	BuildArtifactFingerprint string  `json:"build_artifact_fingerprint"`
	GeneratedAt              string  `json:"generated_at"`
	Checks                   []Check `json:"checks"`
	RunnerVersion            string  `json:"runner_version"`
	GatePlanFingerprint      string  `json:"gate_plan_fingerprint"`
	PrivateDataAuthorized    bool    `json:"private_data_authorized"`
	RealCredentialAuthorized bool    `json:"real_credential_authorized"`
	RealTransportAuthorized  bool    `json:"real_transport_authorized"`
}

func Build(commit, configurationFingerprint, sourceBindingFingerprint string, generatedAt time.Time, checks []Check) (Receipt, error) {
	artifactFingerprint, err := currentExecutableFingerprint()
	if err != nil {
		return Receipt{}, errors.New("pre-live executable binding unavailable")
	}
	receipt := Receipt{
		SchemaVersion:            PreLiveReceiptSchema,
		Commit:                   strings.TrimSpace(commit),
		ConfigurationFingerprint: strings.TrimSpace(configurationFingerprint),
		SourceBindingFingerprint: strings.TrimSpace(sourceBindingFingerprint),
		BuildArtifactFingerprint: artifactFingerprint,
		GeneratedAt:              generatedAt.UTC().Format(time.RFC3339Nano),
		Checks:                   append([]Check{}, checks...),
		RunnerVersion:            FixedGateRunnerVersion,
		GatePlanFingerprint:      fixedGatePlanFingerprint(),
	}
	if receipt.Commit == "" || receipt.ConfigurationFingerprint == "" || !validSHA256Fingerprint(receipt.SourceBindingFingerprint) || generatedAt.IsZero() {
		return Receipt{}, errors.New("incomplete pre-live gate binding")
	}
	if err := validateChecks(receipt.Checks); err != nil {
		return Receipt{}, err
	}
	receipt.PrivateDataAuthorized = true
	receipt.RealCredentialAuthorized = true
	receipt.RealTransportAuthorized = true
	receipt.Fingerprint = fingerprint(receipt)
	return receipt, nil
}

func validSHA256Fingerprint(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func Validate(receipt Receipt, expectedCommit, expectedConfiguration string) error {
	if receipt.SchemaVersion != PreLiveReceiptSchema || receipt.Fingerprint == "" || receipt.Fingerprint != fingerprint(receipt) {
		return errors.New("invalid pre-live gate receipt")
	}
	if receipt.RunnerVersion != FixedGateRunnerVersion || receipt.GatePlanFingerprint != fixedGatePlanFingerprint() {
		return errors.New("pre-live gate runner identity mismatch")
	}
	if receipt.Commit != strings.TrimSpace(expectedCommit) || receipt.ConfigurationFingerprint != strings.TrimSpace(expectedConfiguration) {
		return errors.New("pre-live gate binding drift")
	}
	if _, err := time.Parse(time.RFC3339Nano, receipt.GeneratedAt); err != nil {
		return errors.New("pre-live gate generated time is invalid")
	}
	actualArtifact, err := currentExecutableFingerprint()
	if err != nil || !constantTimeEqual(receipt.BuildArtifactFingerprint, actualArtifact) {
		return errors.New("pre-live gate executable binding drift")
	}
	if !receipt.PrivateDataAuthorized || !receipt.RealCredentialAuthorized || !receipt.RealTransportAuthorized {
		return errors.New("pre-live gate authority incomplete")
	}
	return validateChecks(receipt.Checks)
}

func currentExecutableFingerprint() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("executable is not a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func constantTimeEqual(left, right string) bool {
	return len(left) == len(right) && subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func Write(root, path string, receipt Receipt) error {
	if err := Validate(receipt, receipt.Commit, receipt.ConfigurationFingerprint); err != nil {
		return err
	}
	if err := privateio.ValidateContained(root, filepath.Dir(path)); err != nil {
		return err
	}
	return privateio.WriteJSON(path, receipt)
}

func Load(root, path string) (Receipt, error) {
	var receipt Receipt
	if err := privateio.ReadJSONStrictBounded(root, path, 1<<20, &receipt); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func validateChecks(checks []Check) error {
	if len(checks) != len(RequiredChecks) {
		return errors.New("pre-live gate check set incomplete")
	}
	byName := make(map[string]Check, len(checks))
	for _, check := range checks {
		if strings.TrimSpace(check.Name) == "" || byName[check.Name].Name != "" {
			return errors.New("pre-live gate check set invalid")
		}
		if check.Outcome != "pass" || strings.TrimSpace(check.ToolVersion) == "" || strings.TrimSpace(check.EvidenceFingerprint) == "" {
			return fmt.Errorf("pre-live gate check %s did not pass", check.Name)
		}
		byName[check.Name] = check
	}
	for _, required := range RequiredChecks {
		if byName[required].Name == "" {
			return fmt.Errorf("pre-live gate missing %s", required)
		}
	}
	return nil
}

func fingerprint(receipt Receipt) string {
	receipt.Fingerprint = ""
	receipt.Checks = append([]Check{}, receipt.Checks...)
	sort.Slice(receipt.Checks, func(i, j int) bool { return receipt.Checks[i].Name < receipt.Checks[j].Name })
	data, _ := json.Marshal(receipt)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
