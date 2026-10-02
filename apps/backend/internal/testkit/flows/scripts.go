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

		"F10CastVoteOK":                F10CastVoteOK,
		"F10CastVoteUnauthorized":      F10CastVoteUnauthorized,
		"F10CastVoteProposalNotFound":  F10CastVoteProposalNotFound,
		"F10CastVoteNotAVoter":         F10CastVoteNotAVoter,
		"F10CastVoteProposalClosed":    F10CastVoteProposalClosed,
		"F10CastVoteCrashAfterPublish": F10CastVoteCrashAfterPublish,
	}
}

func Env() map[string][]string {
	return map[string][]string{}
}
