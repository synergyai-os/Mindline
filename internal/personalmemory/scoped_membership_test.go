package personalmemory

import (
	"reflect"
	"sort"
	"testing"
)

func TestCompactExpansionFreezesCompleteEligibilityBeforeContextLimit(t *testing.T) {
	projection := compactRetrievalProjection{
		ownersByDocumentID: map[string][]string{
			"resource": {"record-a", "record-b"},
			"record-c": {"record-c"},
		},
		resourceByDocumentID: map[string]string{"resource": "shared-resource"},
		recordsByID: map[string]CaptureRecord{
			"record-a": {RecordID: "record-a"},
			"record-b": {RecordID: "record-b"},
			"record-c": {RecordID: "record-c"},
		},
	}
	hit := func(id string, contextual, authorization float64) RankedHit {
		return RankedHit{DocumentID: id, Score: contextual, Components: map[string]float64{
			"authorization_base_raw": authorization,
		}}
	}
	firstOrder := []RankedHit{
		hit("resource", 2, 2), hit("record-c", 1, 1),
	}
	secondOrder := []RankedHit{
		hit("record-c", 2, 1), hit("resource", 1, 2),
	}
	first, firstResources, err := expandCompactHits(firstOrder, projection, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	second, secondResources, err := expandCompactHits(secondOrder, projection, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	ids := func(hits []RankedHit) []string {
		result := make([]string, 0, len(hits))
		for _, value := range hits {
			result = append(result, value.DocumentID)
		}
		return result
	}
	if !reflect.DeepEqual(ids(first), []string{"record-a", "record-b"}) ||
		!reflect.DeepEqual(ids(second), []string{"record-c", "record-a"}) ||
		firstResources["record-a"] != "shared-resource" ||
		firstResources["record-b"] != "shared-resource" ||
		secondResources["record-a"] != "shared-resource" || len(secondResources) != 1 {
		t.Fatalf("context was not applied after eligibility: first=%v second=%v first_resources=%v second_resources=%v",
			ids(first), ids(second), firstResources, secondResources)
	}
	firstPool, _, err := expandCompactHits(firstOrder, projection, 3, true)
	if err != nil {
		t.Fatal(err)
	}
	secondPool, _, err := expandCompactHits(secondOrder, projection, 3, true)
	if err != nil {
		t.Fatal(err)
	}
	firstIDs, secondIDs := ids(firstPool), ids(secondPool)
	sort.Strings(firstIDs)
	sort.Strings(secondIDs)
	if !reflect.DeepEqual(firstIDs, []string{"record-a", "record-b", "record-c"}) ||
		!reflect.DeepEqual(firstIDs, secondIDs) {
		t.Fatalf("query-only eligible pool changed: first=%v second=%v", firstIDs, secondIDs)
	}
}
