package evalproof

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/synergyai-os/Mindline/internal/evalreadback"
)

func writePacket(outRoot string, packet Packet, protectedRoots []string) error {
	dir := filepath.Join(outRoot, DirName)
	if err := evalreadback.ValidateOutputPath(outRoot, dir, protectedRoots); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := map[string][]byte{}
	packetJSON, err := json.MarshalIndent(packet, "", "  ")
	if err != nil {
		return err
	}
	files["proof-packet.json"] = append(packetJSON, '\n')
	files["proof-report.md"] = []byte(markdownReport(packet))
	files["chain-capture-draft.md"] = []byte(chainDraft(packet))
	for name, data := range files {
		if evalreadback.ContainsDeniedString(string(data)) {
			return fmt.Errorf("proof output contains unsafe private or secret pattern: %s", name)
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
	b.WriteString("# Eval Proof Gate\n\n")
	b.WriteString(fmt.Sprintf("- Claim: `%s`\n", packet.Claim))
	b.WriteString(fmt.Sprintf("- Verdict: `%s`\n", packet.Verdict))
	b.WriteString(fmt.Sprintf("- Exit code: `%d`\n", packet.ExitCode))
	b.WriteString(fmt.Sprintf("- Generalization limit: %s\n\n", packet.GeneralizationLimit))
	b.WriteString("## Mandatory Gates\n\n")
	for _, gate := range packet.MandatoryGates {
		b.WriteString(fmt.Sprintf("- `%s`: %s (actual: %s)", gate.Gate, gate.Verdict, gate.ActualStatus))
		if len(gate.ReasonCodes) > 0 {
			b.WriteString(fmt.Sprintf(" — reasons: %s", strings.Join(gate.ReasonCodes, ", ")))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n## Blocked Claims\n\n")
	if len(packet.BlockedClaims) == 0 {
		b.WriteString("None.\n")
	} else {
		for _, claim := range packet.BlockedClaims {
			b.WriteString(fmt.Sprintf("- `%s`: %s", claim.Claim, claim.Status))
			if len(claim.ReasonCodes) > 0 {
				b.WriteString(fmt.Sprintf(" (%s)", strings.Join(claim.ReasonCodes, ", ")))
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n## Next Target\n\n")
	b.WriteString(fmt.Sprintf("`%s`: %s\n", packet.TopImprovementTarget.Code, packet.TopImprovementTarget.Rationale))
	return b.String()
}

func chainDraft(packet Packet) string {
	blocked := make([]string, 0, len(packet.BlockedClaims))
	for _, claim := range packet.BlockedClaims {
		blocked = append(blocked, claim.Claim)
	}
	return fmt.Sprintf("WP-36 eval proof gate: claim %s verdict %s. Blocked claims: %s. Generalization limit: %s. Next target: %s. Proof refs: %s.",
		packet.Claim,
		packet.Verdict,
		strings.Join(blocked, ", "),
		packet.GeneralizationLimit,
		packet.TopImprovementTarget.Code,
		strings.Join(firstRefs(packet.SafeArtifactRefs), ", "),
	)
}

func firstRefs(refs []string) []string {
	out := append([]string(nil), refs...)
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}
