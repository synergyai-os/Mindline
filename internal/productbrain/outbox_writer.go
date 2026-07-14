package productbrain

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

func WriteOutbox(outDir string, outbox Outbox, summary OutboxSummary) error {
	if err := privateio.PrepareDir(outDir); err != nil {
		return err
	}
	if err := privateio.WriteJSON(filepath.Join(outDir, "outbox.json"), outbox); err != nil {
		return err
	}
	if err := privateio.WriteJSON(filepath.Join(outDir, "outbox-summary.json"), summary); err != nil {
		return err
	}
	return privateio.WriteFile(filepath.Join(outDir, "review-packet.md"), []byte(outboxReview(outbox, summary)), false)
}
func outboxReview(outbox Outbox, summary OutboxSummary) string {
	var b strings.Builder
	b.WriteString("# Product Brain Outbox Review\n\n")
	b.WriteString(fmt.Sprintf("- Operations: %d (%d entries, %d relations)\n- Public privacy findings: %d\n- Draft only: %t\n\n", summary.OperationCount, summary.EntryOperationCount, summary.RelationOperationCount, summary.PrivacyFindingCount, summary.DraftOnly))
	for _, op := range outbox.Operations {
		b.WriteString("- `" + op.OperationID + "` " + op.Kind + "\n")
	}
	return b.String()
}
