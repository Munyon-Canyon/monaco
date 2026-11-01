package domain_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type adminCast struct {
	creator, admin, inviter, member, outsider, applicant ids.UserID
}

func newAdminCast(t *testing.T) (adminCast, domain.Cabal) {
	t.Helper()
	user := func() ids.UserID { return ids.UserIDFrom(ids.Real{}.NewV7()) }
	cast := adminCast{
		creator: user(), admin: user(), inviter: user(), member: user(), outsider: user(), applicant: user(),
	}
	rules, err := domain.NewRules("request", "list", "majority", 86400, 100)
	if err != nil {
		t.Fatal(err)
	}
	return cast, domain.Cabal{
		Creators: []ids.UserID{cast.creator},
		Admins:   []ids.UserID{cast.creator, cast.admin},
		Rules:    rules,
	}
}

type doorCase struct {
	who   string
	actor domain.Actor
	want  errs.Code
}

func codeOf(err error) errs.Code {
	if err == nil {
		return ""
	}
	return errs.CodeOf(err)
}

func inCabal(u ids.UserID) domain.Actor { return domain.Actor{UserID: u, Member: true} }

func outside(u ids.UserID) domain.Actor { return domain.Actor{UserID: u} }

func TestAdminPermissions_everyDoorPermissionAcrossTheCast(t *testing.T) {
	t.Parallel()
	c, cabal := newAdminCast(t)
	request := domain.AccessRequest{Direction: domain.DirectionRequest, UserID: c.applicant}
	invite := domain.AccessRequest{Direction: domain.DirectionInvite, UserID: c.outsider, InvitedBy: c.inviter}
	for _, tt := range []struct {
		permission string
		check      func(domain.Actor) error
		cases      []doorCase
	}{
		{"invite", func(a domain.Actor) error { return domain.CanInvite(a, cabal) }, []doorCase{
			{"creator", inCabal(c.creator), ""},
			{"admin", inCabal(c.admin), ""},
			{"member inviter", inCabal(c.inviter), errs.CodeNotCabalAdmin},
			{"unrelated member", inCabal(c.member), errs.CodeNotCabalAdmin},
			{"non-member", outside(c.outsider), errs.CodeNotCabalMember},
		}},
		{"decide a request", func(a domain.Actor) error { return domain.CanDecide(a, cabal, request) }, []doorCase{
			{"creator", inCabal(c.creator), ""},
			{"admin", inCabal(c.admin), ""},
			{"member inviter", inCabal(c.inviter), errs.CodeNotCabalAdmin},
			{"unrelated member", inCabal(c.member), errs.CodeNotCabalAdmin},
			{"non-member", outside(c.outsider), errs.CodeNotCabalAdmin},
		}},
		{"revoke an invite", func(a domain.Actor) error { return domain.CanRevoke(a, cabal, invite) }, []doorCase{
			{"creator", inCabal(c.creator), ""},
			{"admin", inCabal(c.admin), ""},
			{"member inviter", inCabal(c.inviter), errs.CodeCannotRevokeAccess},
			{"unrelated member", inCabal(c.member), errs.CodeCannotRevokeAccess},
			{"non-member", outside(c.outsider), errs.CodeCannotRevokeAccess},
		}},
		{"revoke a request", func(a domain.Actor) error { return domain.CanRevoke(a, cabal, request) }, []doorCase{
			{"creator", inCabal(c.creator), errs.CodeCannotRevokeAccess},
			{"admin", inCabal(c.admin), errs.CodeCannotRevokeAccess},
			{"member inviter", inCabal(c.inviter), errs.CodeCannotRevokeAccess},
			{"unrelated member", inCabal(c.member), errs.CodeCannotRevokeAccess},
			{"the requester", outside(c.applicant), ""},
		}},
	} {
		for _, tc := range tt.cases {
			if got := codeOf(tt.check(tc.actor)); got != tc.want {
				t.Errorf("%s by %s = %q, want %q", tt.permission, tc.who, got, tc.want)
			}
		}
	}
}

func TestAdminPermissions_adminsEditNeitherNameNorPictureNorLeave(t *testing.T) {
	t.Parallel()
	c, cabal := newAdminCast(t)
	admin := domain.Actor{UserID: c.admin, Member: true}
	if got := errs.CodeOf(domain.CanEdit(admin, cabal)); got != errs.CodeNotCabalCreator {
		t.Errorf("CanEdit by an admin = %q, want %q", got, errs.CodeNotCabalCreator)
	}
	if err := domain.CanEdit(domain.Actor{UserID: c.creator, Member: true}, cabal); err != nil {
		t.Errorf("CanEdit by the creator = %v, want it allowed", err)
	}
}

func TestAdminRoles_theCreatorIsAnAdminAndAdminsAlwaysVote(t *testing.T) {
	t.Parallel()
	c, cabal := newAdminCast(t)
	for _, tt := range []struct {
		who  string
		user ids.UserID
		role domain.Role
	}{
		{"creator", c.creator, domain.RoleCreator},
		{"admin", c.admin, domain.RoleAdmin},
		{"member", c.member, domain.RoleMember},
		{"outsider", c.outsider, domain.RoleMember},
	} {
		if got := cabal.RoleOf(tt.user); got != tt.role {
			t.Errorf("RoleOf(%s) = %s, want %s", tt.who, got, tt.role)
		}
		if got := cabal.IsAdmin(tt.user); got != tt.role.IsAdmin() {
			t.Errorf("IsAdmin(%s) = %v, want %v", tt.who, got, tt.role.IsAdmin())
		}
		if got := domain.VoterFor(cabal.Rules, tt.role); got != tt.role.IsAdmin() {
			t.Errorf("VoterFor(%s) in a list cabal = %v, want %v", tt.who, got, tt.role.IsAdmin())
		}
	}
	if !(domain.Cabal{Creators: []ids.UserID{c.creator}}).IsAdmin(c.creator) {
		t.Error("a cabal built from its creator alone does not count the creator as an admin")
	}
}

func TestRoles_aUserWithoutAMemberRowHasNoRole(t *testing.T) {
	t.Parallel()
	c, _ := newAdminCast(t)
	empty := domain.Cabal{}
	if empty.IsCreator(c.creator) || empty.IsAdmin(c.creator) {
		t.Error("a cabal with no member rows gives a user creator or admin power")
	}
	if got := empty.RoleOf(c.creator); got != domain.RoleMember {
		t.Errorf("RoleOf with no member row = %s, want %s", got, domain.RoleMember)
	}
	if err := domain.CanEdit(domain.Actor{UserID: c.creator}, empty); errs.CodeOf(err) != errs.CodeNotCabalCreator {
		t.Errorf("CanEdit with no member row = %v, want %s", err, errs.CodeNotCabalCreator)
	}
	if err := domain.CanDecide(
		domain.Actor{UserID: c.creator},
		empty,
		domain.AccessRequest{Direction: domain.DirectionRequest},
	); errs.CodeOf(
		err,
	) != errs.CodeNotCabalAdmin {
		t.Errorf("CanDecide with no member row = %v, want %s", err, errs.CodeNotCabalAdmin)
	}
	invite := domain.AccessRequest{Direction: domain.DirectionInvite, InvitedBy: c.creator}
	if err := domain.CanRevoke(
		domain.Actor{UserID: c.creator},
		empty,
		invite,
	); errs.CodeOf(
		err,
	) != errs.CodeCannotRevokeAccess {
		t.Errorf("CanRevoke with no member row = %v, want %s", err, errs.CodeCannotRevokeAccess)
	}
}

func TestVotersWithAdmins_addsEveryAdminButLeavesTheCreatorToBeChecked(t *testing.T) {
	t.Parallel()
	c, cabal := newAdminCast(t)
	requested := []ids.UserID{c.member}
	got := cabal.VotersWithAdmins(requested)
	if want := []ids.UserID{c.member, c.admin}; !slices.Equal(got, want) {
		t.Errorf("VotersWithAdmins = %v, want %v with the creator left out", got, want)
	}
	if len(requested) != 1 {
		t.Errorf("the requested list became %v, want it left alone", requested)
	}
	again := cabal.VotersWithAdmins([]ids.UserID{c.admin, c.member})
	if want := []ids.UserID{c.admin, c.member}; !slices.Equal(again, want) {
		t.Errorf("VotersWithAdmins with the admin listed = %v, want %v", again, want)
	}
}
