package flows

import (
	"context"
	"crypto/ed25519"
	"net/http"

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
			UnsentRow("fund_transfers", "cabal_id", f.cabal.ID.String(), "fund_not_sent"),
		).
		Then(scenario.ExpectEvents(events.TypeFundSubmitted, 1))
}

func UnsentRow(table, key, id, code string) scenario.Step {
	return func(s *scenario.Scenario) {
		var status, failCode string
		scenario.Eventually(table+" row failed as "+code, func(*scenario.Scenario) bool {
			if err := s.DB().QueryRow(s.Context(), `SELECT status, COALESCE(fail_code, '') FROM `+table+
				` WHERE `+key+` = $1 ORDER BY created_at, id LIMIT 1`, id).Scan(&status, &failCode); err != nil {
				s.Fatalf("flows: read the %s row: %v", table, err)
			}
			return status == "failed"
		})(s)
		if failCode != code {
			s.Fatalf("flows: %s row failed as %q, want %q", table, failCode, code)
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
	if _, err := s.DB().Exec(s.Context(), `INSERT INTO cabal_pauses (id, cabal_id, reason, created_at)
		VALUES (gen_random_uuid(), $1, 'ops', now())`, f.cabal.ID.UUID()); err != nil {
		s.Fatalf("flows: pause cabal: %v", err)
	}
	s.Given(scenario.AsSeededUser("member", f.member.ID)).
		When(scenario.Post(f.path(), fundBody)).
		Then(scenario.ExpectProblem(errs.CodeCabalPaused), scenario.ExpectEvents(events.TypeFundSubmitted, 0))
}

func F07FundCabalPotValueZero(s *scenario.Scenario) {
	f := seedFund(s)
	testkit.NewLedger(seedT{s}, s.DB()).WithFundedMember(f.member.ID, f.cabal.ID, money.MicrosFromUint64(1_000_000))
	drainPot(s, f.cabal.ID, 1_000_000)
	s.Given(scenario.AsSeededUser("member", f.member.ID)).
		When(scenario.Post(f.path(), fundBody)).
		Then(scenario.ExpectProblem(errs.CodePotValueZero), scenario.ExpectEvents(events.TypeFundSubmitted, 0))
}

func drainPot(s *scenario.Scenario, cabal ids.CabalID, micros int64) {
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
		err = db.New(s.DB(), ids.Real{}, clock.Real{}).Do(s.Context(), func(ctx context.Context, tx db.Tx) error {
			return ledger.PostCabalTxn(ctx, tx, out)
		})
	}
	if err != nil {
		s.Fatalf("flows: drain the pot: %v", err)
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
