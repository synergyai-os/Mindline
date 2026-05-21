package artifacts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Output struct {
	SchemaVersion string      `json:"schema_version"`
	RunMode       string      `json:"run_mode"`
	MethodID      string      `json:"method_id"`
	DestinationID string      `json:"destination_id"`
	ItemCount     int         `json:"item_count"`
	BlockedCount  int         `json:"blocked_count"`
	Items         []Item      `json:"items"`
	AuthorityIDs  []string    `json:"authority_ids"`
	Paths         OutputPaths `json:"paths"`
}

type OutputPaths struct {
	Summary string `json:"summary"`
}

type Item struct {
	CandidateID        string          `json:"candidate_id"`
	State              string          `json:"state"`
	ResultPath         string          `json:"result_path"`
	ProcessorPlanPath  string          `json:"processor_plan_path"`
	DestinationPath    string          `json:"destination_summary_path"`
	DestinationSummary any             `json:"destination_summary"`
	Result             any             `json:"result"`
	ProcessorPlan      any             `json:"processor_plan"`
	OperationFiles     []OperationFile `json:"operation_files,omitempty"`
	PreviewFiles       []PreviewFile   `json:"preview_files,omitempty"`
}

type OperationFile struct {
	Path string `json:"path"`
	Body any    `json:"body"`
}

type PreviewFile struct {
	Path string `json:"path"`
	Body string `json:"body"`
}

func Write(outDir string, output Output, protectedRoots []string) error {
	writer := Writer{protectedRoots: append([]string(nil), protectedRoots...)}
	return writer.Write(outDir, output)
}

type Writer struct {
	protectedRoots []string
}

func (w Writer) Write(outDir string, output Output) error {
	realOut, err := prepareOutputRoot(outDir, w.protectedRoots)
	if err != nil {
		return err
	}
	AssignPaths(&output)
	if err := rejectSentinels(output); err != nil {
		return err
	}
	if err := writeJSON(realOut, "pipeline-summary.json", output); err != nil {
		return err
	}
	for _, item := range output.Items {
		if err := writeJSON(realOut, item.ResultPath, item.Result); err != nil {
			return err
		}
		if err := writeJSON(realOut, item.ProcessorPlanPath, item.ProcessorPlan); err != nil {
			return err
		}
		if err := writeJSON(realOut, item.DestinationPath, item.DestinationSummary); err != nil {
			return err
		}
		for _, operation := range item.OperationFiles {
			if err := writeJSON(realOut, operation.Path, operation.Body); err != nil {
				return err
			}
		}
		for _, preview := range item.PreviewFiles {
			if err := writeFile(realOut, preview.Path, []byte(preview.Body)); err != nil {
				return err
			}
		}
	}
	return nil
}

func AssignPaths(output *Output) {
	output.Paths.Summary = "pipeline-summary.json"
	slugs := uniqueSlugs(output.Items)
	for i := range output.Items {
		item := &output.Items[i]
		slug := slugs[i]
		item.ResultPath = filepath.ToSlash(filepath.Join("results", slug+".json"))
		item.ProcessorPlanPath = filepath.ToSlash(filepath.Join("processors", slug+".json"))
		item.DestinationPath = filepath.ToSlash(filepath.Join("destinations", slug, "destination-summary.json"))
	}
}

func prepareOutputRoot(outDir string, protectedRoots []string) (string, error) {
	if strings.TrimSpace(outDir) == "" {
		return "", fmt.Errorf("missing required --out")
	}
	abs, err := filepath.Abs(outDir)
	if err != nil {
		return "", err
	}
	for _, protected := range protectedRoots {
		if strings.TrimSpace(protected) == "" {
			continue
		}
		realProtected, err := filepath.EvalSymlinks(protected)
		if err != nil {
			realProtected = protected
		}
		realProtected, _ = filepath.Abs(realProtected)
		if isInside(realProtected, abs) {
			return "", fmt.Errorf("refusing to write pipeline output inside protected Tolaria vault")
		}
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", err
	}
	realOut, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	for _, protected := range protectedRoots {
		if strings.TrimSpace(protected) == "" {
			continue
		}
		realProtected, err := filepath.EvalSymlinks(protected)
		if err != nil {
			continue
		}
		if isInside(realProtected, realOut) {
			return "", fmt.Errorf("refusing to write pipeline output inside protected Tolaria vault")
		}
	}
	return realOut, nil
}

func writeJSON(root, relative string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFile(root, relative, data)
}

func writeFile(root, relative string, data []byte) error {
	if filepath.IsAbs(relative) {
		return fmt.Errorf("absolute output path rejected")
	}
	target := filepath.Join(root, relative)
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	cleanTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if !isInside(cleanRoot, cleanTarget) || cleanRoot == cleanTarget {
		return fmt.Errorf("output path escaped output directory")
	}
	if err := ensureParentDir(cleanRoot, relative); err != nil {
		return err
	}
	if info, err := os.Lstat(cleanTarget); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("output path escaped output directory")
	}
	return os.WriteFile(cleanTarget, data, 0o644)
}

func ensureParentDir(root, relative string) error {
	dir := filepath.Dir(filepath.Clean(relative))
	if dir == "." {
		return nil
	}
	current := root
	for _, part := range strings.Split(dir, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("output path escaped output directory")
			}
			if !info.IsDir() {
				return fmt.Errorf("output path parent is not a directory")
			}
			continue
		}
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.Mkdir(current, 0o755); err != nil && !os.IsExist(err) {
			return err
		}
	}
	return nil
}

func rejectSentinels(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	body := string(data)
	for _, sentinel := range []string{"PRIVATE_DM_SENTINEL_DO_NOT_WRITE", "sk-test-secret-do-not-leak"} {
		if strings.Contains(body, sentinel) {
			return fmt.Errorf("refusing to write private or secret sentinel")
		}
	}
	return nil
}

func slug(value string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	cleaned := strings.Trim(b.String(), "-")
	if cleaned == "" {
		return "source"
	}
	return cleaned
}

func uniqueSlugs(items []Item) []string {
	used := map[string]int{}
	slugs := make([]string, len(items))
	for i, item := range items {
		base := slug(item.CandidateID)
		used[base]++
		if used[base] == 1 {
			slugs[i] = base
			continue
		}
		for suffix := used[base]; ; suffix++ {
			candidate := fmt.Sprintf("%s-%d", base, suffix)
			if used[candidate] == 0 {
				used[candidate] = 1
				slugs[i] = candidate
				break
			}
		}
	}
	return slugs
}

func isInside(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel))
}
