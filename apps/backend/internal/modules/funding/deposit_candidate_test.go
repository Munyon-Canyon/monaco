package funding_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/concurrency"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chaos"
)

type candidateRPC struct {
	transfers []solana.Transfer
	err       error
	observe   func(context.Context)
}

func (r candidateRPC) InboundTransfersForMint(
	ctx context.Context,
	_ chain.Signature,
	_ chain.SolanaAddress,
	_ chain.SolanaAddress,
) ([]solana.Transfer, error) {
	if r.observe != nil {
		r.observe(ctx)
	}
	return r.transfers, r.err
}

type candidateOwner struct {
	owned   bool
	err     error
	observe func(context.Context)
}

func (o candidateOwner) OwnsSignature(ctx context.Context, _ chain.Signature) (bool, error) {
	if o.observe != nil {
		o.observe(ctx)
	}
	return o.owned, o.err
}

type candidateFixture struct {
	pool *pgxpool.Pool
	uow  *db.UnitOfWork
	ids  *testkit.IDs
	now  time.Time
	user testkit.SeededUser
}

func newCandidateFixture(t *testing.T) candidateFixture {
	t.Helper()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	return candidateFixture{
		pool: pool, uow: db.New(pool, testkit.NewIDs(300), testkit.NewClock(now)), ids: testkit.NewIDs(301), now: now,
		user: testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true}),
	}
}

func (f candidateFixture) candidate() app.DepositCandidate {
	return app.DepositCandidate{
		Signature: chain.Signature(depositSignature), Wallet: f.user.Address, UserID: f.user.ID,
		Slot: 42, BlockTime: f.now.Add(-time.Minute), Source: "poller",
	}
}

func (f candidateFixture) record(t *testing.T, candidates ...app.DepositCandidate) int {
	t.Helper()
	var inserted int
	ctx := observability.WithActor(t.Context(), "system:funding.deposits")
	err := f.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		inserted, err = app.NewCandidateRecorder(f.ids, func() time.Time { return f.now }).Record(ctx, tx, candidates)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return inserted
}

func (f candidateFixture) event() events.DepositCandidateSeen {
	return events.DepositCandidateSeen{
		V: 1, CandidateID: f.ids.NewV7(), UserID: f.user.ID.UUID(), WalletAddress: f.user.Address,
		TxSignature: depositSignature, Slot: 42, BlockTime: ptr(f.now.Add(-time.Minute)), Source: "poller",
	}
}

func (f candidateFixture) applyContext(
	ctx context.Context,
	resolver app.DepositCandidateResolver,
	e events.DepositCandidateSeen,
	resolution app.DepositCandidateResolution,
) error {
	return f.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return resolver.Apply(ctx, tx, e, resolution, f.now)
	})
}

func candidateResolver(
	f candidateFixture,
	rpc candidateRPC,
	owners ...app.SignatureOwner,
) app.DepositCandidateResolver {
	return app.NewDepositCandidateResolver(
		rpc, testkit.USDCMint, owners,
		app.NewCreditDepositHandler(f.uow, &hints{}), f.ids,
	)
}

type candidateDelivery struct {
	*chaos.Msg
	seen    uint64
	outcome bus.Outcome
}

func (m *candidateDelivery) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{NumDelivered: m.seen}, nil
}

func (m *candidateDelivery) Ack() error                      { m.outcome = bus.OutcomeAck; return nil }
func (m *candidateDelivery) DoubleAck(context.Context) error { return m.Ack() }
func (m *candidateDelivery) Nak() error                      { m.outcome = bus.OutcomeNak; return nil }

func (m *candidateDelivery) Term() error                 { m.outcome = bus.OutcomeTerm; return nil }
func (m *candidateDelivery) TermWithReason(string) error { return m.Term() }
func (m *candidateDelivery) NakWithDelay(time.Duration) error {
	m.outcome = bus.OutcomeNak
	return nil
}

type candidateDispatch struct {
	f    candidateFixture
	conn *bus.Conn
	reg  *bus.Registry
}

func newCandidateDispatch(
	t *testing.T,
	f candidateFixture,
	rpc candidateRPC,
	owners ...app.SignatureOwner,
) candidateDispatch {
	t.Helper()
	conn := testkit.NATS(t).Conn
	resolver := candidateResolver(f, rpc, owners...)
	consumer := bus.Consumer{Durable: "funding", Handlers: []bus.HandlerSpec{
		bus.HandleFetched("funding.resolve_deposit_candidate", resolver.Fetch, resolver.Apply),
	}}
	reg, err := bus.NewRegistry(conn, f.uow, testkit.NewClock(f.now), []bus.Consumer{consumer})
	if err != nil {
		t.Fatal(err)
	}
	return candidateDispatch{f: f, conn: conn, reg: reg}
}

func (d candidateDispatch) deliver(t *testing.T, candidate app.DepositCandidate) bus.Outcome {
	t.Helper()
	d.f.record(t, candidate)
	var id uuid.UUID
	var payload []byte
	if err := d.f.pool.QueryRow(
		t.Context(),
		`SELECT id, payload FROM events WHERE type = $1 ORDER BY id DESC LIMIT 1`,
		events.TypeDepositCandidateSeen,
	).Scan(&id, &payload); err != nil {
		t.Fatal(err)
	}
	m := &candidateDelivery{
		Msg:  chaos.NewMsg(d.conn, events.TypeDepositCandidateSeen, ids.EventIDFrom(id), payload),
		seen: 1,
	}
	d.reg.Dispatch(observability.WithActor(t.Context(), "system:worker"), "funding", m)
	return m.outcome
}

func (f candidateFixture) candidateStatus(t *testing.T, signature chain.Signature) string {
	t.Helper()
	var status string
	if err := f.pool.QueryRow(
		t.Context(), `SELECT status FROM deposit_candidates WHERE tx_signature = $1`, signature,
	).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestCandidateRecorder_recordsOnlyNewCandidates(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	if got := f.record(t, f.candidate()); got != 1 {
		t.Fatalf("first Record = %d, want 1", got)
	}
	if got := f.record(t, f.candidate()); got != 0 {
		t.Fatalf("duplicate Record = %d, want 0", got)
	}
	var candidates, eventsCount int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM deposit_candidates`).Scan(&candidates); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE type = $1`, events.TypeDepositCandidateSeen).
		Scan(&eventsCount); err != nil {
		t.Fatal(err)
	}
	if candidates != 1 || eventsCount != 1 {
		t.Fatalf("candidates=%d events=%d, want 1 1", candidates, eventsCount)
	}
	if err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		missingActor := f.candidate()
		missingActor.Signature = "missing-actor"
		_, err := app.NewCandidateRecorder(f.ids, func() time.Time { return f.now }).
			Record(ctx, tx, []app.DepositCandidate{missingActor})
		return err
	}); err == nil {
		t.Fatal("Record without actor error = nil")
	}
	var tx db.Tx
	if _, err := app.NewCreditDepositHandler(f.uow, &hints{}).Apply(t.Context(), tx, app.CreditDeposit{}); err == nil {
		t.Fatal("Apply zero amount error = nil")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := f.uow.Do(t.Context(), func(_ context.Context, tx db.Tx) error {
		_, err := app.NewCandidateRecorder(f.ids, func() time.Time { return f.now }).
			Record(canceled, tx, []app.DepositCandidate{f.candidate()})
		return err
	}); err == nil {
		t.Fatal("Record with cancelled context error = nil")
	}
}

func TestDepositCandidateResolver_creditsAndGuardsResolution(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	f.record(t, f.candidate())
	e := f.event()
	e.BlockTime = nil
	resolver := candidateResolver(f, candidateRPC{transfers: []solana.Transfer{{
		Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6}, Net: money.NewBaseUnits(25_000_000, 6),
	}}})
	resolution, err := resolver.Fetch(t.Context(), e)
	if err != nil || resolution.Amount != money.MicrosFromUint64(25_000_000) || resolution.Ours {
		t.Fatalf("Fetch = %+v, %v", resolution, err)
	}
	if err := f.applyContext(t.Context(), resolver, e, resolution); err != nil {
		t.Fatal(err)
	}
	if err := f.applyContext(t.Context(), resolver, e, resolution); err != nil {
		t.Fatal(err)
	}
	var status string
	var deposits, dismissed int
	if err := f.pool.QueryRow(t.Context(), `SELECT status FROM deposit_candidates`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM deposits`).Scan(&deposits); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE type = $1`, events.TypeDepositCandidateDismissed).
		Scan(&dismissed); err != nil {
		t.Fatal(err)
	}
	if status != "credited" || deposits != 1 || dismissed != 0 {
		t.Fatalf("status=%q deposits=%d dismissed=%d, want credited 1 0", status, deposits, dismissed)
	}
}

func TestDepositCandidateFixtureI3_DuplicatePollerAndRecoveryCreditOnce(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	d := newCandidateDispatch(t, f, candidateRPC{transfers: []solana.Transfer{{
		Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6}, Net: money.NewBaseUnits(1, 6),
	}}})
	candidate := f.candidate()
	f.record(t, candidate)
	recovery := candidate
	recovery.Source = "recovery"
	if got := f.record(t, recovery); got != 0 {
		t.Fatalf("recovery Record = %d, want 0", got)
	}
	if got := d.deliver(t, candidate); got != bus.OutcomeAck {
		t.Fatalf("first delivery = %q, want ack", got)
	}
	if got := d.deliver(t, candidate); got != bus.OutcomeAck {
		t.Fatalf("duplicate delivery = %q, want ack", got)
	}
	var deposits int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM deposits`).Scan(&deposits); err != nil {
		t.Fatal(err)
	}
	if deposits != 1 {
		t.Fatalf("deposits = %d, want 1", deposits)
	}
}

func TestDepositCandidateFixtureI6_PrunedHistoryNaksAndStaysPending(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	candidate := f.candidate()
	d := newCandidateDispatch(t, f, candidateRPC{err: errs.New(errs.CodeNotFound, "test.rpc")})
	if got := d.deliver(t, candidate); got != bus.OutcomeNak {
		t.Fatalf("delivery = %q, want nak", got)
	}
	if got := f.candidateStatus(t, candidate.Signature); got != "pending" {
		t.Fatalf("status = %q, want pending", got)
	}
}

func TestDepositCandidateFixtureI11_MissingMetadataTermsAndStaysPending(t *testing.T) {
	t.Parallel()
	testDepositCandidateTerm(t, errs.CodeDecodeFailed, "i11")
}

func TestDepositCandidateFixtureI14_UnsupportedVersionTermsAndStaysPending(t *testing.T) {
	t.Parallel()
	testDepositCandidateTerm(t, errs.CodeDecodeFailed, "i14")
}

func testDepositCandidateTerm(t *testing.T, code errs.Code, signature chain.Signature) {
	t.Helper()
	f := newCandidateFixture(t)
	candidate := f.candidate()
	candidate.Signature = signature
	d := newCandidateDispatch(t, f, candidateRPC{err: errs.New(code, "test.rpc")})
	if got := d.deliver(t, candidate); got != bus.OutcomeTerm {
		t.Fatalf("delivery = %q, want term", got)
	}
	if got := f.candidateStatus(t, candidate.Signature); got != "pending" {
		t.Fatalf("status = %q, want pending", got)
	}
}

func TestDepositCandidateFixtureI12_MintToDismissesAsNotDeposit(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	candidate := f.candidate()
	if got := newCandidateDispatch(t, f, candidateRPC{}).deliver(t, candidate); got != bus.OutcomeAck {
		t.Fatalf("delivery = %q, want ack", got)
	}
	if got := f.candidateStatus(t, candidate.Signature); got != "not_deposit" {
		t.Fatalf("status = %q, want not_deposit", got)
	}
}

func TestDepositCandidateFixtureI13_MonacoSignedDismissesAsOurs(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	candidate := f.candidate()
	d := newCandidateDispatch(t, f, candidateRPC{transfers: []solana.Transfer{{
		Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6}, Net: money.NewBaseUnits(1, 6),
	}}}, candidateOwner{owned: true})
	if got := d.deliver(t, candidate); got != bus.OutcomeAck {
		t.Fatalf("delivery = %q, want ack", got)
	}
	if got := f.candidateStatus(t, candidate.Signature); got != "ours" {
		t.Fatalf("status = %q, want ours", got)
	}
}

func TestDepositCandidateFixtureI15_SelfTransferDismissesAsNotDeposit(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	candidate := f.candidate()
	d := newCandidateDispatch(t, f, candidateRPC{transfers: []solana.Transfer{{
		Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6}, Net: money.NewBaseUnits(0, 6),
	}}})
	if got := d.deliver(t, candidate); got != bus.OutcomeAck {
		t.Fatalf("delivery = %q, want ack", got)
	}
	if got := f.candidateStatus(t, candidate.Signature); got != "not_deposit" {
		t.Fatalf("status = %q, want not_deposit", got)
	}
}

func TestDepositCandidateResolver_FetchesOutsideTheDeliveryTransaction(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	assertNoIdleTransaction := func(ctx context.Context) {
		t.Helper()
		var idle int
		if err := f.pool.QueryRow(
			ctx,
			`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND state LIKE 'idle in transaction%'`,
		).Scan(&idle); err != nil {
			t.Fatal(err)
		}
		if idle != 0 {
			t.Fatalf("idle transactions = %d, want 0", idle)
		}
	}
	candidate := f.candidate()
	d := newCandidateDispatch(
		t,
		f,
		candidateRPC{transfers: []solana.Transfer{{
			Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6}, Net: money.NewBaseUnits(1, 6),
		}}, observe: assertNoIdleTransaction},
		candidateOwner{observe: assertNoIdleTransaction},
	)
	if got := d.deliver(t, candidate); got != bus.OutcomeAck {
		t.Fatalf("delivery = %q, want ack", got)
	}
}

func TestDepositCandidateResolver_dismissesEmptyAndOwnedTransfers(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	testDepositCandidateDismissal(t, f, "empty", nil, candidateOwner{}, "not_deposit", 1)
	testDepositCandidateDismissal(t, f, "ours", []solana.Transfer{{
		Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6}, Net: money.NewBaseUnits(1, 6),
	}}, candidateOwner{owned: true}, "ours", 2)
}

func testDepositCandidateDismissal(
	t *testing.T,
	f candidateFixture,
	signature chain.Signature,
	transfers []solana.Transfer,
	owner candidateOwner,
	want string,
	wantDismissed int,
) {
	t.Helper()
	candidate := f.candidate()
	candidate.Signature = signature
	f.record(t, candidate)
	e := f.event()
	e.TxSignature = signature
	resolver := candidateResolver(f, candidateRPC{transfers: transfers}, owner)
	resolution, err := resolver.Fetch(t.Context(), e)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.applyContext(t.Context(), resolver, e, resolution); err != nil {
		t.Fatal(err)
	}
	var status string
	var dismissed int
	if err := f.pool.QueryRow(
		t.Context(), `SELECT status FROM deposit_candidates WHERE tx_signature = $1`, signature,
	).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE type = $1`, events.TypeDepositCandidateDismissed).
		Scan(&dismissed); err != nil {
		t.Fatal(err)
	}
	if status != want || dismissed != wantDismissed {
		t.Fatalf("status=%q dismissed=%d, want %q %d", status, dismissed, want, wantDismissed)
	}
}

func TestDepositCandidateResolver_fetchMapsNotFoundAndOwnerFailures(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	e := f.event()
	if _, err := candidateResolver(
		f,
		candidateRPC{err: errs.New(errs.CodeNotFound, "rpc")},
	).Fetch(t.Context(), e); errs.CodeOf(
		err,
	) != errs.CodeRPCUnavailable {
		t.Fatalf("not found code = %q, want %q", errs.CodeOf(err), errs.CodeRPCUnavailable)
	}
	if _, err := candidateResolver(
		f,
		candidateRPC{transfers: []solana.Transfer{{
			Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6}, Net: money.NewBaseUnits(1, 6),
		}}},
		candidateOwner{err: context.DeadlineExceeded},
	).Fetch(t.Context(), e); errs.CodeOf(
		err,
	) != errs.CodeDBUnavailable {
		t.Fatalf("owner failure code = %q, want %q", errs.CodeOf(err), errs.CodeDBUnavailable)
	}
	if _, err := candidateResolver(
		f,
		candidateRPC{err: errs.New(errs.CodeDecodeFailed, "rpc")},
	).Fetch(t.Context(), e); errs.CodeOf(
		err,
	) != errs.CodeDecodeFailed {
		t.Fatalf("decode failure code = %q, want %q", errs.CodeOf(err), errs.CodeDecodeFailed)
	}
	if _, err := candidateResolver(f, candidateRPC{transfers: []solana.Transfer{
		{Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6}, Net: money.NewBaseUnits(math.MaxUint64, 6)},
		{Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6}, Net: money.NewBaseUnits(1, 6)},
	}}).Fetch(t.Context(), e); err == nil {
		t.Fatal("overflow Fetch error = nil")
	}
	if resolution, err := candidateResolver(f, candidateRPC{transfers: []solana.Transfer{{
		Mint: chain.Mint{Address: "other", Decimals: 6}, Net: money.NewBaseUnits(1, 6),
	}}}).Fetch(t.Context(), e); err != nil || !resolution.Amount.IsZero() {
		t.Fatalf("unrelated transfer Fetch = %+v, %v", resolution, err)
	}
	if _, err := candidateResolver(f, candidateRPC{transfers: []solana.Transfer{{
		Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6}, Net: money.NewBaseUnits(1, 5),
	}}}).Fetch(t.Context(), e); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("invalid USDC decimals code = %q, want %q", errs.CodeOf(err), errs.CodeDecodeFailed)
	}
}

func TestDepositCandidateResolver_returnsWriteAndApplyErrors(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	f.record(t, f.candidate())
	resolver := candidateResolver(f, candidateRPC{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	err := f.uow.Do(t.Context(), func(_ context.Context, tx db.Tx) error {
		cancel()
		return resolver.Apply(ctx, tx, f.event(), app.DepositCandidateResolution{}, f.now)
	})
	if err == nil {
		t.Fatal("Apply with cancelled query context error = nil")
	}
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE events RENAME TO events_gone`); err != nil {
		t.Fatal(err)
	}
	err = f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return resolver.Apply(ctx, tx, f.event(), app.DepositCandidateResolution{}, f.now)
	})
	if err == nil {
		t.Fatal("dismissal Apply without actor error = nil")
	}
	err = f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return resolver.Apply(
			ctx, tx, f.event(), app.DepositCandidateResolution{Amount: money.MicrosFromUint64(1)}, f.now,
		)
	})
	if err == nil {
		t.Fatal("credit Apply without actor error = nil")
	}
	var status string
	if err := f.pool.QueryRow(t.Context(), `SELECT status FROM deposit_candidates`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("status after failed Apply = %q, want pending", status)
	}
}

func TestDepositCandidateResolver_twoRunnersCreditOnce(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	f.record(t, f.candidate())
	e := f.event()
	resolver := candidateResolver(f, candidateRPC{})
	resolution := app.DepositCandidateResolution{Amount: money.MicrosFromUint64(1)}
	_, err := concurrency.FanOut(t.Context(), 2, []int{1, 2}, func(ctx context.Context, _ int) (struct{}, error) {
		return struct{}{}, f.applyContext(ctx, resolver, e, resolution)
	})
	if err != nil {
		t.Fatal(err)
	}
	var deposits, credited int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM deposits`).Scan(&deposits); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE type = $1`, events.TypeDepositCredited).
		Scan(&credited); err != nil {
		t.Fatal(err)
	}
	if deposits != 1 || credited != 1 {
		t.Fatalf("deposits=%d credited=%d, want 1 1", deposits, credited)
	}
}

func ptr(value time.Time) *time.Time { return &value }
