package governance_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const robinhoodDecimals = 18

type robinhoodListings struct {
	id       multichain.ListingID
	decimals uint8
	err      error
}

func (l robinhoodListings) ListingOn(
	context.Context, multichain.StockID, chain.Network,
) (multichain.ListingID, multichain.Venue, error) {
	return l.id, multichain.VenueUniswap, l.err
}

func (l robinhoodListings) Listing(context.Context, multichain.ListingID) (multichain.Listing, error) {
	return multichain.Listing{ID: l.id, Network: chain.NetworkRobinhood, Decimals: l.decimals}, l.err
}

func (d readsDB) robinhoodReads() *app.ProposalReads {
	listings := robinhoodListings{id: multichain.ListingIDFrom(d.ids.NewV7()), decimals: robinhoodDecimals}
	return d.reads().Listed(listings)
}

func (d readsDB) robinhoodSell(t *testing.T, tokens int64) uuid.UUID {
	t.Helper()
	p := d.buy(d.caller)
	p.CabalID, p.Kind, p.UsdcMicros = d.cabal, "sell", pgtype.Int8{}
	p.TokenAmount, p.QuoteOutAmount = pgtype.Int8{Int64: tokens, Valid: true}, 100_000_000
	d.insert(t, p)
	d.setNetwork(t, p.ID, string(chain.NetworkRobinhood))
	return p.ID
}

func (d readsDB) robinhoodBuy(t *testing.T) uuid.UUID {
	t.Helper()
	p := d.buy(d.caller)
	p.CabalID = d.cabal
	d.insert(t, p)
	d.setNetwork(t, p.ID, string(chain.NetworkRobinhood))
	return p.ID
}

func (d readsDB) setNetwork(t *testing.T, id uuid.UUID, network string) {
	t.Helper()
	if _, err := d.pool.Exec(t.Context(), `UPDATE proposals SET network = $2 WHERE id = $1`, id, network); err != nil {
		t.Fatal(err)
	}
}

func (d readsDB) getWith(t *testing.T, reads *app.ProposalReads, id uuid.UUID) app.ProposalDetail {
	t.Helper()
	got, err := reads.Get(t.Context(), app.GetProposal{ID: ids.ProposalIDFrom(id), Caller: ids.UserIDFrom(d.caller)})
	if err != nil {
		t.Fatalf("Get(%s) = %v", id, err)
	}
	return got
}

func TestGetProposal_robinhoodSellShowsHalfAShare(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	id := d.robinhoodSell(t, 500_000_000_000_000_000)
	got := d.getWith(t, d.robinhoodReads(), id)
	if got.Stats.TokenDecimals != robinhoodDecimals || got.Stats.Shares == nil ||
		got.Stats.Shares.Decimals() != robinhoodDecimals || got.Stats.Shares.Uint64() != 500_000_000_000_000_000 {
		t.Fatalf("sell stats = decimals %d, shares %v, want 0.5 share at 18 decimals",
			got.Stats.TokenDecimals, got.Stats.Shares)
	}
}

func TestGetProposal_robinhoodBuyFillShowsOneToken(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	id := d.robinhoodBuy(t)
	d.setStatus(t, id, "executed", "")
	d.swaps.Put(trading.SwapView{
		ID: ids.SwapIDFrom(d.ids.NewV7()), Source: trading.Source{Kind: "proposal", ID: id}, Action: "buy",
		InAmount: 200_000_000, OutDecimals: robinhoodDecimals, OutAmount: 1_000_000_000_000_000_000,
		Status: "confirmed", ConfirmedAt: d.now,
	})
	got := d.getWith(t, d.robinhoodReads(), id)
	if got.Fill.TokenDecimals != robinhoodDecimals || got.Fill.PriceMicros != 200_000_000 {
		t.Errorf("fill = decimals %d, price %d, want 18 and $200 a token", got.Fill.TokenDecimals, got.Fill.PriceMicros)
	}
	if got.Stats.Shares == nil || got.Stats.Shares.Decimals() != robinhoodDecimals ||
		got.Stats.Shares.Uint64() != 1_000_000_000_000_000_000 {
		t.Errorf("shares = %v, want 1 token at 18 decimals", got.Stats.Shares)
	}
}

func TestGetProposal_robinhoodBuyFillNeedsNoListingLookup(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	id := d.robinhoodBuy(t)
	d.swaps.Put(trading.SwapView{
		ID: ids.SwapIDFrom(d.ids.NewV7()), Source: trading.Source{Kind: "proposal", ID: id}, Action: "buy",
		InAmount: 200_000_000, OutDecimals: robinhoodDecimals, OutAmount: 1_000_000_000_000_000_000,
		Status: "confirmed", ConfirmedAt: d.now,
	})
	got := d.get(t, id, d.caller)
	if got.Fill.TokenDecimals != robinhoodDecimals || got.Fill.PriceMicros != 200_000_000 {
		t.Fatalf(
			"fill = decimals %d, price %d, want the swap row's 18 decimals",
			got.Fill.TokenDecimals,
			got.Fill.PriceMicros,
		)
	}
}

func TestGetProposal_robinhoodSellFallsBackWhenTheListingIsGone(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	id := d.robinhoodSell(t, 7)
	gone := robinhoodListings{err: errs.New(errs.CodeAssetNotFound, "test")}
	if got := d.getWith(t, d.reads().Listed(gone), id); got.Stats.TokenDecimals != 8 {
		t.Fatalf("decimals = %d, want the asset's 8 when the listing is gone", got.Stats.TokenDecimals)
	}
	broken := robinhoodListings{err: errs.New(errs.CodeDBUnavailable, "test")}
	_, err := d.reads().Listed(broken).Get(t.Context(), app.GetProposal{
		ID: ids.ProposalIDFrom(id), Caller: ids.UserIDFrom(d.caller),
	})
	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Get with a failing listing read = %v, want db_unavailable", err)
	}
}

func TestGetProposal_solanaSellKeepsTheAssetDecimals(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.buy(d.caller)
	p.CabalID, p.Kind, p.UsdcMicros, p.TokenAmount = d.cabal, "sell", pgtype.Int8{}, pgtype.Int8{Int64: 5, Valid: true}
	d.insert(t, p)
	listings := robinhoodListings{id: multichain.ListingIDFrom(d.ids.NewV7()), decimals: robinhoodDecimals}
	if got := d.getWith(t, d.reads().Listed(listings), p.ID); got.Stats.TokenDecimals != 8 {
		t.Fatalf("decimals = %d, want the Solana token's 8", got.Stats.TokenDecimals)
	}
}

func TestPendingVotes_robinhoodSellCarriesTheListingDecimals(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	id := d.robinhoodSell(t, 500_000_000_000_000_000)
	votes := app.NewPendingVoteReads(
		d.robinhoodReads(), app.PendingVotePorts{Assets: d.assets, Replies: &replyBook{}, Cabals: d.roster()},
	)
	got, err := votes.PendingVotes(t.Context(), ids.UserIDFrom(d.caller))
	if err != nil || len(got) != 1 || got[0].ProposalID.UUID() != id {
		t.Fatalf("PendingVotes = %+v, %v, want the one sell", got, err)
	}
	if amount := got[0].TokenAmount; amount.Decimals() != robinhoodDecimals ||
		amount.Uint64() != 500_000_000_000_000_000 {
		t.Fatalf("pending amount = %d at %d decimals, want 0.5 share at 18", amount.Uint64(), amount.Decimals())
	}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: d.caller.String()})
	res, err := adapters.HTTP{Pending: votes}.GetMyPendingVotes(ctx, api.GetMyPendingVotesRequestObject{})
	wire, ok := res.(api.GetMyPendingVotes200JSONResponse)
	if err != nil || !ok || len(wire) != 1 || wire[0].TokenDecimals == nil ||
		*wire[0].TokenDecimals != robinhoodDecimals {
		t.Fatalf("GetMyPendingVotes = %#v, %v, want token_decimals 18", res, err)
	}
}

func TestListProposals_robinhoodSellCarriesTheListingDecimals(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	d.robinhoodSell(t, 500_000_000_000_000_000)
	page, err := d.robinhoodReads().List(t.Context(), app.ListProposals{
		CabalID: ids.CabalIDFrom(d.cabal), Caller: ids.UserIDFrom(d.caller), Limit: 5,
	})
	if err != nil || len(page.Items) != 1 || page.Items[0].TokenDecimals != robinhoodDecimals {
		t.Fatalf("List = %+v, %v, want one item at 18 decimals", page.Items, err)
	}
}

var (
	_ = money.NewBaseUnits
	_ = time.Second
)

func TestListProposals_robinhoodSellOfAnUnknownSymbolFailsTheRead(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	id := d.robinhoodSell(t, 7)
	if _, err := d.pool.Exec(t.Context(), `UPDATE proposals SET symbol = 'GONEx' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	_, err := d.robinhoodReads().List(t.Context(), app.ListProposals{
		CabalID: ids.CabalIDFrom(d.cabal), Caller: ids.UserIDFrom(d.caller), Limit: 5,
	})
	if errs.CodeOf(err) != errs.CodeAssetNotFound {
		t.Fatalf("List = %v, want asset_not_found", err)
	}
}

func TestPendingVotes_robinhoodSellFailsWhenTheListingCannotBeRead(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	d.robinhoodSell(t, 7)
	broken := d.reads().Listed(robinhoodListings{err: errs.New(errs.CodeDBUnavailable, "test")})
	votes := app.NewPendingVoteReads(
		broken, app.PendingVotePorts{Assets: d.assets, Replies: &replyBook{}, Cabals: d.roster()},
	)
	if _, err := votes.PendingVotes(t.Context(), ids.UserIDFrom(d.caller)); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("PendingVotes = %v, want db_unavailable", err)
	}
}
