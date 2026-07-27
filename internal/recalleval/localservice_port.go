package recalleval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"

	"github.com/synergyai-os/Mindline/internal/localservice"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

const (
	ServiceModeCompact = "compact"
	ServiceModeLegacy  = "legacy"
)

type AgentClientPort interface {
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
	capture, err := port.Client.Get(ctx, recordID)
	if err != nil {
		return CanonicalEvidence{}, err
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
	}, nil
}

func commitment(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(digest[:])
}
