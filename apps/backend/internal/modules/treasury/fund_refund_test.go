package treasury_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type refundStubs struct {
	mu         sync.Mutex
	h          *settleHarness
	built      []relayer.TransferSpec
	broadcasts []relayer.SignedTx
	balance    uint64
	priceable  bool
	finalize   bool
	buildErr   error
	onBuild    func()
	nextStatus *solana.Status
	potErr     error
	statusErr  error
	statusRows int
	balErr     error
	factoryErr error
	treasuryAt error
	memberErr  error
	sendErr    error
	onStatuses func()
}

func (s *refundStubs) PotValue(context.Context, ids.CabalID) (money.Micros, error) {
	if s.potErr != nil {
		return money.Micros{}, s.potErr
	}
	if s.priceable {
		return money.MicrosFromUint64(1), nil
	}
	return money.Micros{}, errs.New(errs.CodePriceUnavailable, "stub.PotValue")
}

func (s *refundStubs) SignatureStatuses(ctx context.Context, sigs []chain.Signature) ([]solana.Status, error) {
	if s.onStatuses != nil {
		s.onStatuses()
	}
	if s.statusErr != nil {
		return nil, s.statusErr
	}
	if s.statusRows != 0 {
		return make([]solana.Status, s.statusRows), nil
	}
	if s.nextStatus != nil {
		st := *s.nextStatus
		st.Signature = sigs[0]
		return []solana.Status{st}, nil
	}
	return s.h.chain.SignatureStatuses(ctx, sigs)
}

func (s *refundStubs) Build(_ context.Context, spec relayer.TransferSpec) (relayer.SignedTx, error) {
	if s.onBuild != nil {
		s.onBuild()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.buildErr != nil {
		return relayer.SignedTx{}, s.buildErr
	}
	s.built = append(s.built, spec)
	sig := chain.Signature("refund-" + uuid.NewString())
	if s.finalize {
		s.h.finalize(sig)
	}
	return relayer.SignedTx{Bytes: []byte(sig), Signature: sig, LastValidBlockHeight: 900}, nil
}

func (s *refundStubs) Broadcast(_ context.Context, tx relayer.SignedTx) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.broadcasts = append(s.broadcasts, tx)
	return s.sendErr
}

func (s *refundStubs) TokenBalance(context.Context, chain.SolanaAddress, chain.Mint) (money.BaseUnits, error) {
	return money.NewBaseUnits(s.balance, 6), s.balErr
}

type refundWallets struct{ s *refundStubs }

func (w refundWallets) TreasuryWallet(context.Context, ids.CabalID) (chain.Wallet, error) {
	return chain.Wallet{ID: "treasury-wallet", Address: treasuryAddress}, w.s.treasuryAt
}

func (w refundWallets) MemberAddress(context.Context, ids.UserID) (chain.SolanaAddress, error) {
	return memberAddress, w.s.memberErr
}

type refundHarness struct {
	*settleHarness
	stubs   *refundStubs
	refunds *app.FundRefunds
}

func newRefundHarness(t *testing.T) *refundHarness {
	t.Helper()
	sh := newSettleHarness(t)
	s := &refundStubs{h: sh, balance: 1_000_000_000, finalize: true}
	h := &refundHarness{settleHarness: sh, stubs: s}
	h.refunds = h.refundsWith(func(*app.FundRefundDeps) {})
	return h
}

func (h *refundHarness) refundsWith(change func(*app.FundRefundDeps)) *app.FundRefunds {
	s := h.stubs
	d := app.FundRefundDeps{
		Reads: h.pool, UoW: h.uow, IDs: h.ids, Clock: h.clock, Ledger: h.ledger, Pot: s, Chain: s,
		Transfers: func() (app.PayoutTransfers, error) { return s, s.factoryErr }, Wallets: refundWallets{s: s},
		Balances: s, USDC: chain.Mint{Address: chain.SolanaAddress(h.cfg.Solana.USDCMint), Decimals: 6},
		Hints: h.chain, Poll: time.Millisecond,
	}
	change(&d)
	return app.NewFundRefunds(d)
}

func (h *refundHarness) refund(id uuid.UUID) (app.FundRefundResult, error) {
	return h.refunds.Refund(h.systemCtx(), app.RefundFundTransfer{ID: id, Actor: "operator:test", Reason: "stuck"})
}

type refundRow struct {
	status, signature string
	refundedAt        bool
}

func (h *refundHarness) row(t *testing.T, id uuid.UUID) refundRow {
	t.Helper()
	var r refundRow
	if err := h.pool.QueryRow(t.Context(),
		`SELECT status, coalesce(refund_signature, ''), refunded_at IS NOT NULL FROM fund_transfers WHERE id = $1`, id,
	).Scan(&r.status, &r.signature, &r.refundedAt); err != nil {
		t.Fatal(err)
	}
	return r
}

func (h *refundHarness) assertUntouched(t *testing.T, id uuid.UUID) {
	t.Helper()
	if got := h.row(t, id); got.status != "landed" || got.signature != "" {
		t.Fatalf("transfer = %+v, want landed with no refund stored", got)
	}
	if len(h.stubs.built) != 0 || len(h.stubs.broadcasts) != 0 || h.count(t, "user_txns") != 0 ||
		h.events(t, "cabal.fund_refunded") != 0 {
		t.Fatalf("built %d, broadcast %d, user_txns %d, want nothing",
			len(h.stubs.built), len(h.stubs.broadcasts), h.count(t, "user_txns"))
	}
}

func TestFlow40_RefundFundTransfer_OK(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	id := h.landed(t, "5000000")
	got, err := h.refund(id)
	if err != nil || got.State != app.FundRefundDone {
		t.Fatalf("Refund = %+v, %v, want refunded", got, err)
	}
	h.assertSent(t, id)
	h.assertBooked(t, id)
	if drift := h.drift(t); len(drift) != 0 {
		t.Fatalf("ledger drift = %v", drift)
	}
}

func (h *refundHarness) assertSent(t *testing.T, id uuid.UUID) {
	t.Helper()
	if len(h.stubs.built) != 1 {
		t.Fatalf("built %d transfers, want 1", len(h.stubs.built))
	}
	spec := h.stubs.built[0]
	if spec.FromWallet.Address != treasuryAddress || spec.To != memberAddress || spec.Amount.Uint64() != 5_000_000 ||
		spec.Amount.Decimals() != 6 || !spec.SkipCreate {
		t.Fatalf("spec = %+v, want 5000000 from the treasury to the member", spec)
	}
	row := h.row(t, id)
	if row.status != "refunded" || !row.refundedAt || row.signature != string(h.stubs.broadcasts[0].Signature) {
		t.Fatalf("row = %+v, want refunded with the broadcast signature", row)
	}
}

func (h *refundHarness) assertBooked(t *testing.T, id uuid.UUID) {
	t.Helper()
	if h.events(t, "cabal.fund_refunded") != 1 || h.count(t, "user_txns") != 1 || h.count(t, "cabal_txns") != 0 {
		t.Fatalf("events %d, user_txns %d, cabal_txns %d, want 1, 1, 0",
			h.events(t, "cabal.fund_refunded"), h.count(t, "user_txns"), h.count(t, "cabal_txns"))
	}
	var status, kind string
	var entries int
	if err := h.pool.QueryRow(t.Context(), `SELECT t.status, t.kind, count(e.*) FROM user_txns t
		JOIN user_txn_entries e ON e.txn_id = t.id WHERE t.transfer_id = $1 GROUP BY t.status, t.kind`, id).
		Scan(&status, &kind, &entries); err != nil || status != "failed" || kind != "fund" || entries != 4 {
		t.Fatalf("ledger txn = %s %s %d entries (%v), want a failed fund with 4 entries", kind, status, entries, err)
	}
}

func TestFundRefund_refundingTwiceMovesNothing(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	id := h.landed(t, "5000000")
	if _, err := h.refund(id); err != nil {
		t.Fatal(err)
	}
	again, err := h.refund(id)
	if err != nil || again.State != app.FundRefundAlready {
		t.Fatalf("second Refund = %+v, %v, want already_refunded", again, err)
	}
	plan, err := h.refunds.Plan(h.systemCtx(), id)
	if err != nil || !plan.Refunded {
		t.Fatalf("Plan after refund = %+v, %v, want refunded", plan, err)
	}
	if len(h.stubs.built) != 1 || len(h.stubs.broadcasts) != 1 || h.events(t, "cabal.fund_refunded") != 1 ||
		h.count(t, "user_txns") != 1 {
		t.Fatalf("built %d, broadcast %d, events %d, user_txns %d, want 1 each",
			len(h.stubs.built), len(h.stubs.broadcasts), h.events(t, "cabal.fund_refunded"), h.count(t, "user_txns"))
	}
}

func TestFundRefund_guardedUpdatesChangeOneRowOnce(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	id := h.landed(t, "5000000")
	q, ctx := sqlc.New(h.pool), h.ctx()
	store := sqlc.StoreFundRefundParams{
		ID: id, SignedTx: []byte{1}, Signature: "sig-a", LastValidBlockHeight: "10", SubmittedAt: h.clock.Now(),
	}
	if n, err := q.StoreFundRefund(ctx, store); err != nil || n != 1 {
		t.Fatalf("first store = %d, %v, want 1", n, err)
	}
	if n, err := q.StoreFundRefund(ctx, store); err != nil || n != 0 {
		t.Fatalf("second store = %d, %v, want 0", n, err)
	}
	done := sqlc.RefundFundTransferParams{ID: id, Signature: "sig-b", RefundedAt: h.clock.Now()}
	if n, err := q.RefundFundTransfer(ctx, done); err != nil || n != 0 {
		t.Fatalf("refund with another signature = %d, %v, want 0", n, err)
	}
	done.Signature = "sig-a"
	for i, want := range []int64{1, 0} {
		if n, err := q.RefundFundTransfer(ctx, done); err != nil || n != want {
			t.Fatalf("refund %d = %d, %v, want %d", i, n, err, want)
		}
	}
	units := sqlc.SettleFundTransferParams{ID: id, ShareUnits: "5", SettledAt: h.clock.Now()}
	if n, err := q.SettleFundTransfer(ctx, units); err != nil || n != 0 {
		t.Fatalf("settle after refund = %d, %v, want 0", n, err)
	}
}

func TestFlow40_RefundFundTransfer_NotFound(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	_, err := h.refund(h.ids.NewV7())
	wantCode(t, err, errs.CodeNotFound)
	_, err = h.refunds.Plan(h.systemCtx(), h.ids.NewV7())
	wantCode(t, err, errs.CodeNotFound)
}

func TestFlow40_RefundFundTransfer_FundRefundNotLanded(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	for _, status := range []domain.FundStatus{domain.FundSubmitted, domain.FundSettled, domain.FundFailed} {
		id := h.fundTransferIn(t, h.user, status, 5_000_000)
		_, err := h.refund(id)
		wantCode(t, err, errs.CodeFundRefundNotLanded)
		if len(h.stubs.built) != 0 {
			t.Fatalf("%s transfer built %d refunds, want none", status, len(h.stubs.built))
		}
	}
}

func TestFlow40_RefundFundTransfer_FundRefundPriceable(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	h.stubs.priceable = true
	id := h.landed(t, "5000000")
	_, err := h.refund(id)
	wantCode(t, err, errs.CodeFundRefundPriceable)
	h.assertUntouched(t, id)
}

func TestFlow40_RefundFundTransfer_FundRefundTreasuryLow(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	h.stubs.balance = 4_999_999
	id := h.landed(t, "5000000")
	_, err := h.refund(id)
	wantCode(t, err, errs.CodeFundRefundTreasuryLow)
	h.assertUntouched(t, id)
}

func TestFlow40_RefundFundTransfer_FundRefundRejected(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	h.stubs.finalize = false
	id := h.landed(t, "5000000")
	h.stubs.nextStatus = &solana.Status{State: solana.StateFinalized, Failed: true}
	_, err := h.refund(id)
	wantCode(t, err, errs.CodeFundRefundRejected)
	if got := h.row(t, id); got.status != "landed" || got.signature != "" {
		t.Fatalf("row = %+v, want landed with the rejected signature cleared", got)
	}
	h.stubs.nextStatus, h.stubs.finalize = nil, true
	if got, err := h.refund(id); err != nil || got.State != app.FundRefundDone || len(h.stubs.built) != 2 {
		t.Fatalf("retry = %+v, %v after %d builds, want refunded by a second signature", got, err, len(h.stubs.built))
	}
}

func TestFlow40_RefundFundTransfer_FundRefundExpired(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	h.stubs.finalize = false
	id := h.landed(t, "5000000")
	h.stubs.nextStatus = &solana.Status{State: solana.StateNotFound, BlockHeight: 901}
	_, err := h.refund(id)
	wantCode(t, err, errs.CodeFundRefundExpired)
	if got := h.row(t, id); got.status != "landed" || got.signature != "" {
		t.Fatalf("row = %+v, want landed with the lapsed signature cleared", got)
	}
}

func TestFundRefund_resumesTheStoredSignatureWithoutSigningAgain(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	h.stubs.finalize = false
	id := h.landed(t, "5000000")
	got, err := h.refund(id)
	if err != nil || got.State != app.FundRefundPending {
		t.Fatalf("first Refund = %+v, %v, want pending", got, err)
	}
	stored := h.row(t, id).signature
	h.stubs.priceable, h.stubs.balance = true, 0
	h.finalize(chain.Signature(stored))
	got, err = h.refund(id)
	if err != nil || got.State != app.FundRefundDone || len(h.stubs.built) != 1 || h.row(t, id).signature != stored {
		t.Fatalf("resume = %+v, %v after %d builds, want refunded by %s", got, err, len(h.stubs.built), stored)
	}
	if len(h.stubs.broadcasts) != 2 || h.stubs.broadcasts[1].Signature != chain.Signature(stored) {
		t.Fatalf("broadcasts = %d, want the stored transaction sent again", len(h.stubs.broadcasts))
	}
}

func TestFundRefund_aPendingRefundBlocksTheSettler(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	h.stubs.finalize = false
	id := h.landed(t, "5000000")
	if _, err := h.refund(id); err != nil {
		t.Fatal(err)
	}
	pot := money.MicrosFromUint64(5_000_000)
	h.chain.pot = &pot
	h.tick(t)
	if got := h.row(t, id); got.status != "landed" || got.signature == "" {
		t.Fatalf("row = %+v, want still landed with its refund", got)
	}
	if h.count(t, "user_txns") != 0 || h.events(t, "cabal.funded") != 0 {
		t.Fatalf("settler minted against a transfer being refunded")
	}
}

func TestFundRefund_aTransferThatSettlesWhileSigningIsNotRefunded(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	id := h.landed(t, "5000000")
	h.stubs.onBuild = func() {
		pot := money.MicrosFromUint64(5_000_000)
		h.chain.pot = &pot
		h.tick(t)
	}
	_, err := h.refund(id)
	wantCode(t, err, errs.CodeFundRefundNotLanded)
	if got := h.row(t, id); got.status != "settled" || got.signature != "" || len(h.stubs.broadcasts) != 0 {
		t.Fatalf("row = %+v, broadcasts %d, want settled and nothing sent", got, len(h.stubs.broadcasts))
	}
}

func TestFundRefund_planReportsTheMoveWithoutMakingIt(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	id := h.landed(t, "5000000")
	plan, err := h.refunds.Plan(h.systemCtx(), id)
	if err != nil || plan.Amount.String() != "5000000" || plan.From != treasuryAddress || plan.To != memberAddress ||
		plan.Treasury.String() != "1000000000" {
		t.Fatalf("Plan = %+v, %v, want 5000000 from the treasury to the member", plan, err)
	}
	h.assertUntouched(t, id)
}

func TestFundRefundProperty_refundsExactlyTheLandedAmount(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	rapid.Check(t, func(rt *rapid.T) {
		h.user, h.cabal = h.fixture.user(t), h.fixture.cabal(t)
		micros := rapid.Uint64Range(domain.MinFundMicros, 900_000_000).Draw(rt, "micros")
		h.stubs.built, h.stubs.balance = nil, micros+rapid.Uint64Range(0, 5_000_000).Draw(rt, "spare")
		id := h.landed(t, strconv.FormatUint(micros, 10))
		for range rapid.IntRange(1, 3).Draw(rt, "runs") {
			if _, err := h.refund(id); err != nil {
				rt.Fatal(err)
			}
		}
		var event string
		if err := h.pool.QueryRow(t.Context(),
			`SELECT payload->>'amount_micros' FROM events WHERE type = 'cabal.fund_refunded' AND aggregate_id = $1`, id,
		).Scan(&event); err != nil {
			rt.Fatal(err)
		}
		var net int64
		if err := h.pool.QueryRow(t.Context(), `SELECT coalesce(sum(e.amount), 0) FROM user_txns t
			JOIN user_txn_entries e ON e.txn_id = t.id WHERE t.transfer_id = $1 AND e.account = 'wallet'`, id).
			Scan(&net); err != nil {
			rt.Fatal(err)
		}
		if len(h.stubs.built) != 1 || h.stubs.built[0].Amount.Uint64() != micros ||
			event != strconv.FormatUint(micros, 10) ||
			net != 0 {
			rt.Fatalf("built %d at %v, event %s, wallet net %d, want one transfer of %d", len(h.stubs.built),
				h.stubs.built, event, net, micros)
		}
		if drift := h.drift(h.t); len(drift) != 0 {
			rt.Fatalf("ledger drift = %v", drift)
		}
	})
}

func TestFundRefund_theHistoryShowsOneFailedFundAfterTheRefund(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	id := h.landed(t, "5000000")
	list := func() []sqlc.ListUserTxnsRow {
		rows, err := sqlc.New(h.pool).ListUserTxns(h.ctx(), sqlc.ListUserTxnsParams{
			RowLimit: 20, UserID: h.user.UUID(), UsdcAsset: string(h.usdc()),
		})
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	if rows := list(); len(rows) != 1 || rows[0].Status != "pending" || rows[0].ID != id {
		t.Fatalf("before the refund: %+v, want the landed transfer as pending", rows)
	}
	if _, err := h.refund(id); err != nil {
		t.Fatal(err)
	}
	rows := list()
	if len(rows) != 1 || rows[0].Status != "failed" || rows[0].ID != id || rows[0].Amount != -5_000_000 {
		t.Fatalf("after the refund: %+v, want the one transfer as failed at -5000000", rows)
	}
}
