package app

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type HeldCabals interface {
	CabalsOf(ctx context.Context, user ids.UserID) ([]ids.CabalID, error)
	CabalViews
}

type HeldPots interface {
	CabalPots(ctx context.Context, cabalIDs []ids.CabalID) ([]port.CabalPot, error)
}

type HeldLogos interface {
	LogosBySymbol(ctx context.Context) (map[string]string, error)
}

type HeldQuery struct {
	Viewer ids.UserID
	Symbol string
}

type HeldReads struct {
	cabals HeldCabals
	pots   HeldPots
	logos  HeldLogos
	clock  clock.Clock
}

func NewHeldReads(cabals HeldCabals, pots HeldPots, logos HeldLogos, c clock.Clock) *HeldReads {
	return &HeldReads{cabals: cabals, pots: pots, logos: logos, clock: c}
}

func (h *HeldReads) HeldBy(ctx context.Context, viewer ids.UserID) (port.HeldList, error) {
	return h.Handle(ctx, HeldQuery{Viewer: viewer})
}

func (h *HeldReads) Handle(ctx context.Context, q HeldQuery) (port.HeldList, error) {
	const op = "treasury.HeldReads.Handle"
	active, views, err := h.activeCabals(ctx, q.Viewer)
	if err != nil {
		return port.HeldList{}, err
	}
	b := heldBuilder{
		list: port.HeldList{PricesAsOf: h.clock.Now(), Holdings: []port.HeldRow{}}, seen: map[ids.CabalID]bool{},
	}
	if len(active) == 0 {
		return b.list, nil
	}
	pots, err := h.pots.CabalPots(ctx, active)
	if err != nil {
		return port.HeldList{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if b.logos, err = h.logos.LogosBySymbol(ctx); err != nil {
		return port.HeldList{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	for i, pot := range pots {
		if err := b.addPot(views[active[i]], pot, q.Symbol); err != nil {
			return port.HeldList{}, errs.Wrap(err, errs.CodeOf(err), op)
		}
	}
	return b.finish(), nil
}

type heldBuilder struct {
	list  port.HeldList
	logos map[string]string
	seen  map[ids.CabalID]bool
	asOf  time.Time
}

func (b *heldBuilder) addPot(view cabalport.CabalView, pot port.CabalPot, symbol string) error {
	for _, holding := range pot.Holdings {
		if symbol != "" && holding.Symbol != symbol {
			continue
		}
		total, err := b.list.TotalValueMicros.Add(holding.ValueMicros)
		if err != nil {
			return err
		}
		b.list.TotalValueMicros = total
		if !b.seen[view.ID] {
			b.seen[view.ID] = true
			b.list.CabalCount++
		}
		if b.asOf.IsZero() || pot.PricesAsOf.Before(b.asOf) {
			b.asOf = pot.PricesAsOf
		}
		b.list.Holdings = append(b.list.Holdings, port.HeldRow{
			Cabal:   port.HeldCabal{ID: view.ID, Name: view.Name, PictureURL: view.PictureURL},
			LogoURL: b.logos[holding.Symbol], Holding: holding,
		})
	}
	return nil
}

func (b *heldBuilder) finish() port.HeldList {
	slices.SortFunc(b.list.Holdings, compareHeld)
	if !b.asOf.IsZero() {
		b.list.PricesAsOf = b.asOf
	}
	return b.list
}

func (h *HeldReads) activeCabals(
	ctx context.Context, viewer ids.UserID,
) ([]ids.CabalID, map[ids.CabalID]cabalport.CabalView, error) {
	const op = "treasury.HeldReads.activeCabals"
	mine, err := h.cabals.CabalsOf(ctx, viewer)
	if err != nil {
		return nil, nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if len(mine) == 0 {
		return nil, nil, nil
	}
	views, err := h.cabals.Cabals(ctx, mine)
	if err != nil {
		return nil, nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	active := make([]ids.CabalID, 0, len(mine))
	for _, id := range mine {
		if view, ok := views[id]; ok && view.Status == cabalport.StatusActive {
			active = append(active, id)
		}
	}
	return active, views, nil
}

func compareHeld(a, b port.HeldRow) int {
	if c := b.Holding.ValueMicros.Cmp(a.Holding.ValueMicros); c != 0 {
		return c
	}
	if c := strings.Compare(a.Holding.Symbol, b.Holding.Symbol); c != 0 {
		return c
	}
	return strings.Compare(a.Cabal.ID.String(), b.Cabal.ID.String())
}

var _ port.Held = (*HeldReads)(nil)
