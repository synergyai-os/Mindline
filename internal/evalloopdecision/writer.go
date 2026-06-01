package evalloopdecision

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/synergyai-os/Mindline/internal/evalreadback"
)

func Write(outRoot string, packet Packet, protectedRoots []string) error {
	dir := filepath.Join(outRoot, DirName)
	if err := evalreadback.ValidateOutputPath(outRoot, dir, protectedRoots); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	packetJSON, err := json.MarshalIndent(packet, "", "  ")
	if err != nil {
		return err
	}
	files := map[string][]byte{
		"decision-packet.json":   append(packetJSON, '\n'),
		"decision-report.md":     []byte(markdownReport(packet)),
		"chain-capture-draft.md": []byte(chainDraft(packet)),
	}
	for name, data := range files {
		if evalreadback.ContainsDeniedString(string(data)) {
			return fmt.Errorf("loop decision output contains unsafe private or secret pattern: %s", name)
		}
		target := filepath.Join(dir, name)
		if err := evalreadback.ValidateOutputPath(outRoot, target, protectedRoots); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func markdownReport(packet Packet) string {
	var b strings.Builder
	b.WriteString("# Eval Loop Decision\n\n")
	b.WriteString(fmt.Sprintf("- Decision kind: `%s`\n", packet.DecisionKind))
	b.WriteString(fmt.Sprintf("- Improvement state: `%s`\n", packet.ImprovementState))
	b.WriteString(fmt.Sprintf("- Product-general target: `%s`\n", packet.ProductGeneralTarget))
	b.WriteString(fmt.Sprintf("- Safety: `%s`\n", packet.ClaimStatuses.Safety))
	b.WriteString(fmt.Sprintf("- Generalization: `%s`\n", packet.ClaimStatuses.Generalization))
	b.WriteString(fmt.Sprintf("- DEC-64: `%s`\n\n", packet.ClaimStatuses.DEC64))
	b.WriteString("## Top Target\n\n")
	b.WriteString(fmt.Sprintf("`%s`: %s\n\n", packet.TopImprovementTarget.Code, packet.TopImprovementTarget.Rationale))
	b.WriteString("## Rerun\n\n")
	b.WriteString(packet.RerunInstruction + "\n\n")
	b.WriteString("## Limits\n\n")
	for _, limit := range packet.DecisionLimits {
		b.WriteString("- " + limit + "\n")
	}
	return b.String()
}

func chainDraft(packet Packet) string {
	return fmt.Sprintf("WP-38 eval loop decision: improvement_state=%s; top_target=%s; product_general_target=%s; safety=%s; generalization=%s; dec64=%s; rerun=%s.",
		packet.ImprovementState,
		packet.TopImprovementTarget.Code,
		packet.ProductGeneralTarget,
		packet.ClaimStatuses.Safety,
		packet.ClaimStatuses.Generalization,
		packet.ClaimStatuses.DEC64,
		packet.RerunInstruction,
	)
}
