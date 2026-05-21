package productbrain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/synergyai-os/Mindline/internal/pipeline/runs"
)

func Propose(runDir string, profilePath string, outDir string) (Summary, error) {
	profileData, err := os.ReadFile(profilePath)
	if err != nil {
		return Summary{}, fmt.Errorf("read profile: %w", err)
	}
	profile, err := ParseProfile(profileData)
	if err != nil {
		return Summary{}, fmt.Errorf("parse profile: %w", err)
	}
	manifest, err := readManifest(runDir)
	if err != nil {
		return Summary{}, err
	}
	reviewItems, err := readReviewItems(runDir)
	if err != nil {
		return Summary{}, err
	}
	proposals := make([]Proposal, 0, len(reviewItems))
	for _, item := range reviewItems {
		proposals = append(proposals, Resolve(ResolveInput{
			RunID:        item.RunID,
			ReviewItemID: item.RecordID,
			SafeTitle:    item.SafeTitle,
			SafeContext:  item.SafeContext,
			Reason:       item.Reason,
		}, profile))
	}
	if len(proposals) == 0 {
		return Summary{}, fmt.Errorf("run directory has no review queue items")
	}
	return Write(outDir, WriteInput{
		RunID:        manifest.RunID,
		Profile:      profile,
		Proposals:    proposals,
		AuthorityIDs: WP9AuthorityIDs,
	})
}

func readManifest(runDir string) (runs.Manifest, error) {
	data, err := readFileUnder(runDir, filepath.Join("ledger", "run-manifest.json"))
	if err != nil {
		return runs.Manifest{}, fmt.Errorf("read run manifest: %w", err)
	}
	var manifest runs.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return runs.Manifest{}, fmt.Errorf("decode run manifest: %w", err)
	}
	if manifest.SchemaVersion != runs.RunLedgerSchemaVersion || strings.TrimSpace(manifest.RunID) == "" {
		return runs.Manifest{}, fmt.Errorf("invalid run manifest")
	}
	return manifest, nil
}

func readReviewItems(runDir string) ([]runs.ReviewQueueItem, error) {
	reviewRoot := filepath.Join(runDir, "review-queue")
	data, err := readFileUnder(runDir, filepath.Join("review-queue", "review-queue.json"))
	if err != nil {
		return nil, fmt.Errorf("read review queue: %w", err)
	}
	var queue runs.ReviewQueue
	if err := json.Unmarshal(data, &queue); err != nil {
		return nil, fmt.Errorf("decode review queue: %w", err)
	}
	if queue.SchemaVersion != runs.ReviewQueueSchemaVersion {
		return nil, fmt.Errorf("invalid review queue")
	}
	items := make([]runs.ReviewQueueItem, 0, len(queue.Items))
	for _, entry := range queue.Items {
		itemData, err := readFileUnder(reviewRoot, entry.ReviewItemPath)
		if err != nil {
			return nil, fmt.Errorf("read review item: %w", err)
		}
		var item runs.ReviewQueueItem
		if err := json.Unmarshal(itemData, &item); err != nil {
			return nil, fmt.Errorf("decode review item: %w", err)
		}
		if item.SchemaVersion != runs.ReviewItemSchemaVersion {
			return nil, fmt.Errorf("invalid review item")
		}
		items = append(items, item)
	}
	return items, nil
}

func readFileUnder(root string, relPath string) ([]byte, error) {
	cleanRel := filepath.Clean(filepath.FromSlash(relPath))
	if cleanRel == "." || filepath.IsAbs(cleanRel) || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(os.PathSeparator)) {
		return nil, fmt.Errorf("path escaped run directory")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	realPath, err := filepath.EvalSymlinks(filepath.Join(root, cleanRel))
	if err != nil {
		return nil, err
	}
	relativeToRoot, err := filepath.Rel(realRoot, realPath)
	if err != nil {
		return nil, err
	}
	if relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(os.PathSeparator)) || filepath.IsAbs(relativeToRoot) {
		return nil, fmt.Errorf("path escaped run directory")
	}
	return os.ReadFile(realPath)
}
