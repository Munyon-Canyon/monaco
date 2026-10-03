package fakes

import (
	"context"
	"slices"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type Pauses struct {
	testkit.Faults
	mu      sync.Mutex
	global  bool
	reasons []funding.PauseReason
	cabals  map[ids.CabalID][]funding.PauseReason
}

var _ funding.Pauses = (*Pauses)(nil)

func NewPauses() *Pauses { return &Pauses{cabals: map[ids.CabalID][]funding.PauseReason{}} }

func (f *Pauses) Pause(cabalID ids.CabalID, reasons ...funding.PauseReason) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cabals[cabalID] = append(f.cabals[cabalID], reasons...)
}

func (f *Pauses) PauseAll(reasons ...funding.PauseReason) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.global = true
	f.reasons = append(f.reasons, reasons...)
}

func (f *Pauses) Resume(cabalID ids.CabalID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.cabals, cabalID)
}

func (f *Pauses) ResumeAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.global = false
	f.reasons = nil
}

func (f *Pauses) IsPaused(_ context.Context, cabalID ids.CabalID) (funding.Pause, error) {
	if err := f.Check("IsPaused"); err != nil {
		return funding.Pause{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	own, ok := f.cabals[cabalID]
	if !ok && !f.global {
		return funding.Pause{}, nil
	}
	return funding.Pause{Paused: true, Reasons: slices.Concat(f.reasons, own)}, nil
}

func (f *Pauses) PausedCabals(context.Context) (funding.PausedSet, error) {
	if err := f.Check("PausedCabals"); err != nil {
		return funding.PausedSet{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cabals := make(map[ids.CabalID][]funding.PauseReason, len(f.cabals))
	for id, reasons := range f.cabals {
		cabals[id] = slices.Clone(reasons)
	}
	return funding.PausedSet{Global: f.global, Cabals: cabals}, nil
}
