package adapters

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type RulePassed struct {
	Apply app.ApplyRuleChangeHandler
}

func (h RulePassed) Handle(ctx context.Context, tx db.Tx, e events.ProposalRulePassed, at time.Time) error {
	change, err := app.ParsePassedRule(e.Change)
	if err != nil {
		return err
	}
	return h.Apply.Apply(ctx, tx, app.ApplyRuleChange{
		ProposalID: ids.ProposalIDFrom(e.ProposalID), CabalID: ids.CabalIDFrom(e.CabalID),
		ProposerID: ids.UserIDFrom(e.ProposerID), Change: change, PassedAt: e.PassedAt,
	}, at)
}
