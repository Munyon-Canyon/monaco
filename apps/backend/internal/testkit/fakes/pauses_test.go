package fakes

import (
	"errors"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func newCabalID() ids.CabalID { return ids.CabalIDFrom(ids.Real{}.NewV7()) }

func wantPaused(t *testing.T, pauses *Pauses, cabal ids.CabalID, want []funding.PauseReason) {
	t.Helper()
	got, err := pauses.IsPaused(t.Context(), cabal)
	if err != nil || !got.Paused || !slices.Equal(got.Reasons, want) {
		t.Fatalf("IsPaused = (%+v, %v), want paused with %v", got, err, want)
	}
}

func wantNotPaused(t *testing.T, pauses *Pauses, cabal ids.CabalID) {
	t.Helper()
	if got, err := pauses.IsPaused(t.Context(), cabal); err != nil || got.Paused || got.Reasons != nil {
		t.Fatalf("IsPaused = (%+v, %v), want not paused", got, err)
	}
}

func wantSet(t *testing.T, pauses *Pauses, global bool, cabals map[ids.CabalID][]funding.PauseReason) {
	t.Helper()
	set, err := pauses.PausedCabals(t.Context())
	if err != nil || set.Global != global || len(set.Cabals) != len(cabals) {
		t.Fatalf("PausedCabals = (%+v, %v), want global=%v with %v", set, err, global, cabals)
	}
	for id, reasons := range cabals {
		if !slices.Equal(set.Cabals[id], reasons) {
			t.Fatalf("PausedCabals[%v] = %v, want %v", id, set.Cabals[id], reasons)
		}
	}
}

func TestPauses_CabalPause(t *testing.T) {
	t.Parallel()
	pauses := NewPauses()
	paused, other := newCabalID(), newCabalID()
	pauses.Pause(paused, funding.PauseReasonExternalDeposit)
	pauses.Pause(paused, funding.PauseReasonOps)
	reasons := []funding.PauseReason{funding.PauseReasonExternalDeposit, funding.PauseReasonOps}
	wantPaused(t, pauses, paused, reasons)
	wantNotPaused(t, pauses, other)
	wantSet(t, pauses, false, map[ids.CabalID][]funding.PauseReason{paused: reasons})
	pauses.Resume(paused)
	wantNotPaused(t, pauses, paused)
	wantSet(t, pauses, false, nil)
}

func TestPauses_PauseAllPausesACabalWithNoRowOfItsOwn(t *testing.T) {
	t.Parallel()
	pauses := NewPauses()
	bare := newCabalID()
	pauses.PauseAll()
	wantPaused(t, pauses, bare, nil)
	wantSet(t, pauses, true, nil)
}

func TestPauses_GlobalReasonsComeBeforeTheCabalsOwn(t *testing.T) {
	t.Parallel()
	pauses := NewPauses()
	own := newCabalID()
	pauses.Pause(own, funding.PauseReasonExternalDeposit)
	pauses.PauseAll(funding.PauseReasonOps)
	wantPaused(t, pauses, own, []funding.PauseReason{funding.PauseReasonOps, funding.PauseReasonExternalDeposit})
	wantSet(t, pauses, true, map[ids.CabalID][]funding.PauseReason{own: {funding.PauseReasonExternalDeposit}})
}

func TestPauses_ResumeAllKeepsCabalPauses(t *testing.T) {
	t.Parallel()
	pauses := NewPauses()
	own, bare := newCabalID(), newCabalID()
	pauses.Pause(own, funding.PauseReasonExternalDeposit)
	pauses.PauseAll(funding.PauseReasonOps)
	pauses.ResumeAll()
	wantNotPaused(t, pauses, bare)
	wantPaused(t, pauses, own, []funding.PauseReason{funding.PauseReasonExternalDeposit})
	wantSet(t, pauses, false, map[ids.CabalID][]funding.PauseReason{own: {funding.PauseReasonExternalDeposit}})
}

func TestPauses_Faults(t *testing.T) {
	t.Parallel()
	pauses := NewPauses()
	once, always := errors.New("once"), errors.New("always")
	pauses.FailOnce("IsPaused", once)
	pauses.Fail("IsPaused", always)
	pauses.Fail("PausedCabals", always)
	cabal := newCabalID()
	if _, err := pauses.IsPaused(t.Context(), cabal); !errors.Is(err, once) {
		t.Fatalf("first IsPaused error = %v, want %v", err, once)
	}
	if _, err := pauses.IsPaused(t.Context(), cabal); !errors.Is(err, always) {
		t.Fatalf("second IsPaused error = %v, want %v", err, always)
	}
	if _, err := pauses.PausedCabals(t.Context()); !errors.Is(err, always) {
		t.Fatalf("PausedCabals error = %v, want %v", err, always)
	}
}
