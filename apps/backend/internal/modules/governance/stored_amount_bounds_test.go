package governance_test

import (
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func past64(t *testing.T) money.Units {
	t.Helper()
	wide, err := money.UnitsFromUint64(math.MaxUint64).Add(money.UnitsFromUint64(1))
	if err != nil {
		t.Fatal(err)
	}
	return wide
}

func (d proposalDB) corrupt(t *testing.T, id uuid.UUID, column, value string) {
	t.Helper()
	if _, err := d.pool.Exec(
		t.Context(),
		`ALTER TABLE proposals DROP CONSTRAINT IF EXISTS proposals_one_amount_check`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := d.pool.Exec(
		t.Context(),
		`UPDATE proposals SET `+column+` = $2::numeric WHERE id = $1`,
		id,
		value,
	); err != nil {
		t.Fatal(err)
	}
}

func (d readsDB) sellRow(t *testing.T, tokens string) uuid.UUID {
	t.Helper()
	p := d.buy(d.caller)
	p.CabalID, p.Kind, p.UsdcMicros, p.TokenAmount = d.cabal, "sell", pgtype.Int8{}, tokens
	d.insert(t, p)
	return p.ID
}

func wantCode(t *testing.T, what string, err error, code errs.Code) {
	t.Helper()
	if errs.CodeOf(err) != code {
		t.Errorf("%s err = %v, want %s", what, err, code)
	}
}

func TestStoredAmounts_aCorruptQuoteFailsGetAndList(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	id := d.seed(t, d.now)
	d.corrupt(t, id, "quote_out_amount", "-1")
	_, err := d.reads().Get(t.Context(), app.GetProposal{ID: ids.ProposalIDFrom(id), Caller: ids.UserIDFrom(d.caller)})
	wantCode(t, "Get", err, errs.CodeDecodeFailed)
	_, err = d.reads().List(t.Context(), app.ListProposals{
		CabalID: ids.CabalIDFrom(d.cabal), Caller: ids.UserIDFrom(d.caller), Filter: app.FilterAll,
	})
	wantCode(t, "List", err, errs.CodeDecodeFailed)
}

func TestStoredAmounts_aCorruptQuoteFailsTheDecidingVote(t *testing.T) {
	t.Parallel()
	v := newVoteDB(t)
	voter := v.ids.NewV7()
	p := v.buy(voter)
	v.insert(t, p)
	v.corrupt(t, p.ID, "quote_out_amount", "-1")
	_, err := v.cast(t.Context(), v.handler(), p.ID, voter, domain.ChoiceYes)
	wantCode(t, "the deciding vote", err, errs.CodeDecodeFailed)
	if r := v.row(t, p.ID); r.status.String != "open" {
		t.Errorf("status after the failed decision = %s, want open", r.status.String)
	}
}

func TestStoredAmounts_aCorruptTokenAmountFailsPendingVotesAndCards(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	id := d.sellRow(t, "1")
	d.corrupt(t, id, "token_amount", "-1")
	_, err := d.votes().PendingVotes(t.Context(), ids.UserIDFrom(d.caller))
	wantCode(t, "PendingVotes", err, errs.CodeDecodeFailed)
	_, err = d.votes().ProposalCards(t.Context(), ids.UserIDFrom(d.caller), []ids.ProposalID{ids.ProposalIDFrom(id)})
	wantCode(t, "ProposalCards", err, errs.CodeDecodeFailed)
}

func TestStoredAmounts_aSellQuotePastUint64FailsTheTreasuryShare(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	id := d.sellRow(t, "1")
	d.corrupt(t, id, "quote_out_amount", past64(t).String())
	d.corrupt(t, id, "pot_value_micros", "1000")
	_, err := d.reads().Get(t.Context(), app.GetProposal{ID: ids.ProposalIDFrom(id), Caller: ids.UserIDFrom(d.caller)})
	if err == nil {
		t.Fatal("Get with a sell quote past uint64 micros = nil, want an error")
	}
}

func TestStoredAmounts_aFillWhoseUSDCLegPassesUint64FailsTheRead(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	id := d.proposedBy(t, d.caller)
	d.swaps.Put(trading.SwapView{
		ID: ids.SwapIDFrom(d.ids.NewV7()), Source: trading.Source{Kind: "proposal", ID: id}, Action: "buy",
		InAmount: past64(t), OutAmount: money.UnitsFromUint64(1), Status: "confirmed", ConfirmedAt: d.now,
	})
	_, err := d.reads().Get(t.Context(), app.GetProposal{ID: ids.ProposalIDFrom(id), Caller: ids.UserIDFrom(d.caller)})
	wantCode(t, "Get", err, errs.CodeInternal)
}

func TestStoredAmounts_aCorruptQuoteFailsTheVoteStateAndTheBlockedRetry(t *testing.T) {
	t.Parallel()
	d := newMemberRetry(t)
	sell := d.sellRow(t, "1")
	for name, quote := range map[string]string{"negative": "-1", "past uint64": past64(t).String()} {
		d.corrupt(t, sell, "quote_out_amount", quote)
		_, err := adapters.NewVoteStates(d.pool).VoteState(t.Context(), ids.ProposalIDFrom(sell))
		wantCode(t, "VoteState "+name, err, errs.CodeDecodeFailed)
	}
	blocked := d.blocked(t, "trade_not_started")
	d.corrupt(t, blocked, "quote_out_amount", "-1")
	_, err := d.retry(t, d.caller, blocked)
	wantCode(t, "retry", err, errs.CodeDecodeFailed)
	if n := d.count(t, blocked, "proposal.reopened"); n != 0 {
		t.Errorf("proposal.reopened events = %d, want 0 after the failed retry", n)
	}
}

func TestQuotedStats_aSellQuotePastUint64LeavesThePriceUnset(t *testing.T) {
	t.Parallel()
	got := app.QuotedStats(app.Stats{}, domain.Draft{
		Kind: domain.KindSell, TokenAmount: money.UnitsFromUint64(1), QuoteOut: past64(t), Decimals: 8,
	})
	if got.PriceMicros != nil || got.Shares != nil || got.TokenDecimals != 8 {
		t.Fatalf("QuotedStats = %+v, want decimals only", got)
	}
}

func TestProposeTrade_aSolanaSellPastUint64IsShort(t *testing.T) {
	t.Parallel()
	e := newEVMSellWorld(t)
	e.holdSolana(heldUnits)
	d := newProposalDB(t)
	h := app.NewProposeTradeHandler(db.New(d.pool, d.ids, clock.Real{}), d.ids, clock.Real{}, e.ports(), app.NoHints{})
	_, err := h.Open(t.Context(), app.ProposeTrade{
		CabalID: e.cabal, ProposerID: e.members[0],
		Trade: domain.Trade{Kind: domain.KindSell, Symbol: "AAPLx", TokenAmount: past64(t)},
	})
	wantCode(t, "Open", err, errs.CodeCabalSharesShort)
}
