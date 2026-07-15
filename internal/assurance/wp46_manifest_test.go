package assurance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWP46_ProofRunnerOuterBuildAndEveryGroupInvocationAreExact(t *testing.T) {
	embedded := EmbeddedWP46Manifest()
	digest := sha256.Sum256(embedded)
	if got := hex.EncodeToString(digest[:]); got != WP46ManifestSHA256 {
		t.Fatalf("embedded manifest hash = %s", got)
	}
	manifest, err := ParseWP46Manifest(embedded)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ProofController.Version.StdoutExact != ProofRunnerVersion+"\n" ||
		!reflect.DeepEqual(manifest.ProofController.Version.Argv, []string{"--version"}) {
		t.Fatal("proof runner version contract drifted")
	}
	runnerGroups := 0
	for _, group := range manifest.Groups {
		if group.Tool != "${MINDLINE_PROOF_RUNNER}" {
			continue
		}
		runnerGroups++
		if !reflect.DeepEqual(group.Argv, []string{"group", group.ID}) {
			t.Fatalf("runner group %s argv drifted: %v", group.ID, group.Argv)
		}
	}
	if runnerGroups != 14 {
		t.Fatalf("runner-owned group count = %d", runnerGroups)
	}
	var versionOut, versionErr bytes.Buffer
	if exit := RunProofRunner([]string{"--version"}, &versionOut, &versionErr); exit != 0 || versionOut.String() != ProofRunnerVersion+"\n" || versionErr.Len() != 0 {
		t.Fatalf("proof runner version surface drifted: exit=%d stdout=%q stderr=%q", exit, versionOut.String(), versionErr.String())
	}
	versionOut.Reset()
	versionErr.Reset()
	if exit := RunProofRunner([]string{"group", "unsigned_group"}, &versionOut, &versionErr); exit == 0 {
		t.Fatal("unsigned proof group was dispatched")
	}

	var unknown map[string]any
	if err := json.Unmarshal(embedded, &unknown); err != nil {
		t.Fatal(err)
	}
	unknown["unsigned_extension"] = true
	data, _ := json.Marshal(unknown)
	if _, err := ParseWP46Manifest(data); err == nil {
		t.Fatal("unknown manifest field was accepted")
	}

	var changed map[string]any
	if err := json.Unmarshal(embedded, &changed); err != nil {
		t.Fatal(err)
	}
	groups := changed["groups"].([]any)
	groups[0].(map[string]any)["unsigned_extension"] = true
	data, _ = json.Marshal(changed)
	if _, err := ParseWP46Manifest(data); err == nil {
		t.Fatal("unknown group field was accepted")
	}

	if err := json.Unmarshal(embedded, &changed); err != nil {
		t.Fatal(err)
	}
	groups = changed["groups"].([]any)
	groups[0].(map[string]any)["predicate"].(map[string]any)["unsigned_nested_extension"] = true
	data, _ = json.Marshal(changed)
	if _, err := ParseWP46Manifest(data); err == nil {
		t.Fatal("unknown nested manifest field was accepted")
	}

	duplicate := bytes.Replace(embedded, []byte(`"id": "wp46-stable-control-v1"`), []byte(`"id": "wp46-stable-control-v1", "id": "duplicate"`), 1)
	if _, err := ParseWP46Manifest(duplicate); err == nil {
		t.Fatal("duplicate JSON field was accepted")
	}
}

func TestWP46_ControllerBootstrapCrashMatrixAndSameCommitRetry(t *testing.T) {
	controlRoot := filepath.Join(t.TempDir(), "control-plane")
	port := availableTestPort(t)
	random := bytes.NewReader(append(bytes.Repeat([]byte{0x11}, 48), bytes.Repeat([]byte{0x22}, 48)...))
	options := AttemptOptions{ControlRoot: controlRoot, Port: port, Random: random}
	first, err := BeginWP46ProofAttempt("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", WP46ManifestSHA256, options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BeginWP46ProofAttempt("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", WP46ManifestSHA256, options)
	if err != nil {
		t.Fatal(err)
	}
	if first.Root == second.Root || first.State.AttemptID == second.State.AttemptID || first.State.AttemptGeneration == second.State.AttemptGeneration {
		t.Fatal("same-commit retry reused proof-attempt identity")
	}
	for _, attempt := range []ProofAttempt{first, second} {
		info, err := os.Stat(attempt.Root)
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("attempt root is not durable owner-only: %v", err)
		}
		if _, err := os.Stat(filepath.Join(attempt.Root, "namespace-marker.json")); err != nil {
			t.Fatal(err)
		}
	}

	blockedRoot := filepath.Join(t.TempDir(), "blocked-control")
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	blockedPort := listener.Addr().(*net.TCPAddr).Port
	_, err = BeginWP46ProofAttempt("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", WP46ManifestSHA256, AttemptOptions{ControlRoot: blockedRoot, Port: blockedPort})
	_ = listener.Close()
	if err == nil {
		t.Fatal("port collision was accepted")
	}
	if _, statErr := os.Stat(blockedRoot); !os.IsNotExist(statErr) {
		t.Fatalf("port collision mutated durable state: %v", statErr)
	}
}

func availableTestPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}
