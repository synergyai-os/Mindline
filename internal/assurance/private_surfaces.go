package assurance

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	maximumCleanHEADBytes        = 512 << 20
	maximumCleanHEADArchiveBytes = maximumCleanHEADBytes + (64 << 20)
	maximumRuntimeBytes          = 64 << 20
	maximumSurfaceFiles          = 100_000
)

type privateScanSurface struct {
	root    string
	binding string
}

func (surface privateScanSurface) cleanup() error {
	if surface.root == "" {
		return nil
	}
	base := filepath.Base(surface.root)
	if !strings.HasPrefix(base, "mindline-clean-head-") && !strings.HasPrefix(base, "mindline-runtime-snapshot-") {
		return errors.New("refusing to clean an unrecognized private scan surface")
	}
	return os.RemoveAll(surface.root)
}

func createCleanHEADExport(workdir, revision string) (privateScanSurface, error) {
	if !validGitRevision(revision) {
		return privateScanSurface{}, errors.New("clean HEAD export requires a full hexadecimal revision")
	}
	root, err := os.MkdirTemp("", "mindline-clean-head-")
	if err != nil {
		return privateScanSurface{}, err
	}
	surface := privateScanSurface{root: root}
	if err := os.Chmod(root, 0o700); err != nil {
		_ = surface.cleanup()
		return privateScanSurface{}, err
	}

	gitPath, err := exec.LookPath("git")
	if err != nil {
		_ = surface.cleanup()
		return privateScanSurface{}, errors.New("clean HEAD export requires Git")
	}
	if _, err := validateExecutableIdentity("git", gitPath); err != nil {
		_ = surface.cleanup()
		return privateScanSurface{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, gitPath, "-C", workdir, "archive", "--format=tar", revision)
	command.Env = fixedGateEnvironment(workdir, gitPath)
	configureProcessGroup(command)
	command.Cancel = func() error { return killProcessGroup(command) }
	command.WaitDelay = 5 * time.Second
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = surface.cleanup()
		return privateScanSurface{}, err
	}
	var stderr limitedBuffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		_ = surface.cleanup()
		return privateScanSurface{}, errors.New("clean HEAD export could not start")
	}

	archiveHash := sha256.New()
	archiveReader := io.TeeReader(&boundedInputReader{reader: stdout, maximum: maximumCleanHEADArchiveBytes}, archiveHash)
	files, bytesWritten, extractErr := extractBoundedTar(root, archiveReader, maximumCleanHEADBytes, maximumSurfaceFiles)
	if extractErr == nil {
		_, extractErr = io.Copy(io.Discard, archiveReader)
	}
	if extractErr != nil {
		cancel()
		_ = killProcessGroup(command)
	}
	waitErr := command.Wait()
	if stderr.exceeded {
		waitErr = errors.New("clean HEAD export output exceeded limit")
	}
	if extractErr != nil || waitErr != nil {
		_ = surface.cleanup()
		return privateScanSurface{}, fmt.Errorf("clean HEAD export failed closed: extract=%v command=%v", extractErr, waitErr)
	}
	if err := verifyExportedModule(root); err != nil {
		_ = surface.cleanup()
		return privateScanSurface{}, err
	}
	bindingValue := struct {
		Schema        string `json:"schema"`
		Revision      string `json:"revision"`
		ArchiveSHA256 string `json:"archive_sha256"`
		Files         int    `json:"files"`
		Bytes         int64  `json:"bytes"`
	}{"mindline-clean-head-export/v0.1", revision, hex.EncodeToString(archiveHash.Sum(nil)), files, bytesWritten}
	surface.binding = fingerprintJSON(bindingValue)
	return surface, nil
}

func extractBoundedTar(root string, reader io.Reader, maximumBytes int64, maximumFiles int) (int, int64, error) {
	tarReader := tar.NewReader(reader)
	files := 0
	var total int64
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return files, total, nil
		}
		if err != nil {
			return 0, 0, err
		}
		target, err := containedArchivePath(root, header.Name)
		if err != nil {
			return 0, 0, err
		}
		switch header.Typeflag {
		case tar.TypeXHeader, tar.TypeXGlobalHeader, tar.TypeGNULongName, tar.TypeGNULongLink:
			// Archive metadata is interpreted by archive/tar and never materialized.
			continue
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil {
				return 0, 0, err
			}
		case tar.TypeReg, tar.TypeRegA:
			files++
			if files > maximumFiles || header.Size < 0 || header.Size > maximumBytes-total {
				return 0, 0, errors.New("clean HEAD export exceeded its bounded surface")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return 0, 0, err
			}
			mode := os.FileMode(0o600)
			if header.FileInfo().Mode()&0o111 != 0 {
				mode = 0o700
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return 0, 0, err
			}
			written, copyErr := io.CopyN(file, tarReader, header.Size)
			closeErr := file.Close()
			if copyErr != nil || closeErr != nil || written != header.Size {
				return 0, 0, errors.New("clean HEAD export file was incomplete")
			}
			total += written
		default:
			return 0, 0, fmt.Errorf("clean HEAD export rejects archive entry type %d", header.Typeflag)
		}
	}
}

func containedArchivePath(root, name string) (string, error) {
	if strings.TrimSpace(name) == "" || filepath.IsAbs(name) {
		return "", errors.New("clean HEAD export contains an invalid path")
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("clean HEAD export path escapes its private root")
	}
	target := filepath.Join(root, clean)
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("clean HEAD export path escapes its private root")
	}
	return target, nil
}

type surfaceManifestEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

func createRuntimeSnapshot(runtimeRoot string) (privateScanSurface, error) {
	root, err := os.MkdirTemp("", "mindline-runtime-snapshot-")
	if err != nil {
		return privateScanSurface{}, err
	}
	surface := privateScanSurface{root: root}
	if err := os.Chmod(root, 0o700); err != nil {
		_ = surface.cleanup()
		return privateScanSurface{}, err
	}
	firstBinding, err := copyBoundedSurface(runtimeRoot, root)
	if err != nil {
		_ = surface.cleanup()
		return privateScanSurface{}, fmt.Errorf("runtime surface snapshot failed: %w", err)
	}
	secondBinding, err := fingerprintBoundedSurface(runtimeRoot)
	if err != nil || secondBinding != firstBinding {
		_ = surface.cleanup()
		return privateScanSurface{}, errors.New("runtime surface changed while it was being snapshotted")
	}
	surface.binding = firstBinding
	return surface, nil
}

func copyBoundedSurface(sourceRoot, destinationRoot string) (string, error) {
	return walkBoundedSurface(sourceRoot, func(path, relative string, info os.FileInfo) (string, error) {
		target := filepath.Join(destinationRoot, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return "", err
		}
		input, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer input.Close()
		openedInfo, err := input.Stat()
		if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) || openedInfo.Size() != info.Size() {
			return "", errors.New("runtime surface file identity changed while opened")
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return "", err
		}
		hash := sha256.New()
		written, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(input, info.Size()+1))
		closeErr := output.Close()
		if copyErr != nil || closeErr != nil || written != info.Size() {
			return "", errors.New("runtime surface file changed while copied")
		}
		return hex.EncodeToString(hash.Sum(nil)), nil
	})
}

func fingerprintBoundedSurface(sourceRoot string) (string, error) {
	return walkBoundedSurface(sourceRoot, func(path, _ string, info os.FileInfo) (string, error) {
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer file.Close()
		openedInfo, err := file.Stat()
		if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) || openedInfo.Size() != info.Size() {
			return "", errors.New("runtime surface file identity changed while opened")
		}
		hash := sha256.New()
		written, err := io.Copy(hash, io.LimitReader(file, openedInfo.Size()+1))
		if err != nil || written != info.Size() {
			return "", errors.New("runtime surface file changed while fingerprinted")
		}
		return hex.EncodeToString(hash.Sum(nil)), nil
	})
}

func walkBoundedSurface(root string, inspect func(string, string, os.FileInfo) (string, error)) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("runtime surface root must be a real directory")
	}
	entries := make([]surfaceManifestEntry, 0)
	var total int64
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() && !info.IsDir() {
			return errors.New("runtime surface contains an unsupported filesystem entry")
		}
		if info.IsDir() {
			return nil
		}
		if len(entries) >= maximumSurfaceFiles || info.Size() < 0 || info.Size() > maximumRuntimeBytes-total {
			return errors.New("runtime surface exceeds its bounded scan budget")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("runtime surface path escapes its root")
		}
		relative = filepath.ToSlash(relative)
		digest, err := inspect(path, relative, info)
		if err != nil {
			return err
		}
		total += info.Size()
		entries = append(entries, surfaceManifestEntry{Path: relative, SHA256: digest, Bytes: info.Size()})
		return nil
	})
	if err != nil {
		return "", err
	}
	manifest := struct {
		Schema string                 `json:"schema"`
		Files  []surfaceManifestEntry `json:"files"`
		Bytes  int64                  `json:"bytes"`
	}{"mindline-runtime-snapshot/v0.1", entries, total}
	return fingerprintJSON(manifest), nil
}

func verifyExportedModule(root string) error {
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil || len(goMod) > 1<<20 {
		return errors.New("clean HEAD export module identity unavailable")
	}
	for _, line := range strings.Split(string(goMod), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" && fields[1] == expectedModulePath {
			return nil
		}
	}
	return errors.New("clean HEAD export module identity mismatch")
}

func validGitRevision(revision string) bool {
	revision = strings.TrimSpace(revision)
	if len(revision) != 40 && len(revision) != 64 {
		return false
	}
	_, err := hex.DecodeString(revision)
	return err == nil
}

func fingerprintJSON(value any) string {
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

type boundedInputReader struct {
	reader  io.Reader
	read    int64
	maximum int64
}

func (reader *boundedInputReader) Read(value []byte) (int, error) {
	if reader.read >= reader.maximum {
		return 0, errors.New("clean HEAD archive exceeded its byte budget")
	}
	remaining := reader.maximum - reader.read
	if int64(len(value)) > remaining {
		value = value[:remaining]
	}
	read, err := reader.reader.Read(value)
	reader.read += int64(read)
	return read, err
}
