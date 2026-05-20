package cli

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type MemoryFS struct {
	files       map[string][]byte
	dirs        map[string]bool
	failWriteIn []string
	failProbeIn []string
}

func NewMemoryFS() *MemoryFS {
	return &MemoryFS{
		files: map[string][]byte{},
		dirs:  map[string]bool{".": true},
	}
}

func (m *MemoryFS) ReadFile(path string) ([]byte, error) {
	clean := filepath.Clean(path)
	data, ok := m.files[clean]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	copyData := make([]byte, len(data))
	copy(copyData, data)
	return copyData, nil
}

func (m *MemoryFS) MkdirAll(path string, _ fs.FileMode) error {
	clean := filepath.Clean(path)
	parts := strings.Split(clean, string(filepath.Separator))
	current := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		if current == "" {
			current = part
		} else {
			current = filepath.Join(current, part)
		}
		if _, exists := m.files[current]; exists {
			return fmt.Errorf("file exists: %s", current)
		}
		m.dirs[current] = true
	}
	return nil
}

func (m *MemoryFS) Stat(path string) (fs.FileInfo, error) {
	clean := filepath.Clean(path)
	if data, ok := m.files[clean]; ok {
		return memoryFileInfo{name: filepath.Base(clean), size: int64(len(data)), dir: false}, nil
	}
	if m.dirs[clean] {
		return memoryFileInfo{name: filepath.Base(clean), dir: true}, nil
	}
	return nil, fmt.Errorf("not found: %s", path)
}

func (m *MemoryFS) CanWriteDir(path string) error {
	clean := filepath.Clean(path)
	for _, prefix := range m.failProbeIn {
		if clean == prefix || strings.HasPrefix(clean, prefix+string(filepath.Separator)) {
			return fmt.Errorf("directory cannot be written: %s", clean)
		}
	}
	if !m.dirs[clean] {
		return fmt.Errorf("directory not found: %s", clean)
	}
	return nil
}

func (m *MemoryFS) WriteFile(path string, data []byte) error {
	clean := filepath.Clean(path)
	for _, prefix := range m.failWriteIn {
		if clean == prefix || strings.HasPrefix(clean, prefix+string(filepath.Separator)) {
			return fmt.Errorf("write failed: %s", clean)
		}
	}
	dir := filepath.Dir(clean)
	if !m.dirs[dir] {
		return fmt.Errorf("directory not found: %s", dir)
	}
	copyData := make([]byte, len(data))
	copy(copyData, data)
	m.files[clean] = copyData
	return nil
}

func (m *MemoryFS) Getwd() (string, error) {
	return ".", nil
}

func (m *MemoryFS) RealPath(path string) (string, error) {
	return filepath.Clean(path), nil
}

func (m *MemoryFS) IsSymlink(string) (bool, error) {
	return false, nil
}

func (m *MemoryFS) Exists(path string) bool {
	clean := filepath.Clean(path)
	_, fileExists := m.files[clean]
	return fileExists || m.dirs[clean]
}

func (m *MemoryFS) MustReadFile(path string) []byte {
	data, err := m.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return data
}

func (m *MemoryFS) MustReadFileIfFile(path string) []byte {
	clean := filepath.Clean(path)
	data, ok := m.files[clean]
	if !ok {
		return nil
	}
	copyData := make([]byte, len(data))
	copy(copyData, data)
	return copyData
}

func (m *MemoryFS) WriteCountExcept(except string) int {
	cleanExcept := filepath.Clean(except)
	count := 0
	for path := range m.files {
		if path != cleanExcept {
			count++
		}
	}
	return count
}

func (m *MemoryFS) FailWritesUnder(path string) {
	m.failWriteIn = append(m.failWriteIn, filepath.Clean(path))
}

func (m *MemoryFS) FailCanWriteUnder(path string) {
	m.failProbeIn = append(m.failProbeIn, filepath.Clean(path))
}

func (m *MemoryFS) Paths() []string {
	paths := make([]string, 0, len(m.files)+len(m.dirs))
	for path := range m.files {
		paths = append(paths, path)
	}
	for path := range m.dirs {
		if path != "." {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

type memoryFileInfo struct {
	name string
	size int64
	dir  bool
}

func (m memoryFileInfo) Name() string { return m.name }
func (m memoryFileInfo) Size() int64  { return m.size }
func (m memoryFileInfo) Mode() fs.FileMode {
	if m.dir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
func (m memoryFileInfo) ModTime() time.Time { return time.Time{} }
func (m memoryFileInfo) IsDir() bool        { return m.dir }
func (m memoryFileInfo) Sys() any           { return nil }
