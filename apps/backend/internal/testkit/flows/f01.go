package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	session             = "/v1/auth/session"
	me                  = "/v1/me"
	privyQA1            = "did:privy:qa-1"
	privyNoLogin        = "did:privy:qa-none"
	privyDeleted        = "did:privy:qa-deleted"
	privyOutage         = "did:privy:qa-outage"
	privyCrash          = "did:privy:qa-crash"
	privyProfileOK      = "did:privy:qa-profile-ok"
	privyProfileInvalid = "did:privy:qa-profile-invalid"
	privyPhotoOK        = "did:privy:qa-photo-ok"
	privyPhotoInvalid   = "did:privy:qa-photo-invalid"
	privyPhotoStorage   = "did:privy:qa-photo-storage"
	privyPhotoRate      = "did:privy:qa-photo-rate"
	privyUserPath       = "/privy/v1/users/"
	privyAttempts       = 3
)

func F01OpenSessionOK(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(
			scenario.SignIn(privyQA1),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("auth_state", "CREATED"),
			scenario.ExpectJSON("account_status", "active"),
			scenario.ExpectJSON("phone_linked", false),
			scenario.Remember("id", "user"),
			scenario.SignIn(privyQA1),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectRemembered("id", "user"),
		).
		Then(
			scenario.ExpectEvents(events.TypeUserCreated, 1),
			scenario.ExpectEvents(events.TypeUserAuthStateChanged, 0),
			scenario.EventuallyPublished(events.TypeUserCreated, 1),
			scenario.Get(me),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectRemembered("id", "user"),
		)
}

func F01OpenSessionUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Post(session, "")).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized), scenario.ExpectEvents(events.TypeUserCreated, 0))
}

func F01OpenSessionLoginMethodNotAllowed(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.SignIn(privyNoLogin)).
		Then(scenario.ExpectProblem(errs.CodeLoginMethodNotAllowed), scenario.ExpectEvents(events.TypeUserCreated, 0))
}

func F01OpenSessionAccountDeleted(s *scenario.Scenario) {
	s.Given(seedDeletedUser(privyDeleted)).
		When(scenario.SignIn(privyDeleted)).
		Then(scenario.ExpectProblem(errs.CodeAccountDeleted), scenario.ExpectEvents(events.TypeUserCreated, 0))
}

func F01OpenSessionPrivyUnavailable(s *scenario.Scenario) {
	s.Given(scenario.Anonymous(), scenario.FakeUpstream(fakes.Step{
		Route: privyUserPath + privyOutage, Action: fakes.ActionFail, Status: http.StatusServiceUnavailable,
		Times: privyAttempts,
	})).
		When(scenario.SignIn(privyOutage)).
		Then(scenario.ExpectProblem(errs.CodePrivyUnavailable), scenario.ExpectEvents(events.TypeUserCreated, 0))
}

func F01OpenSessionCrashBeforeCommit(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(
			scenario.SignIn(privyCrash),
			scenario.SignIn(privyCrash),
			scenario.ExpectStatus(http.StatusOK),
			scenario.Remember("id", "user"),
		).
		Then(
			scenario.ExpectEvents(events.TypeUserCreated, 1),
			scenario.EventuallyPublished(events.TypeUserCreated, 1),
			scenario.Get(me),
			scenario.ExpectRemembered("id", "user"),
		)
}

func seedDeletedUser(privyUserID string) scenario.Step {
	return func(s *scenario.Scenario) {
		_, err := s.DB().Exec(s.Context(), `INSERT INTO users (id, privy_user_id, login_provider, account_status,
			auth_state_changed_at, created_at, updated_at, deleted_at)
			VALUES ($1, $2, 'sms', 'deleted', now(), now(), now(), now())`, ids.Real{}.NewV7(), privyUserID)
		if err != nil {
			s.Fatalf("flows: seed the deleted user %s: %v", privyUserID, err)
		}
	}
}
