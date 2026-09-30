package marketfake

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

var _ market.Calendar = (*CalendarFake)(nil)

type CalendarFake struct {
	testkit.Faults

	mu     sync.Mutex
	states map[market.AssetID]market.SessionInfo
}

func NewCalendar() *CalendarFake { return &CalendarFake{} }

func (f *CalendarFake) SetState(asset market.AssetID, info market.SessionInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.states == nil {
		f.states = map[market.AssetID]market.SessionInfo{}
	}
	f.states[asset] = info
}

func (f *CalendarFake) Session(_ context.Context, id market.AssetID, _ time.Time) (market.SessionInfo, error) {
	if err := f.Check("Session"); err != nil {
		return market.SessionInfo{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	info, ok := f.states[id]
	if !ok {
		return market.SessionInfo{}, errs.New(
			errs.CodeAssetNotFound,
			"marketfake.Session",
			slog.String("id", id.String()),
		)
	}
	return info, nil
}
