package social_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

func TestBlockAndFollow_neverLeaveALiveFollowNextToABlock(t *testing.T) {
	t.Parallel()
	for i := range 8 {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			raceBlockAndFollow(t)
		})
	}
}

func raceBlockAndFollow(t *testing.T) {
	t.Helper()
	f := newFixture(t)
	var group errgroup.Group
	start := make(chan struct{})
	group.Go(func() error {
		<-start
		return f.block(t, f.alice, f.bob)
	})
	group.Go(func() error {
		<-start
		err := f.follow.Handle(f.ctx(t), app.Follow{Follower: f.bob, Followee: f.alice, Source: domain.SourceProfile})
		if errs.CodeOf(err) == errs.CodeFollowBlocked {
			return nil
		}
		return err
	})
	close(start)
	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}
	var live int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM follows WHERE deleted_at IS NULL
		AND EXISTS (SELECT 1 FROM user_blocks)`).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != 0 || f.blockRows(t) != 1 {
		t.Fatalf("live follows next to the block = %d, blocks = %d, want 0 and 1", live, f.blockRows(t))
	}
}

func TestBlockAndFollow_failAsUnavailableWhenThePairLockTimesOut(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cfg := f.pool.Config()
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = "50ms"
	short, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(short.Close)
	uow := db.New(short, f.gen, f.clock)
	blocker := app.NewBlockUserHandler(app.BlockUserDeps{UoW: uow, Users: f.users, IDs: f.gen, Clock: f.clock})
	follower := app.NewFollowHandler(app.FollowDeps{UoW: uow, Users: f.users, IDs: f.gen, Clock: f.clock})
	cmd := app.Follow{Follower: f.alice, Followee: f.bob, Source: domain.SourceProfile}
	holder := db.New(f.pool, f.gen, f.clock)
	err = holder.Do(f.ctx(t), func(ctx context.Context, tx db.Tx) error {
		if _, err := tx.Queries().Exec(ctx,
			`SELECT pg_advisory_xact_lock(hashtextextended('user-pair:' ||
				least($1::uuid, $2::uuid)::text || greatest($1::uuid, $2::uuid)::text, 0))`,
			f.alice.UUID(), f.bob.UUID()); err != nil {
			return err
		}
		wantCode(t, blocker.Handle(ctx, app.BlockUser{Blocker: f.alice, Blocked: f.bob}), errs.CodeDBUnavailable)
		wantCode(t, follower.Handle(ctx, cmd), errs.CodeDBUnavailable)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
