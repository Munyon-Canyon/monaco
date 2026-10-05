package treasury_test

import (
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func fundStatuses() []domain.FundStatus {
	return []domain.FundStatus{
		domain.FundCreated, domain.FundSubmitted, domain.FundLanded, domain.FundSettled, domain.FundFailed,
	}
}

func TestFundTransfers_guardedMovesFollowTheStateMachine(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for _, from := range fundStatuses() {
		for _, to := range fundStatuses()[1:] {
			f.assertGuardedMove(t, from, to)
		}
	}
}

func (f fixture) assertGuardedMove(t *testing.T, from, to domain.FundStatus) {
	t.Helper()
	id := f.fundTransferIn(t, f.user(t), from, 5_000_000)
	moved := f.moveFundTransfer(t, id, to)
	if want := from.CanMoveTo(to); moved != want {
		t.Fatalf("%s -> %s moved = %t, want %t", from, to, moved, want)
	}
	if moved && f.moveFundTransfer(t, id, to) {
		t.Fatalf("%s -> %s moved twice", from, to)
	}
	want := from
	if moved {
		want = to
	}
	if got := f.fundTransferStatus(t, id); got != want {
		t.Fatalf("%s -> %s left status %s, want %s", from, to, got, want)
	}
}

func TestFundTransfers_settleRecordsShareUnits(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	id := f.fundTransferIn(t, f.user(t), domain.FundSettled, 5_000_000)
	row, err := sqlc.New(f.pool).GetFundTransfer(f.ctx(), id)
	if err != nil {
		t.Fatal(err)
	}
	if row.AmountMicros != "5000000" || row.ShareUnits != "7" || row.FailCode != "" {
		t.Fatalf("settled row = %+v, want amount 5000000, 7 share units, no fail code", row)
	}
}

func TestFundTransfers_inFlightSumsCreatedAndSubmittedOnly(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user := f.user(t)
	for i, status := range fundStatuses() {
		f.fundTransferIn(t, user, status, int64(i+1)*1_000_000)
	}
	f.fundTransferIn(t, f.user(t), domain.FundCreated, 9_000_000)
	got, err := sqlc.New(f.pool).InFlightFundMicros(f.ctx(), user.UUID())
	if err != nil || got != "3000000" {
		t.Fatalf("InFlightFundMicros() = %q, %v, want 3000000 (created 1 + submitted 2)", got, err)
	}
}

func TestQueries_ownsASubmittedFundSignatureBeforeBroadcast(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	q := adapters.NewQueries(f.pool, nil, nil, f.clock, chain.SolanaAddress(f.cfg.Solana.USDCMint))
	id := f.fundTransferIn(t, f.user(t), domain.FundSubmitted, 5_000_000)
	assertSignatureOwned(f.ctx(), t, q, fundSignature(id), true)
	assertSignatureOwned(f.ctx(), t, q, fundSignature(f.ids.NewV7()), false)
}

func fundSignature(id uuid.UUID) chain.Signature { return chain.Signature("fund-" + id.String()) }

func (f fixture) fundTransferIn(t *testing.T, user ids.UserID, status domain.FundStatus, micros int64) uuid.UUID {
	t.Helper()
	id := f.ids.NewV7()
	err := sqlc.New(f.pool).InsertFundTransfer(f.ctx(), sqlc.InsertFundTransferParams{
		ID: id, UserID: user.UUID(), CabalID: f.cabal(t).UUID(), AmountMicros: strconv.FormatInt(micros, 10),
		FromAddress: "member-wallet", ToAddress: "treasury-wallet", CreatedAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	path := map[domain.FundStatus][]domain.FundStatus{
		domain.FundSubmitted: {domain.FundSubmitted},
		domain.FundLanded:    {domain.FundSubmitted, domain.FundLanded},
		domain.FundSettled:   {domain.FundSubmitted, domain.FundLanded, domain.FundSettled},
		domain.FundFailed:    {domain.FundFailed},
	}[status]
	for _, step := range path {
		if !f.moveFundTransfer(t, id, step) {
			t.Fatalf("seed %s: move to %s changed no row", status, step)
		}
	}
	return id
}

func (f fixture) moveFundTransfer(t *testing.T, id uuid.UUID, to domain.FundStatus) bool {
	t.Helper()
	q, ctx, now := sqlc.New(f.pool), f.ctx(), f.clock.Now()
	moves := map[domain.FundStatus]func() (int64, error){
		domain.FundSubmitted: func() (int64, error) {
			return q.SubmitFundTransfer(ctx, sqlc.SubmitFundTransferParams{
				ID: id, SignedTx: []byte{1}, TxSignature: string(fundSignature(id)), LastValidBlockHeight: "100",
				SubmittedAt: now,
			})
		},
		domain.FundLanded: func() (int64, error) {
			return q.LandFundTransfer(ctx, sqlc.LandFundTransferParams{ID: id, LandedAt: now})
		},
		domain.FundSettled: func() (int64, error) {
			return q.SettleFundTransfer(ctx, sqlc.SettleFundTransferParams{ID: id, ShareUnits: "7", SettledAt: now})
		},
		domain.FundFailed: func() (int64, error) {
			return q.FailFundTransfer(ctx, sqlc.FailFundTransferParams{ID: id, FailCode: "fund_not_sent"})
		},
	}
	n, err := moves[to]()
	if err != nil {
		t.Fatalf("move %s to %s: %v", id, to, err)
	}
	return n == 1
}

func (f fixture) fundTransferStatus(t *testing.T, id uuid.UUID) domain.FundStatus {
	t.Helper()
	row, err := sqlc.New(f.pool).GetFundTransfer(f.ctx(), id)
	if err != nil {
		t.Fatal(err)
	}
	return domain.FundStatus(row.Status)
}
