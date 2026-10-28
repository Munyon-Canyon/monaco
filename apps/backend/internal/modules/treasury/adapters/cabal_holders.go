package adapters

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type heldByCabal interface {
	HeldByCabal(ctx context.Context) ([]sqlc.HeldByCabalRow, error)
	CabalsHoldingAsset(ctx context.Context, asset string) ([]uuid.UUID, error)
}

type ActiveCabals func(ctx context.Context, cabalIDs []ids.CabalID) (map[ids.CabalID]bool, error)

type CabalHolders struct {
	held   heldByCabal
	active ActiveCabals
}

var (
	_ port.CabalHolderCounts = CabalHolders{}
	_ port.CabalsHolding     = CabalHolders{}
	_ heldByCabal            = (*sqlc.Queries)(nil)
)

func NewCabalHolders(db sqlc.DBTX, active ActiveCabals) CabalHolders {
	return CabalHolders{held: sqlc.New(db), active: active}
}

func (h CabalHolders) CabalsHolding(ctx context.Context, asset string) (int, error) {
	const op = "treasury.CabalsHolding"
	rows, err := h.held.CabalsHoldingAsset(ctx, asset)
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	cabalIDs := make([]ids.CabalID, len(rows))
	for i, row := range rows {
		cabalIDs[i] = ids.CabalIDFrom(row)
	}
	active, err := h.active(ctx, cabalIDs)
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), op)
	}
	n := 0
	for _, id := range cabalIDs {
		if active[id] {
			n++
		}
	}
	return n, nil
}

func (h CabalHolders) CabalHolderCounts(ctx context.Context) ([]port.HolderCount, error) {
	const op = "treasury.CabalHolderCounts"
	rows, err := h.held.HeldByCabal(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if len(rows) == 0 {
		return []port.HolderCount{}, nil
	}
	seen := map[ids.CabalID]bool{}
	var cabalIDs []ids.CabalID
	for _, row := range rows {
		id := ids.CabalIDFrom(row.CabalID)
		if !seen[id] {
			seen[id] = true
			cabalIDs = append(cabalIDs, id)
		}
	}
	active, err := h.active(ctx, cabalIDs)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	counts := map[string]int{}
	for _, row := range rows {
		if active[ids.CabalIDFrom(row.CabalID)] {
			counts[row.Asset]++
		}
	}
	out := make([]port.HolderCount, 0, len(counts))
	for mint, n := range counts {
		out = append(out, port.HolderCount{Mint: chain.SolanaAddress(mint), Cabals: n})
	}
	slices.SortFunc(out, func(a, b port.HolderCount) int { return strings.Compare(string(a.Mint), string(b.Mint)) })
	return out, nil
}
