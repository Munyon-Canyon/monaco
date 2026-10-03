//go:build faultpoints

package identity_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

func TestFlow28_EmitNudges_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	r := newNudgeRig(t)
	phone := seedNudgeRow(t, r.pool, r.clock.Now(), nudgeRow{state: "AWAITING_PHONE", changedAgo: 25 * time.Hour})
	socials := seedNudgeRow(t, r.pool, r.clock.Now(), nudgeRow{state: "AWAITING_SOCIALS", changedAgo: 25 * time.Hour})
	crashed := func() (panicValue any) {
		defer func() { panicValue = recover() }()
		_, _ = r.poller.Tick(faultpoint.Armed(nudgeCtx(t), faultpoint.BeforeCommit))
		return nil
	}()
	if crashed != (faultpoint.Crash{Name: faultpoint.BeforeCommit}) {
		t.Fatalf("crash = %v, want before-commit on the page", crashed)
	}
	if n := nudgeEventCount(t, r.pool); n != 0 {
		t.Fatalf("user.nudge_due events after the crash = %d, want 0", n)
	}
	if report := r.tick(t); report.Scanned != 2 || report.Changed != 2 {
		t.Fatalf("retry = %+v, want both missed nudges", report)
	}
	if report := r.tick(t); report.Changed != 0 {
		t.Fatalf("third tick = %+v, want no duplicate", report)
	}
	for user, kind := range map[uuid.UUID]string{phone: "add_phone", socials: "link_x"} {
		got := nudgesFor(t, r.pool, user)
		if len(got) != 1 || got[0].Kind != kind || got[0].NudgeNumber != 1 {
			t.Fatalf("nudges for %s = %+v, want one %s nudge", user, got, kind)
		}
	}
}
