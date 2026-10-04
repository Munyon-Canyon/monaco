package flows

import (
	"net/http"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	invitesPath  = cabalsPath + "/{cabal}/invites"
	myInvitesURL = "/v1/me/cabal-invites"
)

func inviteeOf(script string) string { return "did:privy:qa-f03-" + script + "-invitee" }

func handleOf(script string) string { return "f03_" + strings.ReplaceAll(script, "-", "_") }

func inviteBody(script string) string { return `{"handle":"` + handleOf(script) + `"}` }

func named(sub, handle string) []scenario.Step {
	return []scenario.Step{
		scenario.SignIn(sub),
		scenario.Put(setHandlePath, `{"handle":"`+handle+`"}`),
		scenario.ExpectStatus(http.StatusOK),
	}
}

func inviting(script, join string) []scenario.Step {
	return append(named(inviteeOf(script), handleOf(script)), opened(script, join, "all")...)
}

func invited(script string) []scenario.Step {
	return append(inviting(script, "request"),
		scenario.Post(invitesPath, inviteBody(script)),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.Remember("id", "request"),
	)
}

func F03InviteMemberOK(s *scenario.Scenario) {
	s.Given(inviting("inv-ok", "request")...).
		When(
			scenario.Post(invitesPath, inviteBody("inv-ok")),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectJSON("direction", "invite"),
			scenario.ExpectJSON("status", "pending"),
			scenario.Remember("id", "request"),
			scenario.Replay(),
		).
		Then(
			scenario.ExpectEvents(events.TypeCabalAccessRequested, 1),
			scenario.EventuallyPublished(events.TypeCabalAccessRequested, 1),
			scenario.Get(invitesPath+"?status=pending"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.SignIn(inviteeOf("inv-ok")),
			scenario.EventuallyHint("cabal_invites"),
			scenario.Get(myInvitesURL),
			scenario.ExpectStatus(http.StatusOK),
			scenario.Post(decisionPath, approveBody),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "approved"),
			scenario.ExpectEvents(events.TypeCabalMemberJoined, 2),
			scenario.Get(cabalsPath+"/{cabal}"),
			scenario.ExpectJSON("me", map[string]any{"role": "member", "can_vote": true}),
		)
}

func F03InviteMemberUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Post(missingCabal+"/invites", `{"handle":"anyone"}`)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F03InviteMemberCabalNotFound(s *scenario.Scenario) {
	s.Given(named(inviteeOf("inv-missing"), handleOf("inv-missing"))...).
		When(scenario.Post(missingCabal+"/invites", inviteBody("inv-missing"))).
		Then(refused(errs.CodeCabalNotFound, events.TypeCabalAccessRequested, 0)...)
}

func F03InviteMemberUserNotFound(s *scenario.Scenario) {
	s.Given(opened("inv-nobody", "open", "all")...).
		When(scenario.Post(invitesPath, inviteBody("inv-nobody"))).
		Then(refused(errs.CodeUserNotFound, events.TypeCabalAccessRequested, 0)...)
}

func F03InviteMemberNotCabalMember(s *scenario.Scenario) {
	s.Given(inviting("inv-outsider", "open")...).
		When(scenario.SignIn(outsiderOf("inv-outsider")), scenario.Post(invitesPath, inviteBody("inv-outsider"))).
		Then(refused(errs.CodeNotCabalMember, events.TypeCabalAccessRequested, 0)...)
}

func F03InviteMemberNotCabalCreator(s *scenario.Scenario) {
	s.Given(append(inviting("inv-member", "open"),
		scenario.SignIn(outsiderOf("inv-member")),
		scenario.Post(membersPath, ""),
		scenario.ExpectStatus(http.StatusOK),
		scenario.SignIn(creatorOf("inv-member")),
		scenario.Patch(cabalsPath+"/{cabal}", `{"join_mode":"request"}`),
		scenario.ExpectStatus(http.StatusOK),
	)...).
		When(scenario.SignIn(outsiderOf("inv-member")), scenario.Post(invitesPath, inviteBody("inv-member"))).
		Then(refused(errs.CodeNotCabalCreator, events.TypeCabalAccessRequested, 0)...)
}

func F03InviteMemberCabalBanned(s *scenario.Scenario) {
	s.Given(append(inviting("inv-banned", "open"), banned())...).
		When(scenario.Post(invitesPath, inviteBody("inv-banned"))).
		Then(refused(errs.CodeCabalBanned, events.TypeCabalAccessRequested, 0)...)
}

func F03InviteMemberAlreadyMember(s *scenario.Scenario) {
	s.Given(append(opened("inv-self", "open", "all"),
		scenario.Put(setHandlePath, `{"handle":"`+handleOf("inv-self")+`"}`),
		scenario.ExpectStatus(http.StatusOK),
	)...).
		When(scenario.Post(invitesPath, inviteBody("inv-self"))).
		Then(refused(errs.CodeAlreadyMember, events.TypeCabalAccessRequested, 0)...)
}

func F03InviteMemberRequestPending(s *scenario.Scenario) {
	s.Given(append(inviting("inv-asked", "request"),
		scenario.SignIn(inviteeOf("inv-asked")),
		scenario.Post(requestsPath, ""),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.SignIn(creatorOf("inv-asked")),
	)...).
		When(scenario.Post(invitesPath, inviteBody("inv-asked"))).
		Then(refused(errs.CodeRequestPending, events.TypeCabalAccessRequested, 1)...)
}

func pastDue() scenario.Step {
	return func(s *scenario.Scenario) {
		if _, err := s.DB().Exec(s.Context(), `UPDATE cabal_access_requests
			SET created_at = now() - interval '8 days', expires_at = now() - interval '1 day' WHERE id = $1`,
			s.Recall("request")); err != nil {
			s.Fatalf("move the invite past its expiry: %v", err)
		}
	}
}

func F03DecideAccessInviteExpired(s *scenario.Scenario) {
	s.Given(append(invited("inv-late"), pastDue())...).
		When(scenario.SignIn(inviteeOf("inv-late")), scenario.Post(decisionPath, approveBody)).
		Then(
			scenario.ExpectProblem(errs.CodeInviteExpired),
			scenario.ExpectEvents(events.TypeCabalAccessDecided, 1),
			scenario.ExpectEventPayload(events.TypeCabalAccessDecided, map[string]any{"decision": "expired"}),
			scenario.ExpectEvents(events.TypeCabalMemberJoined, 1),
		)
}
