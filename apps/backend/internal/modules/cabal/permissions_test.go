package cabal_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type cast struct {
	creator, member, outsider, applicant, inviter, invitee ids.UserID
}

func newCast(t *testing.T) cast {
	t.Helper()
	g := testkit.NewIDs(testkit.RandSeed(t))
	user := func() ids.UserID {
		id, err := ids.ParseUserID(g.NewV7().String())
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	return cast{creator: user(), member: user(), outsider: user(), applicant: user(), inviter: user(), invitee: user()}
}

func (c cast) cabal(t *testing.T, join string) domain.Cabal {
	t.Helper()
	a := baseRules()
	a.join = join
	rules, err := a.build()
	if err != nil {
		t.Fatal(err)
	}
	return domain.Cabal{CreatorID: c.creator, Rules: rules}
}

func (c cast) request() domain.AccessRequest {
	return domain.AccessRequest{Direction: domain.DirectionRequest, UserID: c.applicant}
}

func (c cast) invite() domain.AccessRequest {
	return domain.AccessRequest{Direction: domain.DirectionInvite, UserID: c.invitee, InvitedBy: c.inviter}
}

func member(u ids.UserID) domain.Actor { return domain.Actor{UserID: u, Member: true} }

func nobody(u ids.UserID) domain.Actor { return domain.Actor{UserID: u} }

type permissionCase struct {
	name  string
	actor domain.Actor
	want  errs.Code
}

func runPermissionCases(t *testing.T, cases []permissionCase, check func(domain.Actor) error) {
	t.Helper()
	for _, tt := range cases {
		err := check(tt.actor)
		if tt.want == "" {
			if err != nil {
				t.Errorf("%s: %v, want it allowed", tt.name, err)
			}
			continue
		}
		wantCode(t, tt.name, err, tt.want)
	}
}

func TestCanInvite_onlyTheCreatorInvitesWhateverTheJoinMode(t *testing.T) {
	t.Parallel()
	c := newCast(t)
	runPermissionCases(t, []permissionCase{
		{"the creator", member(c.creator), ""},
		{"a member", member(c.member), errs.CodeNotCabalCreator},
		{"a non-member", nobody(c.outsider), errs.CodeNotCabalMember},
		{"a creator who is no longer a member", nobody(c.creator), errs.CodeNotCabalMember},
	}, func(a domain.Actor) error { return domain.CanInvite(a, c.cabal(t, "open")) })
	runPermissionCases(t, []permissionCase{
		{"the creator", member(c.creator), ""},
		{"a member", member(c.member), errs.CodeNotCabalCreator},
		{"a non-member", nobody(c.outsider), errs.CodeNotCabalMember},
		{"a creator who is no longer a member", nobody(c.creator), errs.CodeNotCabalMember},
	}, func(a domain.Actor) error { return domain.CanInvite(a, c.cabal(t, "request")) })
}

func TestCanDecide_theCreatorDecidesRequestsAndTheInviteeDecidesTheirInvite(t *testing.T) {
	t.Parallel()
	c := newCast(t)
	cabal := c.cabal(t, "request")
	runPermissionCases(t, []permissionCase{
		{"the creator", member(c.creator), ""},
		{"a member", member(c.member), errs.CodeNotCabalCreator},
		{"the requester", nobody(c.applicant), errs.CodeNotCabalCreator},
		{"a stranger", nobody(c.outsider), errs.CodeNotCabalCreator},
	}, func(a domain.Actor) error { return domain.CanDecide(a, cabal, c.request()) })
	runPermissionCases(t, []permissionCase{
		{"the invitee", nobody(c.invitee), ""},
		{"the creator", member(c.creator), errs.CodeForbidden},
		{"the inviter", member(c.inviter), errs.CodeForbidden},
		{"a stranger", nobody(c.outsider), errs.CodeForbidden},
	}, func(a domain.Actor) error { return domain.CanDecide(a, cabal, c.invite()) })
	unknown := domain.AccessRequest{Direction: "ask", UserID: c.applicant}
	wantCode(t, "an unknown direction", domain.CanDecide(member(c.creator), cabal, unknown), errs.CodeInternal)
}

func TestCanRevoke_theRequesterWithdrawsAndTheInviterOrCreatorRevokes(t *testing.T) {
	t.Parallel()
	c := newCast(t)
	cabal := c.cabal(t, "open")
	runPermissionCases(t, []permissionCase{
		{"the requester", nobody(c.applicant), ""},
		{"the creator", member(c.creator), errs.CodeCannotRevokeAccess},
		{"a member", member(c.member), errs.CodeCannotRevokeAccess},
	}, func(a domain.Actor) error { return domain.CanRevoke(a, cabal, c.request()) })
	runPermissionCases(t, []permissionCase{
		{"the inviter", member(c.inviter), ""},
		{"the creator", member(c.creator), ""},
		{"the invitee", nobody(c.invitee), errs.CodeCannotRevokeAccess},
		{"another member", member(c.member), errs.CodeCannotRevokeAccess},
		{"a stranger", nobody(c.outsider), errs.CodeCannotRevokeAccess},
	}, func(a domain.Actor) error { return domain.CanRevoke(a, cabal, c.invite()) })
	unknown := domain.AccessRequest{Direction: "ask", UserID: c.applicant}
	wantCode(t, "an unknown direction", domain.CanRevoke(member(c.creator), cabal, unknown), errs.CodeInternal)
}

func TestCabalRoleOf_theCreatorOfRecordIsTheCreatorAndEveryoneElseIsAMember(t *testing.T) {
	t.Parallel()
	c := newCast(t)
	cabal := c.cabal(t, "open")
	for _, tt := range []struct {
		name string
		user ids.UserID
		want domain.Role
	}{
		{"the creator", c.creator, domain.RoleCreator},
		{"a member", c.member, domain.RoleMember},
		{"an outsider", c.outsider, domain.RoleMember},
	} {
		if got := cabal.RoleOf(tt.user); got != tt.want {
			t.Errorf("RoleOf(%s) = %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestCabalRoleOf_aReturningCreatorVotesInAListCabal(t *testing.T) {
	t.Parallel()
	c := newCast(t)
	a := baseRules()
	a.voters = "list"
	rules, err := a.build()
	if err != nil {
		t.Fatal(err)
	}
	cabal := domain.Cabal{CreatorID: c.creator, Rules: rules}
	if !domain.VoterFor(cabal.Rules, cabal.RoleOf(c.creator)) || domain.VoterFor(cabal.Rules, cabal.RoleOf(c.member)) {
		t.Fatal("in a list cabal the creator must vote by the role RoleOf gives and a member must not")
	}
}

func TestCanJoin_refusesABannedCabalThenAMemberThenARequestCabal(t *testing.T) {
	t.Parallel()
	c := newCast(t)
	open, request := c.cabal(t, "open"), c.cabal(t, "request")
	banned := open
	banned.Banned = true
	for _, tt := range []struct {
		name  string
		cabal domain.Cabal
		cases []permissionCase
	}{
		{"open", open, []permissionCase{
			{"outsider joins", nobody(c.outsider), ""},
			{"member", member(c.member), errs.CodeAlreadyMember},
		}},
		{"request", request, []permissionCase{
			{"outsider must ask", nobody(c.outsider), errs.CodeJoinNeedsRequest},
			{"member before mode", member(c.member), errs.CodeAlreadyMember},
		}},
		{"banned", banned, []permissionCase{
			{"outsider", nobody(c.outsider), errs.CodeCabalBanned},
			{"member", member(c.member), errs.CodeCabalBanned},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runPermissionCases(t, tt.cases, func(a domain.Actor) error { return domain.CanJoin(a, tt.cabal) })
		})
	}
}

func TestCanRequest_refusesABannedCabalThenAMemberThenAnOpenCabal(t *testing.T) {
	t.Parallel()
	c := newCast(t)
	open, request := c.cabal(t, "open"), c.cabal(t, "request")
	banned := request
	banned.Banned = true
	for _, tt := range []struct {
		name  string
		cabal domain.Cabal
		cases []permissionCase
	}{
		{"request", request, []permissionCase{
			{"outsider asks", nobody(c.outsider), ""},
			{"member", member(c.member), errs.CodeAlreadyMember},
		}},
		{"open", open, []permissionCase{
			{"outsider joins instead", nobody(c.outsider), errs.CodeRequestNotNeeded},
			{"member before mode", member(c.member), errs.CodeAlreadyMember},
		}},
		{"banned", banned, []permissionCase{
			{"outsider", nobody(c.outsider), errs.CodeCabalBanned},
			{"member", member(c.member), errs.CodeCabalBanned},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runPermissionCases(t, tt.cases, func(a domain.Actor) error { return domain.CanRequest(a, tt.cabal) })
		})
	}
}

func TestCanAdmit_refusesABannedCabalAndAnExpiredInvite(t *testing.T) {
	t.Parallel()
	c := newCast(t)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	active := c.cabal(t, "request")
	banned := active
	banned.Banned = true
	fresh, stale := c.invite(), c.invite()
	fresh.ExpiresAt, stale.ExpiresAt = now.Add(time.Hour), now.Add(-time.Second)
	for _, tt := range []struct {
		name  string
		cabal domain.Cabal
		req   domain.AccessRequest
		want  errs.Code
	}{
		{"a request", active, c.request(), ""},
		{"a fresh invite", active, fresh, ""},
		{"an expired invite", active, stale, errs.CodeInviteExpired},
		{"a banned cabal", banned, c.request(), errs.CodeCabalBanned},
	} {
		err := domain.CanAdmit(tt.cabal, tt.req, now)
		if tt.want == "" && err != nil || tt.want != "" && errs.CodeOf(err) != tt.want {
			t.Errorf("%s: %v, want %q", tt.name, err, tt.want)
		}
	}
}
