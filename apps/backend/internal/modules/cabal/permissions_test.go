package cabal_test

import (
	"testing"

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

func TestCanInvite_anyMemberInAnOpenCabalAndOnlyTheCreatorInARequestCabal(t *testing.T) {
	t.Parallel()
	c := newCast(t)
	runPermissionCases(t, []permissionCase{
		{"the creator", member(c.creator), ""},
		{"a member", member(c.member), ""},
		{"a non-member", nobody(c.outsider), errs.CodeNotCabalMember},
		{"a creator who is no longer a member", nobody(c.creator), errs.CodeNotCabalMember},
	}, func(a domain.Actor) error { return domain.CanInvite(a, c.cabal(t, "open")) })
	runPermissionCases(t, []permissionCase{
		{"the creator", member(c.creator), ""},
		{"a member", member(c.member), errs.CodeNotCabalCreator},
		{"a non-member", nobody(c.outsider), errs.CodeNotCabalMember},
		{"a creator who is no longer a member", nobody(c.creator), errs.CodeNotCabalMember},
	}, func(a domain.Actor) error { return domain.CanInvite(a, c.cabal(t, "request")) })
	wantCode(
		t,
		"a cabal with no join mode",
		domain.CanInvite(member(c.creator), domain.Cabal{CreatorID: c.creator}),
		errs.CodeInternal,
	)
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
