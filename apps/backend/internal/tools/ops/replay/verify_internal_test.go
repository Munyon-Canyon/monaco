package replay

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestDiffRows_namesRowsOnlyOneSideHas(t *testing.T) {
	t.Parallel()
	got := diffRows("t", []string{"a", "b", "d", "f"}, []string{"b", "c", "d", "e"})
	want := []string{"t: only in source: a", "t: only in replay: c", "t: only in replay: e", "t: only in source: f"}
	if !slices.Equal(got, want) {
		t.Fatalf("diffRows = %q, want %q", got, want)
	}
}

func TestVerify_failsWhenTheTargetCannotListTables(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	pool.Close()
	if _, err := verify(t.Context(), Options{Target: pool}); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err = %v, want internal", err)
	}
}
