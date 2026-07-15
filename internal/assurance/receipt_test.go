package assurance

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func passingChecks() []Check {
	checks := make([]Check, 0, len(RequiredChecks))
	for _, name := range RequiredChecks {
		checks = append(checks, Check{Name: name, ToolVersion: "pinned-v1", Outcome: "pass", EvidenceFingerprint: "sha256-" + name})
	}
	return checks
}

const testSourceBinding = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestReceiptBindsCommitConfigurationAndCompleteChecks(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	receipt, err := Build("commit-1", "config-1", testSourceBinding, now, passingChecks())
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(receipt, "commit-1", "config-1", now.Add(time.Hour), 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ commit, config string }{{"commit-2", "config-1"}, {"commit-1", "config-2"}} {
		if err := Validate(receipt, test.commit, test.config, now.Add(time.Hour), 24*time.Hour); err == nil {
			t.Fatal("drifted receipt accepted")
		}
	}
}

func TestReceiptFailsClosedForMissingFailedDuplicateOrExpiredChecks(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		checks []Check
	}{
		{name: "missing", checks: passingChecks()[:len(RequiredChecks)-1]},
		{name: "failed", checks: func() []Check { values := passingChecks(); values[0].Outcome = "fail"; return values }()},
		{name: "duplicate", checks: func() []Check { values := passingChecks(); values[0] = values[1]; return values }()},
		{name: "tool unavailable", checks: func() []Check { values := passingChecks(); values[0].ToolVersion = ""; return values }()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Build("commit", "config", testSourceBinding, now, test.checks); err == nil {
				t.Fatal("unsafe receipt built")
			}
		})
	}
	receipt, err := Build("commit", "config", testSourceBinding, now, passingChecks())
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(receipt, "commit", "config", now.Add(25*time.Hour), 24*time.Hour); err == nil {
		t.Fatal("expired receipt accepted")
	}
}

func TestReceiptTamperAndPrivateRoundTrip(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	receipt, err := Build("commit", "config", testSourceBinding, now, passingChecks())
	if err != nil {
		t.Fatal(err)
	}
	tampered := receipt
	tampered.Checks = append([]Check{}, receipt.Checks...)
	tampered.Checks[0].EvidenceFingerprint = "changed"
	if err := Validate(tampered, "commit", "config", now, 24*time.Hour); err == nil {
		t.Fatal("tampered receipt accepted")
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "pre-live-receipt.json")
	if err := Write(root, path, receipt); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(root, path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Fingerprint != receipt.Fingerprint {
		t.Fatal("receipt fingerprint changed")
	}
}
