// Package repositorysnapshot materializes byte-exact regular files from a
// committed Git tree without consulting the working tree or checkout filters.
package repositorysnapshot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	maximumEntries     = 20_000
	maximumListingSize = 16 << 20
	maximumFileSize    = 32 << 20
	maximumTreeSize    = 256 << 20
)

var objectIDPattern = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)

// Materialize writes only regular tracked blobs from revision into an empty,
// private destination. Ignored files, sparse-checkout state, smudge filters,
// symlinks, and submodules cannot enter the result.
func Materialize(ctx context.Context, gitPath, repositoryRoot, revision, destination string) error {
	gitPath = filepath.Clean(strings.TrimSpace(gitPath))
	repositoryRoot = filepath.Clean(strings.TrimSpace(repositoryRoot))
	destination = filepath.Clean(strings.TrimSpace(destination))
	if !filepath.IsAbs(gitPath) || !filepath.IsAbs(repositoryRoot) || !filepath.IsAbs(destination) ||
		strings.TrimSpace(revision) == "" {
		return errors.New("repository snapshot input is invalid")
	}
	gitInfo, err := os.Lstat(gitPath)
	if err != nil || !gitInfo.Mode().IsRegular() || gitInfo.Mode()&0o111 == 0 || gitInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("repository snapshot Git tool is unavailable")
	}
	destinationInfo, err := os.Lstat(destination)
	if err != nil || !destinationInfo.IsDir() || destinationInfo.Mode()&os.ModeSymlink != 0 || destinationInfo.Mode().Perm()&0o077 != 0 {
		return errors.New("repository snapshot destination is not private")
	}
	children, err := os.ReadDir(destination)
	if err != nil || len(children) != 0 {
		return errors.New("repository snapshot destination is not empty")
	}
	listing, err := gitOutput(ctx, gitPath, repositoryRoot, maximumListingSize,
		"ls-tree", "-r", "-z", "--full-tree", revision)
	if err != nil {
		return errors.New("repository snapshot tree is unavailable")
	}
	entries := bytes.Split(listing, []byte{0})
	seen := map[string]bool{}
	total := 0
	count := 0
	for _, encoded := range entries {
		if len(encoded) == 0 {
			continue
		}
		count++
		if count > maximumEntries {
			return errors.New("repository snapshot entry limit exceeded")
		}
		metadata, name, ok := bytes.Cut(encoded, []byte{'\t'})
		fields := strings.Fields(string(metadata))
		if !ok || len(fields) != 3 || fields[1] != "blob" ||
			(fields[0] != "100644" && fields[0] != "100755") || !objectIDPattern.MatchString(fields[2]) {
			return errors.New("repository snapshot contains an unsupported entry")
		}
		relative, err := safeRelativePath(string(name))
		if err != nil || seen[relative] {
			return errors.New("repository snapshot contains an invalid path")
		}
		seen[relative] = true
		content, err := gitOutput(ctx, gitPath, repositoryRoot, maximumFileSize, "cat-file", "blob", fields[2])
		if err != nil {
			return errors.New("repository snapshot blob is unavailable")
		}
		total += len(content)
		if total > maximumTreeSize {
			return errors.New("repository snapshot size limit exceeded")
		}
		target := filepath.Join(destination, relative)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return errors.New("prepare repository snapshot directory")
		}
		mode := os.FileMode(0o600)
		if fields[0] == "100755" {
			mode = 0o700
		}
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			return errors.New("create repository snapshot file")
		}
		written, writeErr := file.Write(content)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil || written != len(content) {
			return errors.New("write repository snapshot file")
		}
	}
	if count == 0 {
		return errors.New("repository snapshot tree is empty")
	}
	return nil
}

// InitializeIndex adds an inert local Git index so exact-snapshot proof groups
// that inspect tracked paths or run `git diff --check` keep working.
func InitializeIndex(ctx context.Context, gitPath, destination string) error {
	if _, err := gitOutput(ctx, gitPath, destination, 1<<20, "init", "--quiet"); err != nil {
		return errors.New("initialize repository snapshot index")
	}
	if _, err := gitOutput(ctx, gitPath, destination, 1<<20, "add", "-f", "--all"); err != nil {
		return errors.New("index repository snapshot files")
	}
	return nil
}

func safeRelativePath(value string) (string, error) {
	if value == "" || strings.ContainsRune(value, '\x00') || strings.Contains(value, "\\") || path.IsAbs(value) {
		return "", errors.New("invalid repository path")
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != value {
		return "", errors.New("invalid repository path")
	}
	return filepath.FromSlash(clean), nil
}

func gitOutput(parent context.Context, gitPath, root string, maximum int, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, gitPath, args...)
	command.Dir = root
	command.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
	var stdout limitedBuffer
	stdout.maximum = maximum
	command.Stdout, command.Stderr = &stdout, nil
	if err := command.Run(); err != nil || ctx.Err() != nil || stdout.exceeded {
		return nil, fmt.Errorf("repository snapshot Git operation failed")
	}
	return stdout.Bytes(), nil
}

type limitedBuffer struct {
	bytes.Buffer
	maximum  int
	exceeded bool
}

func (buffer *limitedBuffer) Write(value []byte) (int, error) {
	original := len(value)
	remaining := buffer.maximum - buffer.Len()
	if remaining <= 0 {
		buffer.exceeded = true
		return original, nil
	}
	if len(value) > remaining {
		value = value[:remaining]
		buffer.exceeded = true
	}
	_, _ = buffer.Buffer.Write(value)
	return original, nil
}
