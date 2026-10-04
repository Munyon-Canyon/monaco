package flows

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const leavePath = membersPath + "/me"

func (defined) SeedsF04() map[string]Seeder {
	return map[string]Seeder{
		"F04LeaveCabalOK":                         seedLeave(asMember, nil),
		"F04LeaveCabalCrashBeforeCommit":          seedLeave(asMember, nil),
		"F04LeaveCabalUnauthorized":               seedAnonymous,
		"F04LeaveCabalNotCabalMember":             seedLeave(asOutsider, nil),
		"F04LeaveCabalLeaveHoldsShares":           seedLeave(asMember, fundedLeaver),
		"F04LeaveCabalLeaveCreatorWithMembers":    seedLeave(asCreatorWithMember, nil),
		"F04LeaveCabalLeaveLastMemberPotNotEmpty": seedLeave(asCreator, potHolding(testkit.USDCMint)),
		"F04LeaveCabalPriceUnavailable":           seedLeave(asCreator, potHolding(unpricedMint)),
	}
}

func fundedLeaver(l *testkit.Ledger, cabal ids.CabalID, leaver ids.UserID) {
	l.WithFundedMember(leaver, cabal, money.MicrosFromUint64(1_000_000))
}

func potHolding(mint chain.SolanaAddress) func(*testkit.Ledger, ids.CabalID, ids.UserID) {
	return func(l *testkit.Ledger, cabal ids.CabalID, _ ids.UserID) {
		l.WithHolding(cabal, mint, money.NewBaseUnits(1, 6))
	}
}

func asMember(leaver ids.UserID) []testkit.CabalOption {
	return []testkit.CabalOption{testkit.WithJoiner(leaver)}
}

func asOutsider(ids.UserID) []testkit.CabalOption { return nil }

func asCreator(leaver ids.UserID) []testkit.CabalOption {
	return []testkit.CabalOption{testkit.WithCreator(leaver)}
}

func asCreatorWithMember(leaver ids.UserID) []testkit.CabalOption {
	return []testkit.CabalOption{testkit.WithCreator(leaver), testkit.WithMembers(2)}
}

func seedLeave(
	seat func(leaver ids.UserID) []testkit.CabalOption,
	fill func(l *testkit.Ledger, cabal ids.CabalID, leaver ids.UserID),
) Seeder {
	return seedSignedInWith(func(t testkit.SeedT, pool *pgxpool.Pool, leaver ids.UserID) map[string]string {
		cabal := testkit.NewCabal(t, pool, seat(leaver)...)
		if fill != nil {
			fill(testkit.NewLedger(t, pool), cabal.ID, leaver)
		}
		return map[string]string{"cabal": cabal.ID.String()}
	})
}

func cabalIn(s *scenario.Scenario) ids.CabalID {
	id, err := ids.ParseCabalID(s.Recall("cabal"))
	if err != nil {
		s.Fatalf("flows: remembered cabal %q: %v", s.Recall("cabal"), err)
	}
	return id
}

func founderOf(script string) string { return "did:privy:qa-f04-" + script + "-creator" }

func memberOf(script string) string { return "did:privy:qa-f04-" + script + "-member" }

func founded(script string) []scenario.Step {
	return []scenario.Step{
		scenario.SignIn(founderOf(script)),
		scenario.Post(cabalsPath, cabalOf("open", "all")),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.Remember("id", "cabal"),
	}
}

func joined(script string) []scenario.Step {
	return append(founded(script),
		scenario.SignIn(memberOf(script)),
		scenario.Post(membersPath, ""),
		scenario.ExpectStatus(http.StatusOK),
	)
}

const unpricedMint chain.SolanaAddress = "So11111111111111111111111111111111111111112"

func funded(micros uint64) scenario.Step {
	return func(s *scenario.Scenario) {
		testkit.NewLedger(s, s.DB()).WithFundedMember(s.ActorID(), cabalIn(s), money.MicrosFromUint64(micros))
	}
}

func holding(mint chain.SolanaAddress, units uint64) scenario.Step {
	return func(s *scenario.Scenario) {
		testkit.NewLedger(s, s.DB()).WithHolding(cabalIn(s), mint, money.NewBaseUnits(units, 6))
	}
}

func heldShares(s *scenario.Scenario) string {
	var units string
	err := s.DB().QueryRow(s.Context(),
		`SELECT share_units::text FROM user_positions WHERE cabal_id = $1 AND user_id = $2`,
		cabalIn(s).UUID(), s.ActorID().UUID(),
	).Scan(&units)
	if err != nil {
		s.Fatalf("flows: read the leaver's share units: %v", err)
	}
	return units
}

func guarded(code errs.Code, have func(*scenario.Scenario) string) []scenario.Step {
	return append(refused(code, events.TypeCabalMemberLeft, 0), func(s *scenario.Scenario) {
		scenario.EventuallyLog(observability.HTTPProblem, map[string]string{
			"code": string(code), "detail.have": have(s),
		})(s)
	})
}

func left() []scenario.Step {
	return []scenario.Step{
		scenario.ExpectEvents(events.TypeCabalMemberLeft, 1),
		scenario.EventuallyPublished(events.TypeCabalMemberLeft, 1),
		scenario.EventuallyHint("cabal_access"),
		cabalHolds(),
	}
}

func F04LeaveCabalOK(s *scenario.Scenario) {
	s.Given(joined("ok")...).
		When(
			scenario.Delete(leavePath),
			scenario.ExpectStatus(http.StatusNoContent),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusNoContent),
		).
		Then(left()...)
}

func F04LeaveCabalUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Delete(missingCabal + "/members/me")).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F04LeaveCabalNotCabalMember(s *scenario.Scenario) {
	s.Given(founded("outsider")...).
		When(scenario.SignIn(memberOf("outsider")), scenario.Delete(leavePath)).
		Then(refused(errs.CodeNotCabalMember, events.TypeCabalMemberLeft, 0)...)
}

func F04LeaveCabalLeaveHoldsShares(s *scenario.Scenario) {
	s.Given(append(joined("shares"), funded(1_000_000))...).
		When(scenario.Delete(leavePath)).
		Then(guarded(errs.CodeLeaveHoldsShares, heldShares)...)
}

func F04LeaveCabalLeaveLastMemberPotNotEmpty(s *scenario.Scenario) {
	s.Given(append(founded("last"), holding(testkit.USDCMint, 1))...).
		When(scenario.Delete(leavePath)).
		Then(guarded(errs.CodeLeaveLastMemberPotNotEmpty, func(*scenario.Scenario) string { return "1" })...)
}

func F04LeaveCabalLeaveCreatorWithMembers(s *scenario.Scenario) {
	s.Given(joined("creator")...).
		When(scenario.SignIn(founderOf("creator")), scenario.Delete(leavePath)).
		Then(refused(errs.CodeLeaveCreatorWithMembers, events.TypeCabalMemberLeft, 0)...)
}

func F04LeaveCabalPriceUnavailable(s *scenario.Scenario) {
	s.Given(append(founded("unpriced"), holding(unpricedMint, 1))...).
		When(scenario.Delete(leavePath)).
		Then(refused(errs.CodePriceUnavailable, events.TypeCabalMemberLeft, 0)...)
}

func F04LeaveCabalCrashBeforeCommit(s *scenario.Scenario) {
	s.Given(joined("crash")...).
		When(
			scenario.Delete(leavePath),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusNoContent),
		).
		Then(left()...)
}
