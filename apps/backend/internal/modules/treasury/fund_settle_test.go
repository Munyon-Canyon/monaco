package treasury_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type settleStubs struct {
	mu           sync.Mutex
	statuses     map[chain.Signature]solana.Status
	statusErr    error
	broadcasts   []relayer.SignedTx
	broadcastErr error
	reads        *adapters.Queries
	pot          *money.Micros
	potErr       error
	onTotal      func()
	onStatuses   func()
	hints        []string
}

func (s *settleStubs) SignatureStatuses(_ context.Context, sigs []chain.Signature) ([]solana.Status, error) {
	if s.onStatuses != nil {
		s.onStatuses()
	}
	out := make([]solana.Status, len(sigs))
	for i, sig := range sigs {
		out[i] = solana.Status{Signature: sig, State: solana.StateProcessing}
		if st, ok := s.statuses[sig]; ok {
			out[i] = st
		}
	}
	return out, s.statusErr
}

func (s *settleStubs) Broadcast(_ context.Context, tx relayer.SignedTx) error {
	s.broadcasts = append(s.broadcasts, tx)
	return s.broadcastErr
}

func (s *settleStubs) PotValue(ctx context.Context, cabal ids.CabalID) (money.Micros, error) {
	if s.potErr != nil {
		return money.Micros{}, s.potErr
	}
	if s.pot != nil {
		return *s.pot, nil
	}
	return s.reads.PotValue(ctx, cabal)
}

func (s *settleStubs) TotalShares(ctx context.Context, cabal ids.CabalID) (money.SharesUnits, error) {
	total, err := s.reads.TotalShares(ctx, cabal)
	if s.onTotal != nil {
		s.onTotal()
	}
	return total, err
}

func (s *settleStubs) PublishHint(_ context.Context, key string, _ []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hints = append(s.hints, key)
}

type settleHarness struct {
	*fundHarness
	chain   *settleStubs
	settler *app.FundSettler
}

const fundSendWindow = 2 * time.Minute

func newSettleHarness(t *testing.T) *settleHarness {
	t.Helper()
	h := newFundHarness(t)
	s := &settleStubs{statuses: map[chain.Signature]solana.Status{}, reads: h.stubs.owner}
	settler := app.NewFundSettler(app.FundSettlerDeps{
		Reads: h.pool, UoW: h.uow, IDs: h.ids, Clock: h.clock, Chain: s, Transfers: s, Pot: s,
		Ledger: h.ledger, USDC: h.usdc(), Hints: s, SendWindow: fundSendWindow,
	})
	return &settleHarness{fundHarness: h, chain: s, settler: settler}
}

func (h *settleHarness) systemCtx() context.Context {
	return auth.WithActor(h.ctx(), auth.Actor{Kind: auth.ActorSystem, ID: "poller.treasury.fund-transfers"})
}

func (h *settleHarness) tick(t *testing.T) {
	t.Helper()
	if _, err := h.settler.Tick(h.systemCtx()); err != nil {
		t.Fatal(err)
	}
}

func (h *settleHarness) submitted(t *testing.T, micros uint64) (uuid.UUID, chain.Signature) {
	t.Helper()
	id, err := h.fund(micros)
	if err != nil {
		t.Fatal(err)
	}
	var sig string
	if err := h.pool.QueryRow(t.Context(), `SELECT tx_signature FROM fund_transfers WHERE id = $1`, id).
		Scan(&sig); err != nil {
		t.Fatal(err)
	}
	return id, chain.Signature(sig)
}

func (h *settleHarness) finalize(sig chain.Signature) {
	h.chain.statuses[sig] = solana.Status{Signature: sig, State: solana.StateFinalized}
}

func (h *settleHarness) status(t *testing.T, id uuid.UUID) (string, string, string) {
	t.Helper()
	row := h.transfer(t, id)
	return row.Status, row.ShareUnits, row.FailCode
}

func TestFundSettler_mintsOnceTheTransferIsFinal(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	id, sig := h.submitted(t, 5_000_000)
	h.tick(t)
	if status, _, _ := h.status(t, id); status != "submitted" {
		t.Fatalf("processing transfer status = %s, want submitted", status)
	}
	h.finalize(sig)
	h.tick(t)
	if status, units, _ := h.status(t, id); status != "settled" || units != "5000000" {
		t.Fatalf("status = %s, units = %s, want settled 5000000", status, units)
	}
	h.assertMinted(t, id)
	h.tick(t)
	if n := h.events(t, "cabal.funded"); n != 1 {
		t.Fatalf("cabal.funded after a second tick = %d, want 1", n)
	}
}

func (h *settleHarness) assertMinted(t *testing.T, id uuid.UUID) {
	t.Helper()
	shares, err := h.stubs.owner.ShareUnits(h.ctx(), h.cabal, h.user)
	if err != nil || shares != money.SharesUnitsFromUint64(5_000_000) {
		t.Fatalf("ShareUnits = %v, %v, want 5000000", shares, err)
	}
	var headers, settled int
	if err := h.pool.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM user_txns WHERE transfer_id = $1) + (SELECT count(*) FROM cabal_txns WHERE transfer_id = $1),
		(SELECT count(*) FROM user_txns WHERE transfer_id = $1 AND status = 'settled') +
		(SELECT count(*) FROM cabal_txns WHERE transfer_id = $1 AND status = 'settled')`, id).Scan(&headers, &settled); err != nil {
		t.Fatal(err)
	}
	if headers != 2 || settled != 2 || h.events(t, "cabal.funded") != 1 {
		t.Fatalf(
			"headers = %d, settled = %d, funded events = %d, want 2, 2, 1",
			headers,
			settled,
			h.events(t, "cabal.funded"),
		)
	}
	if drift := h.drift(t); len(drift) != 0 {
		t.Fatalf("ledger drift = %v", drift)
	}
	wantHints := []string{
		"user." + h.user.String() + ".balance_changed",
		events.CabalActivityChangedHint(h.cabal),
	}
	if len(h.chain.hints) != 2 || h.chain.hints[0] != wantHints[0] || h.chain.hints[1] != wantHints[1] {
		t.Fatalf("hints = %v, want %v", h.chain.hints, wantHints)
	}
}

func TestFund_productExample(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	h.stubs.onChain = money.MicrosFromUint64(200_000_000)
	alex, alexSig := h.submitted(t, 100_000_000)
	h.finalize(alexSig)
	h.tick(t)
	pot := money.MicrosFromUint64(110_000_000)
	h.chain.pot = &pot
	h.user = h.fixture.user(t)
	blair, blairSig := h.submitted(t, 110_000_000)
	h.finalize(blairSig)
	h.tick(t)
	for _, id := range []uuid.UUID{alex, blair} {
		if status, units, _ := h.status(t, id); status != "settled" || units != "100000000" {
			t.Fatalf("%s: status = %s, units = %s, want settled 100000000", id, status, units)
		}
	}
}

func TestFund_MintWaitsForPrice(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	id, sig := h.submitted(t, 5_000_000)
	h.finalize(sig)
	h.chain.potErr = errs.New(errs.CodePriceUnavailable, "stub")
	h.tick(t)
	if status, _, _ := h.status(t, id); status != "landed" {
		t.Fatalf("status without a price = %s, want landed", status)
	}
	h.chain.potErr = nil
	h.tick(t)
	if status, units, _ := h.status(t, id); status != "settled" || units != "5000000" {
		t.Fatalf("status once priced = %s/%s, want settled 5000000", status, units)
	}
}

func TestFundSettler_failsUnsentRejectedAndExpiredTransfers(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	rejected, rejectedSig := h.submitted(t, 5_000_000)
	expired, expiredSig := h.submitted(t, 5_000_000)
	h.chain.statuses[rejectedSig] = solana.Status{Signature: rejectedSig, State: solana.StateFinalized, Failed: true}
	h.chain.statuses[expiredSig] = solana.Status{Signature: expiredSig, State: solana.StateNotFound, BlockHeight: 901}
	unsent := h.fundTransferIn(t, h.user, domain.FundCreated, 5_000_000)
	h.tick(t)
	if status, _, _ := h.status(t, unsent); status != "created" {
		t.Fatalf("fresh created row = %s, want created", status)
	}
	h.clock.Advance(fundSendWindow + time.Second)
	h.tick(t)
	for id, want := range map[uuid.UUID]string{
		rejected: "fund_rejected", expired: "fund_expired", unsent: "fund_not_sent",
	} {
		if status, _, code := h.status(t, id); status != "failed" || code != want {
			t.Fatalf("%s = %s/%s, want failed/%s", id, status, code, want)
		}
	}
	if n := h.events(t, "cabal.fund_failed"); n != 2 {
		t.Fatalf("cabal.fund_failed = %d, want 2 (an unsent row never announced itself)", n)
	}
	if n := h.count(t, "user_txns"); n != 0 {
		t.Fatalf("user_txns = %d, want none for failed funds", n)
	}
}

func TestFundSettler_rebroadcastsTheStoredBytesWhileTheBlockhashLives(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	id, sig := h.submitted(t, 5_000_000)
	h.chain.statuses[sig] = solana.Status{Signature: sig, State: solana.StateNotFound, BlockHeight: 900}
	h.chain.broadcastErr = errs.New(errs.CodeRPCUnavailable, "stub")
	h.tick(t)
	if len(h.chain.broadcasts) != 1 || string(h.chain.broadcasts[0].Bytes) != string(sig) ||
		h.chain.broadcasts[0].Signature != sig {
		t.Fatalf("broadcasts = %+v, want the stored bytes for %s", h.chain.broadcasts, sig)
	}
	if status, _, _ := h.status(t, id); status != "submitted" {
		t.Fatalf("status = %s, want submitted", status)
	}
}

func (h *settleHarness) landed(t *testing.T, micros string) uuid.UUID {
	t.Helper()
	id := h.fundTransferIn(t, h.user, domain.FundLanded, 5_000_000)
	if _, err := h.pool.Exec(
		t.Context(),
		`UPDATE fund_transfers SET amount_micros = $2::numeric, cabal_id = $3 WHERE id = $1`,
		id,
		micros,
		h.cabal.UUID(),
	); err != nil {
		t.Fatal(err)
	}
	return id
}

func (h *settleHarness) tickErr() error {
	_, err := h.settler.Tick(h.systemCtx())
	return err
}

func TestFundSettler_keepsALandedTransferWhenItCannotMint(t *testing.T) {
	t.Parallel()
	zero, huge := money.Micros{}, money.MicrosFromUint64(1_000_000_000_000_000_000)
	tests := []struct {
		name   string
		micros string
		setup  func(*settleHarness)
		want   errs.Code
	}{
		{"amount past uint64", "99999999999999999999", func(*settleHarness) {}, errs.CodeInvalidInput},
		{
			"amount past the ledger's int64",
			"10000000000000000000",
			func(*settleHarness) {},
			errs.CodeInvalidInput,
		},
		{"pot read fails", "5000000", func(h *settleHarness) {
			h.chain.potErr = errs.New(errs.CodeInternal, "stub")
		}, errs.CodeInternal},
		{"shares but no pot", "5000000", func(h *settleHarness) {
			h.seedShares(10)
			h.chain.pot = &zero
		}, errs.CodePotValueZero},
		{"share price past uint64", "5000000", func(h *settleHarness) {
			h.seedShares(1)
			h.chain.pot = &huge
		}, errs.CodeInvalidInput},
		{"mint rounds to zero shares", "5000000", func(h *settleHarness) {
			h.seedShares(1)
			pot := money.MicrosFromUint64(1_000_000_000_000)
			h.chain.pot = &pot
		}, errs.CodeInternal},
		{"ledger already holds the user header", "5000000", func(h *settleHarness) {
			h.seedHeader("user_txns")
		}, errs.CodeInternal},
		{"ledger already holds the cabal header", "5000000", func(h *settleHarness) {
			h.seedHeader("cabal_txns")
		}, errs.CodeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newSettleHarness(t)
			id := h.landed(t, tt.micros)
			tt.setup(h)
			if err := h.tickErr(); err == nil || (tt.want != errs.CodeInternal && errs.CodeOf(err) != tt.want) {
				t.Fatalf("Tick = %v, want %s", err, tt.want)
			}
			if status, _, _ := h.status(t, id); status != "landed" || h.events(t, "cabal.funded") != 0 {
				t.Fatalf("status = %s, want landed with no cabal.funded", status)
			}
		})
	}
}

func (h *settleHarness) seedShares(units int64) {
	t := h.t
	t.Helper()
	u, c, err := h.fixture.fund(h.fixture.user(t), h.cabal, units, units, domain.TxnSettled)
	if err == nil {
		err = h.postPair(u, c)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func (h *settleHarness) seedHeader(table string) {
	t := h.t
	t.Helper()
	var id uuid.UUID
	if err := h.pool.QueryRow(t.Context(), `SELECT id FROM fund_transfers WHERE status = 'landed'`).
		Scan(&id); err != nil {
		t.Fatal(err)
	}
	sql := `INSERT INTO user_txns (id, user_id, cabal_id, kind, status, transfer_id, created_at)
		VALUES ($1, $2, $2, 'fund', 'pending', $3, now())`
	if table == "cabal_txns" {
		sql = `INSERT INTO cabal_txns (id, cabal_id, kind, status, transfer_id, created_at, seq)
			VALUES ($1, $2, 'fund', 'pending', $3, now(), 1)`
	}
	if _, err := h.pool.Exec(t.Context(), sql, h.ids.NewV7(), h.cabal.UUID(), id); err != nil {
		t.Fatal(err)
	}
}

func TestFundSettler_mintsOnlyOnceWhenAnotherRunnerSettlesFirst(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	id := h.landed(t, "5000000")
	h.chain.onTotal = func() {
		if _, err := h.pool.Exec(context.Background(), `UPDATE fund_transfers SET status = 'settled',
			share_units = 5000000, settled_at = now() WHERE id = $1`, id); err != nil {
			t.Error(err)
		}
	}
	h.tick(t)
	if n := h.count(t, "user_txns"); n != 0 || h.events(t, "cabal.funded") != 0 {
		t.Fatalf("user_txns = %d, want the other runner's settle to win", n)
	}
}

func TestFundSettler_reportsReadAndWriteFailures(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	p := adapters.FundPoller{Settler: h.settler}
	if p.Interval() != app.FundSettleInterval || p.Name() != "treasury.fund-transfers" {
		t.Fatalf("poller = %s every %s", p.Name(), p.Interval())
	}
	if _, err := p.Tick(canceled(h.systemCtx())); err == nil {
		t.Fatal("Tick on a canceled context = nil error")
	}
	_, sig := h.submitted(t, 5_000_000)
	h.chain.statusErr = errs.New(errs.CodeRPCUnavailable, "stub")
	wantCode(t, h.tickErr(), errs.CodeRPCUnavailable)
	h.chain.statusErr = nil
	h.chain.statuses[sig] = solana.Status{Signature: sig, State: solana.StateFinalized, Failed: true}
	if _, err := h.settler.Tick(h.ctx()); err == nil {
		t.Fatal("Tick that cannot append cabal.fund_failed = nil error")
	}
	h.landed(t, "99999999999999999999")
	if _, err := h.settler.Tick(h.systemCtx()); err == nil {
		t.Fatal("Tick over an unparsable amount = nil error")
	}
}

func TestStatuses_readsThroughTheConfiguredRPC(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cfg := f.cfg
	cfg.Solana.RPCURL = "http://127.0.0.1:1/rpc/"
	cfg.Timeouts.RPC = time.Second
	if _, err := adapters.NewStatuses(cfg, f.clock).SignatureStatuses(f.ctx(), []chain.Signature{"s"}); err == nil {
		t.Fatal("SignatureStatuses against an unreachable RPC = nil error")
	}
}

func TestFundSettler_failsOnlyRowsStillOpen(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	moved, movedSig := h.submitted(t, 5_000_000)
	huge, hugeSig := h.submitted(t, 5_000_000)
	if _, err := h.pool.Exec(
		t.Context(),
		`UPDATE fund_transfers SET amount_micros = 99999999999999999999 WHERE id = $1`,
		huge,
	); err != nil {
		t.Fatal(err)
	}
	for _, sig := range []chain.Signature{movedSig, hugeSig} {
		h.chain.statuses[sig] = solana.Status{Signature: sig, State: solana.StateFinalized, Failed: true}
	}
	h.chain.onStatuses = func() {
		if _, err := h.pool.Exec(
			context.Background(),
			`UPDATE fund_transfers SET status = 'failed', fail_code = 'fund_expired' WHERE id = $1`,
			moved,
		); err != nil {
			t.Error(err)
		}
	}
	if err := h.tickErr(); err == nil {
		t.Fatal("Tick over an unparsable amount = nil error")
	}
	if _, _, code := h.status(t, moved); code != "fund_expired" || h.events(t, "cabal.fund_failed") != 0 {
		t.Fatalf("moved row code = %s, want the first writer's fund_expired and no event", code)
	}
}
