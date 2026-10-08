package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/bucket"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Dashboard struct {
	q *sqlc.Queries
}

var _ port.Dashboard = Dashboard{}

func NewDashboard(db sqlc.DBTX) Dashboard { return Dashboard{q: sqlc.New(db)} }

func (d Dashboard) ProposalCounts(
	ctx context.Context, from, to time.Time, size bucket.Size,
) ([]port.ProposalBucket, error) {
	rows, err := d.q.ProposalCounts(ctx, sqlc.ProposalCountsParams{Bucket: string(size), FromAt: from, ToAt: to})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "governance.Dashboard.ProposalCounts")
	}
	out := make([]port.ProposalBucket, len(rows))
	for i, r := range rows {
		out[i] = port.ProposalBucket{
			Start: r.BucketStart.UTC(), Created: r.Created, Passed: r.Passed, Failed: r.Failed, Expired: r.Expired,
			ExecutionBlocked: r.ExecutionBlocked,
		}
	}
	return out, nil
}

func (d Dashboard) MedianTimeToPass(ctx context.Context, from, to time.Time) (port.PassTime, error) {
	row, err := d.q.MedianTimeToPass(ctx, sqlc.MedianTimeToPassParams{FromAt: from, ToAt: to})
	if err != nil {
		return port.PassTime{}, errs.Wrap(err, errs.CodeOf(err), "governance.Dashboard.MedianTimeToPass")
	}
	return port.PassTime{Passed: row.Passed, Median: time.Duration(row.MedianSeconds) * time.Second}, nil
}

func (d Dashboard) VoteParticipation(ctx context.Context, from, to time.Time) ([]port.CabalParticipation, error) {
	rows, err := d.q.VoteParticipation(ctx, sqlc.VoteParticipationParams{FromAt: from, ToAt: to})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "governance.Dashboard.VoteParticipation")
	}
	out := make([]port.CabalParticipation, len(rows))
	for i, r := range rows {
		out[i] = port.CabalParticipation{
			CabalID: ids.CabalIDFrom(r.CabalID), Proposals: r.Proposals, Eligible: r.Eligible, Voted: r.Voted,
		}
	}
	return out, nil
}

func (d Dashboard) OpenCount(ctx context.Context) (int64, error) {
	n, err := d.q.OpenProposalCount(ctx)
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), "governance.Dashboard.OpenCount")
	}
	return n, nil
}
