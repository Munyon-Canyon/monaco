package funding_test

import (
	"context"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func pausesOf(env pauseEnv) *funding.Module {
	return funding.New(module.Deps{Pool: env.pool, UoW: env.uow, Clock: env.clock, IDs: ids.Real{}})
}

func TestPauses_GlobalPausesEveryCabal(t *testing.T) {
	t.Parallel()
	env := newPauseEnv(t)
	own, bystander := testkit.NewCabal(t, env.pool), testkit.NewCabal(t, env.pool)
	m := pausesOf(env)
	assertUnpaused(t, m, bystander.ID, "before any pause")
	mustPause(t, env, own.ID, funding.PauseReasonExternalDeposit)
	if _, err := m.PauseFromOps(t.Context(), nil, "maintenance"); err != nil {
		t.Fatal(err)
	}
	got, err := m.Pauses().IsPaused(t.Context(), bystander.ID)
	if err != nil || !got.Paused || !slices.Equal(got.Reasons, []funding.PauseReason{funding.PauseReasonOps}) ||
		got.Since.IsZero() {
		t.Fatalf("IsPaused(cabal with no row) = %+v, %v, want paused by the global ops pause", got, err)
	}
	got, err = m.Pauses().IsPaused(t.Context(), own.ID)
	want := []funding.PauseReason{funding.PauseReasonOps, funding.PauseReasonExternalDeposit}
	if err != nil || !got.Paused || !slices.Equal(got.Reasons, want) {
		t.Fatalf("IsPaused(own row) = %+v, %v, want %v", got, err, want)
	}
	assertPausedSet(t, m, own.ID)
	if err := m.ResumeFromOps(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	assertUnpaused(t, m, bystander.ID, "after resume --all")
}

func assertUnpaused(t *testing.T, m *funding.Module, cabal ids.CabalID, when string) {
	t.Helper()
	if got, err := m.Pauses().IsPaused(t.Context(), cabal); err != nil || got.Paused {
		t.Fatalf("IsPaused %s = %+v, %v, want unpaused", when, got, err)
	}
}

func assertPausedSet(t *testing.T, m *funding.Module, own ids.CabalID) {
	t.Helper()
	set, err := m.Pauses().PausedCabals(t.Context())
	if err != nil || !set.Global || len(set.Cabals) != 1 ||
		!slices.Equal(set.Cabals[own], []funding.PauseReason{funding.PauseReasonExternalDeposit}) {
		t.Fatalf("PausedCabals = %+v, %v, want global plus %s", set, err, own)
	}
}

func TestPauses_ListsEachCabalReasonOnce(t *testing.T) {
	t.Parallel()
	env := newPauseEnv(t)
	cabal := testkit.NewCabal(t, env.pool)
	mustPause(t, env, cabal.ID, funding.PauseReasonOps)
	mustPause(t, env, cabal.ID, funding.PauseReasonOps)
	set, err := pausesOf(env).Pauses().PausedCabals(t.Context())
	if err != nil || set.Global || !slices.Equal(set.Cabals[cabal.ID], []funding.PauseReason{funding.PauseReasonOps}) {
		t.Fatalf("PausedCabals = %+v, %v, want one ops reason", set, err)
	}
}

func TestPauses_SeesAPauseInsideTheWritersTransaction(t *testing.T) {
	t.Parallel()
	env := newPauseEnv(t)
	cabal := testkit.NewCabal(t, env.pool)
	m := pausesOf(env)
	err := env.uow.Do(opsContext(t), func(ctx context.Context, tx db.Tx) error {
		if _, err := tx.Queries().Exec(ctx, `INSERT INTO cabal_pauses (id, cabal_id, reason, created_at)
			VALUES ($1, $2, 'external_deposit', now())`, testkit.NewIDs(40).NewV7(), cabal.ID.UUID()); err != nil {
			return err
		}
		inside, err := m.PausesIn(tx).IsPaused(ctx, cabal.ID)
		if err != nil {
			return err
		}
		outside, err := m.Pauses().IsPaused(ctx, cabal.ID)
		if !inside.Paused || outside.Paused {
			t.Errorf("IsPaused inside = %+v, outside = %+v; only the writer's transaction sees its own pause",
				inside, outside)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPauses_FailClosedWhenTheTableIsGone(t *testing.T) {
	t.Parallel()
	env := newPauseEnv(t)
	exec(t, env.pool, `ALTER TABLE cabal_pauses RENAME TO cabal_pauses_gone`)
	pauses := pausesOf(env).Pauses()
	_, isPausedErr := pauses.IsPaused(t.Context(), ids.CabalIDFrom(testkit.NewIDs(41).NewV7()))
	_, pausedCabalsErr := pauses.PausedCabals(t.Context())
	for name, err := range map[string]error{"IsPaused": isPausedErr, "PausedCabals": pausedCabalsErr} {
		if errs.CodeOf(err) != errs.CodeInternal {
			t.Errorf("%s error = %v, want %s", name, err, errs.CodeInternal)
		}
	}
}
