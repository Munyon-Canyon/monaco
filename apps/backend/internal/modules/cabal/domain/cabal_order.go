package domain

import (
	"log/slog"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const MaxOrderedCabals = 100

type Membership struct {
	CabalID   ids.CabalID
	SortIndex int32
}

func ParseCabalOrder(requested []ids.CabalID) error {
	const op = "cabal.ParseCabalOrder"
	if len(requested) > MaxOrderedCabals {
		return errs.New(errs.CodeInvalidInput, op, slog.String("reason", "too_many"))
	}
	seen := make(map[ids.CabalID]bool, len(requested))
	for _, id := range requested {
		if seen[id] {
			return errs.New(errs.CodeInvalidInput, op, slog.String("reason", "duplicate"))
		}
		seen[id] = true
	}
	return nil
}

func Reorder(current []Membership, requested []ids.CabalID) []Membership {
	owned := make(map[ids.CabalID]bool, len(current))
	for _, m := range current {
		owned[m.CabalID] = true
	}
	order := make([]ids.CabalID, 0, len(current))
	for _, id := range requested {
		if owned[id] && !slices.Contains(order, id) {
			order = append(order, id)
		}
	}
	for _, m := range current {
		if !slices.Contains(order, m.CabalID) {
			order = append(order, m.CabalID)
		}
	}
	out := make([]Membership, len(order))
	for i, id := range order {
		out[i] = Membership{CabalID: id, SortIndex: int32(i)}
	}
	return out
}
