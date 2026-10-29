package app

import (
	"context"
	"log/slog"
	"slices"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	resultKind = "proposal_result"
	opResult   = "notify.Result"
)

type ResultPorts struct {
	Cabals  Cabals
	Assets  Assets
	Ballots Ballots
	Heads   ProposalHeads
}

func resultItemKey(proposal uuid.UUID) string {
	return domain.ItemKeyProposalPrefix + proposal.String()
}

type resultCast struct {
	yes, no int
	voted   map[ids.UserID]string
}

func (p ResultPorts) cast(ctx context.Context, proposal uuid.UUID) (resultCast, error) {
	ballots, err := p.Ballots.Ballots(ctx, ids.ProposalIDFrom(proposal))
	if err != nil {
		return resultCast{}, err
	}
	cast := resultCast{voted: map[ids.UserID]string{}}
	for _, b := range ballots {
		if b.Choice == nil {
			continue
		}
		switch *b.Choice {
		case "yes":
			cast.yes++
			cast.voted[b.VoterID] = "Yes"
		case "no":
			cast.no++
			cast.voted[b.VoterID] = "No"
		default:
			return resultCast{}, errs.New(errs.CodeInvalidInput, opResult)
		}
	}
	return cast, nil
}

func (p ResultPorts) voters(ctx context.Context, proposal, cabalID uuid.UUID) ([]ids.UserID, error) {
	cast, err := p.cast(ctx, proposal)
	if err != nil || len(cast.voted) == 0 {
		return nil, err
	}
	members, err := memberIDs(ctx, p.Cabals, &cabalID)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(members, func(user ids.UserID) bool { _, ok := cast.voted[user]; return !ok }), nil
}

func (p ResultPorts) head(ctx context.Context, proposal uuid.UUID) (governanceport.ProposalHead, error) {
	return p.Heads.ProposalHead(ctx, ids.ProposalIDFrom(proposal))
}

func (p ResultPorts) message(
	ctx context.Context, cabalID, proposal uuid.UUID, outcome, title, body string,
) (Message, error) {
	pres, err := cabalPresentation(ctx, p.Cabals, cabalID)
	if err != nil {
		return Message{}, err
	}
	msg := proposalMessage(resultKind, cabalID, proposal, title, body, pres)
	msg.Data["outcome"] = outcome
	return msg, nil
}

func (p ResultPorts) fillRecipients(
	ctx context.Context, source events.TradeSource, cabalID uuid.UUID,
) ([]ids.UserID, error) {
	if source.Kind != proposalSource {
		return nil, nil
	}
	return p.voters(ctx, source.ID, cabalID)
}

func (p ResultPorts) decided(
	ctx context.Context, proposal, cabalID uuid.UUID,
) ([]ids.UserID, error) {
	head, err := p.head(ctx, proposal)
	if err != nil || head.Kind == ruleKind {
		return nil, err
	}
	return p.voters(ctx, proposal, cabalID)
}

type ResultFilled struct{ ResultPorts }

func (ResultFilled) Name() string { return "proposal_result" }

func (ResultFilled) ItemKey(e events.TradeConfirmed) string {
	return resultItemKey(e.Source.ID)
}

func (k ResultFilled) Recipients(ctx context.Context, e events.TradeConfirmed) ([]ids.UserID, error) {
	return k.fillRecipients(ctx, e.Source, e.CabalID)
}

func (k ResultFilled) Render(ctx context.Context, e events.TradeConfirmed, to ids.UserID) (Message, error) {
	if _, err := side(e.Action); err != nil {
		return Message{}, err
	}
	asset, found, err := assetOf(ctx, k.Assets, e.Symbol)
	if err != nil {
		return Message{}, err
	}
	if !found {
		return Message{}, errs.New(errs.CodeAssetNotFound, opResult, slog.String("symbol", e.Symbol))
	}
	cast, err := k.cast(ctx, e.Source.ID)
	if err != nil {
		return Message{}, err
	}
	filled := e.OutAmount
	if e.Action == "sell" {
		filled = e.InAmount
	}
	body := filledBody(
		e.Action, units(filled, asset.Decimals), fillPrice(e.USDCMicros, filled, asset.Decimals), cast.voted[to],
	)
	return k.message(ctx, e.CabalID, e.Source.ID, outcomeFilled,
		resultTitle(assetLabel(asset), "passed", tally(cast.yes, cast.no)), body)
}

type ResultFillFailed struct{ ResultPorts }

func (ResultFillFailed) Name() string { return "proposal_result" }

func (ResultFillFailed) ItemKey(e events.TradeFailed) string {
	return resultItemKey(e.Source.ID)
}

func (k ResultFillFailed) Recipients(ctx context.Context, e events.TradeFailed) ([]ids.UserID, error) {
	return k.fillRecipients(ctx, e.Source, e.CabalID)
}

func (k ResultFillFailed) Render(ctx context.Context, e events.TradeFailed, to ids.UserID) (Message, error) {
	return k.fillFailed(ctx, e.CabalID, e.Source.ID, e.Action, e.Symbol, false, to)
}

type ResultFillBlocked struct{ ResultPorts }

func (ResultFillBlocked) Name() string { return "proposal_result" }

func (ResultFillBlocked) ItemKey(e events.TradeBlocked) string {
	return resultItemKey(e.Source.ID)
}

func (k ResultFillBlocked) Recipients(ctx context.Context, e events.TradeBlocked) ([]ids.UserID, error) {
	return k.fillRecipients(ctx, e.Source, e.CabalID)
}

func (k ResultFillBlocked) Render(ctx context.Context, e events.TradeBlocked, to ids.UserID) (Message, error) {
	return k.fillFailed(ctx, e.CabalID, e.Source.ID, e.Action, e.Symbol, true, to)
}

func (p ResultPorts) fillFailed(
	ctx context.Context, cabalID, proposal uuid.UUID, action, symbol string, blocked bool, to ids.UserID,
) (Message, error) {
	if _, err := side(action); err != nil {
		return Message{}, err
	}
	label, err := labelOf(ctx, p.Assets, symbol)
	if err != nil {
		return Message{}, err
	}
	cast, err := p.cast(ctx, proposal)
	if err != nil {
		return Message{}, err
	}
	return p.message(ctx, cabalID, proposal, outcomeFillFailed,
		resultTitle(label, "passed", tally(cast.yes, cast.no)), fillFailedBody(action, blocked, cast.voted[to]))
}

type ResultFailed struct{ ResultPorts }

func (ResultFailed) Name() string { return "proposal_result" }

func (ResultFailed) ItemKey(e events.ProposalFailed) string {
	return resultItemKey(e.ProposalID)
}

func (k ResultFailed) Recipients(ctx context.Context, e events.ProposalFailed) ([]ids.UserID, error) {
	return k.decided(ctx, e.ProposalID, e.CabalID)
}

func (k ResultFailed) Render(ctx context.Context, e events.ProposalFailed, to ids.UserID) (Message, error) {
	return k.closed(ctx, e.CabalID, e.ProposalID, outcomeFailed, "failed", failedBody, to)
}

type ResultExpired struct{ ResultPorts }

func (ResultExpired) Name() string { return "proposal_result" }

func (ResultExpired) ItemKey(e events.ProposalExpired) string {
	return resultItemKey(e.ProposalID)
}

func (k ResultExpired) Recipients(ctx context.Context, e events.ProposalExpired) ([]ids.UserID, error) {
	return k.decided(ctx, e.ProposalID, e.CabalID)
}

func (k ResultExpired) Render(ctx context.Context, e events.ProposalExpired, to ids.UserID) (Message, error) {
	return k.closed(ctx, e.CabalID, e.ProposalID, outcomeExpired, "closed", expiredBody, to)
}

func (p ResultPorts) closed(
	ctx context.Context, cabalID, proposal uuid.UUID, outcome, verb string, body func(string) string, to ids.UserID,
) (Message, error) {
	head, err := p.head(ctx, proposal)
	if err != nil {
		return Message{}, err
	}
	label, err := labelOf(ctx, p.Assets, head.Symbol)
	if err != nil {
		return Message{}, err
	}
	cast, err := p.cast(ctx, proposal)
	if err != nil {
		return Message{}, err
	}
	return p.message(ctx, cabalID, proposal, outcome,
		resultTitle(label, verb, tally(cast.yes, cast.no)), body(cast.voted[to]))
}
