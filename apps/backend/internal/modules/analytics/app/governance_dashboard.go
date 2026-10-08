package app

import (
	"context"
	"time"

	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
)

const basisPoints = 10_000

type Participation struct {
	governanceport.CabalParticipation
	Bps int64
}

type GovernanceView struct {
	Window        Window
	Open          int64
	Passed        governanceport.PassTime
	Buckets       []governanceport.ProposalBucket
	Participation []Participation
}

type Governance struct {
	Read ReadOnly
	Bind func(db dbsqlc.DBTX) governanceport.Dashboard
}

func (g Governance) Dashboard(ctx context.Context, w Window) (GovernanceView, error) {
	view := GovernanceView{Window: w}
	err := g.Read(ctx, func(ctx context.Context, db dbsqlc.DBTX) error {
		src := g.Bind(db)
		steps := []func(context.Context) error{
			func(ctx context.Context) (err error) {
				view.Buckets, err = src.ProposalCounts(ctx, w.From, w.To, w.Size)
				return err
			},
			func(ctx context.Context) (err error) {
				view.Passed, err = src.MedianTimeToPass(ctx, w.From, w.To)
				return err
			},
			func(ctx context.Context) (err error) {
				view.Participation, err = participation(ctx, src, w.From, w.To)
				return err
			},
			func(ctx context.Context) (err error) {
				view.Open, err = src.OpenCount(ctx)
				return err
			},
		}
		for _, step := range steps {
			if err := step(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return GovernanceView{}, err
	}
	return view, nil
}

func participation(
	ctx context.Context, src governanceport.Dashboard, from, to time.Time,
) ([]Participation, error) {
	rows, err := src.VoteParticipation(ctx, from, to)
	out := make([]Participation, len(rows))
	for i, row := range rows {
		out[i] = Participation{CabalParticipation: row}
		if row.Eligible > 0 {
			out[i].Bps = row.Voted * basisPoints / row.Eligible
		}
	}
	return out, err
}
