package documents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceMeaningPacketBuildsCompressedReviewPacketWithoutRawExcerpts(t *testing.T) {
	root := t.TempDir()
	pressureOut := filepath.Join(root, "pressure")
	pressure, _, err := BuildCorpusPressure(fixturePath(t, "markdown"), pressureOut, CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build corpus pressure: %v", err)
	}
	if pressure.GraphAtomCount == 0 {
		t.Fatalf("fixture should produce graph atoms: %+v", pressure)
	}

	packetOut := filepath.Join(root, "packet")
	packet, groups, err := BuildSourceMeaningPacket(pressureOut, packetOut)
	if err != nil {
		t.Fatalf("build source meaning packet: %v", err)
	}

	if packet.SchemaVersion != SourceMeaningPacketSchemaVersion {
		t.Fatalf("unexpected schema version %s", packet.SchemaVersion)
	}
	if packet.ReviewGroupCount == 0 || packet.ReviewGroupCount >= packet.AtomCount {
		t.Fatalf("expected compressed groups, got atoms=%d groups=%d", packet.AtomCount, packet.ReviewGroupCount)
	}
	if packet.EvidenceOrBlockerGroupRatio != 1 {
		t.Fatalf("expected every group to have evidence or blockers: %+v", packet)
	}
	if packet.Guardrails.DestinationWrites != 0 || packet.Guardrails.ProductBrainWrites != 0 || packet.Guardrails.TolariaWrites != 0 {
		t.Fatalf("expected zero write guardrails: %+v", packet.Guardrails)
	}
	if packet.Guardrails.HostedInferenceCalls != 0 || packet.Guardrails.HostedTelemetryExports != 0 {
		t.Fatalf("expected zero hosted side effects: %+v", packet.Guardrails)
	}
	if len(groups) != packet.ReviewGroupCount {
		t.Fatalf("expected group count to match summary")
	}
	for _, group := range groups {
		if group.WriteEligible {
			t.Fatalf("group should be write-ineligible: %+v", group)
		}
		if group.EvidenceReferenceCount == 0 && len(group.BlockerReasons) == 0 {
			t.Fatalf("group lacks evidence and blockers: %+v", group)
		}
	}

	packetText := allPacketText(t, filepath.Join(packetOut, SourceMeaningPacketDirName))
	for _, denied := range sourceFixtureDeniedLines(t, fixturePath(t, "markdown")) {
		if strings.Contains(packetText, denied) {
			t.Fatalf("packet leaked raw source line %q\n%s", denied, packetText)
		}
	}
	for _, expected := range []string{"review-packet.md", "evidence-map.json", "blocked-items.json"} {
		if _, err := os.Stat(filepath.Join(packetOut, SourceMeaningPacketDirName, expected)); err != nil {
			t.Fatalf("missing packet artifact %s: %v", expected, err)
		}
	}
}

func allPacketText(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatalf("read packet files: %v", err)
	}
	return b.String()
}

func sourceFixtureDeniedLines(t *testing.T, root string) []string {
	t.Helper()
	lines := []string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if len(line) < 32 || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "|") || strings.HasPrefix(line, "-") {
				continue
			}
			lines = append(lines, line)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read source fixture lines: %v", err)
	}
	if len(lines) == 0 {
		t.Fatalf("fixture denied line set is empty")
	}
	return lines
}
