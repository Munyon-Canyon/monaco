package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const photoPurgePoller = "identity.photo_purges"

const (
	deleteOK      = "did:privy:qa-del-ok"
	deleteOther   = "did:privy:qa-del-ok-other"
	deleteStaked  = "did:privy:qa-del-staked"
	deleteFunded  = "did:privy:qa-del-funded"
	deleteCrash   = "did:privy:qa-del-crash"
	deletedHandle = "del_gone"
)

func (defined) WorkerEnvF01e() []string { return []string{"IDENTITY_PHOTO_PURGES_INTERVAL=1s"} }

func F01eDeleteAccountOK(s *scenario.Scenario) {
	s.Given(withHandle(deleteOK, deletedHandle)...).
		When(scenario.Delete(me), scenario.ExpectStatus(http.StatusNoContent)).
		Then(
			actorLogged(observability.IdentityAccountDeleted),
			scenario.ExpectEvents(events.TypeUserDeleted, 1),
			scenario.EventuallyPublished(events.TypeUserDeleted, 1),
			photoPurged(deleteOK),
			scenario.SignIn(deleteOK),
			scenario.ExpectProblem(errs.CodeAccountDeleted),
			scenario.SignIn(deleteOther),
			scenario.ExpectStatus(http.StatusOK),
			scenario.Put(setHandlePath, `{"handle":"`+deletedHandle+`"}`),
			scenario.ExpectProblem(errs.CodeHandleTaken),
		)
}

func F01eDeleteAccountAccountHasPositions(s *scenario.Scenario) {
	deleteRefused(s, deleteStaked, errs.CodeAccountHasPositions, staked)
}

func F01eDeleteAccountAccountHasBalance(s *scenario.Scenario) {
	deleteRefused(s, deleteFunded, errs.CodeAccountHasBalance)
}

func staked(s *scenario.Scenario) {
	cabal := testkit.NewCabal(s, s.DB(), testkit.WithJoiner(s.ActorID()))
	testkit.NewLedger(s, s.DB()).WithFundedMember(s.ActorID(), cabal.ID, money.MicrosFromUint64(1_000_000))
}

func F01eDeleteAccountCrashBeforeCommit(s *scenario.Scenario) {
	s.Given(scenario.SignIn(deleteCrash), scenario.ExpectStatus(http.StatusOK)).
		When(scenario.Delete(me), scenario.Retry(), scenario.ExpectStatus(http.StatusNoContent)).
		Then(
			scenario.ExpectEvents(events.TypeUserDeleted, 1),
			scenario.EventuallyPublished(events.TypeUserDeleted, 1),
			scenario.SignIn(deleteCrash),
			scenario.ExpectProblem(errs.CodeAccountDeleted),
		)
}

func photoPurged(sub string) scenario.Step {
	return scenario.EventuallyPolled(photoPurgePoller, "photo_purged_at set for "+sub, func(s *scenario.Scenario) bool {
		var purged bool
		if err := s.DB().QueryRow(s.Context(),
			`SELECT photo_purged_at IS NOT NULL FROM users WHERE privy_user_id = $1`, sub).Scan(&purged); err != nil {
			s.Fatalf("flows: read photo_purged_at for %s: %v", sub, err)
		}
		return purged
	})
}

func deleteRefused(s *scenario.Scenario, sub string, code errs.Code, seeds ...scenario.Step) {
	s.Given(append([]scenario.Step{scenario.SignIn(sub), scenario.ExpectStatus(http.StatusOK)}, seeds...)...).
		When(scenario.Delete(me)).
		Then(
			scenario.ExpectProblem(code),
			scenario.ExpectEvents(events.TypeUserDeleted, 0),
			scenario.Get(me),
			scenario.ExpectStatus(http.StatusOK),
		)
}
