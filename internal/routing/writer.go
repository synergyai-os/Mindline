package routing

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

func Write(outDir string, result Result) error {
	if err := ValidateResult(result); err != nil {
		return fmt.Errorf("invalid routing result: %w", err)
	}
	if err := privateio.PrepareDir(outDir); err != nil {
		return err
	}
	if err := privateio.WriteJSON(filepath.Join(outDir, "source-graph.json"), result.Graph); err != nil {
		return err
	}
	if err := privateio.WriteJSON(filepath.Join(outDir, "route-decisions.json"), result.Decisions); err != nil {
		return err
	}
	if err := privateio.WriteJSON(filepath.Join(outDir, "route-summary.json"), result.Summary); err != nil {
		return err
	}
	return privateio.WriteFile(filepath.Join(outDir, "review-packet.md"), []byte(routeReviewPacket(result)), false)
}

func LoadResult(dir string) (Result, error) {
	var result Result
	if err := privateio.ReadJSONStrict(dir, filepath.Join(dir, "source-graph.json"), &result.Graph); err != nil {
		return Result{}, err
	}
	if err := privateio.ReadJSONStrict(dir, filepath.Join(dir, "route-decisions.json"), &result.Decisions); err != nil {
		return Result{}, err
	}
	if err := privateio.ReadJSONStrict(dir, filepath.Join(dir, "route-summary.json"), &result.Summary); err != nil {
		return Result{}, err
	}
	if err := ValidateResult(result); err != nil {
		return Result{}, fmt.Errorf("invalid routing authority: %w", err)
	}
	return result, nil
}

func routeReviewPacket(result Result) string {
	var b strings.Builder
	b.WriteString("# Mindline Context-Lens Routing Review\n\n")
	b.WriteString(fmt.Sprintf("- Original captures: %d\n- Canonical sources: %d\n- Lens results: %d/%d\n- Operator judged: %t\n\n", result.Summary.InputRecordCount, result.Summary.CanonicalSourceCount, result.Summary.LensResultCount, result.Summary.RequiredLensResultCount, result.Summary.OperatorJudged))
	for _, source := range result.Decisions.Sources {
		b.WriteString("## " + source.CanonicalURL + "\n\n")
		b.WriteString("- Meaning: " + source.SemanticAssessment.Summary + "\n")
		b.WriteString("- Role: " + source.SemanticAssessment.PrimaryRole + "\n")
		for _, lens := range source.LensResults {
			b.WriteString(fmt.Sprintf("- Lens `%s`: %s (%.2f) — %s\n", lens.LensID, lens.Result, lens.Confidence, lens.Rationale))
		}
		b.WriteString("- Disposition: " + source.Disposition + " — " + source.DispositionRationale + "\n\n")
	}
	return b.String()
}
