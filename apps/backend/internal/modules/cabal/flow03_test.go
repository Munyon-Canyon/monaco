package cabal_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestFlow03_JoinCabal_OK(t *testing.T) {
	t.Parallel()
	flows.F03JoinCabalOK(cabalScenario(t))
}

func TestFlow03_JoinCabal_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F03JoinCabalUnauthorized(cabalScenario(t))
}

func TestFlow03_JoinCabal_CabalNotFound(t *testing.T) {
	t.Parallel()
	flows.F03JoinCabalCabalNotFound(cabalScenario(t))
}

func TestFlow03_JoinCabal_CabalBanned(t *testing.T) {
	t.Parallel()
	flows.F03JoinCabalCabalBanned(cabalScenario(t))
}

func TestFlow03_JoinCabal_AlreadyMember(t *testing.T) {
	t.Parallel()
	flows.F03JoinCabalAlreadyMember(cabalScenario(t))
}

func TestFlow03_JoinCabal_JoinNeedsRequest(t *testing.T) {
	t.Parallel()
	flows.F03JoinCabalJoinNeedsRequest(cabalScenario(t))
}

func TestFlow03_RequestAccess_OK(t *testing.T) {
	t.Parallel()
	flows.F03RequestAccessOK(cabalScenario(t))
}

func TestFlow03_RequestAccess_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F03RequestAccessUnauthorized(cabalScenario(t))
}

func TestFlow03_RequestAccess_CabalNotFound(t *testing.T) {
	t.Parallel()
	flows.F03RequestAccessCabalNotFound(cabalScenario(t))
}

func TestFlow03_RequestAccess_CabalBanned(t *testing.T) {
	t.Parallel()
	flows.F03RequestAccessCabalBanned(cabalScenario(t))
}

func TestFlow03_RequestAccess_AlreadyMember(t *testing.T) {
	t.Parallel()
	flows.F03RequestAccessAlreadyMember(cabalScenario(t))
}

func TestFlow03_RequestAccess_RequestNotNeeded(t *testing.T) {
	t.Parallel()
	flows.F03RequestAccessRequestNotNeeded(cabalScenario(t))
}

func TestFlow03_RequestAccess_RequestPending(t *testing.T) {
	t.Parallel()
	flows.F03RequestAccessRequestPending(cabalScenario(t))
}

func TestFlow03_DecideAccess_OK(t *testing.T) {
	t.Parallel()
	flows.F03DecideAccessOK(cabalScenario(t))
}

func TestFlow03_DecideAccess_InvalidInput(t *testing.T) {
	t.Parallel()
	flows.F03DecideAccessInvalidInput(cabalScenario(t))
}

func TestFlow03_DecideAccess_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F03DecideAccessUnauthorized(cabalScenario(t))
}

func TestFlow03_DecideAccess_CabalNotFound(t *testing.T) {
	t.Parallel()
	flows.F03DecideAccessCabalNotFound(cabalScenario(t))
}

func TestFlow03_DecideAccess_NotCabalCreator(t *testing.T) {
	t.Parallel()
	flows.F03DecideAccessNotCabalCreator(cabalScenario(t))
}

func TestFlow03_DecideAccess_AccessRequestNotPending(t *testing.T) {
	t.Parallel()
	flows.F03DecideAccessAccessRequestNotPending(cabalScenario(t))
}

func TestFlow03_DecideAccess_CabalBanned(t *testing.T) {
	t.Parallel()
	flows.F03DecideAccessCabalBanned(cabalScenario(t))
}

func TestFlow03_RevokeAccess_OK(t *testing.T) {
	t.Parallel()
	flows.F03RevokeAccessOK(cabalScenario(t))
}

func TestFlow03_RevokeAccess_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F03RevokeAccessUnauthorized(cabalScenario(t))
}

func TestFlow03_RevokeAccess_CabalNotFound(t *testing.T) {
	t.Parallel()
	flows.F03RevokeAccessCabalNotFound(cabalScenario(t))
}

func TestFlow03_RevokeAccess_AccessRequestNotPending(t *testing.T) {
	t.Parallel()
	flows.F03RevokeAccessAccessRequestNotPending(cabalScenario(t))
}

func TestFlow03_RevokeAccess_CannotRevokeAccess(t *testing.T) {
	t.Parallel()
	flows.F03RevokeAccessCannotRevokeAccess(cabalScenario(t))
}

func TestFlow03_InviteMember_OK(t *testing.T) {
	t.Parallel()
	flows.F03InviteMemberOK(cabalScenario(t))
}

func TestFlow03_InviteMember_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F03InviteMemberUnauthorized(cabalScenario(t))
}

func TestFlow03_InviteMember_CabalNotFound(t *testing.T) {
	t.Parallel()
	flows.F03InviteMemberCabalNotFound(cabalScenario(t))
}

func TestFlow03_InviteMember_UserNotFound(t *testing.T) {
	t.Parallel()
	flows.F03InviteMemberUserNotFound(cabalScenario(t))
}

func TestFlow03_InviteMember_NotCabalMember(t *testing.T) {
	t.Parallel()
	flows.F03InviteMemberNotCabalMember(cabalScenario(t))
}

func TestFlow03_InviteMember_NotCabalCreator(t *testing.T) {
	t.Parallel()
	flows.F03InviteMemberNotCabalCreator(cabalScenario(t))
}

func TestFlow03_InviteMember_CabalBanned(t *testing.T) {
	t.Parallel()
	flows.F03InviteMemberCabalBanned(cabalScenario(t))
}

func TestFlow03_InviteMember_AlreadyMember(t *testing.T) {
	t.Parallel()
	flows.F03InviteMemberAlreadyMember(cabalScenario(t))
}

func TestFlow03_InviteMember_RequestPending(t *testing.T) {
	t.Parallel()
	flows.F03InviteMemberRequestPending(cabalScenario(t))
}

func TestFlow03_DecideAccess_InviteExpired(t *testing.T) {
	t.Parallel()
	flows.F03DecideAccessInviteExpired(cabalScenario(t))
}
