package recallproof

import (
	"regexp"
	"testing"
)

func TestLiveConfigurationFingerprintIsDeterministicAndClosed(t *testing.T) {
	first := LiveConfigurationFingerprint()
	second := LiveConfigurationFingerprint()
	if first != second {
		t.Fatalf("live configuration fingerprint drifted: %s != %s", first, second)
	}
	if !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(first) {
		t.Fatalf("invalid live configuration fingerprint: %s", first)
	}
	configuration := currentLiveConfiguration()
	if configuration.SchemaVersion != liveConfigurationSchema ||
		configuration.ResourceProfile.Fingerprint == "" ||
		configuration.CompactPolicy.Fingerprint == "" ||
		configuration.MaximumCaptureLibraryBytes <= 0 ||
		configuration.MaximumEnvelopeBytes <= 0 ||
		configuration.MaximumRetrievalContentBytes <= 0 ||
		configuration.MaximumCompactSnippetRunes <= 0 ||
		configuration.MaximumSearchLimit <= 0 {
		t.Fatalf("live configuration projection is incomplete: %#v", configuration)
	}
}
