package processing

import (
	"fmt"
	"strings"
	"testing"
)

func TestStrategyDoesNotImposeAProductLensCountCap(t *testing.T) {
	lenses := make([]string, 200)
	for index := range lenses {
		lenses[index] = fmt.Sprintf("User-defined lens %03d", index+1)
	}
	strategy := SealStrategy(StrategySnapshot{
		StrategyID:    "user-owned-lenses",
		Version:       "1",
		ContextLenses: strings.Join(lenses, "\n"),
		RoutingPolicy: "Retain everything; route only evidence-backed approved derivatives.",
	})
	if err := ValidateStrategy(strategy); err != nil {
		t.Fatalf("strategy imposed a product lens count cap: %v", err)
	}
	if len(ContextLenses(strategy)) != len(lenses) {
		t.Fatalf("lens parsing changed the user-defined set: got=%d want=%d", len(ContextLenses(strategy)), len(lenses))
	}
}
