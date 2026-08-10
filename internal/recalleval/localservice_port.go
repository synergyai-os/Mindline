package recalleval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/synergyai-os/Mindline/internal/localservice"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

const (
	ServiceModeCompact = "compact"
	ServiceModeLegacy  = "legacy"
)

type AgentClientPort interface {
	Status(context.Context) (localservice.Status, error)
	Search(context.Context, localservice.SearchInput) (personalmemory.ContextPacket, error)
	SearchCompact(context.Context, localservice.SearchInput) (personalmemory.CompactContextPacket, error)
	Get(context.Context, string) (personalmemory.HydratedCapture, error)
}

// LocalServicePort evaluates the actual installed Unix-socket API. Compact
// mode proves the bounded v0.3 surface; legacy mode adapts the unchanged v0.2
// baseline for comparable ranking only.
type LocalServicePort struct {
	Client AgentClientPort
	Mode   string
}

func (port LocalServicePort) SearchCompact(ctx context.Context, query string) (CompactSearchResult, error) {
	if port.Client == nil {
		return CompactSearchResult{}, errors.New("local service evaluation client is required")
	}
	result := CompactSearchResult{}
	switch port.Mode {
	case ServiceModeCompact:
		packet, err := port.Client.SearchCompact(ctx, localservice.SearchInput{Query: query, Limit: 5})
		if err != nil {
			return CompactSearchResult{}, err
		}
		if packet.SchemaVersion != personalmemory.CompactPacketSchemaVersion {
			return CompactSearchResult{}, errors.New("local service returned an unsupported compact packet")
		}
		result.LibraryFingerprint = libraryFingerprintCommitment(packet.LibraryFingerprint)
		for _, citation := range packet.Citations {
			result.Citations = append(result.Citations, CompactCitation{RecordID: citation.RecordID})
		}
	case ServiceModeLegacy:
		packet, err := port.Client.Search(ctx, localservice.SearchInput{Query: query, Limit: 5})
		if err != nil {
			return CompactSearchResult{}, err
		}
		if packet.SchemaVersion != personalmemory.ContextPacketSchemaVersion {
			return CompactSearchResult{}, errors.New("local service returned an unsupported legacy packet")
		}
		result.LibraryFingerprint = libraryFingerprintCommitment(packet.LibraryFingerprint)
		for _, citation := range packet.Citations {
			result.Citations = append(result.Citations, CompactCitation{RecordID: citation.RecordID})
		}
		// Legacy v0.2 intentionally hydrates its packet. This flag is ignored
		// for the baseline and must be false for the compact candidate.
		result.UnselectedHydratedContent = true
	default:
		return CompactSearchResult{}, errors.New("unsupported local service evaluation mode")
	}
	return result, nil
}

func (port LocalServicePort) GetCanonicalEvidence(ctx context.Context, recordID string) (CanonicalEvidence, error) {
	if port.Client == nil {
		return CanonicalEvidence{}, errors.New("local service evaluation client is required")
	}
	before, err := port.Client.Status(ctx)
	if err != nil {
		return CanonicalEvidence{}, fmt.Errorf("%w: read status before hydration", ErrFrozenLibraryBinding)
	}
	capture, err := port.Client.Get(ctx, recordID)
	if err != nil {
		return CanonicalEvidence{}, err
	}
	after, err := port.Client.Status(ctx)
	if err != nil {
		return CanonicalEvidence{}, fmt.Errorf("%w: read status after hydration", ErrFrozenLibraryBinding)
	}
	libraryFingerprint := libraryFingerprintCommitment(before.Memory.Fingerprint)
	if libraryFingerprint == "" || libraryFingerprint != libraryFingerprintCommitment(after.Memory.Fingerprint) {
		return CanonicalEvidence{}, fmt.Errorf("%w: canonical evidence changed during hydration", ErrFrozenLibraryBinding)
	}
	if capture.VersionState != "current" || capture.Record.RecordID != recordID {
		return CanonicalEvidence{}, errors.New("selected citation is not current canonical evidence")
	}
	missingness := append([]string(nil), capture.Record.Missingness...)
	resourceStates := make([]string, 0, len(capture.Resources))
	for _, resource := range capture.Resources {
		missingness = append(missingness, resource.Missingness...)
		resourceStates = append(resourceStates, resource.ResourceID+"\x00"+resource.State+"\x00"+resource.AccessClass+"\x00"+resource.ContentHash)
	}
	sort.Strings(missingness)
	sort.Strings(resourceStates)
	return CanonicalEvidence{
		RecordID: recordID, SourceCommitment: commitment(capture.Record.SourceRef),
		AuthorityClass: capture.Record.AuthorityClass, Current: true,
		ContentHash: "sha256:" + capture.Record.ContentHash,
		Missingness: missingness, ResourceStates: resourceStates,
		LibraryFingerprint: libraryFingerprint,
	}, nil
}

func libraryFingerprintCommitment(value string) string {
	value = strings.TrimSpace(value)
	if isFingerprint(value) {
		return value
	}
	if isFingerprint("sha256:" + value) {
		return "sha256:" + value
	}
	return ""
}

func commitment(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(digest[:])
}
