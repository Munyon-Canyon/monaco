package cabal_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type createFixture struct {
	pool    *pgxpool.Pool
	ids     *testkit.IDs
	clock   *testkit.Clock
	uow     *db.UnitOfWork
	user    testkit.SeededUser
	wallets *chainfake.Wallets
	random  io.Reader
}

func newCreate(t *testing.T) createFixture {
	t.Helper()
	pool := testkit.DB(t)
	g := testkit.NewIDs(testkit.RandSeed(t))
	clk := testkit.NewClock(time.Date(2026, 10, 2, 15, 0, 0, 0, time.UTC))
	return createFixture{
		pool: pool, ids: g, clock: clk, uow: db.New(pool, g, clk),
		user:    testkit.SeedUser(t, pool, testkit.UserOpts{}),
		wallets: &chainfake.Wallets{},
	}
}

func (f createFixture) handler() *app.CreateCabalHandler {
	return app.NewCreateCabalHandler(app.CreateCabalDeps{
		UoW: f.uow, Reads: f.pool, Wallets: adapters.AppWallets{Client: f.wallets}, IDs: f.ids, Clock: f.clock,
		Random: f.random,
	})
}

func (f createFixture) actor(ctx context.Context) context.Context {
	return auth.WithActor(ctx, auth.Actor{Kind: auth.ActorUser, ID: f.user.ID.String(), Standing: auth.StandingActive})
}

func (f createFixture) command(t *testing.T, key string) app.CreateCabal {
	t.Helper()
	name, err := domain.ParseName("Friends pot")
	if err != nil {
		t.Fatal(err)
	}
	rules, err := domain.NewRules("request", "list", "unanimous", domain.ExpiryWeek, 50)
	if err != nil {
		t.Fatal(err)
	}
	return app.CreateCabal{ActorID: f.user.ID, IdempotencyKey: key, Name: name, Rules: rules}
}

func (f createFixture) count(ctx context.Context, t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func takeInvite(t *testing.T, f createFixture) {
	t.Helper()
	taken := testkit.NewCabal(t, f.pool)
	_, err := f.pool.Exec(t.Context(),
		`UPDATE cabals SET invite_code = $1 WHERE id = $2`, "0123456789", taken.ID.UUID())
	if err != nil {
		t.Fatal(err)
	}
}

func wantFailure(t *testing.T, err error, wallet string, cabals, got int) {
	t.Helper()
	gotWallet := attrString(err, "privy_wallet_id")
	if errs.CodeOf(err) != errs.CodeInternal || gotWallet != wallet || got != cabals {
		t.Fatalf("err=%v attr=%q cabals=%d; want internal %q and %d cabals", err, gotWallet, got, wallet, cabals)
	}
}

func attrString(err error, key string) string {
	var coded *errs.Error
	if !errors.As(err, &coded) {
		return ""
	}
	for _, a := range coded.Attrs {
		if a.Key == key {
			return a.Value.String()
		}
	}
	return ""
}

type byteChunks struct {
	parts [][]byte
	i     int
	err   error
}

func (c *byteChunks) Read(p []byte) (int, error) {
	if c.i >= len(c.parts) {
		if c.err != nil {
			return 0, c.err
		}
		return 0, io.EOF
	}
	n := copy(p, c.parts[c.i])
	c.i++
	return n, nil
}

type walletStub struct {
	wallet chain.Wallet
	err    error
}

type raceTracer struct {
	seed func()
	done bool
}

type walletLookupKey struct{}

func (r *raceTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "WHERE privy_wallet_id") {
		return context.WithValue(ctx, walletLookupKey{}, true)
	}
	return ctx
}

func (r *raceTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if !r.done && ctx.Value(walletLookupKey{}) != nil {
		r.done = true
		r.seed()
	}
}

func (s walletStub) CreateAppWallet(context.Context, string) (chain.Wallet, error) {
	return s.wallet, s.err
}

func TestCreateCabal_writesTheCabalItsCreatorItsWalletAndBothEvents(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	got, err := f.handler().Handle(f.actor(t.Context()), f.command(t, "c1"))
	if err != nil {
		t.Fatal(err)
	}
	assertCabalShape(t, f, got)
	assertCabalMeta(t, f, got)
	assertCreator(t, f, got)
	assertTreasury(t, f, got)
	assertEvents(t, f, got)
}

func assertCabalShape(t *testing.T, f createFixture, got app.CreatedCabal) {
	t.Helper()
	row, err := sqlc.New(f.pool).FindCabal(t.Context(), got.ID.UUID())
	if err != nil {
		t.Fatal(err)
	}
	if row.Name != "Friends pot" || row.JoinMode != "request" || row.VoterMode != "list" {
		t.Fatalf("FindCabal = %+v", row)
	}
	if row.Threshold != "unanimous" || row.ProposalExpirySeconds != domain.ExpiryWeek || row.SlippageBps != 50 {
		t.Fatalf("rules = %+v", row)
	}
}

func assertCabalMeta(t *testing.T, f createFixture, got app.CreatedCabal) {
	t.Helper()
	row, err := sqlc.New(f.pool).FindCabal(t.Context(), got.ID.UUID())
	if err != nil {
		t.Fatal(err)
	}
	if row.InviteCode != got.InviteCode || row.Status != "active" || row.MemberCount != 1 {
		t.Fatalf("meta = %+v", row)
	}
	if row.CreatorID != f.user.ID.UUID() || !row.CreatedAt.Equal(f.clock.Now()) {
		t.Fatalf("creator = %+v", row)
	}
}

func assertCreator(t *testing.T, f createFixture, got app.CreatedCabal) {
	t.Helper()
	member, err := sqlc.New(f.pool).FindMember(t.Context(), sqlc.FindMemberParams{
		CabalID: got.ID.UUID(), UserID: f.user.ID.UUID(),
	})
	if err != nil || member.Role != "creator" || !member.CanVote {
		t.Fatalf("FindMember = %+v, %v; want the creator, who can vote", member, err)
	}
}

func assertTreasury(t *testing.T, f createFixture, got app.CreatedCabal) {
	t.Helper()
	wallet, err := sqlc.New(f.pool).FindTreasuryWallet(t.Context(), got.ID.UUID())
	if err != nil {
		t.Fatal(err)
	}
	if wallet.PrivyWalletID != got.PrivyWalletID || wallet.Address != string(got.TreasuryAddress) {
		t.Fatalf("wallet = %+v", wallet)
	}
	same, err := f.wallets.CreateAppWallet(t.Context(), app.TreasuryKey(f.user.ID, "c1"))
	if err != nil || same.Address != got.TreasuryAddress || same.ID != got.PrivyWalletID || f.wallets.Creates() != 1 {
		t.Fatalf("privy = %+v, %v; creates %d", same, err, f.wallets.Creates())
	}
}

func assertEvents(t *testing.T, f createFixture, got app.CreatedCabal) {
	t.Helper()
	createdType, createdRaw, joinedType, joinedRaw, actor := readTwoEvents(t, f)
	var created events.CabalCreated
	var joined events.CabalMemberJoined
	if err := json.Unmarshal(createdRaw, &created); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(joinedRaw, &joined); err != nil {
		t.Fatal(err)
	}
	wantCreated := events.CabalCreated{
		V: 1, CabalID: got.ID.UUID(), CreatorID: f.user.ID.UUID(), Name: "Friends pot",
		JoinMode: "request", VoterMode: "list", Threshold: "unanimous",
		ProposalExpirySeconds: domain.ExpiryWeek, SlippageBps: 50, TreasuryAddress: got.TreasuryAddress,
	}
	wantJoined := events.CabalMemberJoined{
		V: 1, CabalID: got.ID.UUID(), UserID: f.user.ID.UUID(), Role: "creator", Via: "create",
	}
	if createdType != string(events.TypeCabalCreated) || actor != "user:"+f.user.ID.String() || created != wantCreated {
		t.Fatalf("created %s %+v actor %s", createdType, created, actor)
	}
	if joinedType != string(events.TypeCabalMemberJoined) || joined != wantJoined {
		t.Fatalf("joined %s %+v", joinedType, joined)
	}
}

func readTwoEvents(t *testing.T, f createFixture) (string, []byte, string, []byte, string) {
	t.Helper()
	rows, err := f.pool.Query(t.Context(),
		`SELECT type, actor_type || ':' || actor_id, payload FROM events ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var createdType, joinedType, actor string
	var createdRaw, joinedRaw []byte
	if !rows.Next() {
		t.Fatal(rows.Err())
	}
	if err := rows.Scan(&createdType, &actor, &createdRaw); err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatal(rows.Err())
	}
	if err := rows.Scan(&joinedType, new(string), &joinedRaw); err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		t.Fatal("extra event")
	}
	return createdType, createdRaw, joinedType, joinedRaw, actor
}

func TestCreateCabal_aRetryAfterCommitReturnsTheCabalWithoutWritingAgain(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	first, err := f.handler().Handle(f.actor(t.Context()), f.command(t, "c1"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.handler().Handle(f.actor(t.Context()), f.command(t, "c1"))
	if err != nil || second != first {
		t.Fatalf("second Handle = %+v, %v; want %+v", second, err, first)
	}
	cabals := f.count(t.Context(), t, "cabals")
	eventsN := f.count(t.Context(), t, "events")
	creates := f.wallets.Creates()
	if cabals != 1 || eventsN != 2 || creates != 1 {
		t.Fatalf("cabals %d events %d creates %d; want 1, 2, 1", cabals, eventsN, creates)
	}
}

func TestCreateCabal_rejectsAChangedRequestForACommittedWallet(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	if _, err := f.handler().Handle(f.actor(t.Context()), f.command(t, "c1")); err != nil {
		t.Fatal(err)
	}
	changed := f.command(t, "c1")
	name, err := domain.ParseName("Other pot")
	if err != nil {
		t.Fatal(err)
	}
	changed.Name = name
	_, err = f.handler().Handle(f.actor(t.Context()), changed)
	if errs.CodeOf(err) != errs.CodeIdempotencyMismatch || f.count(t.Context(), t, "cabals") != 1 ||
		f.count(t.Context(), t, "treasury_wallets") != 1 || f.count(t.Context(), t, "events") != 2 {
		t.Fatalf("Handle = %v, want idempotency mismatch with one cabal, wallet, and two events", err)
	}
}

func TestCreateCabal_recoversWhenTheTreasuryWalletAlreadyExistsAtWrite(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	seeded := testkit.NewCabal(t, f.pool)
	if _, err := f.pool.Exec(t.Context(), `UPDATE cabals SET name = 'Friends pot', creator_id = $1,
		join_mode = 'request', voter_mode = 'list', threshold = 'unanimous', proposal_expiry_seconds = 604800,
		slippage_bps = 50 WHERE id = $2`, f.user.ID.UUID(), seeded.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	tracer := &raceTracer{seed: func() {
		if _, err := f.pool.Exec(t.Context(), `UPDATE treasury_wallets SET privy_wallet_id = 'wallet-race'
			WHERE cabal_id = $1`, seeded.ID.UUID()); err != nil {
			t.Fatal(err)
		}
	}}
	config := f.pool.Config().Copy()
	config.ConnConfig.Tracer = tracer
	reads, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reads.Close)
	h := app.NewCreateCabalHandler(app.CreateCabalDeps{
		UoW: f.uow, Reads: reads, Wallets: adapters.AppWallets{Client: walletStub{
			wallet: chain.Wallet{ID: "wallet-race", Address: "race-address"},
		}}, IDs: f.ids, Clock: f.clock,
	})
	got, err := h.Handle(f.actor(t.Context()), f.command(t, "c1"))
	cabals := f.count(t.Context(), t, "cabals")
	wallets := f.count(t.Context(), t, "treasury_wallets")
	events := f.count(t.Context(), t, "events")
	if err != nil || got.ID != seeded.ID || cabals != 1 || wallets != 1 || events != 0 {
		t.Fatalf("Handle = %+v, %v; seeded %s cabals %d wallets %d events %d",
			got, err, seeded.ID, cabals, wallets, events)
	}
}

func TestCreateCabal_wrapsACommittedWalletLookupFailure(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE treasury_wallets RENAME TO wallets_gone`); err != nil {
		t.Fatal(err)
	}
	_, err := f.handler().Handle(f.actor(t.Context()), f.command(t, "c1"))
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Handle = %v, want internal", err)
	}
}

func TestCreateCabal_wrapsACommittedCabalLookupFailure(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	if _, err := f.handler().Handle(f.actor(t.Context()), f.command(t, "c1")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE cabals RENAME TO cabals_gone`); err != nil {
		t.Fatal(err)
	}
	_, err := f.handler().Handle(f.actor(t.Context()), f.command(t, "c1"))
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Handle = %v, want internal", err)
	}
}

func TestCreateCabal_rollsBackWhenTheTreasuryWalletWriteFails(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	statement := `ALTER TABLE treasury_wallets ADD CHECK (address <> 'forbidden')`
	if _, err := f.pool.Exec(t.Context(), statement); err != nil {
		t.Fatal(err)
	}
	h := app.NewCreateCabalHandler(app.CreateCabalDeps{
		UoW: f.uow, Reads: f.pool, Wallets: adapters.AppWallets{Client: walletStub{
			wallet: chain.Wallet{ID: "wallet-forbidden", Address: "forbidden"},
		}}, IDs: f.ids, Clock: f.clock,
	})
	_, err := h.Handle(f.actor(t.Context()), f.command(t, "c1"))
	if errs.CodeOf(err) != errs.CodeInternal || f.count(t.Context(), t, "cabals") != 0 {
		t.Fatalf("Handle = %v, cabals %d; want internal and no cabal", err, f.count(t.Context(), t, "cabals"))
	}
}

func TestCreateCabal_refusesAnEmptyIdempotencyKeyBeforeCreatingAWallet(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	_, err := f.handler().Handle(f.actor(t.Context()), f.command(t, ""))
	creates, cabals := f.wallets.Creates(), f.count(t.Context(), t, "cabals")
	if errs.CodeOf(err) != errs.CodeInvalidInput || creates != 0 || cabals != 0 {
		t.Fatalf("Handle = %v creates %d cabals %d", err, creates, cabals)
	}
}

func TestCreateCabal_validatesBeforeLookingUpACommittedWallet(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	cmd := f.command(t, "c1")
	cmd.Name = domain.Name{}
	_, err := f.handler().Handle(f.actor(t.Context()), cmd)
	if errs.CodeOf(err) != errs.CodeInvalidInput || f.wallets.Creates() != 0 || f.count(t.Context(), t, "cabals") != 0 {
		t.Fatalf("Handle = %v creates %d cabals %d", err, f.wallets.Creates(), f.count(t.Context(), t, "cabals"))
	}
}

func TestCreateCabal_validatesRulesBeforeCreatingAWallet(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	cmd := f.command(t, "c1")
	cmd.Rules = domain.Rules{}
	_, err := f.handler().Handle(f.actor(t.Context()), cmd)
	if errs.CodeOf(err) != errs.CodeInvalidInput || f.wallets.Creates() != 0 || f.count(t.Context(), t, "cabals") != 0 {
		t.Fatalf("Handle = %v creates %d cabals %d", err, f.wallets.Creates(), f.count(t.Context(), t, "cabals"))
	}
}

func TestCreateCabal_aPrivyOutageWritesNothing(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	f.wallets.Fail("CreateAppWallet", errs.New(errs.CodePrivyUnavailable, "test"))
	_, err := f.handler().Handle(f.actor(t.Context()), f.command(t, "c1"))
	if errs.CodeOf(err) != errs.CodePrivyUnavailable || attrString(err, "privy_wallet_id") != "" ||
		f.count(t.Context(), t, "cabals") != 0 {
		t.Fatalf("Handle = %v, cabals %d; want privy_unavailable and no cabal", err, f.count(t.Context(), t, "cabals"))
	}
}

func TestCreateCabal_retriesOnceWhenTheInviteCodeIsTaken(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	takeInvite(t, f)
	f.random = &byteChunks{parts: [][]byte{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		{22, 23, 24, 25, 26, 27, 28, 29, 30, 31},
	}}
	got, err := f.handler().Handle(f.actor(t.Context()), f.command(t, "c1"))
	if err != nil || got.InviteCode != "PQRSTVWXYZ" || f.count(t.Context(), t, "cabals") != 2 {
		t.Fatalf("Handle = %+v, %v, cabals %d; want the second code", got, err, f.count(t.Context(), t, "cabals"))
	}
}

func TestCreateCabal_anInviteCodeThatClashesTwiceNamesTheOrphanWallet(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	takeInvite(t, f)
	f.random = &byteChunks{parts: [][]byte{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
	}}
	_, err := f.handler().Handle(f.actor(t.Context()), f.command(t, "c1"))
	same, _ := f.wallets.CreateAppWallet(t.Context(), app.TreasuryKey(f.user.ID, "c1"))
	wantFailure(t, err, same.ID, 1, f.count(t.Context(), t, "cabals"))
}

func TestCreateCabal_aSecondInviteDrawThatFailsNamesTheOrphanWallet(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	takeInvite(t, f)
	f.random = &byteChunks{parts: [][]byte{{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}}, err: io.ErrUnexpectedEOF}
	_, err := f.handler().Handle(f.actor(t.Context()), f.command(t, "c1"))
	same, _ := f.wallets.CreateAppWallet(t.Context(), app.TreasuryKey(f.user.ID, "c1"))
	wantFailure(t, err, same.ID, 1, f.count(t.Context(), t, "cabals"))
}

func TestCreateCabal_aFailedInviteDrawNamesTheOrphanWallet(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	f.random = &byteChunks{}
	_, err := f.handler().Handle(f.actor(t.Context()), f.command(t, "c1"))
	same, _ := f.wallets.CreateAppWallet(t.Context(), app.TreasuryKey(f.user.ID, "c1"))
	wantFailure(t, err, same.ID, 0, f.count(t.Context(), t, "cabals"))
}

func TestCreateCabal_aWriteWithoutAnActorNamesTheOrphanWallet(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	_, err := f.handler().Handle(t.Context(), f.command(t, "c1"))
	same, _ := f.wallets.CreateAppWallet(t.Context(), app.TreasuryKey(f.user.ID, "c1"))
	wantFailure(t, err, same.ID, 0, f.count(t.Context(), t, "cabals"))
}

func TestCreateCabal_aMemberInsertErrorNamesTheOrphanWallet(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE cabal_members RENAME TO cabal_members_gone`); err != nil {
		t.Fatal(err)
	}
	_, err := f.handler().Handle(f.actor(t.Context()), f.command(t, "c1"))
	same, _ := f.wallets.CreateAppWallet(t.Context(), app.TreasuryKey(f.user.ID, "c1"))
	wantFailure(t, err, same.ID, 0, f.count(t.Context(), t, "cabals"))
}

func TestAppWallets_returnsTheWalletAndWrapsOnlyAnOutage(t *testing.T) {
	t.Parallel()
	ok := adapters.AppWallets{Client: walletStub{wallet: chain.Wallet{ID: "wallet-1", Address: "addr"}}}
	id, addr, err := ok.CreateAppOwned(t.Context(), "k")
	if err != nil || id != "wallet-1" || addr != "addr" {
		t.Fatalf("CreateAppOwned = %s, %s, %v", id, addr, err)
	}
	down := adapters.AppWallets{Client: walletStub{err: errs.New(errs.CodeUpstreamUnavailable, "up")}}
	if _, _, err := down.CreateAppOwned(t.Context(), "k"); errs.CodeOf(err) != errs.CodePrivyUnavailable {
		t.Fatalf("outage = %v, want privy_unavailable", err)
	}
	other := adapters.AppWallets{Client: walletStub{err: errs.New(errs.CodeInvalidInput, "up")}}
	if _, _, err := other.CreateAppOwned(t.Context(), "k"); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("other = %v, want invalid_input", err)
	}
}

func TestModule_createUsesTheInjectedWalletPort(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	wallets := adapters.AppWallets{Client: f.wallets}
	m := cabal.New(
		module.Deps{Pool: f.pool, UoW: f.uow, IDs: f.ids, Clock: f.clock},
		cabal.WithTreasuryWallets(wallets),
	)
	got, err := m.CreateCabalHandler().Handle(f.actor(t.Context()), f.command(t, "c1"))
	if err != nil || got.PrivyWalletID == "" || f.wallets.Creates() != 1 {
		t.Fatalf("Handle = %+v, %v, creates %d", got, err, f.wallets.Creates())
	}
}

func TestModule_buildsAPrivyClientWhenNoWalletPortIsInjected(t *testing.T) {
	t.Parallel()
	deps := module.Deps{Clock: clock.Real{}, Config: config.Config{
		Privy:    config.Privy{BaseURL: "http://127.0.0.1", VerificationKey: fakes.PrivyVerificationKey()},
		Timeouts: config.Timeouts{Privy: time.Second},
	}}
	if cabal.New(deps).CreateCabalHandler() == nil {
		t.Fatal("CreateCabalHandler = nil")
	}
}

func TestModule_createPanicsWithoutAPrivyVerificationKey(t *testing.T) {
	t.Parallel()
	defer func() {
		err, ok := recover().(error)
		if !ok || errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("CreateCabalHandler panicked with %v, want invalid_input from privy.New", err)
		}
	}()
	cabal.New(module.Deps{Clock: clock.Real{}}).CreateCabalHandler()
	t.Fatal("CreateCabalHandler did not panic")
}
