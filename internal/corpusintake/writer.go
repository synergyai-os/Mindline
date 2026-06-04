package corpusintake

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/synergyai-os/Mindline/internal/documents"
	"github.com/synergyai-os/Mindline/internal/sbos"
)

type FileSystem interface {
	MkdirAll(path string, perm os.FileMode) error
	ReadDir(path string) ([]fs.DirEntry, error)
	WriteFile(path string, data []byte) error
	Remove(path string) error
	RemoveAll(path string) error
	IsSymlink(path string) (bool, error)
}

type SourceInput struct {
	SourceID  string
	Candidate sbos.Candidate
}

type SourceOutput struct {
	SourceID string
	Path     string
	Error    error
}

func PrepareRoot(fileSystem FileSystem, root string) error {
	if err := RejectSymlinkAncestors(fileSystem, root); err != nil {
		return err
	}
	return fileSystem.MkdirAll(root, 0o755)
}

func WriteSourcesAndManifest(fileSystem FileSystem, root, corpusID string, sources []SourceInput, markdown func(sbos.Candidate) string) (map[string]SourceOutput, string, error) {
	keep := map[string]bool{}
	for _, source := range sources {
		keep[source.SourceID] = true
	}
	if err := PruneSources(fileSystem, root, keep); err != nil {
		return nil, "", err
	}
	outputs := map[string]SourceOutput{}
	manifest := documents.CorpusPressureManifest{
		SchemaVersion: documents.CorpusPressureManifestSchemaVersion,
		CorpusID:      corpusID,
	}
	for _, source := range sources {
		path, err := WriteSource(fileSystem, root, source.SourceID, markdown(source.Candidate))
		output := SourceOutput{SourceID: source.SourceID, Path: path, Error: err}
		outputs[source.SourceID] = output
		if err == nil {
			manifest.Sources = append(manifest.Sources, documents.CorpusPressureManifestSource{
				SourceID:   source.SourceID,
				SourceKind: documents.SourceKindMarkdown,
				Path:       path,
			})
		}
	}
	if err := PruneSources(fileSystem, root, manifestSourceIDs(manifest)); err != nil {
		return nil, "", err
	}
	if len(manifest.Sources) == 0 {
		if err := RemoveManifest(fileSystem, root); err != nil {
			return nil, "", err
		}
		if err := RemoveSources(fileSystem, root); err != nil {
			return nil, "", err
		}
		return outputs, "", nil
	}
	if err := WriteManifest(fileSystem, root, manifest); err != nil {
		return nil, "", err
	}
	return outputs, "corpus-pressure-manifest.json", nil
}

func WriteSource(fileSystem FileSystem, root, sourceID, markdown string) (string, error) {
	rel := filepath.ToSlash(filepath.Join("sources", sourceID, "source.md"))
	target := filepath.Join(root, filepath.FromSlash(rel))
	if !IsInside(root, target) {
		return "", fmt.Errorf("source path escaped output directory")
	}
	if err := RejectSymlinkPath(fileSystem, root, filepath.Dir(target)); err != nil {
		return "", err
	}
	if err := fileSystem.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	if err := RejectSymlinkPath(fileSystem, root, target); err != nil {
		return "", err
	}
	if err := fileSystem.WriteFile(target, []byte(markdown)); err != nil {
		return "", err
	}
	return rel, nil
}

func WriteManifest(fileSystem FileSystem, root string, manifest documents.CorpusPressureManifest) error {
	target := filepath.Join(root, "corpus-pressure-manifest.json")
	if err := RejectSymlinkPath(fileSystem, root, target); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fileSystem.WriteFile(target, data)
}

func RemoveManifest(fileSystem FileSystem, root string) error {
	target := filepath.Join(root, "corpus-pressure-manifest.json")
	if !IsInside(root, target) {
		return fmt.Errorf("manifest path escaped output directory")
	}
	return fileSystem.Remove(target)
}

func RemoveSources(fileSystem FileSystem, root string) error {
	target := filepath.Join(root, "sources")
	if !IsInside(root, target) {
		return fmt.Errorf("sources path escaped output directory")
	}
	return fileSystem.RemoveAll(target)
}

func PruneSources(fileSystem FileSystem, root string, keepSourceIDs map[string]bool) error {
	sourcesDir := filepath.Join(root, "sources")
	if !IsInside(root, sourcesDir) {
		return fmt.Errorf("sources path escaped output directory")
	}
	isSymlink, err := fileSystem.IsSymlink(sourcesDir)
	if err != nil {
		return err
	}
	if isSymlink {
		return nil
	}
	entries, err := fileSystem.ReadDir(sourcesDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if keepSourceIDs[entry.Name()] {
			continue
		}
		target := filepath.Join(sourcesDir, entry.Name())
		if !IsInside(root, target) {
			return fmt.Errorf("source path escaped output directory")
		}
		if err := fileSystem.RemoveAll(target); err != nil {
			return err
		}
	}
	return nil
}

func WriteJSON(fileSystem FileSystem, root, rel string, value any) error {
	target := filepath.Join(root, filepath.FromSlash(rel))
	if err := RejectSymlinkPath(fileSystem, root, filepath.Dir(target)); err != nil {
		return err
	}
	if err := fileSystem.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := RejectSymlinkPath(fileSystem, root, target); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fileSystem.WriteFile(target, data)
}

func WriteMarkdown(fileSystem FileSystem, root, rel, body string) error {
	target := filepath.Join(root, filepath.FromSlash(rel))
	if err := RejectSymlinkPath(fileSystem, root, filepath.Dir(target)); err != nil {
		return err
	}
	if err := fileSystem.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := RejectSymlinkPath(fileSystem, root, target); err != nil {
		return err
	}
	return fileSystem.WriteFile(target, []byte(body))
}

func manifestSourceIDs(manifest documents.CorpusPressureManifest) map[string]bool {
	sourceIDs := map[string]bool{}
	for _, source := range manifest.Sources {
		sourceIDs[source.SourceID] = true
	}
	return sourceIDs
}

func IsInside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func RejectSymlinkPath(fileSystem FileSystem, root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	if rel == "." {
		return rejectSymlink(fileSystem, root)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escaped output directory")
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		if err := rejectSymlink(fileSystem, current); err != nil {
			return err
		}
	}
	return nil
}

func rejectSymlink(fileSystem FileSystem, path string) error {
	isSymlink, err := fileSystem.IsSymlink(path)
	if err != nil {
		return err
	}
	if isSymlink {
		return fmt.Errorf("output path contains symlink: %s", path)
	}
	return nil
}

func RejectSymlinkAncestors(fileSystem FileSystem, path string) error {
	clean := filepath.Clean(path)
	current := ""
	rel := clean
	if filepath.IsAbs(clean) {
		current = string(filepath.Separator)
		var err error
		rel, err = filepath.Rel(current, clean)
		if err != nil {
			return err
		}
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		if current == "" {
			current = part
		} else {
			current = filepath.Join(current, part)
		}
		isSymlink, err := fileSystem.IsSymlink(current)
		if err != nil {
			return err
		}
		if isSymlink {
			if isPlatformTempAlias(current) {
				continue
			}
			return fmt.Errorf("output path contains symlink: %s", current)
		}
	}
	return nil
}

func isPlatformTempAlias(path string) bool {
	switch filepath.Clean(path) {
	case "/tmp", "/var":
		return true
	default:
		return false
	}
}

type OSFileSystem struct{}

func (OSFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (OSFileSystem) ReadDir(path string) ([]fs.DirEntry, error) {
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return entries, err
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

func (OSFileSystem) IsSymlink(path string) (bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.Mode()&os.ModeSymlink != 0, nil
}
