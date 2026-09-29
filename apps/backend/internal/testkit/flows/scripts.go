package flows

import "github.com/monaco/monaco/apps/backend/internal/testkit/scenario"

type Script func(*scenario.Scenario)

func Scripts() map[string]Script {
	return map[string]Script{
		"F00RecordPingOK":                F00RecordPingOK,
		"F00RecordPingInvalidInput":      F00RecordPingInvalidInput,
		"F00RecordPingUnauthorized":      F00RecordPingUnauthorized,
		"F00RecordPingCrashAfterPublish": F00RecordPingCrashAfterPublish,
	}
}
