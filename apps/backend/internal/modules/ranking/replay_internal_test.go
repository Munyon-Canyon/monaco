package ranking

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestProjection_isEmptyWithoutASourceRankingModule(t *testing.T) {
	t.Parallel()
	if got := New(module.Deps{}).Projection(module.Set{}); got != nil {
		t.Fatalf("Projection = %v, want none", got)
	}
}
