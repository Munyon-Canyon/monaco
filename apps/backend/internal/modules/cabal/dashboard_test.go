package cabal_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestDashboard_Counts_CountsCabalsBansAndMembersPerCabal(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	banned := testkit.NewCabal(t, pool, testkit.WithMembers(4))
	for _, members := range []int{1, 2, 3, 10} {
		testkit.NewCabal(t, pool, testkit.WithMembers(members))
	}
	if _, err := pool.Exec(
		t.Context(),
		`UPDATE cabals SET status = 'banned' WHERE id = $1`,
		banned.ID.UUID(),
	); err != nil {
		t.Fatal(err)
	}
	got, err := cabal.New(module.Deps{Pool: pool}).DashboardOn(pool).Counts(t.Context())
	want := port.Counts{Cabals: 5, Banned: 1, MembersP50: 3, MembersP90: 10, MembersMax: 10}
	if err != nil || got != want {
		t.Fatalf("Counts() = %+v, %v, want %+v", got, err, want)
	}
}

func TestDashboard_Counts_AreZeroWithNoCabals(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	got, err := cabal.New(module.Deps{Pool: pool}).DashboardOn(pool).Counts(t.Context())
	if err != nil || got != (port.Counts{}) {
		t.Fatalf("Counts() = %+v, %v, want zeros", got, err)
	}
}

func TestDashboard_Counts_FailOnACancelledContext(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := cabal.New(module.Deps{Pool: pool}).DashboardOn(pool).Counts(ctx); err == nil {
		t.Fatal("Counts() error = nil on a cancelled context")
	}
}
