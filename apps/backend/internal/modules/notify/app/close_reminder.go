package app

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const opCloseReminder = "notify.CloseReminder"

type CloseReminder struct {
	Cabals Cabals
	Assets Assets
	Clock  clock.Clock
}

func (CloseReminder) Name() string { return "close_reminder" }

func (k CloseReminder) Recipients(ctx context.Context, e events.ProposalCloseReminderDue) ([]ids.UserID, error) {
	user := ids.UserIDFrom(e.UserID)
	members, err := memberIDs(ctx, k.Cabals, &e.CabalID)
	if err != nil || !slices.Contains(members, user) {
		return nil, err
	}
	return []ids.UserID{user}, nil
}

func (k CloseReminder) Render(ctx context.Context, e events.ProposalCloseReminderDue, _ ids.UserID) (Message, error) {
	action, err := side(e.Kind)
	if err != nil {
		return Message{}, err
	}
	asset, found, err := assetOf(ctx, k.Assets, e.Symbol)
	if err != nil {
		return Message{}, err
	}
	label, amount := unnamedAsset, dollars(e.USDCMicros)
	if found {
		label = assetLabel(asset)
	}
	if action == "Sell" {
		if !found {
			return Message{}, errs.New(errs.CodeAssetNotFound, opCloseReminder, slog.String("symbol", e.Symbol))
		}
		amount = units(e.TokenAmount, tokenDecimals(e.TokenDecimals, asset.Decimals)) + " sh"
	}
	view, err := k.Cabals.Cabal(ctx, ids.CabalIDFrom(e.CabalID))
	if err != nil {
		return Message{}, err
	}
	pres := ForCabal(view, CabalSender(view))
	pres.Category, pres.InterruptionLevel = voteCategory, InterruptionTimeSensitive
	title := fmt.Sprintf("%s %s %s closes in %s", action, label, amount, closesIn(e.ExpiresAt.Sub(k.Clock.Now())))
	body := fmt.Sprintf("You asked for a reminder. %d yes, %d no so far. Yours counts.", e.Yes, e.No)
	return proposalMessage(k.Name(), e.CabalID, e.ProposalID, title, body, pres), nil
}
