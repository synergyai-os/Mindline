package privateio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	DirMode  fs.FileMode = 0o700
	FileMode fs.FileMode = 0o600
)

var (
	ErrInvalidReadLimit  = errors.New("private read limit must be positive")
	ErrReadLimitExceeded = errors.New("private read limit exceeded")
	ErrInvalidJSON       = errors.New("private JSON is invalid")
)

// ValidateContained requires every supplied path to resolve beneath root. It also
// rejects symlink components, including components that exist above a path that
// has not been created yet.
func ValidateContained(root string, paths ...string) error {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if err := rejectSymlinkComponents(rootAbs); err != nil {
		return fmt.Errorf("private runtime root: %w", err)
	}
	rootInfo, err := os.Lstat(rootAbs)
	if err != nil {
		return fmt.Errorf("private runtime root: %w", err)
	}
	if !rootInfo.IsDir() || rootInfo.Mode().Perm() != DirMode {
		return errors.New("private runtime root must be an existing 0700 directory")
	}
	if err := requireCurrentOwner(rootAbs); err != nil {
		return fmt.Errorf("private runtime root: %w", err)
	}
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			return errors.New("private runtime path is empty")
		}
		pathAbs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(rootAbs, pathAbs)
		if err != nil || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return errors.New("path escapes MINDLINE_PRIVATE_RUNTIME_ROOT")
		}
		if err := rejectSymlinkComponents(pathAbs); err != nil {
			return fmt.Errorf("private runtime path: %w", err)
		}
		if err := requirePrivateExistingComponents(rootAbs, pathAbs); err != nil {
			return fmt.Errorf("private runtime path: %w", err)
		}
		if err := requirePrivateExistingTree(pathAbs); err != nil {
			return fmt.Errorf("private runtime tree: %w", err)
		}
	}
	return nil
}

func requirePrivateExistingTree(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.IsDir() {
		return err
	}
	return filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("symlink component is not allowed")
		}
		if err := requireCurrentOwner(current); err != nil {
			return err
		}
		if info.IsDir() {
			if info.Mode().Perm() != DirMode {
				return errors.New("private directory must use mode 0700")
			}
			return nil
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != FileMode {
			return errors.New("private file must be regular and use mode 0600")
		}
		return nil
	})
}

// CreateRuntimeRoot allocates a fresh, unpredictable owner-only root. The
// operating-system primitive is exclusive: it never adopts or reuses an
// existing path.
func CreateRuntimeRoot(parent, pattern string) (string, error) {
	if strings.TrimSpace(parent) == "" || strings.TrimSpace(pattern) == "" {
		return "", errors.New("private runtime allocation is incomplete")
	}
	if err := rejectSymlinkComponents(parent); err != nil {
		return "", err
	}
	root, err := os.MkdirTemp(parent, pattern)
	if err != nil {
		return "", err
	}
	if err := os.Chmod(root, DirMode); err != nil {
		_ = os.Remove(root)
		return "", err
	}
	if err := requireCurrentOwner(root); err != nil {
		_ = os.Remove(root)
		return "", err
	}
	return root, nil
}

func PrepareDir(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("private directory is empty")
	}
	if err := rejectSymlinkComponents(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, DirMode); err != nil {
		return err
	}
	if err := rejectSymlinkComponents(path); err != nil {
		return err
	}
	if err := os.Chmod(path, DirMode); err != nil {
		return err
	}
	return requireCurrentOwner(path)
}

func ReadJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return nil
}

// ReadFile opens an existing authority file without following its final path,
// after proving that the complete path is owner-only and contained in root.
func ReadFile(root, path string) ([]byte, error) {
	if err := ValidateContained(root, path); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return nil, errors.New("private file is not owned by current user")
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != FileMode {
		return nil, errors.New("private file must be regular and use mode 0600")
	}
	return io.ReadAll(file)
}

// ReadFileBounded opens an owner-only contained regular file without following
// its final path and refuses to allocate or read beyond maxBytes.
func ReadFileBounded(root, path string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, ErrInvalidReadLimit
	}
	if err := ValidateContained(root, path); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return nil, errors.New("private file is not owned by current user")
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != FileMode {
		return nil, errors.New("private file must be regular and use mode 0600")
	}
	if info.Size() > maxBytes {
		return nil, ErrReadLimitExceeded
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrReadLimitExceeded
	}
	return data, nil
}

// ReadJSONStrictBounded combines the owner-only bounded read with the closed
// JSON decoder used at authority boundaries.
func ReadJSONStrictBounded(root, path string, maxBytes int64, target any) error {
	data, err := ReadFileBounded(root, path, maxBytes)
	if err != nil {
		return err
	}
	if err := DecodeJSONStrict(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return nil
}

// ReadJSONStrict is the closed-schema authority decoder. It rejects unknown
// fields, trailing JSON, symlinks, non-regular files, wrong modes, and owners.
func ReadJSONStrict(root, path string, target any) error {
	data, err := ReadFile(root, path)
	if err != nil {
		return err
	}
	if err := DecodeJSONStrict(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return nil
}

// DecodeJSONStrict applies the closed JSON contract shared by durable control
// documents. In addition to unknown fields and trailing data, it rejects
// duplicate object keys before decoding into target.
func DecodeJSONStrict(data []byte, target any) error {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return ErrInvalidJSON
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return ErrInvalidJSON
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return ErrInvalidJSON
	}
	return nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := consumeJSONValue(decoder, 0); err != nil {
		return err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err == nil || token != nil {
			return errors.New("trailing JSON is not allowed")
		}
		return err
	}
	return nil
}

func consumeJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 128 {
		return errors.New("JSON nesting limit exceeded")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			nameToken, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := nameToken.(string)
			if !ok {
				return errors.New("invalid JSON object key")
			}
			if _, duplicate := seen[name]; duplicate {
				return errors.New("duplicate JSON object key")
			}
			seen[name] = struct{}{}
			if err := consumeJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return errors.New("invalid JSON object")
		}
	case '[':
		for decoder.More() {
			if err := consumeJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return errors.New("invalid JSON array")
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	return nil
}

func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return WriteFile(path, append(data, '\n'), false)
}

func WriteJSONNoReplace(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return WriteFile(path, append(data, '\n'), true)
}

func WriteFile(path string, data []byte, noReplace bool) error {
	dir := filepath.Dir(path)
	if err := PrepareDir(dir); err != nil {
		return err
	}
	if err := rejectSymlinkComponents(path); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("private output is a symlink")
		}
		if noReplace {
			return fs.ErrExist
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	temp, err := os.CreateTemp(dir, ".mindline-private-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(FileMode); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if noReplace {
		if err := os.Link(tempName, path); err != nil {
			return err
		}
	} else if err := os.Rename(tempName, path); err != nil {
		return err
	}
	if err := os.Chmod(path, FileMode); err != nil {
		return err
	}
	dirHandle, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer dirHandle.Close()
	return dirHandle.Sync()
}

func rejectSymlinkComponents(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(abs, string(filepath.Separator)), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// macOS exposes the system temp hierarchy through stable root aliases
			// (/tmp -> /private/tmp and /var -> /private/var). These are outside the
			// caller-owned runtime root and are canonical platform mounts, not an
			// attacker-controlled nested escape.
			if current == "/tmp" || current == "/var" {
				continue
			}
			return errors.New("symlink component is not allowed")
		}
	}
	return nil
}

func requireCurrentOwner(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("cannot verify private directory ownership")
	}
	if int(stat.Uid) != os.Geteuid() {
		return errors.New("private directory is not owned by current user")
	}
	return nil
}

func requirePrivateExistingComponents(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	current := root
	parts := strings.Split(rel, string(filepath.Separator))
	if rel == "." {
		parts = nil
	}
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			return nil
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("symlink component is not allowed")
		}
		if err := requireCurrentOwner(current); err != nil {
			return err
		}
		if info.IsDir() {
			if info.Mode().Perm() != DirMode {
				return errors.New("private runtime directory permissions must be 0700")
			}
			continue
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != FileMode {
			return errors.New("private runtime file permissions must be 0600")
		}
	}
	return nil
}
