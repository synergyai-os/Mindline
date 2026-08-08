// mindline-secret-check scans repository source and generated proof without
// printing paths, matching bytes, or other private content.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	contractVersion  = "mindline-secret-check/v0.1"
	defaultProofRoot = ".productbrain/proof"
)

var (
	errBlocked       = errors.New("credential-shaped content detected")
	errInvalidInput  = errors.New("invalid secret-check input")
	errScannerFailed = errors.New("secret-check scan failed")

	fixedCredentialPatterns = []*regexp.Regexp{
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`),
		regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{40,}\b`),
		regexp.MustCompile(`\bxox[baprs]-[0-9]{8,}(?:-[0-9A-Za-z]{8,}){2,}\b`),
		regexp.MustCompile(`\bxapp-[0-9]+-[A-Za-z0-9]{8,}(?:-[A-Za-z0-9]{20,}){2,}\b`),
		regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`),
		regexp.MustCompile(`(?i)\bauthorization\b["']?\s*[:=]\s*["']?bearer\s+[A-Za-z0-9._~-]{16,}`),
		regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----`),
	}
	prefixedCredentialPattern = regexp.MustCompile(`\b(?:pb_sk_|sk_(?:live|test)_|sk-(?:proj-|svcacct-|admin-)?)[A-Za-z0-9_-]{24,}\b`)
	assignedCredentialPattern = regexp.MustCompile(`(?i)\b(?:[a-z0-9]+[_-])*(?:password|passwd|pwd|api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|private[_-]?key|session[_-]?token)\b["']?\s*[:=]\s*["']?([A-Za-z0-9_./+=-]{24,})`)
)

type detector func([]byte) bool

type stringList []string

func (values *stringList) String() string { return strings.Join(*values, ",") }
func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}

type receipt struct {
	SchemaVersion    string `json:"schema_version"`
	State            string `json:"state"`
	SelfTest         string `json:"self_test"`
	TrackedFileCount int    `json:"tracked_file_count"`
	ProofFileCount   int    `json:"proof_file_count"`
	FindingCount     int    `json:"finding_count"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout, detectCredential); err != nil {
		fmt.Fprintln(os.Stderr, "mindline-secret-check: operation failed")
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, detect detector) error {
	flags := flag.NewFlagSet("mindline-secret-check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("root", "", "repository root")
	selfTest := flags.Bool("self-test", false, "verify the detector before scanning")
	var proofDirectories stringList
	flags.Var(&proofDirectories, "proof-dir", "additional generated proof directory (repeatable)")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*root) == "" || !*selfTest || detect == nil {
		return errInvalidInput
	}
	if !detect(selfTestSentinel()) {
		return errScannerFailed
	}

	repositoryRoot, err := canonicalDirectory(*root)
	if err != nil {
		return errInvalidInput
	}
	tracked, err := gitTrackedFiles(repositoryRoot)
	if err != nil {
		return errScannerFailed
	}
	// The signed command has no optional arguments, so its authoritative proof
	// set must be deterministic. Extra proof directories may be added, but the
	// owner-private default is always required and scanned.
	proofRoots := append(stringList{filepath.Join(repositoryRoot, defaultProofRoot)}, proofDirectories...)
	proof, err := proofFiles(proofRoots)
	if err != nil {
		return errScannerFailed
	}
	if len(proof) == 0 {
		return errScannerFailed
	}

	seen := make(map[string]struct{}, len(tracked)+len(proof))
	trackedCount, trackedFindings, err := scanFiles(tracked, seen, detect)
	if err != nil {
		return errScannerFailed
	}
	proofCount, proofFindings, err := scanFiles(proof, seen, detect)
	if err != nil {
		return errScannerFailed
	}
	findings := trackedFindings + proofFindings
	state := "clean"
	if findings > 0 {
		state = "blocked"
	}
	if err := json.NewEncoder(stdout).Encode(receipt{
		SchemaVersion: contractVersion, State: state, SelfTest: "passed",
		TrackedFileCount: trackedCount, ProofFileCount: proofCount, FindingCount: findings,
	}); err != nil {
		return errScannerFailed
	}
	if findings > 0 {
		return errBlocked
	}
	return nil
}

func selfTestSentinel() []byte {
	// Split the value so the credential-shaped sentinel never exists in source.
	return []byte("AK" + "IA" + "Q7W8E9R0T1Y2U3I4")
}

func detectCredential(data []byte) bool {
	for _, pattern := range fixedCredentialPatterns {
		if pattern.Match(data) {
			return true
		}
	}
	for _, value := range prefixedCredentialPattern.FindAll(data, -1) {
		if plausiblePrefixedSecret(value) {
			return true
		}
	}
	for _, match := range assignedCredentialPattern.FindAllSubmatch(data, -1) {
		if len(match) == 2 && plausibleSecret(match[1]) {
			return true
		}
	}
	return false
}

func plausiblePrefixedSecret(value []byte) bool {
	text := string(value)
	for _, prefix := range []string{"pb_sk_", "sk_live_", "sk_test_", "sk-proj-", "sk-svcacct-", "sk-admin-", "sk-"} {
		if strings.HasPrefix(text, prefix) {
			value = value[len(prefix):]
			break
		}
	}
	var lower, upper, digit int
	unique := make(map[byte]struct{})
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z':
			lower++
		case character >= 'A' && character <= 'Z':
			upper++
		case character >= '0' && character <= '9':
			digit++
		}
		unique[character] = struct{}{}
	}
	return lower > 0 && upper > 0 && digit > 0 && len(unique) >= 12
}

func plausibleSecret(value []byte) bool {
	var lower, upper, digit int
	unique := make(map[byte]struct{})
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z':
			lower++
		case character >= 'A' && character <= 'Z':
			upper++
		case character >= '0' && character <= '9':
			digit++
		}
		unique[character] = struct{}{}
	}
	return digit > 0 && lower+upper >= 8 && len(unique) >= 12
}

func canonicalDirectory(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(absolute)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errInvalidInput
	}
	return filepath.Clean(absolute), nil
}

func gitTrackedFiles(root string) ([]string, error) {
	command := exec.Command("git", "-C", root, "ls-files", "-z", "--cached")
	output, err := command.Output()
	if err != nil {
		return nil, err
	}
	parts := bytes.Split(output, []byte{0})
	files := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(string(part)))
		if !within(root, path) {
			return nil, errInvalidInput
		}
		info, statErr := os.Lstat(path)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return nil, statErr
		}
		if !info.IsDir() {
			files = append(files, path)
		}
	}
	return files, nil
}

func proofFiles(directories []string) ([]string, error) {
	var files []string
	for _, raw := range directories {
		root, err := canonicalDirectory(raw)
		if err != nil {
			return nil, err
		}
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return errInvalidInput
			}
			if !entry.IsDir() {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

func scanFiles(files []string, seen map[string]struct{}, detect detector) (int, int, error) {
	count, findings := 0, 0
	for _, path := range files {
		path = filepath.Clean(path)
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		info, err := os.Lstat(path)
		if err != nil {
			return 0, 0, err
		}
		var data []byte
		if info.Mode()&os.ModeSymlink != 0 {
			target, readErr := os.Readlink(path)
			if readErr != nil {
				return 0, 0, readErr
			}
			data = []byte(target)
		} else if info.Mode().IsRegular() {
			data, err = os.ReadFile(path)
			if err != nil {
				return 0, 0, err
			}
		} else {
			continue
		}
		count++
		if detect(data) {
			findings++
		}
	}
	return count, findings, nil
}

func within(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
