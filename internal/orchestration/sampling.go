package orchestration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

const (
	MaximumProofItemsPerStratum = 3
	MaximumProofStrata          = 128
	MaximumProofSelectedItems   = 192
)

type SampleManifest struct {
	SchemaVersion        string          `json:"schema_version"`
	Fingerprint          string          `json:"fingerprint"`
	InventoryFingerprint string          `json:"inventory_fingerprint"`
	OrderingVersion      string          `json:"ordering_version"`
	SelectedItemIDs      []string        `json:"selected_item_ids"`
	Strata               []StratumSample `json:"strata"`
}

type StratumSample struct {
	RetrievalStrategyID string   `json:"retrieval_strategy_id"`
	FormatVariant       string   `json:"format_variant"`
	CanonicalCount      int      `json:"canonical_count"`
	SelectedItemIDs     []string `json:"selected_item_ids"`
	UnselectedItemIDs   []string `json:"unselected_item_ids"`
}

func SelectProofSample(inventory InventorySnapshot, orderingVersion string) (SampleManifest, error) {
	if ValidateInventorySnapshot(inventory) != nil || strings.TrimSpace(orderingVersion) == "" {
		return SampleManifest{}, ErrInvalidInventory
	}
	groups := map[string][]InventoryItem{}
	parts := map[string][2]string{}
	for _, item := range inventory.CanonicalItems {
		key := item.RetrievalStrategyID + "\x00" + item.FormatVariant
		groups[key] = append(groups[key], item)
		parts[key] = [2]string{item.RetrievalStrategyID, item.FormatVariant}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > MaximumProofStrata {
		return SampleManifest{}, ErrSampleBudget
	}
	selectedTotal := 0
	for _, key := range keys {
		selectedTotal += minimum(len(groups[key]), MaximumProofItemsPerStratum)
	}
	if selectedTotal > MaximumProofSelectedItems {
		return SampleManifest{}, ErrSampleBudget
	}
	manifest := SampleManifest{SchemaVersion: SampleManifestSchemaVersion, InventoryFingerprint: inventory.Fingerprint, OrderingVersion: orderingVersion}
	for _, key := range keys {
		items := groups[key]
		sort.Slice(items, func(i, j int) bool {
			left := sampleOrderKey(orderingVersion, parts[key][0], parts[key][1], items[i].CanonicalItemID)
			right := sampleOrderKey(orderingVersion, parts[key][0], parts[key][1], items[j].CanonicalItemID)
			if left == right {
				return items[i].CanonicalItemID < items[j].CanonicalItemID
			}
			return left < right
		})
		limit := MaximumProofItemsPerStratum
		if len(items) < limit {
			limit = len(items)
		}
		stratum := StratumSample{RetrievalStrategyID: parts[key][0], FormatVariant: parts[key][1], CanonicalCount: len(items)}
		for index, item := range items {
			if index < limit {
				stratum.SelectedItemIDs = append(stratum.SelectedItemIDs, item.CanonicalItemID)
				manifest.SelectedItemIDs = append(manifest.SelectedItemIDs, item.CanonicalItemID)
			} else {
				stratum.UnselectedItemIDs = append(stratum.UnselectedItemIDs, item.CanonicalItemID)
			}
		}
		manifest.Strata = append(manifest.Strata, stratum)
	}
	manifest.Fingerprint = Fingerprint(manifest)
	return manifest, nil
}

func minimum(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func ValidateSampleManifest(inventory InventorySnapshot, manifest SampleManifest) error {
	if manifest.SchemaVersion != SampleManifestSchemaVersion || manifest.Fingerprint == "" || manifest.InventoryFingerprint != inventory.Fingerprint {
		return ErrSampleChanged
	}
	want, err := SelectProofSample(inventory, manifest.OrderingVersion)
	if err != nil {
		return err
	}
	gotData, _ := json.Marshal(manifest)
	wantData, _ := json.Marshal(want)
	if string(gotData) != string(wantData) {
		return ErrSampleChanged
	}
	return nil
}

func sampleOrderKey(version, strategy, format, itemID string) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{version, strategy, format, itemID}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
