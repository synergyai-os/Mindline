package cli

import (
	"io/fs"
	"os"
)

type OSFileSystem struct{}

func NewOSFileSystem() OSFileSystem {
	return OSFileSystem{}
}

func (OSFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (OSFileSystem) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
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

func (OSFileSystem) Getwd() (string, error) {
	return os.Getwd()
}
