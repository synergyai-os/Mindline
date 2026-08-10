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
	revision, err := NativeTimestampToRFC3339("1785000001.000002")
	if err != nil {
		t.Fatal(err)
	}
	originalTime, originalErr := time.Parse(time.RFC3339Nano, original)
	revisionTime, revisionErr := time.Parse(time.RFC3339Nano, revision)
	if originalErr != nil || revisionErr != nil || !originalTime.Before(revisionTime) ||
		!strings.HasSuffix(original, ".000001Z") || !strings.HasSuffix(revision, ".000002Z") {
		t.Fatalf("Slack edit chronology was truncated: original=%q revision=%q", original, revision)
	}
}
