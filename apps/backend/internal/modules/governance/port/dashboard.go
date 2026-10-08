package port

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/bucket"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type ProposalBucket struct {
	Start            time.Time
	Created          int64
	Passed           int64
	Failed           int64
	Expired          int64
	ExecutionBlocked int64
}

type PassTime struct {
	Passed int64
	Median time.Duration
}

type CabalParticipation struct {
	CabalID   ids.CabalID
	Proposals int64
	Eligible  int64
	Voted     int64
}

type Dashboard interface {
	ProposalCounts(ctx context.Context, from, to time.Time, size bucket.Size) ([]ProposalBucket, error)
	MedianTimeToPass(ctx context.Context, from, to time.Time) (PassTime, error)
	VoteParticipation(ctx context.Context, from, to time.Time) ([]CabalParticipation, error)
	OpenCount(ctx context.Context) (int64, error)
}
