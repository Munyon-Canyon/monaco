package funding_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestPlaceDepositWallet_neverShowsTheDepositAddressBeforeItsBackdatedCreatedAt(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	want := time.Unix(1790000000-100, 0).UTC()
	stop := make(chan struct{})
	var wrong []time.Time
	var reader errgroup.Group
	reader.Go(func() error {
		for {
			select {
			case <-stop:
				return nil
			default:
			}
			wrong = append(wrong, walletsNotCreatedAt(t.Context(), pool, want)...)
		}
	})
	var last testkit.SeededUser
	for range 40 {
		last = testkit.SeedUser(t, pool, testkit.UserOpts{})
		if err := flows.PlaceDepositWallet(t.Context(), pool, &last, want); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	if err := reader.Wait(); err != nil {
		t.Fatal(err)
	}
	if len(wrong) != 0 {
		t.Fatalf("the deposit address was readable with created_at %v, want only %v", wrong, want)
	}
	var owner string
	if err := pool.QueryRow(t.Context(),
		`SELECT user_id::text FROM user_wallets WHERE address = $1`, string(last.Address)).Scan(&owner); err != nil ||
		owner != last.ID.String() {
		t.Fatalf("deposit address owner = %q, %v; want the last user %s", owner, err, last.ID)
	}
}

func walletsNotCreatedAt(ctx context.Context, pool *pgxpool.Pool, want time.Time) []time.Time {
	rows, err := pool.Query(ctx,
		`SELECT created_at FROM user_wallets WHERE address = $1 AND created_at <> $2`,
		"5kwEmpcR8Txq1b4bDazRm9j4cx8Qo2aiE53rYA1dCDDP", want)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var got []time.Time
	for rows.Next() {
		var at time.Time
		if rows.Scan(&at) == nil {
			got = append(got, at)
		}
	}
	return got
}
