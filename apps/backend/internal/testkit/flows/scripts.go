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
	}
}

func Env() map[string][]string {
	return map[string][]string{}
}
