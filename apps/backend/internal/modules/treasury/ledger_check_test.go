package treasury_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type seeded struct {
	f            fixture
	alice, bob   ids.UserID
	cabal        ids.CabalID
	fundTransfer string
}

func seed(t *testing.T) seeded {
	t.Helper()
	f := newFixture(t)
	s := seeded{f: f, alice: f.user(t), bob: f.user(t), cabal: f.cabal(t)}
	testkit.NewLedger(t, f.pool).
		WithFundedMember(s.alice, s.cabal, money.MicrosFromUint64(100_000_000)).
		WithHolding(s.cabal, "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", money.NewBaseUnits(5, 8)).
		WithFundedMember(s.bob, s.cabal, money.MicrosFromUint64(50_000_000))
	if err := f.pool.QueryRow(t.Context(), `SELECT transfer_id::text FROM user_txns
		WHERE user_id = $1 AND kind = 'fund'`, s.bob.UUID()).Scan(&s.fundTransfer); err != nil {
		t.Fatal(err)
	}
	return s
}

func (s seeded) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := s.f.pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func TestLedgerCheck_passesOnASeededLedgerAndNamesEveryDrift(t *testing.T) {
	t.Parallel()
	s := seed(t)
	check := treasury.LedgerCheck()
	if diffs, err := check.Check(t.Context(), s.f.pool); err != nil || len(diffs) != 0 {
		t.Fatalf("seeded ledger = %q, %v, want no diffs", diffs, err)
	}
	var shares string
	if err := s.f.pool.QueryRow(t.Context(), `SELECT share_units::text FROM user_positions WHERE user_id = $1`,
		s.bob.UUID()).Scan(&shares); err != nil || shares != "50000000" {
		t.Fatalf("bob holds %s shares, %v; want 50000000 minted at $1 a share", shares, err)
	}
	s.exec(t, `UPDATE cabal_positions SET units = units + 1 WHERE asset <> $1`, usdcMint)
	s.exec(t, `UPDATE user_positions SET withdrawn_micros = 7 WHERE user_id = $1`, s.alice.UUID())
	s.exec(t, `UPDATE cabal_txns SET status = 'failed' WHERE transfer_id = $1`, s.fundTransfer)
	s.exec(t, `INSERT INTO user_txn_entries (txn_id, seq, account, asset, amount)
		SELECT id, 9, 'wallet', $1, 3 FROM user_txns WHERE user_id = $2 AND kind = 'deposit'`, usdcMint, s.bob.UUID())
	diffs, err := check.Check(t.Context(), s.f.pool)
	var bobDeposit string
	if err := s.f.pool.QueryRow(t.Context(), `SELECT id::text FROM user_txns WHERE user_id = $1 AND kind = 'deposit'`,
		s.bob.UUID()).Scan(&bobDeposit); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"user_txns " + bobDeposit + " sums to 3 in " + usdcMint,
		"transfer " + s.fundTransfer + " has headers in failed,settled",
		"cabal_positions " + s.cabal.String() + " XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp: entries 5, position 6",
		"user_positions " + s.alice.String() + " " + s.cabal.String() +
			": entries 100000000/100000000/0, position 100000000/100000000/7",
	}
	if err != nil || !slices.Equal(diffs, want) {
		t.Fatalf("drifted ledger =\n%q, %v\nwant\n%q", diffs, err, want)
	}
	if check.Name != "treasury" || len(check.Tables) != 6 || check.Handlers != nil {
		t.Fatalf("check = %+v, want the six treasury tables and no handlers", check)
	}
}

func pinged(payload []byte) ([]adapters.Balance, error) {
	var e events.SystemPinged
	if err := json.Unmarshal(payload, &e); err != nil {
		return nil, errs.Wrap(err, errs.CodeDecodeFailed, "fixture.pinged")
	}
	if e.Note == "bad" {
		return nil, errs.New(errs.CodeDecodeFailed, "fixture.pinged")
	}
	return []adapters.Balance{{
		Owner: "wallet:" + e.UserID.String(), Asset: usdcMint, Amount: money.SignedMicrosFromInt64(int64(len(e.Note))),
	}}, nil
}

func (s seeded) ping(t *testing.T, user ids.UserID, note string) {
	t.Helper()
	ctx := observability.WithActor(s.f.ctx(), "user:"+user.String())
	err := s.f.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		ping := events.SystemPinged{V: 1, PingID: s.f.ids.NewV7(), UserID: user.UUID(), Note: note}
		return tx.Events.Append(ctx, ping)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCheckLedger_comparesWalletAndTreasuryBalancesWithTheMoneyEvents(t *testing.T) {
	t.Parallel()
	s := seed(t)
	s.ping(t, s.alice, "abc")
	check := adapters.CheckLedger(map[events.Type]adapters.BalanceRule{events.TypeSystemPinged: pinged})
	diffs, err := check(t.Context(), s.f.pool)
	want := []string{
		"balance treasury:" + s.cabal.String() + " " + usdcMint + ": ledger 150000000, events 0",
		"balance treasury:" + s.cabal.String() + " XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp: ledger 5, events 0",
		"balance wallet:" + s.alice.String() + " " + usdcMint + ": ledger 0, events 3",
	}
	if err != nil || !slices.Equal(diffs, want) {
		t.Fatalf("diffs =\n%q, %v\nwant\n%q", diffs, err, want)
	}
	s.ping(t, s.bob, "bad")
	if _, err := check(t.Context(), s.f.pool); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("a rule error = %v, want decode_failed", err)
	}
}

func TestCheckLedger_failsWhenTheDatabaseDoes(t *testing.T) {
	t.Parallel()
	s := seed(t)
	s.ping(t, s.alice, "abc")
	ctx, cancel := context.WithCancel(t.Context())
	cancelling := adapters.CheckLedger(map[events.Type]adapters.BalanceRule{
		events.TypeSystemPinged: func([]byte) ([]adapters.Balance, error) {
			cancel()
			return nil, nil
		},
	})
	if _, err := cancelling(ctx, s.f.pool); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("balances after a cancel = %v, want internal", err)
	}
	withRule := adapters.CheckLedger(map[events.Type]adapters.BalanceRule{events.TypeSystemPinged: pinged})
	s.exec(t, `DROP TABLE events CASCADE`)
	if _, err := withRule(t.Context(), s.f.pool); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("without events = %v, want internal", err)
	}
	for _, table := range []string{"user_positions", "cabal_positions", "user_txns", "cabal_txn_entries"} {
		s.exec(t, `DROP TABLE `+table+` CASCADE`)
		if _, err := treasury.LedgerCheck().Check(t.Context(), s.f.pool); errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("without %s = %v, want internal", table, err)
		}
	}
}
