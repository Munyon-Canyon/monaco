package flows

import (
	"context"
	"crypto/ed25519"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const fundBody = `{"amount_micros":"5000000"}`

func (defined) SeedsF07() map[string]Seeder {
	return map[string]Seeder{
		"F07FundCabalOK":                  seedFunder(nil),
		"F07FundCabalCrashBeforeCommit":   seedFunder(nil),
		"F07FundCabalCrashAfterSign":      seedFunder(nil),
		"F07FundCabalCrashAfterBroadcast": seedFunder(nil),
		"F07FundCabalInvalidInput":        seedFunder(nil),
		"F07FundCabalNotCabalMember":      seedFundOutsider,
		"F07FundCabalInsufficientFunds":   seedFunder(nil),
		"F07FundCabalCabalPaused":         seedFunder(pausedCabal),
		"F07FundCabalPotValueZero":        seedFunderOfDrainedPot,
		"F07FundCabalPrivyUnavailable":    seedFunder(nil),
	}
}

func seedFunder(arrange func(t testkit.SeedT, pool *pgxpool.Pool, cabal ids.CabalID)) Seeder {
	return seedSignedInWith(func(t testkit.SeedT, pool *pgxpool.Pool, member ids.UserID) map[string]string {
		cabal := testkit.NewCabal(t, pool, testkit.WithCreator(member))
		if arrange != nil {
			arrange(t, pool, cabal.ID)
		}
		return map[string]string{"cabal": cabal.ID.String()}
	})
}

func seedFunderOfDrainedPot(ctx context.Context, env SeedEnv) (SeedResult, error) {
	return seedSignedInWith(func(t testkit.SeedT, pool *pgxpool.Pool, member ids.UserID) map[string]string {
		cabal := testkit.NewCabal(t, pool, testkit.WithCreator(member))
		drainedPot(t, pool, cabal.ID, member)
		return map[string]string{"cabal": cabal.ID.String()}
	})(ctx, env)
}

func seedFundOutsider(ctx context.Context, env SeedEnv) (SeedResult, error) {
	return seedSignedInWith(func(t testkit.SeedT, pool *pgxpool.Pool, _ ids.UserID) map[string]string {
		return map[string]string{"cabal": testkit.NewCabal(t, pool).ID.String()}
	})(ctx, env)
}

func pausedCabal(t testkit.SeedT, pool *pgxpool.Pool, cabal ids.CabalID) {
	if _, err := pool.Exec(t.Context(), `INSERT INTO cabal_pauses (id, cabal_id, reason, created_at)
		VALUES (gen_random_uuid(), $1, 'ops', now())`, cabal.UUID()); err != nil {
		t.Fatalf("flows: pause cabal: %v", err)
	}
}

func drainedPot(t testkit.SeedT, pool *pgxpool.Pool, cabal ids.CabalID, member ids.UserID) {
	testkit.NewLedger(t, pool).WithFundedMember(member, cabal, money.MicrosFromUint64(1_000_000))
	drainPot(t, pool, cabal, 1_000_000)
}

type fundSeed struct {
	member testkit.SeededUser
	cabal  testkit.SeededCabal
}

func seedFund(s *scenario.Scenario) fundSeed {
	member := testkit.SeedUser(seedT{s}, s.DB(), testkit.UserOpts{WithWallet: true})
	member.Address = chain.AddressOf(fakes.PrivyWalletKey(member.PrivyWalletID).Public().(ed25519.PublicKey))
	if _, err := s.DB().Exec(s.Context(), `UPDATE user_wallets SET address = $1 WHERE user_id = $2`,
		string(member.Address), member.ID.UUID()); err != nil {
		s.Fatalf("flows: give the member the fake Privy wallet's address: %v", err)
	}
	return fundSeed{member: member, cabal: testkit.NewCabal(seedT{s}, s.DB(), testkit.WithCreator(member.ID))}
}

func (f fundSeed) path() string { return "/v1/cabals/" + f.cabal.ID.String() + "/fund" }

func F07FundCabalOK(s *scenario.Scenario) {
	f := seedFund(s)
	s.Given(scenario.AsSeededUser("member", f.member.ID)).
		When(
			scenario.Post(f.path(), fundBody),
			scenario.ExpectStatus(http.StatusAccepted),
			scenario.ExpectJSON("status", "submitted"),
			scenario.Remember("transfer_id", "transfer"),
			scenario.EventuallyHint("balance_changed"),
			scenario.AwaitTick("treasury.fund-transfers"),
			scenario.AwaitTick("treasury.fund-transfers"),
			scenario.Get("/v1/fund-transfers/{transfer}"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "settled"),
			scenario.ExpectJSON("share_units", "5000000"),
		).
		Then(scenario.ExpectEvents(events.TypeFundSubmitted, 1))
	s.Then(scenario.EventuallyCapturedBy(events.TypeFunded, "cabal_funded", "transfer_id", s.Recall("transfer")))
}

func F07FundCabalCrashBeforeCommit(s *scenario.Scenario) {
	f := seedFund(s)
	s.Given(scenario.AsSeededUser("member", f.member.ID)).
		When(
			scenario.Post(f.path(), fundBody),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusAccepted),
			scenario.Remember("transfer_id", "transfer"),
			scenario.AwaitTick("treasury.fund-transfers"),
			scenario.AwaitTick("treasury.fund-transfers"),
			scenario.Get("/v1/fund-transfers/{transfer}"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "settled"),
		).
		Then(scenario.ExpectEvents(events.TypeFundSubmitted, 1))
}

func F07FundCabalCrashAfterSign(s *scenario.Scenario) {
	f := seedFund(s)
	s.Given(scenario.AsSeededUser("member", f.member.ID)).
		When(
			scenario.Post(f.path(), fundBody),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusAccepted),
			scenario.Remember("transfer_id", "transfer"),
			scenario.AwaitTick("treasury.fund-transfers"),
			scenario.AwaitTick("treasury.fund-transfers"),
			scenario.Get("/v1/fund-transfers/{transfer}"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "settled"),
			OnlyRow("fund_transfers", "cabal_id", f.cabal.ID.String()),
		).
		Then(scenario.ExpectEvents(events.TypeFundSubmitted, 1), scenario.ExpectEvents(events.TypeFundFailed, 0))
}

func F07FundCabalCrashAfterBroadcast(s *scenario.Scenario) {
	f := seedFund(s)
	s.Given(scenario.AsSeededUser("member", f.member.ID)).
		When(
			scenario.Post(f.path(), fundBody),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusAccepted),
			scenario.Remember("transfer_id", "transfer"),
			scenario.AwaitTick("treasury.fund-transfers"),
			scenario.AwaitTick("treasury.fund-transfers"),
			scenario.Get("/v1/fund-transfers/{transfer}"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "settled"),
			OnlyRow("fund_transfers", "cabal_id", f.cabal.ID.String()),
		).
		Then(scenario.ExpectEvents(events.TypeFundSubmitted, 1), scenario.ExpectEvents(events.TypeFundFailed, 0))
}

func OnlyRow(table, key, id string) scenario.Step {
	return func(s *scenario.Scenario) {
		var n int
		query := `SELECT count(*) FROM ` + table + ` WHERE ` + key + ` = $1`
		if err := s.DB().QueryRow(s.Context(), query, id).Scan(&n); err != nil {
			s.Fatalf("flows: count the %s rows: %v", table, err)
		}
		if n != 1 {
			s.Fatalf("flows: %s rows for %s = %d, want the retry to resume the one row", table, id, n)
		}
	}
}

func F07FundCabalInvalidInput(s *scenario.Scenario) {
	f := seedFund(s)
	s.Given(scenario.AsSeededUser("member", f.member.ID)).
		When(scenario.Post(f.path(), `{"amount_micros":"999999"}`)).
		Then(scenario.ExpectProblem(errs.CodeInvalidInput), scenario.ExpectEvents(events.TypeFundSubmitted, 0))
}

func F07FundCabalNotCabalMember(s *scenario.Scenario) {
	f := seedFund(s)
	stranger := testkit.SeedUser(seedT{s}, s.DB(), testkit.UserOpts{WithWallet: true})
	s.Given(scenario.AsSeededUser("stranger", stranger.ID)).
		When(scenario.Post(f.path(), fundBody)).
		Then(scenario.ExpectProblem(errs.CodeNotCabalMember), scenario.ExpectEvents(events.TypeFundSubmitted, 0))
}

func F07FundCabalInsufficientFunds(s *scenario.Scenario) {
	f := seedFund(s)
	s.Given(scenario.AsSeededUser("member", f.member.ID)).
		When(scenario.Post(f.path(), `{"amount_micros":"900000000000"}`)).
		Then(scenario.ExpectProblem(errs.CodeInsufficientFunds), scenario.ExpectEvents(events.TypeFundSubmitted, 0))
}

func F07FundCabalCabalPaused(s *scenario.Scenario) {
	f := seedFund(s)
	pausedCabal(seedT{s}, s.DB(), f.cabal.ID)
	s.Given(scenario.AsSeededUser("member", f.member.ID)).
		When(scenario.Post(f.path(), fundBody)).
		Then(scenario.ExpectProblem(errs.CodeCabalPaused), scenario.ExpectEvents(events.TypeFundSubmitted, 0))
}

func F07FundCabalPotValueZero(s *scenario.Scenario) {
	f := seedFund(s)
	drainedPot(seedT{s}, s.DB(), f.cabal.ID, f.member.ID)
	s.Given(scenario.AsSeededUser("member", f.member.ID)).
		When(scenario.Post(f.path(), fundBody)).
		Then(scenario.ExpectProblem(errs.CodePotValueZero), scenario.ExpectEvents(events.TypeFundSubmitted, 0))
}

func drainPot(t testkit.SeedT, pool *pgxpool.Pool, cabal ids.CabalID, micros int64) {
	usdc := domain.MintAsset(testkit.USDCMint)
	out, err := domain.NewCabalTxn(domain.CabalTxnHeader{
		ID: ids.Real{}.NewV7(), CabalID: cabal, Kind: domain.CabalSwap, Status: domain.TxnSettled,
		SwapID: ids.Real{}.NewV7(),
	}, []domain.CabalEntry{
		{Account: domain.CabalTreasury, Asset: usdc, Amount: money.SignedMicrosFromInt64(-micros)},
		{Account: domain.CabalVenue, Asset: usdc, Amount: money.SignedMicrosFromInt64(micros)},
	})
	if err == nil {
		ledger := app.NewLedger(testkit.USDCMint, clock.Real{})
		err = db.New(pool, ids.Real{}, clock.Real{}).Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
			return ledger.PostCabalTxn(ctx, tx, out)
		})
	}
	if err != nil {
		t.Fatalf("flows: drain the pot: %v", err)
	}
}

func F07FundCabalPrivyUnavailable(s *scenario.Scenario) {
	f := seedFund(s)
	s.Given(
		scenario.AsSeededUser("member", f.member.ID),
		scenario.FakeUpstream(fakes.Step{
			Route: "/privy/v1/wallets/" + f.member.PrivyWalletID + "/rpc", Action: fakes.ActionFail, Status: 503,
			Times: 100, Reset: true,
		}),
	).
		When(scenario.Post(f.path(), fundBody)).
		Then(scenario.ExpectProblem(errs.CodePrivyUnavailable), scenario.ExpectEvents(events.TypeFundSubmitted, 0))
}
