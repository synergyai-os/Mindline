package main

import (
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/localservice"
)

func TestAuditedBuildUsesTheSupportedExactTreeLinkerContract(t *testing.T) {
	fingerprint := "sha256:" + strings.Repeat("a", 64)
	flag, err := localservice.AuditedSourceTreeLinkerFlag(fingerprint)
	if err != nil || !strings.Contains(flag, "sourceTreeFingerprint="+fingerprint) {
		t.Fatalf("exact tree linker contract unavailable: flag=%q err=%v", flag, err)
	}
	if _, err := localservice.AuditedSourceTreeLinkerFlag("owner-supplied-label"); err == nil {
		t.Fatal("non-commitment tree label was accepted")
	}
	if !pathInside("/repo", "/repo/bin/mindline") || pathInside("/repo", "/tmp/mindline") {
		t.Fatal("audited build output boundary drifted")
	}
}
