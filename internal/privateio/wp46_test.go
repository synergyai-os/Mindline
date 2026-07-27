package privateio

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWP46_PrivateJSONRejectsDuplicateKeysAtEveryDepth(t *testing.T) {
	type record struct {
		Name   string `json:"name"`
		Nested struct {
			Value int `json:"value"`
		} `json:"nested"`
	}
	for _, raw := range []string{
		`{"name":"one","name":"two","nested":{"value":1}}`,
		`{"name":"one","nested":{"value":1,"value":2}}`,
	} {
		var target record
		if err := DecodeJSONStrict([]byte(raw), &target); err == nil {
			t.Fatal("duplicate JSON key was accepted")
		}
	}
}

func TestWP46_DefaultControlRootIsStableAndProcessIndependent(t *testing.T) {
	root, err := ControlPlaneRootFromConfigDir("/Users/example/Library/Application Support")
	if err != nil {
		t.Fatal(err)
	}
	want := "/Users/example/Library/Application Support/Mindline/control-plane"
	if root != want {
		t.Fatalf("control root = %q, want %q", root, want)
	}
	if _, err := ControlPlaneRootFromConfigDir("relative/config"); err == nil {
		t.Fatal("relative configuration root was accepted")
	}
}

func TestWP46_PrivateAdvisoryLockIsOwnerOnlyAndNonblocking(t *testing.T) {
	root, err := CreateRuntimeRoot(t.TempDir(), "wp46-lock-")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "control", "settings.lock")
	first, err := AcquireAdvisoryLock(root, path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err := AcquireAdvisoryLock(root, path); !errors.Is(err, ErrLockBusy) {
		t.Fatalf("second lock = %v, want busy", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != FileMode {
		t.Fatalf("unsafe lock descriptor mode=%v", info.Mode())
	}
}

func TestWP46_PrivateAtomicFaultsLeaveOnlyOldOrNewCurrent(t *testing.T) {
	points := []FaultPoint{
		FaultBeforeBackupWrite, FaultAfterBackupWrite, FaultBeforeBackupSync,
		FaultAfterBackupSync, FaultBeforeBackupRename, FaultAfterBackupRename,
		FaultBeforeBackupDirSync, FaultAfterBackupDirSync,
		FaultBeforeCurrentWrite, FaultAfterCurrentWrite, FaultBeforeCurrentSync,
		FaultAfterCurrentSync, FaultBeforeCurrentRename, FaultAfterCurrentRename,
		FaultBeforeCurrentDirSync, FaultAfterCurrentDirSync,
		FaultBeforeReread, FaultAfterReread,
	}
	oldData := []byte(`{"version":1}` + "\n")
	newData := []byte(`{"version":2}` + "\n")
	validate := func(data []byte) error {
		var value struct {
			Version int `json:"version"`
		}
		return DecodeJSONStrict(data, &value)
	}
	for _, point := range points {
		t.Run(string(point), func(t *testing.T) {
			root, err := CreateRuntimeRoot(t.TempDir(), "wp46-atomic-")
			if err != nil {
				t.Fatal(err)
			}
			current := filepath.Join(root, "control", "settings.json")
			backup := filepath.Join(root, "control", "settings.backup.json")
			if err := WriteFile(current, oldData, false); err != nil {
				t.Fatal(err)
			}
			err = AtomicReplaceWithBackup(root, current, backup, newData, oldData, 1024, validate, func(observed FaultPoint) error {
				if observed == point {
					return errors.New("injected sentinel must not be reflected")
				}
				return nil
			})
			if !errors.Is(err, ErrAtomicWriteFailed) {
				t.Fatalf("fault %s = %v", point, err)
			}
			persisted, readErr := ReadFileBounded(root, current, 1024)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(persisted) != string(oldData) && string(persisted) != string(newData) {
				t.Fatalf("fault %s left torn current %q", point, persisted)
			}
			if validate(persisted) != nil {
				t.Fatalf("fault %s left invalid current", point)
			}
		})
	}
}
