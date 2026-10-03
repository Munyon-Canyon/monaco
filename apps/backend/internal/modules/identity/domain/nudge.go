package domain

import "time"

type NudgeKind string

const (
	NudgeAddPhone NudgeKind = "add_phone"
	NudgeLinkX    NudgeKind = "link_x"
)

const (
	NudgeSettle       = 24 * time.Hour
	NudgeGap          = 7 * 24 * time.Hour
	MaxNudges   int16 = 3
)

func NudgeKindFor(awaiting AuthState) NudgeKind {
	if awaiting == AuthAwaitingPhone {
		return NudgeAddPhone
	}
	return NudgeLinkX
}
