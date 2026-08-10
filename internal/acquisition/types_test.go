package acquisition

import (
	"strings"
	"testing"
	"time"
)

func TestNativeTimestampPreservesSlackSubsecondChronology(t *testing.T) {
	original, err := NativeTimestampToRFC3339("1785000001.000001")
	if err != nil {
		t.Fatal(err)
	}
	revision, err := NativeRevisionTimestampToRFC3339("1785000001.000002")
	if err != nil {
		t.Fatal(err)
	}
	originalTime, originalErr := time.Parse(time.RFC3339Nano, original)
	revisionTime, revisionErr := time.Parse(time.RFC3339Nano, revision)
	if originalErr != nil || revisionErr != nil || !originalTime.Before(revisionTime) ||
		strings.Contains(original, ".") || !strings.HasSuffix(revision, ".000002Z") {
		t.Fatalf("canonical replay compatibility or Slack edit chronology drifted: original=%q revision=%q", original, revision)
	}
}
