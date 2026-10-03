package domain

import "testing"

func TestNudgeKindFor_namesTheMissingStep(t *testing.T) {
	t.Parallel()
	if got := NudgeKindFor(AuthAwaitingPhone); got != NudgeAddPhone {
		t.Errorf("NudgeKindFor(AWAITING_PHONE) = %q, want add_phone", got)
	}
	if got := NudgeKindFor(AuthAwaitingSocials); got != NudgeLinkX {
		t.Errorf("NudgeKindFor(AWAITING_SOCIALS) = %q, want link_x", got)
	}
}
