package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	membersPath   = cabalsPath + "/{cabal}/members"
	requestsPath  = cabalsPath + "/{cabal}/access-requests"
	requestPath   = requestsPath + "/{request}"
	decisionPath  = requestPath + "/decision"
	missingCabal  = cabalsPath + "/01890a5d-ac96-774b-bcce-000000000003"
	missingAccess = missingCabal + "/access-requests/01890a5d-ac96-774b-bcce-000000000004"
	approveBody   = `{"decision":"approve"}`
	denyBody      = `{"decision":"deny"}`
)

func creatorOf(script string) string { return "did:privy:qa-f03-" + script + "-creator" }

func outsiderOf(script string) string { return "did:privy:qa-f03-" + script + "-outsider" }

func cabalOf(join, voters string) string {
	return `{"name":"Friends pot","join_mode":"` + join + `","voter_mode":"` + voters +
		`","threshold":"majority","proposal_expiry_seconds":86400}`
}

func opened(script, join, voters string) []scenario.Step {
	return []scenario.Step{
		scenario.SignIn(creatorOf(script)),
		scenario.Post(cabalsPath, cabalOf(join, voters)),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.Remember("id", "cabal"),
	}
}

func asked(script string) []scenario.Step {
	return append(opened(script, "request", "all"),
		scenario.SignIn(outsiderOf(script)),
		scenario.Post(requestsPath, ""),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.Remember("id", "request"),
	)
}

func banned() scenario.Step {
	return func(s *scenario.Scenario) {
		if _, err := s.DB().Exec(s.Context(), `UPDATE cabals SET status = 'banned' WHERE id = $1`,
			s.Recall("cabal")); err != nil {
			s.Fatalf("ban the cabal: %v", err)
		}
	}
}

func refused(code errs.Code, typ events.Type, n int) []scenario.Step {
	return []scenario.Step{scenario.ExpectProblem(code), scenario.ExpectEvents(typ, n)}
}

func F03JoinCabalOK(s *scenario.Scenario) {
	s.Given(opened("join-ok", "open", "all")...).
		When(
			scenario.SignIn(outsiderOf("join-ok")),
			scenario.Post(membersPath, ""),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("member_count", 2),
			scenario.ExpectJSON("me", map[string]any{"role": "member", "can_vote": true}),
			scenario.Replay(),
		).
		Then(
			scenario.ExpectEvents(events.TypeCabalMemberJoined, 2),
			scenario.EventuallyPublished(events.TypeCabalMemberJoined, 2),
			scenario.EventuallyHint("cabal_access"),
		)
}

func F03JoinCabalUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Post(missingCabal+"/members", "")).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F03JoinCabalCabalNotFound(s *scenario.Scenario) {
	s.Given(scenario.SignIn(outsiderOf("join-missing"))).
		When(scenario.Post(missingCabal+"/members", "")).
		Then(refused(errs.CodeCabalNotFound, events.TypeCabalMemberJoined, 0)...)
}

func F03JoinCabalCabalBanned(s *scenario.Scenario) {
	s.Given(append(opened("join-banned", "open", "all"), banned())...).
		When(scenario.SignIn(outsiderOf("join-banned")), scenario.Post(membersPath, "")).
		Then(refused(errs.CodeCabalBanned, events.TypeCabalMemberJoined, 1)...)
}

func F03JoinCabalAlreadyMember(s *scenario.Scenario) {
	s.Given(opened("join-member", "open", "all")...).
		When(scenario.Post(membersPath, "")).
		Then(refused(errs.CodeAlreadyMember, events.TypeCabalMemberJoined, 1)...)
}

func F03JoinCabalJoinNeedsRequest(s *scenario.Scenario) {
	s.Given(opened("join-request", "request", "all")...).
		When(scenario.SignIn(outsiderOf("join-request")), scenario.Post(membersPath, "")).
		Then(refused(errs.CodeJoinNeedsRequest, events.TypeCabalMemberJoined, 1)...)
}

func F03RequestAccessOK(s *scenario.Scenario) {
	s.Given(opened("request-ok", "request", "all")...).
		When(
			scenario.SignIn(outsiderOf("request-ok")),
			scenario.Post(requestsPath, ""),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectJSON("status", "pending"),
			scenario.Replay(),
		).
		Then(
			scenario.ExpectEvents(events.TypeCabalAccessRequested, 1),
			scenario.EventuallyPublished(events.TypeCabalAccessRequested, 1),
			scenario.SignIn(creatorOf("request-ok")),
			scenario.Get(requestsPath+"?status=pending"),
			scenario.ExpectStatus(http.StatusOK),
		)
}

func F03RequestAccessUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Post(missingCabal+"/access-requests", "")).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F03RequestAccessCabalNotFound(s *scenario.Scenario) {
	s.Given(scenario.SignIn(outsiderOf("request-missing"))).
		When(scenario.Post(missingCabal+"/access-requests", "")).
		Then(refused(errs.CodeCabalNotFound, events.TypeCabalAccessRequested, 0)...)
}

func F03RequestAccessCabalBanned(s *scenario.Scenario) {
	s.Given(append(opened("request-banned", "request", "all"), banned())...).
		When(scenario.SignIn(outsiderOf("request-banned")), scenario.Post(requestsPath, "")).
		Then(refused(errs.CodeCabalBanned, events.TypeCabalAccessRequested, 0)...)
}

func F03RequestAccessAlreadyMember(s *scenario.Scenario) {
	s.Given(opened("request-member", "request", "all")...).
		When(scenario.Post(requestsPath, "")).
		Then(refused(errs.CodeAlreadyMember, events.TypeCabalAccessRequested, 0)...)
}

func F03RequestAccessRequestNotNeeded(s *scenario.Scenario) {
	s.Given(opened("request-open", "open", "all")...).
		When(scenario.SignIn(outsiderOf("request-open")), scenario.Post(requestsPath, "")).
		Then(refused(errs.CodeRequestNotNeeded, events.TypeCabalAccessRequested, 0)...)
}

func F03RequestAccessRequestPending(s *scenario.Scenario) {
	s.Given(asked("request-twice")...).
		When(scenario.Post(requestsPath, "")).
		Then(refused(errs.CodeRequestPending, events.TypeCabalAccessRequested, 1)...)
}

func F03DecideAccessOK(s *scenario.Scenario) {
	s.Given(append(opened("decide-ok", "request", "list"),
		scenario.SignIn(outsiderOf("decide-ok")),
		scenario.Post(requestsPath, ""),
		scenario.Remember("id", "request"),
	)...).
		When(
			scenario.SignIn(creatorOf("decide-ok")),
			scenario.Post(decisionPath, approveBody),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "approved"),
			scenario.Replay(),
		).
		Then(
			scenario.ExpectEvents(events.TypeCabalAccessDecided, 1),
			scenario.ExpectEvents(events.TypeCabalMemberJoined, 2),
			scenario.EventuallyPublished(events.TypeCabalMemberJoined, 2),
			scenario.SignIn(outsiderOf("decide-ok")),
			scenario.EventuallyHint("cabal_access"),
			scenario.Get(cabalsPath+"/{cabal}"),
			scenario.ExpectJSON("me", map[string]any{"role": "member", "can_vote": false}),
		)
}

func F03DecideAccessInvalidInput(s *scenario.Scenario) {
	s.Given(asked("decide-invalid")...).
		When(scenario.SignIn(creatorOf("decide-invalid")), scenario.Post(decisionPath, `{"decision":"maybe"}`)).
		Then(refused(errs.CodeInvalidInput, events.TypeCabalAccessDecided, 0)...)
}

func F03DecideAccessUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Post(missingAccess+"/decision", approveBody)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F03DecideAccessCabalNotFound(s *scenario.Scenario) {
	s.Given(scenario.SignIn(creatorOf("decide-missing"))).
		When(scenario.Post(missingAccess+"/decision", approveBody)).
		Then(refused(errs.CodeCabalNotFound, events.TypeCabalAccessDecided, 0)...)
}

func F03DecideAccessNotCabalCreator(s *scenario.Scenario) {
	s.Given(asked("decide-self")...).
		When(scenario.Post(decisionPath, approveBody)).
		Then(refused(errs.CodeNotCabalCreator, events.TypeCabalAccessDecided, 0)...)
}

func F03DecideAccessAccessRequestNotPending(s *scenario.Scenario) {
	s.Given(append(asked("decide-twice"),
		scenario.SignIn(creatorOf("decide-twice")),
		scenario.Post(decisionPath, denyBody),
		scenario.ExpectStatus(http.StatusOK),
	)...).
		When(scenario.Post(decisionPath, approveBody)).
		Then(refused(errs.CodeAccessRequestNotPending, events.TypeCabalAccessDecided, 1)...)
}

func F03DecideAccessCabalBanned(s *scenario.Scenario) {
	s.Given(append(asked("decide-banned"), banned())...).
		When(scenario.SignIn(creatorOf("decide-banned")), scenario.Post(decisionPath, approveBody)).
		Then(refused(errs.CodeCabalBanned, events.TypeCabalMemberJoined, 1)...)
}

func F03RevokeAccessOK(s *scenario.Scenario) {
	s.Given(asked("revoke-ok")...).
		When(
			scenario.Delete(requestPath),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "revoked"),
			scenario.Replay(),
		).
		Then(
			scenario.ExpectEvents(events.TypeCabalAccessDecided, 1),
			scenario.EventuallyPublished(events.TypeCabalAccessDecided, 1),
		)
}

func F03RevokeAccessUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Delete(missingAccess)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F03RevokeAccessCabalNotFound(s *scenario.Scenario) {
	s.Given(scenario.SignIn(outsiderOf("revoke-missing"))).
		When(scenario.Delete(missingAccess)).
		Then(refused(errs.CodeCabalNotFound, events.TypeCabalAccessDecided, 0)...)
}

func F03RevokeAccessAccessRequestNotPending(s *scenario.Scenario) {
	s.Given(append(asked("revoke-twice"), scenario.Delete(requestPath), scenario.ExpectStatus(http.StatusOK))...).
		When(scenario.Delete(requestPath)).
		Then(refused(errs.CodeAccessRequestNotPending, events.TypeCabalAccessDecided, 1)...)
}

func F03RevokeAccessCannotRevokeAccess(s *scenario.Scenario) {
	s.Given(asked("revoke-creator")...).
		When(scenario.SignIn(creatorOf("revoke-creator")), scenario.Delete(requestPath)).
		Then(refused(errs.CodeCannotRevokeAccess, events.TypeCabalAccessDecided, 0)...)
}
