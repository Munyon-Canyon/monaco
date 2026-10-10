package adapters

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	bannedStatus   = "banned"
	banVoidsActor  = "consumer.ban_voids"
	proposerBanned = domain.VoidReason("proposer banned")
)

type Voider interface {
	Within(ctx context.Context, tx db.Tx, cmd app.VoidProposal) error
}

type BanVoids struct{ Voids Voider }

func (h BanVoids) Handle(ctx context.Context, tx db.Tx, e events.UserAccountStatusChanged, _ time.Time) error {
	const op = "governance.BanVoids"
	if e.To != bannedStatus {
		return nil
	}
	open, err := sqlc.New(tx.Queries()).ProposalsToVoidForBan(ctx, e.UserID)
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	ctx = auth.WithActor(ctx, auth.Actor{Kind: auth.ActorSystem, ID: banVoidsActor})
	for _, proposal := range open {
		cmd := app.VoidProposal{ProposalID: ids.ProposalIDFrom(proposal), Reason: proposerBanned}
		err := h.Voids.Within(ctx, tx, cmd)
		code := errs.CodeOf(err)
		if err != nil && code != errs.CodeLiveSwapExists && code != errs.CodeProposalClosed {
			return err
		}
		if err != nil {
			observability.Info(ctx, observability.GovernanceBanVoidSkipped,
				slog.String("proposal_id", proposal.String()), slog.String("user_id", e.UserID.String()),
				slog.String("code", string(code)))
		}
	}
	return nil
}
