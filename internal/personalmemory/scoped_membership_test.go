package personalmemory

import (
	"reflect"
	"testing"
)

func TestCompactExpansionFreezesRecordMembershipBeforeContextOrder(t *testing.T) {
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
	first, firstResources, err := expandCompactHits([]RankedHit{
		hit("resource", 2, 2), hit("record-c", 1, 1),
	}, projection, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	second, secondResources, err := expandCompactHits([]RankedHit{
		hit("record-c", 2, 1), hit("resource", 1, 2),
	}, projection, 2, true)
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
	want := []string{"record-a", "record-b"}
	if !reflect.DeepEqual(ids(first), want) || !reflect.DeepEqual(ids(second), want) ||
		firstResources["record-a"] != "shared-resource" ||
		firstResources["record-b"] != "shared-resource" ||
		!reflect.DeepEqual(firstResources, secondResources) {
		t.Fatalf("record membership changed: first=%v second=%v first_resources=%v second_resources=%v",
			ids(first), ids(second), firstResources, secondResources)
	}
}
