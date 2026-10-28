package adapters

import (
	"cmp"
	"context"
	"log/slog"
	"math"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	wire "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type HTTP struct {
	Propose     *app.ProposeTradeHandler
	Vote        *app.CastVoteHandler
	RetractVote *app.RetractVoteHandler
	Withdraw    *app.WithdrawProposalHandler
	Void        *app.VoidProposalHandler
	Admin       *app.AdminReads
	Reads       *app.ProposalReads
	Pending     *app.PendingVoteReads
	Comparisons *app.ProposalComparisons
}

var _ api.StrictServerInterface = HTTP{}

func (h HTTP) PostProposalVote(
	ctx context.Context, req api.PostProposalVoteRequestObject,
) (api.PostProposalVoteResponseObject, error) {
	voter, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	choice, err := domain.ParseChoice(string(req.Body.Choice))
	if err != nil {
		return nil, err
	}
	got, err := h.Vote.Handle(ctx, app.CastVote{
		ProposalID: ids.ProposalIDFrom(req.Id), VoterID: voter, Choice: choice,
	})
	if err != nil {
		return nil, err
	}
	return api.PostProposalVote200JSONResponse{
		ProposalId: got.ProposalID.UUID(),
		Status:     api.ProposalStatus(got.Status),
		MyBallot:   api.BallotChoice(got.MyBallot),
		Tally:      wireTally(got.Tally),
		UndoUntil:  got.UndoUntil,
		MyVote:     wireMyVote(got.MyVote),
	}, nil
}

func wireMyVote(v app.MyVote) api.MyVote {
	return api.MyVote{Order: v.Order, Of: v.Of, YesOrder: v.YesOrder, NoOrder: v.NoOrder, Decided: v.Decided}
}

func wireStats(s app.Stats) api.ProposalStats {
	return api.ProposalStats{TreasuryShareBps: s.TreasuryShareBps, ClosesAt: s.ClosesAt, ClosedAt: s.ClosedAt}
}

func (h HTTP) DeleteProposalVote(
	ctx context.Context, req api.DeleteProposalVoteRequestObject,
) (api.DeleteProposalVoteResponseObject, error) {
	voter, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	got, err := h.RetractVote.Handle(ctx, app.RetractVote{ProposalID: ids.ProposalIDFrom(req.Id), VoterID: voter})
	if err != nil {
		return nil, err
	}
	return api.DeleteProposalVote200JSONResponse{
		ProposalId: got.ProposalID.UUID(),
		Status:     api.ProposalStatus(got.Status),
		Tally:      wireTally(got.Tally),
	}, nil
}

func (h HTTP) GetCabalProposals(
	ctx context.Context, req api.GetCabalProposalsRequestObject,
) (api.GetCabalProposalsResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	list := app.ListProposals{CabalID: ids.CabalIDFrom(req.Id), Caller: user}
	if req.Params.Filter != nil {
		list.Filter = app.Filter(*req.Params.Filter)
	}
	if req.Params.Limit != nil {
		list.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		list.Cursor = *req.Params.Cursor
	}
	page, err := h.Reads.List(ctx, list)
	if err != nil {
		return nil, err
	}
	out := api.GetCabalProposals200JSONResponse{Proposals: make([]api.Proposal, len(page.Items))}
	for i, v := range page.Items {
		out.Proposals[i] = wireProposal(v)
	}
	if page.NextCursor != "" {
		out.NextCursor = &page.NextCursor
	}
	return out, nil
}

func (h HTTP) GetMyProposals(
	ctx context.Context, req api.GetMyProposalsRequestObject,
) (api.GetMyProposalsResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	list := app.ListMyProposals{Caller: user}
	if req.Params.Filter != nil {
		list.Filter = app.Filter(*req.Params.Filter)
	}
	if req.Params.Limit != nil {
		list.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		list.Cursor = *req.Params.Cursor
	}
	page, err := h.Reads.ListMine(ctx, list)
	if err != nil {
		return nil, err
	}
	out := api.GetMyProposals200JSONResponse{Proposals: make([]api.Proposal, len(page.Items))}
	for i, v := range page.Items {
		out.Proposals[i] = wireProposal(v)
	}
	if page.NextCursor != "" {
		out.NextCursor = &page.NextCursor
	}
	return out, nil
}

func (h HTTP) GetProposal(
	ctx context.Context, req api.GetProposalRequestObject,
) (api.GetProposalResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	out, err := h.detail(ctx, ids.ProposalIDFrom(req.Id), user)
	if err != nil {
		return nil, err
	}
	return api.GetProposal200JSONResponse(out), nil
}

func (h HTTP) PostCabalProposal(
	ctx context.Context, req api.PostCabalProposalRequestObject,
) (api.PostCabalProposalResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	trade, err := domain.NewTrade(domain.Trade{
		Kind: domain.Kind(req.Body.Kind), Symbol: req.Body.Symbol,
		USDCMicros: money.MicrosFromUint64(amount(req.Body.UsdcMicros)), TokenAmount: amount(req.Body.TokenAmount),
		Thesis: deref(req.Body.Thesis), Network: network(req.Body.Network),
	})
	if err != nil {
		return nil, err
	}
	opened, err := h.Propose.Open(ctx, app.ProposeTrade{
		CabalID: ids.CabalIDFrom(req.Id), ProposerID: user, Trade: trade,
	})
	if err != nil {
		return nil, err
	}
	name, err := h.Reads.CabalName(ctx, opened.Draft.CabalID)
	if err != nil {
		return nil, err
	}
	return api.PostCabalProposal201JSONResponse(wireOpenedProposal(opened, name)), nil
}

func (h HTTP) GetCabalProposalPreview(
	ctx context.Context, req api.GetCabalProposalPreviewRequestObject,
) (api.GetCabalProposalPreviewResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	trade, err := previewTrade(req.Params)
	if err != nil {
		return nil, err
	}
	got, err := h.Propose.Preview(
		ctx,
		app.ProposeTrade{CabalID: ids.CabalIDFrom(req.Id), ProposerID: user, Trade: trade},
	)
	if err != nil {
		return nil, err
	}
	pot, err := wireInt(got.Pot.Uint64(), "pot_value_micros")
	if err != nil {
		return nil, err
	}
	quote, err := wireInt(got.QuoteOut, "quote_out_amount")
	if err != nil {
		return nil, err
	}
	out := api.GetCabalProposalPreview200JSONResponse{
		PotValueMicros: pot, QuoteOutAmount: positive(quote), PriceComparison: wireComparison(got.PriceComparison),
	}
	if err := wireSpendable(&out, got.Spendable); err != nil {
		return nil, err
	}
	if got.Advisory != "" {
		out.AdvisoryCode, out.AdvisoryMessage = ptr(string(got.Advisory)), ptr(errs.Message(got.Advisory))
	}
	return out, nil
}

func previewTrade(params api.GetCabalProposalPreviewParams) (domain.Trade, error) {
	trade := domain.Trade{
		Kind: domain.Kind(params.Kind), Symbol: params.Symbol,
		USDCMicros: money.MicrosFromUint64(amount(params.UsdcMicros)), TokenAmount: amount(params.TokenAmount),
		Network: network(params.Network),
	}
	amountless := trade.Kind == domain.KindBuy && params.UsdcMicros == nil && params.TokenAmount == nil
	if amountless && strings.TrimSpace(trade.Symbol) != "" {
		return trade, nil
	}
	return domain.NewTrade(trade)
}

func wireSpendable(out *api.GetCabalProposalPreview200JSONResponse, s *app.Spendable) error {
	if s == nil {
		return nil
	}
	cash, cashErr := wireInt(s.Cash.Uint64(), "spendable_cash_micros")
	maxBuy, maxErr := wireInt(s.MaxBuy.Uint64(), "max_buy_micros")
	if err := cmp.Or(cashErr, maxErr); err != nil {
		return errs.Wrap(err, errs.CodeDecodeFailed, "governance.wireSpendable")
	}
	out.SpendableCashMicros, out.MaxBuyMicros = &cash, &maxBuy
	return nil
}

func wireInt(v uint64, field string) (int64, error) {
	if v > math.MaxInt64 {
		return 0, errs.New(errs.CodeDecodeFailed, "governance.wireInt", slog.String("field", field))
	}
	return int64(v), nil
}

func (h HTTP) DeleteProposal(
	ctx context.Context, req api.DeleteProposalRequestObject,
) (api.DeleteProposalResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	id := ids.ProposalIDFrom(req.Id)
	if err := h.Withdraw.Handle(ctx, app.WithdrawProposal{ProposalID: id, ActorID: user}); err != nil {
		return nil, err
	}
	out, err := h.detail(ctx, id, user)
	if err != nil {
		return nil, err
	}
	return api.DeleteProposal200JSONResponse(out), nil
}

func (h HTTP) detail(ctx context.Context, id ids.ProposalID, user ids.UserID) (api.ProposalDetail, error) {
	got, err := h.Reads.Get(ctx, app.GetProposal{ID: id, Caller: user})
	if err != nil {
		return api.ProposalDetail{}, err
	}
	p := wireProposal(got.Proposal)
	out := api.ProposalDetail{
		Id: p.Id, CabalId: p.CabalId, CabalName: p.CabalName, ProposerId: p.ProposerId, Kind: p.Kind,
		Symbol: p.Symbol, UsdcMicros: p.UsdcMicros, TokenAmount: p.TokenAmount, QuoteOutAmount: p.QuoteOutAmount,
		Thesis: p.Thesis, Status: p.Status, StatusReason: p.StatusReason, StatusMessage: p.StatusMessage,
		ExpiresAt: p.ExpiresAt, CreatedAt: p.CreatedAt, Tally: p.Tally, MyBallot: p.MyBallot,
		Voters: make([]api.ProposalVoter, len(got.Voters)), CanVote: got.CanVote, CanWithdraw: got.CanWithdraw,
		PriceComparison: p.PriceComparison, Network: p.Network,
		ProposerReturn1mBps: got.ProposerReturnBps, ProposerReturnPrivate: got.ProposerReturnPrivate,
		Stats: wireStats(got.Stats), ReplyCount: got.ReplyCount,
	}
	if m := got.MyVote; m != nil {
		mine := api.ProposalMyVote(wireMyVote(*m))
		out.MyVote = &mine
	}
	for i, v := range got.Voters {
		out.Voters[i] = api.ProposalVoter{UserId: v.UserID.UUID()}
		if v.Choice != "" {
			choice, at := api.BallotChoice(v.Choice), v.CastAt
			out.Voters[i].Choice, out.Voters[i].CastAt = &choice, &at
		}
	}
	if s := got.Swap; s != nil {
		pr := got.Proposal
		retryable := pr.Status == domain.StatusPassed ||
			(pr.Status == domain.StatusExecutionBlocked && pr.StatusReason == errs.CodeSwapFailed)
		out.Swap = &api.LinkedSwap{
			SwapId: s.ID.UUID(), Status: api.ProposalDetailSwapStatus(s.Status), Retryable: s.Retryable && retryable,
			FailureCode: present(string(s.FailureCode)), TxSignature: present(string(s.TxSignature)),
		}
		if s.Status == "failed" && retryable {
			out.Swap.FailureMessage = ptr(errs.SwapFailureMessage(string(s.FailureCode)))
		}
		wireFill(out.Swap, got.Fill)
	}
	return out, nil
}

func wireFill(out *api.LinkedSwap, f app.Fill) {
	out.UsdcMicros, out.TokenAmount = baseAmount(f.USDCMicros), baseAmount(f.TokenAmount)
	out.TokenDecimals = int(f.TokenDecimals)
	if f.Confirmed {
		out.PriceMicros = baseAmount(f.PriceMicros)
	}
}

func baseAmount(n uint64) *int64 {
	if n == 0 || n > math.MaxInt64 {
		return nil
	}
	v := int64(n)
	return &v
}

func (h HTTP) GetMyPendingVotes(
	ctx context.Context, _ api.GetMyPendingVotesRequestObject,
) (api.GetMyPendingVotesResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	pending, err := h.Pending.PendingVotes(ctx, user)
	if err != nil {
		return nil, err
	}
	out := make(api.GetMyPendingVotes200JSONResponse, len(pending))
	for i, v := range pending {
		out[i] = api.PendingVote{
			ProposalId: v.ProposalID.UUID(), CabalId: v.CabalID.UUID(), CabalName: v.CabalName,
			Kind: api.ProposalKind(v.Kind), Symbol: v.Symbol, ExpiresAt: v.ExpiresAt, CreatedAt: v.CreatedAt,
			UsdcMicros: baseAmount(v.USDC.Uint64()), TokenAmount: baseAmount(v.TokenAmount.Uint64()),
			AssetName: v.AssetName, AssetLogoUrl: present(v.AssetLogo),
			Proposer: api.PendingVoteProposer{
				UserId: v.Proposer.ID.UUID(), Handle: present(v.Proposer.Handle),
				DisplayName: v.Proposer.DisplayName, PhotoUrl: present(v.Proposer.PhotoURL),
			},
			Tally: wireTally(v.Tally), VotedCount: v.VotedCount, ReplyCount: v.ReplyCount,
		}
	}
	return out, nil
}

func wireProposal(v app.ProposalView) api.Proposal {
	out := api.Proposal{
		Id:         v.ID.UUID(),
		CabalId:    v.CabalID.UUID(),
		CabalName:  v.CabalName,
		ProposerId: v.ProposerID.UUID(),
		Kind: api.ProposalKind(
			v.Kind,
		),
		Symbol:          v.Symbol,
		UsdcMicros:      positive(v.USDCMicros),
		TokenAmount:     positive(v.TokenAmount),
		QuoteOutAmount:  v.QuoteOut,
		Status:          api.ProposalStatus(v.Status),
		ExpiresAt:       v.ExpiresAt,
		CreatedAt:       v.CreatedAt,
		Tally:           wireTally(v.Tally),
		CanVote:         v.CanVote,
		PriceComparison: wireComparison(v.Comparison),
		Network:         wireNetwork(v.Network),
	}
	out.Thesis = present(v.Thesis)
	if v.StatusReason != "" {
		reason, message := string(v.StatusReason), errs.Message(v.StatusReason)
		out.StatusReason, out.StatusMessage = &reason, &message
	}
	if v.MyBallot != "" {
		ballot := api.BallotChoice(v.MyBallot)
		out.MyBallot = &ballot
	}
	return out
}

func wireTally(t app.Tally) api.Tally {
	return api.Tally{Yes: t.Yes, No: t.No, Voters: t.Voters, Needed: t.Needed}
}

func wireOpenedProposal(opened app.OpenedProposal, cabalName string) api.ProposalDetail {
	d := opened.Draft
	p := api.Proposal{
		Id:             d.ID.UUID(),
		CabalId:        d.CabalID.UUID(),
		CabalName:      cabalName,
		ProposerId:     d.ProposerID.UUID(),
		Kind:           api.ProposalKind(d.Kind),
		Symbol:         d.Symbol,
		UsdcMicros:     positive(opened.USDCMicros),
		TokenAmount:    positive(opened.TokenAmount),
		QuoteOutAmount: opened.QuoteOutAmount,
		Status:         api.ProposalStatusOpen,
		ExpiresAt:      d.ExpiresAt,
		CreatedAt:      opened.CreatedAt,
		Tally:          wireTally(app.Tally{Voters: len(opened.Voters), Needed: opened.Needed}),
	}
	p.Thesis = present(d.Thesis)
	voters := make([]api.ProposalVoter, len(opened.Voters))
	for i, voter := range opened.Voters {
		voters[i] = api.ProposalVoter{UserId: voter.UUID()}
	}
	return api.ProposalDetail{
		Id: p.Id, CabalId: p.CabalId, CabalName: p.CabalName, ProposerId: p.ProposerId, Kind: p.Kind,
		Symbol: p.Symbol, UsdcMicros: p.UsdcMicros, TokenAmount: p.TokenAmount, QuoteOutAmount: p.QuoteOutAmount,
		Thesis: p.Thesis, Status: p.Status, StatusReason: p.StatusReason, StatusMessage: p.StatusMessage,
		ExpiresAt: p.ExpiresAt, CreatedAt: p.CreatedAt, Tally: p.Tally, MyBallot: p.MyBallot, Voters: voters,
		CanVote: opened.ProposerCanVote, CanWithdraw: true,
		PriceComparison: wireComparison(opened.PriceComparison), Network: wireNetwork(d.Network),
		Stats: wireStats(opened.Stats),
	}
}

func network(n *wire.Network) chain.Network {
	if n == nil {
		return ""
	}
	return chain.Network(*n)
}

func wireNetwork(n chain.Network) *wire.Network {
	if n == "" {
		return nil
	}
	return ptr(wire.Network(n))
}

func present(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ptr[T any](v T) *T { return &v }

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func amount(v *int64) uint64 {
	if v == nil || *v < 0 {
		return 0
	}
	return uint64(*v)
}

func positive(n int64) *int64 {
	if n <= 0 {
		return nil
	}
	return &n
}

func caller(ctx context.Context) (ids.UserID, error) {
	const op = "governance.caller"
	actor, ok := auth.ActorFrom(ctx)
	if !ok {
		return ids.UserID{}, errs.New(errs.CodeUnauthorized, op)
	}
	if actor.Kind != auth.ActorUser {
		return ids.UserID{}, errs.New(errs.CodeForbidden, op, slog.String("actor_kind", string(actor.Kind)))
	}
	user, err := ids.ParseUserID(actor.ID)
	if err != nil {
		return ids.UserID{}, errs.Wrap(err, errs.CodeUnauthorized, op)
	}
	return user, nil
}
