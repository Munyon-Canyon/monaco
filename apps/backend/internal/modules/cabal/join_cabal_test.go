package cabal_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/cabalapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type accessFixture struct {
	pool  *pgxpool.Pool
	ids   *testkit.IDs
	clock *testkit.Clock
	uow   *db.UnitOfWork
	users app.UserCards
}

func newAccess(t *testing.T) accessFixture {
	t.Helper()
	pool := testkit.DB(t)
	clk := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Second))
	g := testkit.NewIDs(testkit.RandSeed(t))
	return accessFixture{
		pool: pool, ids: g, clock: clk, uow: db.New(pool, g, clk),
		users: identity.New(module.Deps{Pool: pool}).Queries(),
	}
}

func as(ctx context.Context, user ids.UserID) context.Context {
	return auth.WithActor(ctx, auth.Actor{Kind: auth.ActorUser, ID: user.String(), Standing: auth.StandingActive})
}

func (f accessFixture) user(t *testing.T) ids.UserID {
	t.Helper()
	return testkit.SeedUser(t, f.pool, testkit.UserOpts{}).ID
}

func (f accessFixture) join(ctx context.Context, user ids.UserID, c ids.CabalID) error {
	return app.NewJoinCabalHandler(f.uow, f.clock).Handle(as(ctx, user), app.JoinCabal{ActorID: user, CabalID: c})
}

func (f accessFixture) exec(t *testing.T, statement string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), statement, args...); err != nil {
		t.Fatal(err)
	}
}

type membership struct {
	role    string
	canVote bool
}

func (f accessFixture) membership(t *testing.T, c ids.CabalID, user ids.UserID) (membership, bool) {
	t.Helper()
	var m membership
	err := f.pool.QueryRow(t.Context(), `SELECT role, can_vote FROM cabal_members WHERE cabal_id = $1 AND user_id = $2`,
		c.UUID(), user.UUID()).Scan(&m.role, &m.canVote)
	if errors.Is(err, sql.ErrNoRows) {
		return membership{}, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return m, true
}

func decoded[E any](t *testing.T, f accessFixture, typ events.Type) []E {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT payload FROM events WHERE type = $1 ORDER BY id`, string(typ))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []E
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var e E
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (f accessFixture) joins(t *testing.T) []events.CabalMemberJoined {
	t.Helper()
	var out []events.CabalMemberJoined
	for _, e := range decoded[events.CabalMemberJoined](t, f, events.TypeCabalMemberJoined) {
		if e.Via != "create" {
			out = append(out, e)
		}
	}
	return out
}

func wantErr(t *testing.T, err error, code errs.Code) {
	t.Helper()
	if errs.CodeOf(err) != code {
		t.Fatalf("err = %v, want %s", err, code)
	}
}

func TestJoinCabal_addsAVotingMemberToAnOpenCabalAndAppendsTheJoin(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	joiner := f.user(t)
	if err := f.join(t.Context(), joiner, c.ID); err != nil {
		t.Fatal(err)
	}
	if m, ok := f.membership(t, c.ID, joiner); !ok || m != (membership{"member", true}) {
		t.Fatalf("membership = %+v, %v; want a voting member", m, ok)
	}
	want := events.CabalMemberJoined{V: 1, CabalID: c.ID.UUID(), UserID: joiner.UUID(), Role: "member", Via: "open"}
	if joins := f.joins(t); len(joins) != 1 || joins[0] != want {
		t.Fatalf("member_joined events = %+v, want %+v", joins, want)
	}
}

func TestJoinCabal_addsANonVotingMemberWhenTheCabalVotesByList(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithVoterMode("list"))
	joiner := f.user(t)
	if err := f.join(t.Context(), joiner, c.ID); err != nil {
		t.Fatal(err)
	}
	if m, ok := f.membership(t, c.ID, joiner); !ok || m != (membership{"member", false}) {
		t.Fatalf("membership = %+v, %v; want a member without a vote", m, ok)
	}
}

func TestJoinCabal_refusesWithoutWritingAnything(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	open := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	request := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"), testkit.WithMembers(2))
	banned := testkit.NewCabal(t, f.pool)
	f.exec(t, `UPDATE cabals SET status = 'banned' WHERE id = $1`, banned.ID.UUID())
	outsider := f.user(t)
	for _, tt := range []struct {
		name  string
		user  ids.UserID
		cabal ids.CabalID
		want  errs.Code
	}{
		{"an unknown cabal", outsider, ids.CabalIDFrom(ids.Real{}.NewV7()), errs.CodeCabalNotFound},
		{"a banned cabal", outsider, banned.ID, errs.CodeCabalBanned},
		{"a member of an open cabal", open.Members[1].ID, open.ID, errs.CodeAlreadyMember},
		{"a member of a request cabal", request.Members[1].ID, request.ID, errs.CodeAlreadyMember},
		{"an outsider of a request cabal", outsider, request.ID, errs.CodeJoinNeedsRequest},
	} {
		if err := f.join(t.Context(), tt.user, tt.cabal); errs.CodeOf(err) != tt.want {
			t.Errorf("%s: err = %v, want %s", tt.name, err, tt.want)
		}
	}
	if joins := f.joins(t); len(joins) != 0 {
		t.Fatalf("member_joined events = %+v, want none", joins)
	}
	if _, ok := f.membership(t, request.ID, outsider); ok {
		t.Fatal("the outsider joined the request cabal")
	}
}

func (f accessFixture) holdWrite(t *testing.T, statement string, args ...any) func() {
	t.Helper()
	held, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var holder sync.WaitGroup
	t.Cleanup(holder.Wait)
	t.Cleanup(unblock)
	failed := make(chan error, 1)
	holder.Go(func() {
		failed <- f.uow.Do(context.WithoutCancel(t.Context()), func(ctx context.Context, tx db.Tx) error {
			if _, err := tx.Queries().Exec(ctx, statement, args...); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	})
	select {
	case <-held:
	case err := <-failed:
		t.Fatalf("hold write: %v", err)
	}
	return func() {
		unblock()
		holder.Wait()
		if err := <-failed; err != nil {
			t.Fatalf("hold write: %v", err)
		}
	}
}

func (f accessFixture) waitForALockWaiter(t *testing.T) {
	t.Helper()
	testkit.Eventually(t, func() bool {
		var waiting int
		err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting)
		return err == nil && waiting == 1
	}, 10*time.Second)
}

func TestJoinCabal_reportsAJoinThatLostTheRaceToAConcurrentInsertAsAlreadyMember(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	joiner := f.user(t)
	commit := f.holdWrite(t, `INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at)
		VALUES ($1, $2, 'member', true, now())`, c.ID.UUID(), joiner.UUID())
	done := make(chan error, 1)
	var joining sync.WaitGroup
	t.Cleanup(joining.Wait)
	joining.Go(func() { done <- f.join(t.Context(), joiner, c.ID) })
	f.waitForALockWaiter(t)
	commit()
	wantErr(t, <-done, errs.CodeAlreadyMember)
	if joins := f.joins(t); len(joins) != 0 {
		t.Fatalf("member_joined events = %+v, want none from the losing join", joins)
	}
}

func TestJoinCabal_wrapsStoreFailuresAsInternal(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		statement string
	}{
		{"the cabal lock", `ALTER TABLE cabal_members RENAME TO members_gone`},
		{"stored rules the domain refuses", `ALTER TABLE cabals DROP CONSTRAINT cabals_join_mode_check;
			UPDATE cabals SET join_mode = 'secret'`},
		{"the member insert", `ALTER TABLE cabal_members ADD CONSTRAINT no_members CHECK (role <> 'member')`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newAccess(t)
			c := testkit.NewCabal(t, f.pool)
			joiner := f.user(t)
			f.exec(t, tt.statement)
			wantErr(t, f.join(t.Context(), joiner, c.ID), errs.CodeInternal)
		})
	}
}

func (f accessFixture) routes(users app.UserCards) adapters.HTTP {
	if users == nil {
		users = f.users
	}
	return adapters.HTTP{
		Join:    app.NewJoinCabalHandler(f.uow, f.clock),
		Request: app.NewRequestAccessHandler(f.uow, f.ids, f.clock),
		Revoke:  app.NewRevokeAccessHandler(f.uow, f.clock),
		Decide:  app.NewDecideAccessHandler(f.uow, f.clock),
		Update:  app.NewUpdateCabalHandler(f.uow, f.clock),
		Picture: app.NewSetCabalPictureHandler(f.uow, f.pool, f.ids, f.clock, nil),
		Invite:  f.inviteHandler(),
		DB:      f.pool, Users: users, Clock: f.clock,
	}
}

func TestPostCabalMember_returnsTheCabalAsItsNewMemberSeesIt(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithVoterMode("list"))
	joiner := f.user(t)
	req := api.PostCabalMemberRequestObject{Id: c.ID.UUID()}
	res, err := f.routes(nil).PostCabalMember(as(t.Context(), joiner), req)
	got, ok := res.(api.PostCabalMember200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("PostCabalMember = %T, %v", res, err)
	}
	if got.Id != c.ID.UUID() || got.MemberCount != 2 || got.Me == nil || got.Me.Role != "member" || got.Me.CanVote ||
		got.InviteCode == nil || *got.InviteCode != c.InviteCode {
		t.Fatalf("cabal = %+v, me %+v; want the member view with the invite code", got, got.Me)
	}
}

func TestPostCabalMember_refusesWithTheCommandOrReadError(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	request := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	open := testkit.NewCabal(t, f.pool)
	joiner := f.user(t)
	for _, tt := range []struct {
		name     string
		signedIn bool
		routes   adapters.HTTP
		cabal    ids.CabalID
		want     errs.Code
	}{
		{"no caller", false, f.routes(nil), open.ID, errs.CodeUnauthorized},
		{"a request cabal", true, f.routes(nil), request.ID, errs.CodeJoinNeedsRequest},
		{"the read after the join", true, f.routes(failCards{}), open.ID, errs.CodeInternal},
	} {
		ctx := t.Context()
		if tt.signedIn {
			ctx = as(ctx, joiner)
		}
		_, err := tt.routes.PostCabalMember(ctx, api.PostCabalMemberRequestObject{Id: tt.cabal.UUID()})
		if errs.CodeOf(err) != tt.want {
			t.Errorf("%s: err = %v, want %s", tt.name, err, tt.want)
		}
	}
}

func TestGetCabalByCode_resolvesAPastedCodeToThePreview(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"), testkit.WithMembers(3))
	f.exec(t, `UPDATE cabals SET picture_url = 'https://cdn.example/c.jpg' WHERE id = $1`, c.ID.UUID())
	res, err := f.routes(nil).GetCabalByCode(as(t.Context(), f.user(t)),
		api.GetCabalByCodeRequestObject{Code: " " + strings.ToLower(c.InviteCode) + "\n"})
	got, ok := res.(api.GetCabalByCode200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("GetCabalByCode = %T, %v", res, err)
	}
	want := api.CabalPreview{
		Id: c.ID.UUID(), Name: got.Name, JoinMode: "request", MemberCount: 3, PictureUrl: got.PictureUrl,
	}
	if api.CabalPreview(got) != want || got.Name == "" || got.PictureUrl == nil ||
		*got.PictureUrl != "https://cdn.example/c.jpg" {
		t.Fatalf("preview = %+v, want %+v with the picture", got, want)
	}
}

func TestGetCabalByCode_refusesAnAnonymousCallerAnUnknownCodeAndAStoreFailure(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	user := f.user(t)
	_, err := f.routes(nil).GetCabalByCode(t.Context(), api.GetCabalByCodeRequestObject{Code: c.InviteCode})
	wantErr(t, err, errs.CodeUnauthorized)
	_, err = f.routes(nil).GetCabalByCode(as(t.Context(), user), api.GetCabalByCodeRequestObject{Code: "ZZZZZZZZZZ"})
	wantErr(t, err, errs.CodeCabalNotFound)
	f.exec(t, `ALTER TABLE cabal_members RENAME TO members_gone`)
	_, err = f.routes(nil).GetCabalByCode(as(t.Context(), user), api.GetCabalByCodeRequestObject{Code: c.InviteCode})
	wantErr(t, err, errs.CodeInternal)
}
