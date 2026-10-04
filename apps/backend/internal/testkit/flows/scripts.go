package flows

import (
	"maps"

	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

type Script func(*scenario.Scenario)

func Scripts() map[string]Script {
	scripts := identityScripts()
	maps.Copy(scripts, leaveScripts())
	maps.Copy(scripts, map[string]Script{
		"F00RecordPingOK":                          F00RecordPingOK,
		"F00RecordPingInvalidInput":                F00RecordPingInvalidInput,
		"F00RecordPingUnauthorized":                F00RecordPingUnauthorized,
		"F00RecordPingCrashAfterPublish":           F00RecordPingCrashAfterPublish,
		"F02CreateCabalOK":                         F02CreateCabalOK,
		"F02CreateCabalInvalidInput":               F02CreateCabalInvalidInput,
		"F02CreateCabalUnauthorized":               F02CreateCabalUnauthorized,
		"F02CreateCabalPrivyUnavailable":           F02CreateCabalPrivyUnavailable,
		"F02CreateCabalCrashBeforeCommit":          F02CreateCabalCrashBeforeCommit,
		"F03JoinCabalOK":                           F03JoinCabalOK,
		"F03JoinCabalUnauthorized":                 F03JoinCabalUnauthorized,
		"F03JoinCabalCabalNotFound":                F03JoinCabalCabalNotFound,
		"F03JoinCabalCabalBanned":                  F03JoinCabalCabalBanned,
		"F03JoinCabalAlreadyMember":                F03JoinCabalAlreadyMember,
		"F03JoinCabalJoinNeedsRequest":             F03JoinCabalJoinNeedsRequest,
		"F03RequestAccessOK":                       F03RequestAccessOK,
		"F03RequestAccessUnauthorized":             F03RequestAccessUnauthorized,
		"F03RequestAccessCabalNotFound":            F03RequestAccessCabalNotFound,
		"F03RequestAccessCabalBanned":              F03RequestAccessCabalBanned,
		"F03RequestAccessAlreadyMember":            F03RequestAccessAlreadyMember,
		"F03RequestAccessRequestNotNeeded":         F03RequestAccessRequestNotNeeded,
		"F03RequestAccessRequestPending":           F03RequestAccessRequestPending,
		"F03DecideAccessOK":                        F03DecideAccessOK,
		"F03DecideAccessInvalidInput":              F03DecideAccessInvalidInput,
		"F03DecideAccessUnauthorized":              F03DecideAccessUnauthorized,
		"F03DecideAccessCabalNotFound":             F03DecideAccessCabalNotFound,
		"F03DecideAccessNotCabalCreator":           F03DecideAccessNotCabalCreator,
		"F03DecideAccessAccessRequestNotPending":   F03DecideAccessAccessRequestNotPending,
		"F03DecideAccessCabalBanned":               F03DecideAccessCabalBanned,
		"F03RevokeAccessOK":                        F03RevokeAccessOK,
		"F03RevokeAccessUnauthorized":              F03RevokeAccessUnauthorized,
		"F03RevokeAccessCabalNotFound":             F03RevokeAccessCabalNotFound,
		"F03RevokeAccessAccessRequestNotPending":   F03RevokeAccessAccessRequestNotPending,
		"F03RevokeAccessCannotRevokeAccess":        F03RevokeAccessCannotRevokeAccess,
		"F05CreditDepositOK":                       F05CreditDepositOK,
		"F05CreditDepositRPCUnavailable":           F05CreditDepositRPCUnavailable,
		"F18SamplePricesOK":                        F18SamplePricesOK,
		"F18SamplePricesJupiterUnavailable":        F18SamplePricesJupiterUnavailable,
		"F18SamplePricesUpstreamTimeout":           F18SamplePricesUpstreamTimeout,
		"F18SamplePricesCrashBeforeCommit":         F18SamplePricesCrashBeforeCommit,
		"F20FollowOK":                              F20FollowOK,
		"F20FollowCannotFollowSelf":                F20FollowCannotFollowSelf,
		"F20FollowUserNotFound":                    F20FollowUserNotFound,
		"F20FollowUserBanned":                      F20FollowUserBanned,
		"F20FollowUnauthorized":                    F20FollowUnauthorized,
		"F20FollowCrashBeforeCommit":               F20FollowCrashBeforeCommit,
		"F20UnfollowOK":                            F20UnfollowOK,
		"F23UpdateProfileOK":                       F23UpdateProfileOK,
		"F23UpdateProfileDisplayNameInvalid":       F23UpdateProfileDisplayNameInvalid,
		"F23aUploadProfilePhotoOK":                 F23aUploadProfilePhotoOK,
		"F23aUploadProfilePhotoPhotoInvalid":       F23aUploadProfilePhotoPhotoInvalid,
		"F23aUploadProfilePhotoStorageUnavailable": F23aUploadProfilePhotoStorageUnavailable,
		"F23aUploadProfilePhotoRateLimited":        F23aUploadProfilePhotoRateLimited,
	})
	maps.Copy(scripts, f10Scripts())
	maps.Copy(scripts, f13Scripts())
	maps.Copy(scripts, f28Scripts())
	maps.Copy(scripts, f03InviteScripts())
	return scripts
}

func identityScripts() map[string]Script {
	return map[string]Script{
		"F01OpenSessionOK":                     F01OpenSessionOK,
		"F01OpenSessionUnauthorized":           F01OpenSessionUnauthorized,
		"F01OpenSessionLoginMethodNotAllowed":  F01OpenSessionLoginMethodNotAllowed,
		"F01OpenSessionAccountDeleted":         F01OpenSessionAccountDeleted,
		"F01OpenSessionPrivyUnavailable":       F01OpenSessionPrivyUnavailable,
		"F01OpenSessionCrashBeforeCommit":      F01OpenSessionCrashBeforeCommit,
		"F01aSetHandleOK":                      F01aSetHandleOK,
		"F01aSetHandleHandleInvalid":           F01aSetHandleHandleInvalid,
		"F01aSetHandleHandleReserved":          F01aSetHandleHandleReserved,
		"F01aSetHandleHandleTaken":             F01aSetHandleHandleTaken,
		"F01aSetHandleHandleTooSoon":           F01aSetHandleHandleTooSoon,
		"F01bLinkPhoneOK":                      F01bLinkPhoneOK,
		"F01bLinkPhoneHandleRequired":          F01bLinkPhoneHandleRequired,
		"F01bLinkPhonePhoneNotLinked":          F01bLinkPhonePhoneNotLinked,
		"F01bLinkPhonePrivyUnavailable":        F01bLinkPhonePrivyUnavailable,
		"F01cLinkSocialsOK":                    F01cLinkSocialsOK,
		"F01cLinkSocialsHandleRequired":        F01cLinkSocialsHandleRequired,
		"F01cLinkSocialsXNotLinked":            F01cLinkSocialsXNotLinked,
		"F01cLinkSocialsPrivyUnavailable":      F01cLinkSocialsPrivyUnavailable,
		"F01dSkipOnboardingStepOK":             F01dSkipOnboardingStepOK,
		"F01dSkipOnboardingStepHandleRequired": F01dSkipOnboardingStepHandleRequired,
		"F01dSkipOnboardingStepInvalidInput":   F01dSkipOnboardingStepInvalidInput,
		"F01eDeleteAccountOK":                  F01eDeleteAccountOK,
		"F01eDeleteAccountAccountHasPositions": F01eDeleteAccountAccountHasPositions,
		"F01eDeleteAccountAccountHasBalance":   F01eDeleteAccountAccountHasBalance,
		"F01eDeleteAccountCrashBeforeCommit":   F01eDeleteAccountCrashBeforeCommit,
	}
}

func leaveScripts() map[string]Script {
	return map[string]Script{
		"F04LeaveCabalOK":                         F04LeaveCabalOK,
		"F04LeaveCabalUnauthorized":               F04LeaveCabalUnauthorized,
		"F04LeaveCabalNotCabalMember":             F04LeaveCabalNotCabalMember,
		"F04LeaveCabalLeaveHoldsShares":           F04LeaveCabalLeaveHoldsShares,
		"F04LeaveCabalLeaveLastMemberPotNotEmpty": F04LeaveCabalLeaveLastMemberPotNotEmpty,
		"F04LeaveCabalLeaveCreatorWithMembers":    F04LeaveCabalLeaveCreatorWithMembers,
		"F04LeaveCabalPriceUnavailable":           F04LeaveCabalPriceUnavailable,
		"F04LeaveCabalCrashBeforeCommit":          F04LeaveCabalCrashBeforeCommit,
	}
}

func Env() map[string][]string {
	return map[string][]string{
		"05": {"FUNDING_DEPOSIT_POLL_INTERVAL=2s"},
		"18": {"MARKET_PRICE_POLL_INTERVAL=2s", "MONACO_TIMEOUT_JUPITER_QUOTE=1s"},
		"28": {"IDENTITY_NUDGES_INTERVAL=1s"},
	}
}
