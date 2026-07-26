package cli

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

type OSFileSystem struct{}

func NewOSFileSystem() OSFileSystem {
	return OSFileSystem{}
}

func (OSFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (OSFileSystem) ReadFileBounded(path string, maximumBytes int64) ([]byte, error) {
	if maximumBytes <= 0 {
		return nil, errors.New("read limit must be positive")
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maximumBytes {
		return nil, errors.New("file exceeds bounded input contract")
	}
	payload, err := io.ReadAll(io.LimitReader(file, maximumBytes+1))
	if err != nil || int64(len(payload)) > maximumBytes {
		return nil, errors.New("file exceeds bounded input contract")
	}
	return payload, nil
}

func (OSFileSystem) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (OSFileSystem) ReadDir(path string) ([]fs.DirEntry, error) {
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return entries, err
}

func (OSFileSystem) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func (OSFileSystem) CanWriteDir(path string) error {
	file, err := os.CreateTemp(path, ".mindline-write-test-*")
	if err != nil {
		return err
	}
	name := file.Name()
	closeErr := file.Close()
	removeErr := os.Remove(name)
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}

func (OSFileSystem) WriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

func (OSFileSystem) Remove(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (OSFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (OSFileSystem) Getwd() (string, error) {
	return os.Getwd()
}

func (OSFileSystem) RealPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Abs(real)
	}

	current := abs
	var missing []string
	for {
		if real, err := filepath.EvalSymlinks(current); err == nil {
			resolved := real
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Abs(resolved)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return abs, nil
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func (OSFileSystem) IsSymlink(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.Mode()&os.ModeSymlink != 0, nil
}
