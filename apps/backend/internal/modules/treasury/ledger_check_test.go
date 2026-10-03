package treasury_test

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
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
	check := treasury.LedgerCheck(testkit.Config())
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

func (f fixture) postFund(t *testing.T, cabal ids.CabalID, micros int64) {
	t.Helper()
	u, c, err := f.fund(f.user(t), cabal, micros, micros, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
}

func (f fixture) postSwaps(t *testing.T, cabal ids.CabalID, trades ...[2]int64) {
	t.Helper()
	for _, trade := range trades {
		swap, err := f.swap(cabal, trade[0], trade[1])
		if err != nil {
			t.Fatal(err)
		}
		if err := f.postCabal(swap); err != nil {
			t.Fatal(err)
		}
	}
}

func (f fixture) postTrades(t *testing.T, cabal ids.CabalID, trades ...[2]int64) {
	t.Helper()
	f.postFund(t, cabal, 100)
	f.postSwaps(t, cabal, trades...)
}

func TestLedgerCheck_namesACostBasisThatDiffersFromItsEntries(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.cabal(t)
	f.postTrades(t, cabal, [2]int64{60, 3}, [2]int64{-25, -1})
	check := treasury.LedgerCheck(testkit.Config())
	if diffs, err := check.Check(t.Context(), f.pool); err != nil || len(diffs) != 0 {
		t.Fatalf("ledger = %q, %v, want no diffs", diffs, err)
	}
	for _, plant := range []struct {
		sql   string
		asset domain.Asset
	}{
		{`UPDATE cabal_positions SET cost_basis_micros = 64 WHERE asset = $1`, usdc},
		{`UPDATE cabal_positions SET units = 3, cost_basis_micros = 41 WHERE asset = $1`, aapl},
	} {
		if _, err := f.pool.Exec(t.Context(), plant.sql, string(plant.asset)); err != nil {
			t.Fatal(err)
		}
	}
	diffs, err := check.Check(t.Context(), f.pool)
	prefix := "cabal_positions " + cabal.String() + " "
	want := []string{
		prefix + string(aapl) + ": entries 2, position 3",
		prefix + string(usdc) + ": cost basis entries 65, position 64",
		prefix + string(aapl) + ": cost basis entries 40, position 41",
	}
	if err != nil || !slices.Equal(diffs, want) {
		t.Fatalf("drifted ledger =\n%q, %v\nwant\n%q", diffs, err, want)
	}
}

func TestLedgerCheck_namesAPositionWithoutEntriesAndEntriesWithoutAPosition(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.cabal(t)
	f.postTrades(t, cabal, [2]int64{60, 3})
	if _, err := f.pool.Exec(t.Context(), `DELETE FROM cabal_positions WHERE asset = $1`, string(aapl)); err != nil {
		t.Fatal(err)
	}
	stray := `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
		VALUES ($1, 'STRAY', 0, 9, now())`
	if _, err := f.pool.Exec(t.Context(), stray, cabal.UUID()); err != nil {
		t.Fatal(err)
	}
	diffs, err := treasury.LedgerCheck(testkit.Config()).Check(t.Context(), f.pool)
	prefix := "cabal_positions " + cabal.String() + " "
	want := []string{
		prefix + string(aapl) + ": entries 3, position 0",
		prefix + "STRAY: cost basis entries 0, position 9",
		prefix + string(aapl) + ": cost basis entries 60, position 0",
	}
	if err != nil || !slices.Equal(diffs, want) {
		t.Fatalf("drifted ledger =\n%q, %v\nwant\n%q", diffs, err, want)
	}
}

func TestLedgerCheck_reportsASaleOfWhatTheLedgerNeverBoughtWithoutStopping(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal, txn := f.cabal(t), f.ids.NewV7()
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO cabal_txns (id, cabal_id, kind, status, created_at, seq)
		VALUES ($1, $2, 'swap', 'settled', now(), 1)`, txn, cabal.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO cabal_txn_entries (txn_id, seq, account, asset, amount)
		VALUES ($1, 0, 'treasury', 'SOLD', -5), ($1, 1, 'venue', 'SOLD', 5)`, txn); err != nil {
		t.Fatal(err)
	}
	diffs, err := treasury.LedgerCheck(testkit.Config()).Check(t.Context(), f.pool)
	want := []string{"cabal_positions " + cabal.String() + " SOLD: entries -5, position 0"}
	if err != nil || !slices.Equal(diffs, want) {
		t.Fatalf("ledger = %q, %v, want %q", diffs, err, want)
	}
}

func TestLedgerCheck_replaysHeadersWithSeveralTreasuryLegs(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.cabal(t)
	f.postFund(t, cabal, 100)
	const bee, cee = domain.Asset("BEE"), domain.Asset("CEE")
	leg := func(account domain.CabalAccount, asset domain.Asset, v int64) domain.CabalEntry {
		return domain.CabalEntry{Account: account, Asset: asset, Amount: amount(v)}
	}
	for _, entries := range [][]domain.CabalEntry{
		{
			leg(domain.CabalTreasury, usdc, -20), leg(domain.CabalVenue, usdc, 20),
			leg(domain.CabalTreasury, aapl, 4), leg(domain.CabalVenue, aapl, -4),
			leg(domain.CabalTreasury, bee, 3), leg(domain.CabalTreasury, bee, -3),
		},
		{
			leg(domain.CabalTreasury, usdc, 10), leg(domain.CabalVenue, usdc, -10),
			leg(domain.CabalTreasury, aapl, -1), leg(domain.CabalVenue, aapl, 1),
			leg(domain.CabalTreasury, cee, 2), leg(domain.CabalVenue, cee, -2),
		},
	} {
		txn, err := domain.NewCabalTxn(domain.CabalTxnHeader{
			ID: f.ids.NewV7(), CabalID: cabal, Kind: domain.CabalSwap, Status: domain.TxnSettled, SwapID: f.ids.NewV7(),
		}, entries)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.postCabal(txn); err != nil {
			t.Fatal(err)
		}
	}
	want := []position{
		{Asset: string(cee), Units: "2", Cost: "0"},
		{Asset: string(usdc), Units: "90", Cost: "90"},
		{Asset: string(aapl), Units: "3", Cost: "15"},
	}
	if got := f.cabalPositions(t); !slices.Equal(got, want) {
		t.Fatalf("cabal positions = %+v, want %+v", got, want)
	}
	if diffs, err := treasury.LedgerCheck(testkit.Config()).Check(t.Context(), f.pool); err != nil || len(diffs) != 0 {
		t.Fatalf("ledger = %q, %v, want no diffs", diffs, err)
	}
}

func TestLedgerCheck_replaysCostBasisPastInt64(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.cabal(t)
	const half = 5_000_000_000_000_000_000
	for range 3 {
		f.postFund(t, cabal, half)
	}
	f.postSwaps(t, cabal, [2]int64{half, 3}, [2]int64{half, 3}, [2]int64{-1, -3})
	want := []position{
		{Asset: string(usdc), Units: "5000000000000000001", Cost: "5000000000000000001"},
		{Asset: string(aapl), Units: "3", Cost: "5000000000000000000"},
	}
	if got := f.cabalPositions(t); !slices.Equal(got, want) {
		t.Fatalf("cabal positions = %+v, want %+v", got, want)
	}
	if diffs, err := treasury.LedgerCheck(testkit.Config()).Check(t.Context(), f.pool); err != nil || len(diffs) != 0 {
		t.Fatalf("ledger = %q, %v, want a cost of 10^19 released in big integers", diffs, err)
	}
}

func TestLedgerCheck_pricesEveryPositionInTheConfiguredUSDCMint(t *testing.T) {
	t.Parallel()
	const devnetUSDC = "4zMMC9srt5Ri5X14GAgXhaHii3GnPAEERYPJgZJDncDU"
	f := newFixtureOn(t, config.Config{Solana: config.Solana{USDCMint: devnetUSDC}})
	cabal := f.funded(t)
	trade := buy(cabal, f.ids.NewV7(), 60_000_000, 3_000_000, 0)
	trade.InMint = devnetUSDC
	if err := f.deliver(t, trade, confirmedAt()); err != nil {
		t.Fatal(err)
	}
	wantPositions(t, f,
		position{Asset: devnetUSDC, Units: "40000000", Cost: "40000000"},
		position{Asset: string(aapl), Units: "3000000", Cost: "60000000"})
	if diffs, err := treasury.LedgerCheck(testkit.Config()).Check(t.Context(), f.pool); err != nil || len(diffs) == 0 {
		t.Fatalf("mainnet check of a devnet ledger = %q, %v, want cost drift", diffs, err)
	}
}

func TestLedgerCheck_replaysCostBasisInTheOrderTheEntriesWereWritten(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.cabal(t)
	f.postTrades(t, cabal, [2]int64{10, 3}, [2]int64{-6, -2}, [2]int64{7, 1})
	want := []position{{Asset: string(usdc), Units: "89", Cost: "89"}, {Asset: string(aapl), Units: "2", Cost: "11"}}
	if got := f.cabalPositions(t); !slices.Equal(got, want) {
		t.Fatalf("cabal positions = %+v, want %+v", got, want)
	}
	if diffs, err := treasury.LedgerCheck(testkit.Config()).Check(t.Context(), f.pool); err != nil || len(diffs) != 0 {
		t.Fatalf("ledger = %q, %v, want the replay to end where the writes did", diffs, err)
	}
}

func TestLedgerCheck_replaysInPostOrderWhenTheClockStepsBack(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.cabal(t)
	f.postTrades(t, cabal, [2]int64{10, 3})
	sell, err := f.swap(cabal, -6, -2)
	if err != nil {
		t.Fatal(err)
	}
	buy, err := f.swap(cabal, 7, 1)
	if err != nil {
		t.Fatal(err)
	}
	start := f.clock.Now()
	for i, txn := range []domain.CabalTxn{buy, sell} {
		f.clock.Set(start.Add(time.Duration(2-i) * time.Second))
		if err := f.postCabal(txn); err != nil {
			t.Fatal(err)
		}
	}
	want := []position{{Asset: string(usdc), Units: "89", Cost: "89"}, {Asset: string(aapl), Units: "2", Cost: "9"}}
	if got := f.cabalPositions(t); !slices.Equal(got, want) {
		t.Fatalf("cabal positions = %+v, want %+v", got, want)
	}
	if diffs, err := treasury.LedgerCheck(testkit.Config()).Check(t.Context(), f.pool); err != nil || len(diffs) != 0 {
		t.Fatalf("ledger = %q, %v, want the replay to follow the posts, not their clock or ids", diffs, err)
	}
}

func TestCheckLedger_comparesWalletAndTreasuryBalancesWithTheMoneyEvents(t *testing.T) {
	t.Parallel()
	s := seed(t)
	s.ping(t, s.alice, "abc")
	check := adapters.CheckLedger(usdc, map[events.Type]adapters.BalanceRule{events.TypeSystemPinged: pinged})
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
	_, err = check(t.Context(), s.f.pool)
	wantCode(t, err, errs.CodeDecodeFailed)
}

func TestCheckLedger_addsEventAmountsPastInt64(t *testing.T) {
	t.Parallel()
	s := seed(t)
	s.ping(t, s.alice, "abc")
	move := adapters.Balance{
		Owner: "wallet:" + s.alice.String(), Asset: usdcMint, Amount: money.SignedMicrosFromInt64(math.MaxInt64),
	}
	check := adapters.CheckLedger(usdc, map[events.Type]adapters.BalanceRule{
		events.TypeSystemPinged: func([]byte) ([]adapters.Balance, error) { return []adapters.Balance{move, move}, nil },
	})
	diffs, err := check(t.Context(), s.f.pool)
	want := []string{
		"balance treasury:" + s.cabal.String() + " " + usdcMint + ": ledger 150000000, events 0",
		"balance treasury:" + s.cabal.String() + " XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp: ledger 5, events 0",
		"balance wallet:" + s.alice.String() + " " + usdcMint + ": ledger 0, events 18446744073709551614",
	}
	if err != nil || !slices.Equal(diffs, want) {
		t.Fatalf("diffs =\n%q, %v\nwant\n%q", diffs, err, want)
	}
}

func TestCheckLedger_reportsLedgerOnlyWalletBalancesForDepositEvents(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user := f.user(t)
	h := adapters.UserLedger{Ledger: f.ledger, IDs: f.ids, USDC: usdc}
	e := events.DepositCredited{
		V: 1, DepositID: f.ids.NewV7(), UserID: user.UUID(), TxSignature: "deposit",
		AmountMicros: money.MicrosFromUint64(25),
	}
	if err := f.do(func(ctx context.Context, tx db.Tx) error {
		return h.Handle(ctx, tx, e, f.clock.Now())
	}); err != nil {
		t.Fatal(err)
	}
	check := adapters.CheckLedger(usdc, map[events.Type]adapters.BalanceRule{
		events.TypeDepositCredited: adapters.DepositCreditedBalances(usdc),
	})
	diffs, err := check(t.Context(), f.pool)
	want := []string{"balance wallet:" + user.String() + " " + usdcMint + ": ledger 25, events 0"}
	if err != nil || !slices.Equal(diffs, want) {
		t.Fatalf("diffs = %q, %v, want %q", diffs, err, want)
	}
	cabal := f.cabal(t)
	f.postFund(t, cabal, 10)
	diffs, err = check(t.Context(), f.pool)
	if err != nil || !slices.Equal(diffs, want) {
		t.Fatalf("diffs with treasury balance = %q, %v, want %q", diffs, err, want)
	}
}

func TestCheckLedger_failsWhenTheDatabaseDoes(t *testing.T) {
	t.Parallel()
	s := seed(t)
	s.ping(t, s.alice, "abc")
	ctx, cancel := context.WithCancel(t.Context())
	cancelling := adapters.CheckLedger(usdc, map[events.Type]adapters.BalanceRule{
		events.TypeSystemPinged: func([]byte) ([]adapters.Balance, error) {
			cancel()
			return nil, nil
		},
	})
	_, err := cancelling(ctx, s.f.pool)
	wantCode(t, err, errs.CodeInternal)
	withRule := adapters.CheckLedger(usdc, map[events.Type]adapters.BalanceRule{events.TypeSystemPinged: pinged})
	s.exec(t, `DROP TABLE events CASCADE`)
	_, err = withRule(t.Context(), s.f.pool)
	wantCode(t, err, errs.CodeInternal)
	s.exec(t, `ALTER TABLE cabal_txns DROP COLUMN seq`)
	_, err = treasury.LedgerCheck(testkit.Config()).Check(t.Context(), s.f.pool)
	wantCode(t, err, errs.CodeInternal)
	for _, table := range []string{"user_positions", "cabal_positions", "user_txns", "cabal_txn_entries"} {
		s.exec(t, `DROP TABLE `+table+` CASCADE`)
		_, err = treasury.LedgerCheck(testkit.Config()).Check(t.Context(), s.f.pool)
		t.Logf("without %s: %v", table, err)
		wantCode(t, err, errs.CodeInternal)
	}
}
