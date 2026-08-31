package localservice

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

const wp53ReleasedMainCommit = "bf49078a6c1317b6d383285f52ab6e2a51ee2738"

func TestWP53ReadonlyBetaUpgradeRollback(t *testing.T) {
	repositoryRoot := wp53RepositoryRoot(t)
	root, err := os.MkdirTemp("/tmp", "mindline-wp53-upgrade-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	mainSource := filepath.Join(root, "released-main")
	wp53ExtractGitTree(t, repositoryRoot, wp53ReleasedMainCommit, mainSource)

	mainBinary := filepath.Join(root, "mindline-main")
	candidateBinary := filepath.Join(root, "mindline-candidate")
	wp53BuildMindline(t, mainSource, mainBinary)
	wp53BuildMindline(t, repositoryRoot, candidateBinary)

	config, err := ConfigFromRoots(filepath.Join(root, "runtime"), filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(config.RuntimeRoot, "config.json")
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatal(err)
	}
	recordID := wp53SeedUpgradeEvidence(t, config.MemoryRoot)
	connectionDigest := strings.Repeat("7", 64)
	contextIDs := ScopedSearchInput{
		Query:   "orchard nebula compass willow lantern governance archive",
		ScopeID: "wp53-upgrade-project", LensID: "wp53-upgrade-lens",
		AgentID: "agent-wp53-upgrade", Limit: 3,
	}

	main := wp53StartExternalService(t, mainBinary, configPath, config.SocketPath)
	if _, err := main.client.PutScope(context.Background(), agentstate.Scope{
		ID: contextIDs.ScopeID, Name: "WP-53 upgrade project", Purpose: "upgrade-safe personal recall",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := main.client.PutScopedLens(context.Background(), agentstate.ScopedLens{
		ScopeID: contextIDs.ScopeID, ID: contextIDs.LensID, Name: "Upgrade lens", Query: "governance archive",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := main.client.PutActor(context.Background(), agentstate.AgentActor{
		ID: contextIDs.AgentID, Name: "WP-53 upgrade agent",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := main.client.BindProjectConnection(context.Background(), ProjectConnectionInput{
		Digest: connectionDigest, ScopeID: contextIDs.ScopeID,
		LensID: contextIDs.LensID, AgentID: contextIDs.AgentID,
	}); err != nil {
		t.Fatal(err)
	}
	mainPacket := wp53RequireScopedRecord(t, main.client, contextIDs, recordID)
	mainStatus := wp53RequireUpgradeState(t, main.client, connectionDigest, recordID, 1)
	main.stop(t)

	candidate := wp53StartExternalService(t, candidateBinary, configPath, config.SocketPath)
	wp53RequireUpgradeStateEqual(t, candidate.client, connectionDigest, recordID, mainStatus)
	if _, err := candidate.client.GetScoped(context.Background(), ScopedGetInput{
		RunID: mainPacket.RunID, ScopeID: contextIDs.ScopeID, LensID: contextIDs.LensID,
		AgentID: contextIDs.AgentID, RecordID: recordID,
	}); err == nil {
		t.Fatal("released-main receipt without a qualifying-source binding authorized candidate hydration")
	}
	candidatePacket := wp53RequireScopedRecord(t, candidate.client, contextIDs, recordID)
	wp53RequireScopedHydration(t, candidate.client, candidatePacket.RunID, contextIDs, recordID)
	candidate.stop(t)

	rolledBack := wp53StartExternalService(t, mainBinary, configPath, config.SocketPath)
	wp53RequireUpgradeStateEqual(t, rolledBack.client, connectionDigest, recordID, mainStatus)
	rolledBack.stop(t)

	rolledForward := wp53StartExternalService(t, candidateBinary, configPath, config.SocketPath)
	wp53RequireUpgradeStateEqual(t, rolledForward.client, connectionDigest, recordID, mainStatus)
	wp53RequireScopedHydration(t, rolledForward.client, candidatePacket.RunID, contextIDs, recordID)
	rolledForward.stop(t)
}

type wp53ExternalService struct {
	client *Client
	cmd    *exec.Cmd
	done   chan error
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func wp53StartExternalService(t *testing.T, binary, configPath, socketPath string) *wp53ExternalService {
	t.Helper()
	var stdout, stderr bytes.Buffer
	command := exec.Command(binary, "agent", "service-run", "--config", configPath)
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	service := &wp53ExternalService{
		client: NewClient(socketPath), cmd: command, done: make(chan error, 1),
		stdout: &stdout, stderr: &stderr,
	}
	go func() { service.done <- command.Wait() }()
	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case err := <-service.done:
			t.Fatalf("external Mindline service exited before readiness: %v: %s", err, stderr.String())
		default:
		}
		if _, err := service.client.Status(context.Background()); err == nil {
			return service
		} else {
			lastErr = err
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = command.Process.Kill()
	<-service.done
	t.Fatalf("external Mindline service did not become ready: %v: %s", lastErr, stderr.String())
	return nil
}

func (service *wp53ExternalService) stop(t *testing.T) {
	t.Helper()
	if err := service.cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-service.done:
		if err != nil {
			t.Fatalf("external Mindline service did not stop cleanly: %v: %s", err, service.stderr.String())
		}
	case <-time.After(10 * time.Second):
		_ = service.cmd.Process.Kill()
		<-service.done
		t.Fatal("external Mindline service did not stop before deadline")
	}
}

func wp53RequireScopedRecord(t *testing.T, client *Client, input ScopedSearchInput, recordID string) personalmemory.CompactContextPacket {
	t.Helper()
	packet, err := client.SearchScoped(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	for _, citation := range packet.Citations {
		if citation.RecordID == recordID {
			return packet
		}
	}
	t.Fatalf("scoped search did not return preserved record %s: %+v", recordID, packet.Citations)
	return personalmemory.CompactContextPacket{}
}

func wp53RequireScopedHydration(t *testing.T, client *Client, runID string, input ScopedSearchInput, recordID string) {
	t.Helper()
	capture, err := client.GetScoped(context.Background(), ScopedGetInput{
		RunID: runID, ScopeID: input.ScopeID, LensID: input.LensID,
		AgentID: input.AgentID, RecordID: recordID,
	})
	if err != nil || capture.RecordID != recordID || !capture.AgentRecallApproved {
		t.Fatalf("scoped hydration=%+v err=%v", capture, err)
	}
}

type wp53UpgradeState struct {
	memoryFingerprint string
	recordCount       int
	connectionCount   int
}

func wp53RequireUpgradeState(t *testing.T, client *Client, connectionDigest, recordID string, expectedRecords int) wp53UpgradeState {
	t.Helper()
	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Memory.RecordCount != expectedRecords || status.State.ProjectConnectionCount != 1 ||
		status.State.ActiveConnectionCount != 1 {
		t.Fatalf("unexpected preserved state: %+v", status)
	}
	if capture, err := client.Get(context.Background(), recordID); err != nil || capture.RecordID != recordID {
		t.Fatalf("preserved capture=%+v err=%v", capture, err)
	}
	resolution, err := client.ResolveProjectConnection(context.Background(), connectionDigest)
	if err != nil || resolution.State != "ready" {
		t.Fatalf("project connection resolution=%+v err=%v", resolution, err)
	}
	return wp53UpgradeState{
		memoryFingerprint: status.Memory.Fingerprint,
		recordCount:       status.Memory.RecordCount, connectionCount: status.State.ProjectConnectionCount,
	}
}

func wp53RequireUpgradeStateEqual(t *testing.T, client *Client, connectionDigest, recordID string, expected wp53UpgradeState) {
	t.Helper()
	actual := wp53RequireUpgradeState(t, client, connectionDigest, recordID, expected.recordCount)
	if actual != expected {
		t.Fatalf("upgrade changed evidence or connection state: got=%+v want=%+v", actual, expected)
	}
}

func wp53SeedUpgradeEvidence(t *testing.T, root string) string {
	t.Helper()
	repository, err := personalmemory.NewFileRepository(root, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	record, err := personalmemory.NewCaptureRecord(personalmemory.CaptureRecordInput{
		SourceAdapter: "wp53-public-fixture", SourceScopeID: "upgrade", SourceContainerID: "records",
		ExternalID: "upgrade-record", OccurredAt: "2026-08-31T12:00:00Z",
		SourceRef: "fixture://wp53/upgrade-record",
		RawText:   "orchard nebula compass willow lantern governance archive",
	})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := personalmemory.NewCaptureBatch(personalmemory.CaptureBatchInput{
		SourceIdentity: "wp53-upgrade-scorecard", LowerInclusive: "1", UpperInclusive: "1",
		Watermark: "1", DeclaredRecords: 1, Records: []personalmemory.CaptureRecord{record},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Import(batch); err != nil {
		t.Fatal(err)
	}
	return record.RecordID
}

func wp53RepositoryRoot(t *testing.T) string {
	t.Helper()
	command := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(output))
}

func wp53BuildMindline(t *testing.T, sourceRoot, output string) {
	t.Helper()
	command := exec.Command("go", "build", "-o", output, "./cmd/mindline")
	command.Dir = sourceRoot
	if outputBytes, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build Mindline from %s: %v: %s", sourceRoot, err, outputBytes)
	}
}

func wp53ExtractGitTree(t *testing.T, repositoryRoot, commit, outputRoot string) {
	t.Helper()
	command := exec.Command("git", "archive", "--format=tar", commit)
	command.Dir = repositoryRoot
	archiveBytes, err := command.Output()
	if err != nil {
		t.Fatalf("archive released main %s: %v", commit, err)
	}
	if err := os.MkdirAll(outputRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(bytes.NewReader(archiveBytes))
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		clean := filepath.Clean(header.Name)
		if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			t.Fatalf("unsafe released-main archive path %q", header.Name)
		}
		target := filepath.Join(outputRoot, clean)
		switch header.Typeflag {
		case tar.TypeXGlobalHeader, tar.TypeXHeader:
			continue
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil {
				t.Fatal(err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				t.Fatal(err)
			}
			mode := os.FileMode(header.Mode) & 0o700
			if mode == 0 {
				mode = 0o600
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				t.Fatal(err)
			}
			_, copyErr := io.Copy(file, reader)
			closeErr := file.Close()
			if copyErr != nil || closeErr != nil {
				t.Fatalf("extract %s: copy=%v close=%v", header.Name, copyErr, closeErr)
			}
		default:
			t.Fatalf("unsupported released-main archive entry %s type %d", header.Name, header.Typeflag)
		}
	}
}

func TestWP53ReleasedMainCommitIsAvailable(t *testing.T) {
	root := wp53RepositoryRoot(t)
	command := exec.Command("git", "cat-file", "-e", fmt.Sprintf("%s^{commit}", wp53ReleasedMainCommit))
	command.Dir = root
	if err := command.Run(); err != nil {
		t.Fatalf("frozen released-main commit is unavailable: %v", err)
	}
}
