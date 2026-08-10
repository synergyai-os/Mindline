package repositorysnapshot

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMaterializeUsesTrackedBlobsDespiteIgnoreFilterAndSparseState(t *testing.T) {
	gitPath := testGitPath(t)
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	runGit(t, gitPath, repository, "init", "--quiet")
	writeTestFile(t, filepath.Join(repository, ".gitignore"), "ignored.go\n")
	writeTestFile(t, filepath.Join(repository, ".gitattributes"), "tracked.go filter=mutate\n")
	writeTestFile(t, filepath.Join(repository, "tracked.go"), "package fixture // original\n")
	writeTestFile(t, filepath.Join(repository, "ignored.go"), "package fixture // injected\n")
	runGit(t, gitPath, repository, "config", "filter.mutate.smudge", "sed s/original/mutated/")
	runGit(t, gitPath, repository, "config", "filter.mutate.clean", "cat")
	runGit(t, gitPath, repository, "add", ".gitignore", ".gitattributes", "tracked.go")
	runGit(t, gitPath, repository, "commit", "-m", "fixture", "--author", "Test <test@example.invalid>")
	original := gitValue(t, gitPath, repository, "rev-parse", "HEAD")
	writeTestFile(t, filepath.Join(repository, "tracked.go"), "package fixture // replacement\n")
	runGit(t, gitPath, repository, "add", "tracked.go")
	runGit(t, gitPath, repository, "commit", "-m", "replacement", "--author", "Test <test@example.invalid>")
	replacement := gitValue(t, gitPath, repository, "rev-parse", "HEAD")
	runGit(t, gitPath, repository, "reset", "--hard", original)
	runGit(t, gitPath, repository, "replace", original, replacement)
	runGit(t, gitPath, repository, "update-index", "--skip-worktree", "tracked.go")
	if err := os.Remove(filepath.Join(repository, "tracked.go")); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(t.TempDir(), "snapshot")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Materialize(context.Background(), gitPath, repository, "HEAD", destination); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "tracked.go"))
	if err != nil || string(content) != "package fixture // original\n" {
		t.Fatalf("tracked blob was rewritten or omitted: content=%q err=%v", content, err)
	}
	if _, err := os.Stat(filepath.Join(destination, "ignored.go")); !os.IsNotExist(err) {
		t.Fatal("ignored source entered exact snapshot")
	}
	if err := InitializeIndex(context.Background(), gitPath, destination); err != nil {
		t.Fatal(err)
	}
	runGit(t, gitPath, destination, "diff", "--check")
}

func gitValue(t *testing.T, gitPath, root string, args ...string) string {
	t.Helper()
	command := exec.Command(gitPath, args...)
	command.Dir = root
	command.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(output))
}

func testGitPath(t *testing.T) string {
	t.Helper()
	gitPath := "/usr/bin/git"
	if runtime.GOOS != "darwin" {
		if _, err := os.Lstat(gitPath); err != nil {
			gitPath = "/bin/git"
		}
	}
	return gitPath
}

func runGit(t *testing.T, gitPath, root string, args ...string) {
	t.Helper()
	command := exec.Command(gitPath, args...)
	command.Dir = root
	command.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid"}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %s", args, output)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
