package treasury_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type ledgerModel struct {
	f         fixture
	users     []ids.UserID
	cabals    []ids.CabalID
	transfers []uuid.UUID
}

func (m *ledgerModel) fund(t *rapid.T) {
	user, cabal := rapid.SampledFrom(m.users).Draw(t, "user"), rapid.SampledFrom(m.cabals).Draw(t, "cabal")
	micros := rapid.Int64Range(1, 1_000_000_000).Draw(t, "micros")
	status := rapid.SampledFrom([]domain.TxnStatus{domain.TxnPending, domain.TxnSettled}).Draw(t, "status")
	build := m.f.fund
	if rapid.Bool().Draw(t, "cash out") {
		build = m.f.cashOut
	}
	u, c, err := build(user, cabal, micros, rapid.Int64Range(1, micros).Draw(t, "shares"), status)
	if err != nil {
		t.Fatal(err)
	}
	if m.f.postPair(u, c) == nil {
		m.transfers = append(m.transfers, u.TransferID)
	}
}

func (m *ledgerModel) swap(t *rapid.T) {
	micros := rapid.Int64Range(1, 1_000_000_000).Draw(t, "micros")
	units := rapid.Int64Range(1, 1_000_000).Draw(t, "units")
	if rapid.Bool().Draw(t, "sell") {
		micros, units = -micros, -units
	}
	txn, err := m.f.swap(rapid.SampledFrom(m.cabals).Draw(t, "cabal"), micros, units)
	if err != nil {
		t.Fatal(err)
	}
	_ = m.f.postCabal(txn)
}

func (m *ledgerModel) setStatus(t *rapid.T) {
	if len(m.transfers) == 0 {
		return
	}
	id := rapid.SampledFrom(m.transfers).Draw(t, "transfer")
	to := rapid.SampledFrom([]domain.TxnStatus{domain.TxnSettled, domain.TxnFailed}).Draw(t, "to")
	_ = m.f.do(func(ctx context.Context, tx db.Tx) error {
		_, err := m.f.ledger.SetStatus(ctx, tx, id, domain.TxnPending, to)
		return err
	})
}

func (m *ledgerModel) Check(t *rapid.T) {
	drift, err := m.f.findDrift(m.f.ctx())
	if err != nil || len(drift) != 0 {
		t.Fatalf("ledger drifted: %q, %v", drift, err)
	}
}

func TestLedgerProperty_positionsEqualEntrySumsAndTransfersShareAStatus(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	m := &ledgerModel{f: f}
	for range 2 {
		m.users = append(m.users, f.user(t))
		m.cabals = append(m.cabals, f.cabal(t))
	}
	ops := []func(*rapid.T){m.fund, m.swap, m.setStatus}
	rapid.Check(t, func(rt *rapid.T) {
		for range rapid.IntRange(1, 4).Draw(rt, "ops") {
			rapid.SampledFrom(ops).Draw(rt, "op")(rt)
		}
		m.Check(rt)
	})
}
