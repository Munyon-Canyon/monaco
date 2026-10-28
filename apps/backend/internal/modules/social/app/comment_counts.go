package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func ReplyCounts(
	ctx context.Context, db sqlc.DBTX, proposalIDs []ids.ProposalID,
) (map[ids.ProposalID]int, error) {
	counts := make(map[ids.ProposalID]int, len(proposalIDs))
	if len(proposalIDs) == 0 {
		return counts, nil
	}
	raw := make([]uuid.UUID, len(proposalIDs))
	for i, id := range proposalIDs {
		raw[i] = id.UUID()
	}
	rows, err := sqlc.New(db).ProposalReplyCounts(ctx, raw)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "social.ReplyCounts")
	}
	for _, row := range rows {
		counts[ids.ProposalIDFrom(row.ProposalID)] = int(row.ReplyCount)
	}
	return counts, nil
}
