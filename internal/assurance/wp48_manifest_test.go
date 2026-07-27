package assurance

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestWP48ManifestIsStrictCompleteAndShellFree(t *testing.T) {
	path := filepath.Join("manifests", "wp48-complete-recall-v1.json")
	manifest, err := LoadSignedWP48Manifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Groups) != len(requiredWP48Groups) {
		t.Fatal("required groups missing")
	}
	for _, group := range manifest.Groups {
		if group.Shell {
			t.Fatalf("group %s uses shell", group.ID)
		}
	}

	unknown := bytes.Replace(EmbeddedWP48Manifest(), []byte(`"shell":false}`), []byte(`"shell":false,"private_field":true}`), 1)
	if _, err := ParseWP48Manifest(unknown); err == nil {
		t.Fatal("unknown field accepted")
	}
	duplicate := bytes.Replace(EmbeddedWP48Manifest(), []byte(`"id":"wp48_ingestion_fixture"`), []byte(`"id":"wp48_ingestion_fixture","id":"duplicate"`), 1)
	if _, err := ParseWP48Manifest(duplicate); err == nil {
		t.Fatal("duplicate key accepted")
	}
	missingGroup := []byte(`{"schema_version":"mindline-proof-manifest/wp48-v1","id":"wp48-complete-recall-v1","work_package":"WP-48","groups":[]}`)
	if _, err := ParseWP48Manifest(missingGroup); err == nil {
		t.Fatal("omitted required group accepted")
	}
}
