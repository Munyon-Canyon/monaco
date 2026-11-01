package adapters_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestLeadsCabalWithMembers_isTrueOnlyForACreatorWhoseCabalHasOtherMembers(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	alone := testkit.NewCabal(t, pool)
	crowd := testkit.NewCabal(t, pool, testkit.WithMembers(2))
	for _, tc := range []struct {
		name string
		user testkit.SeededUser
		want bool
	}{
		{"a creator alone in the cabal", alone.Creator, false},
		{"a creator with another member", crowd.Creator, true},
		{"a member who did not create it", crowd.Members[1], false},
		{"a user with no cabal", testkit.SeedUser(t, pool, testkit.UserOpts{}), false},
	} {
		got, err := adapters.CabalLeaders{}.LeadsCabalWithMembers(t.Context(), pool, tc.user.ID)
		if err != nil || got != tc.want {
			t.Fatalf("%s: LeadsCabalWithMembers = %t, %v, want %t", tc.name, got, err, tc.want)
		}
	}
}

func TestLeadsCabalWithMembers_locksTheCabalSoAJoinWaitsForTheDeletion(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	cabal := testkit.NewCabal(t, pool)
	held, letGo, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var deleteErr error
	var deleter sync.WaitGroup
	deleter.Go(func() {
		defer close(finished)
		deleteErr = db.New(pool, ids.Real{}, clock.Real{}).Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
			led, err := adapters.CabalLeaders{}.LeadsCabalWithMembers(ctx, tx.Queries(), cabal.Creator.ID)
			if err != nil || led {
				return errs.New(errs.CodeInternal, "test", slog.Bool("led", led))
			}
			close(held)
			<-letGo
			return nil
		})
	})
	select {
	case <-held:
	case <-finished:
		t.Fatalf("LeadsCabalWithMembers in a transaction: %v", deleteErr)
	}
	_, err := pool.Exec(t.Context(), `SELECT id FROM cabals WHERE id = $1 FOR SHARE NOWAIT`, cabal.ID.UUID())
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
		t.Fatalf("a join's share lock while the deletion holds the cabal = %v, want lock_not_available", err)
	}
	close(letGo)
	deleter.Wait()
	if deleteErr != nil {
		t.Fatal(deleteErr)
	}
	if _, err := pool.Exec(
		t.Context(),
		`SELECT id FROM cabals WHERE id = $1 FOR SHARE NOWAIT`,
		cabal.ID.UUID(),
	); err != nil {
		t.Fatalf("a join's share lock after the deletion committed = %v, want it granted", err)
	}
}

func TestLeadsCabalWithMembers_surfacesAQueryFailure(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	if _, err := pool.Exec(t.Context(), `ALTER TABLE cabals RENAME TO cabals_gone`); err != nil {
		t.Fatal(err)
	}
	_, err := adapters.CabalLeaders{}.LeadsCabalWithMembers(
		t.Context(),
		pool,
		testkit.SeedUser(t, pool, testkit.UserOpts{}).ID,
	)
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("LeadsCabalWithMembers = %v, want internal", err)
	}
}
