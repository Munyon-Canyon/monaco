package verify

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/tools/ops/replay"
)

func TestFromReplay_failsACheckOnItsDiffsOrItsError(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeDBUnavailable, "fixture.check")
	returning := func(diffs []string, err error) func(context.Context, *pgxpool.Pool) ([]string, error) {
		return func(context.Context, *pgxpool.Pool) ([]string, error) { return diffs, err }
	}
	checks := fromReplay([]replay.LedgerCheck{
		{Name: "clean", Check: returning(nil, nil)},
		{Name: "drift", Check: returning([]string{"cabal off by 1", "user off by 2"}, nil)},
		{Name: "broken", Check: returning(nil, boom)},
	})
	if len(checks) != 3 || checks[0].Name != "clean" || checks[1].Name != "drift" || checks[2].Name != "broken" {
		t.Fatalf("checks = %+v, want one per replay check in order", checks)
	}
	if err := checks[0].Check(t.Context(), nil); err != nil {
		t.Fatalf("clean = %v, want nil", err)
	}
	var diffs LedgerDiffsError
	if err := checks[1].Check(t.Context(), nil); !errors.As(err, &diffs) ||
		err.Error() != "cabal off by 1; user off by 2" {
		t.Fatalf("drift = %v, want both diffs", err)
	}
	if err := checks[2].Check(t.Context(), nil); !errors.Is(err, boom) {
		t.Fatalf("broken = %v, want the check's error", err)
	}
}
