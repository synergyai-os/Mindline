package productbrain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type WriteInput struct {
	RunID        string
	Profile      Profile
	Proposals    []Proposal
	AuthorityIDs []string
}

func WriteProposals(outDir string, proposals []Proposal, profile Profile) error {
	_, err := Write(outDir, WriteInput{
		RunID:        runIDFromProposals(proposals),
		Profile:      profile,
		Proposals:    proposals,
		AuthorityIDs: WP9AuthorityIDs,
	})
	return err
}

func Write(outDir string, input WriteInput) (Summary, error) {
	if strings.TrimSpace(outDir) == "" {
		return Summary{}, fmt.Errorf("missing required --out")
	}
	root, err := filepath.Abs(outDir)
	if err != nil {
		return Summary{}, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return Summary{}, err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return Summary{}, err
	}
	summary := buildSummary(input)
	if err := rejectSentinels(summary); err != nil {
		return Summary{}, err
	}
	if err := rejectDuplicateProposalIDs(input.Proposals); err != nil {
		return Summary{}, err
	}
	for _, proposal := range input.Proposals {
		if err := rejectSentinels(proposal); err != nil {
			return Summary{}, err
		}
	}
	if err := writeJSON(realRoot, "productbrain-proposals/proposal-summary.json", summary); err != nil {
		return Summary{}, err
	}
	for _, proposal := range input.Proposals {
		if err := writeJSON(realRoot, filepath.ToSlash(filepath.Join("productbrain-proposals", "proposals", proposal.ProposalID+".json")), proposal); err != nil {
			return Summary{}, err
		}
		if err := writeFile(realRoot, filepath.ToSlash(filepath.Join("productbrain-proposals", "previews", proposal.ProposalID+".md")), []byte(previewMarkdown(proposal))); err != nil {
			return Summary{}, err
		}
	}
	return summary, nil
}

func buildSummary(input WriteInput) Summary {
	authorityIDs := input.AuthorityIDs
	if len(authorityIDs) == 0 {
		authorityIDs = WP9AuthorityIDs
	}
	summary := Summary{
		SchemaVersion: ProposalSummarySchemaVersion,
		RunID:         input.RunID,
		WorkspaceProfile: WorkspaceProfile{
			SchemaVersion: input.Profile.SchemaVersion,
			WorkspaceSlug: input.Profile.Workspace.Slug,
			Fingerprint:   ProfileFingerprint(input.Profile),
		},
		Proposals:    make([]SummaryItem, 0, len(input.Proposals)),
		AuthorityIDs: append([]string(nil), authorityIDs...),
	}
	for _, proposal := range input.Proposals {
		item := SummaryItem{
			ProposalID:           proposal.ProposalID,
			Status:               proposal.Status,
			Intent:               proposal.Intent,
			TargetCollectionSlug: targetCollectionSlug(proposal),
			ProposalPath:         filepath.ToSlash(filepath.Join("proposals", proposal.ProposalID+".json")),
			PreviewPath:          filepath.ToSlash(filepath.Join("previews", proposal.ProposalID+".md")),
		}
		if proposal.Status == ProposalStatusBlocked {
			summary.BlockedCount++
		}
		summary.Proposals = append(summary.Proposals, item)
	}
	summary.ProposalCount = len(summary.Proposals)
	return summary
}

func writeJSON(root string, relative string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFile(root, relative, data)
}

func writeFile(root string, relative string, data []byte) error {
	if filepath.IsAbs(relative) || strings.Contains(relative, "..") {
		return fmt.Errorf("output path escaped output directory")
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

func ensureParentDir(root string, relative string) error {
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

func rejectDuplicateProposalIDs(proposals []Proposal) error {
	seen := map[string]bool{}
	for _, proposal := range proposals {
		if strings.TrimSpace(proposal.ProposalID) == "" {
			return fmt.Errorf("missing proposal id")
		}
		if seen[proposal.ProposalID] {
			return fmt.Errorf("duplicate proposal id: %s", proposal.ProposalID)
		}
		seen[proposal.ProposalID] = true
	}
	return nil
}

func isInside(root string, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel))
}

func previewMarkdown(proposal Proposal) string {
	var b strings.Builder
	b.WriteString("# ")
	if proposal.Operation == nil {
		b.WriteString(proposal.ProposalID)
	} else {
		b.WriteString(proposal.Operation.EntryName)
	}
	b.WriteString("\n\n")
	b.WriteString("- Proposal: ")
	b.WriteString(proposal.ProposalID)
	b.WriteString("\n")
	b.WriteString("- Intent: ")
	b.WriteString(string(proposal.Intent))
	b.WriteString("\n")
	b.WriteString("- Target: ")
	b.WriteString(targetCollectionSlug(proposal))
	b.WriteString("\n")
	for _, blocker := range proposal.Blockers {
		b.WriteString("- Blocker: ")
		b.WriteString(blocker.Code)
		if blocker.Message != "" {
			b.WriteString(" - ")
			b.WriteString(blocker.Message)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func targetCollectionSlug(proposal Proposal) string {
	if proposal.Operation == nil {
		return ""
	}
	return proposal.Operation.TargetCollectionSlug
}

func rejectSentinels(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	body := strings.ToLower(string(data))
	for _, sentinel := range []string{
		"private_dm_sentinel_do_not_write",
		"sk-test-secret-do-not-leak",
		"http" + "://",
		"https" + "://",
		"/private",
		"token",
	} {
		if strings.Contains(body, sentinel) {
			return fmt.Errorf("refusing to write private or secret sentinel")
		}
	}
	return nil
}

func runIDFromProposals(proposals []Proposal) string {
	if len(proposals) == 0 {
		return ""
	}
	return proposals[0].RunID
}
