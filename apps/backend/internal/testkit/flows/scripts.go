package flows

import "github.com/monaco/monaco/apps/backend/internal/testkit/scenario"

type Script func(*scenario.Scenario)

func Scripts() map[string]Script {
	return map[string]Script{
		"F00RecordPingOK":                F00RecordPingOK,
		"F00RecordPingInvalidInput":      F00RecordPingInvalidInput,
		"F00RecordPingUnauthorized":      F00RecordPingUnauthorized,
		"F00RecordPingCrashAfterPublish": F00RecordPingCrashAfterPublish,

		"F01OpenSessionOK":                    F01OpenSessionOK,
		"F01OpenSessionUnauthorized":          F01OpenSessionUnauthorized,
		"F01OpenSessionLoginMethodNotAllowed": F01OpenSessionLoginMethodNotAllowed,
		"F01OpenSessionAccountDeleted":        F01OpenSessionAccountDeleted,
		"F01OpenSessionPrivyUnavailable":      F01OpenSessionPrivyUnavailable,
		"F01OpenSessionCrashBeforeCommit":     F01OpenSessionCrashBeforeCommit,
		"F01aSetHandleOK":                     F01aSetHandleOK,
		"F01aSetHandleHandleInvalid":          F01aSetHandleHandleInvalid,
		"F01aSetHandleHandleReserved":         F01aSetHandleHandleReserved,
		"F01aSetHandleHandleTaken":            F01aSetHandleHandleTaken,
		"F01aSetHandleHandleTooSoon":          F01aSetHandleHandleTooSoon,

		"F02CreateCabalOK":                         F02CreateCabalOK,
		"F02CreateCabalInvalidInput":               F02CreateCabalInvalidInput,
		"F02CreateCabalUnauthorized":               F02CreateCabalUnauthorized,
		"F02CreateCabalPrivyUnavailable":           F02CreateCabalPrivyUnavailable,
		"F02CreateCabalCrashBeforeCommit":          F02CreateCabalCrashBeforeCommit,
		"F10CastVoteOK":                            F10CastVoteOK,
		"F10CastVoteUnauthorized":                  F10CastVoteUnauthorized,
		"F10CastVoteProposalNotFound":              F10CastVoteProposalNotFound,
		"F10CastVoteNotAVoter":                     F10CastVoteNotAVoter,
		"F10CastVoteProposalClosed":                F10CastVoteProposalClosed,
		"F10CastVoteCrashAfterPublish":             F10CastVoteCrashAfterPublish,
		"F18SamplePricesOK":                        F18SamplePricesOK,
		"F18SamplePricesJupiterUnavailable":        F18SamplePricesJupiterUnavailable,
		"F18SamplePricesUpstreamTimeout":           F18SamplePricesUpstreamTimeout,
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
	}
}

func Env() map[string][]string {
	return map[string][]string{
		"18": {"MARKET_PRICE_POLL_INTERVAL=2s", "MONACO_TIMEOUT_JUPITER_QUOTE=1s"},
	}
}
