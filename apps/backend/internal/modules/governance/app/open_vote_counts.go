package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

var _ port.Queries = Queries{}

func (s Queries) OpenVoteCounts(ctx context.Context, cabals []ids.CabalID) (map[ids.CabalID]int, error) {
	if len(cabals) == 0 {
		return map[ids.CabalID]int{}, nil
	}
	raw := make([]uuid.UUID, len(cabals))
	for i, id := range cabals {
		raw[i] = id.UUID()
	}
	rows, err := s.q.OpenProposalCountsByCabal(ctx, raw)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "governance.OpenVoteCounts")
	}
	out := make(map[ids.CabalID]int, len(rows))
	for _, row := range rows {
		out[ids.CabalIDFrom(row.CabalID)] = int(row.OpenCount)
	}
	return out, nil
}
