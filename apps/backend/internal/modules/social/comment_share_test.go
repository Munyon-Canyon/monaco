package social_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/feedtest"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const (
	catalogOK int32 = iota
	catalogHidden
	catalogBroken
)

type shareCatalog struct {
	market.Catalog
	mode atomic.Int32
}

func (c *shareCatalog) AssetByID(ctx context.Context, id market.AssetID) (market.Asset, error) {
	switch c.mode.Load() {
	case catalogHidden:
		return market.Asset{}, errs.New(errs.CodeAssetNotFound, "test.AssetByID")
	case catalogBroken:
		return market.Asset{}, errs.New(errs.CodeInternal, "test.AssetByID")
	}
	return c.Catalog.AssetByID(ctx, id)
}

type countingPrices struct {
	mu                        sync.Mutex
	latest, earlier           map[market.AssetID]market.Price
	latestCalls, earlierCalls int
	latestErr, earlierErr     error
}

func (p *countingPrices) LatestPrices(context.Context) (map[market.AssetID]market.Price, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.latestCalls++
	return p.latest, p.latestErr
}

func (p *countingPrices) PricesAsOf(
	_ context.Context, _ []market.AssetID, _ time.Time,
) (map[market.AssetID]market.Price, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.earlierCalls++
	return p.earlier, p.earlierErr
}

func (p *countingPrices) calls() (int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.latestCalls, p.earlierCalls
}

func (p *countingPrices) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.latestCalls, p.earlierCalls = 0, 0
}

func price(micros uint64) market.Price {
	return market.Price{
		Micros:     money.MicrosFromUint64(micros),
		ObservedAt: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC),
	}
}

type shareThread struct {
	quoteThread
	catalog  *shareCatalog
	prices   *countingPrices
	proposal uuid.UUID
	thesis   uuid.UUID
}

func newShareThread(t *testing.T, thesis string) shareThread {
	t.Helper()
	base := newQuoteThread(t)
	cat := &shareCatalog{
		Catalog: marketfake.NewCatalog(marketfake.AAPLx(), marketfake.TSLAx(), marketfake.JPSTx(), marketfake.SPACEX()),
	}
	prices := &countingPrices{
		latest: map[market.AssetID]market.Price{
			marketfake.AAPLx().ID: price(180_000_000), marketfake.SPACEX().ID: price(95_000_000),
			marketfake.JPSTx().ID: price(50_000_000),
		},
		earlier: map[market.AssetID]market.Price{
			marketfake.AAPLx().ID: price(181_000_000), marketfake.SPACEX().ID: price(90_000_000),
			marketfake.JPSTx().ID: price(0),
		},
	}
	deps := module.Deps{Pool: base.pool, UoW: base.uow, IDs: base.gen, Clock: base.clock}
	base.routes = social.HTTPOf(
		social.New(deps, social.WithUsers(base.users), social.WithAssets(cat), social.WithPrices(prices)),
	)
	ref := base.gen.NewV7()
	item := feedtest.Item(t, base.pool, base.clock, base.gen, func(it *feed.Item) {
		it.Kind, it.CabalID, it.ActorID, it.RefID, it.Body = feed.KindProposal, base.cabal.ID, base.cabal.Creator.ID, ref, thesis
		it.Payload = feed.Payload{
			CabalName: "Alpha Cabal", ActorName: "Ada", Symbol: "AAPLx", AssetName: "Apple", Action: feed.ActionBuy,
			USDCMicros: money.MicrosFromUint64(250_000_000), TokenAmount: 1_500_000, TokenDecimals: 6,
			ShareOfPotBps: 1000,
		}
	})
	return shareThread{quoteThread: base, catalog: cat, prices: prices, proposal: ref, thesis: item}
}

func (f shareThread) member() ids.UserID { return f.cabal.Members[0].ID }

func thesisRequest(start, end *int) *api.CreateCommentRequest {
	var u api.CommentAttachmentInput
	if err := u.FromThesisQuoteAttachmentInput(api.ThesisQuoteAttachmentInput{
		Kind: api.ThesisQuoteAttachmentInputKindThesisQuote, Start: start, End: end,
	}); err != nil {
		panic(err)
	}
	return &api.CreateCommentRequest{Attachment: &u}
}

func stockRequest(asset uuid.UUID) *api.CreateCommentRequest {
	var u api.CommentAttachmentInput
	if err := u.FromStockAttachmentInput(api.StockAttachmentInput{
		Kind: api.StockAttachmentInputKindStock, AssetId: asset,
	}); err != nil {
		panic(err)
	}
	return &api.CreateCommentRequest{Attachment: &u}
}

func reasonIs(t *testing.T, err error, want string) {
	t.Helper()
	wantCode(t, err, errs.CodeInvalidInput)
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %v, want an errs.Error", err)
	}
	for _, a := range e.Attrs {
		if a.Key == "reason" && a.Value.String() == want {
			return
		}
	}
	t.Fatalf("err attrs = %v, want reason=%s", e.Attrs, want)
}

func (f shareThread) thesisOf(t *testing.T, c api.Comment) api.ThesisQuoteAttachment {
	t.Helper()
	if c.Attachment == nil {
		t.Fatalf("comment %+v has no attachment", c)
	}
	q, err := c.Attachment.AsThesisQuoteAttachment()
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func stockOf(t *testing.T, c api.Comment) api.StockAttachment {
	t.Helper()
	if c.Attachment == nil {
		t.Fatalf("comment %+v has no attachment", c)
	}
	s, err := c.Attachment.AsStockAttachment()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (f shareThread) post2(t *testing.T, item uuid.UUID, req *api.CreateCommentRequest) (api.Comment, error) {
	t.Helper()
	return f.post(t, item, f.member(), req)
}

func TestThesisQuote_aFullQuoteStoresTheServerCutThesis(t *testing.T) {
	t.Parallel()
	thesis := "Small add to the position. Earnings next week."
	f := newShareThread(t, thesis)
	created, err := f.post2(t, f.thesis, thesisRequest(nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]api.Comment{"created": created, "listed": f.pageOf(t, f.thesis).Items[0].Comment} {
		q := f.thesisOf(t, c)
		if q.Kind != api.ThesisQuoteAttachmentKindThesisQuote || q.Text != thesis || q.ProposalId != f.proposal ||
			q.ProposerName != "Ada" || c.Body != nil || c.BodyDisplay != "Quoted the thesis" {
			t.Errorf("%s = %+v quote %+v, want the whole thesis by Ada and body_display Quoted the thesis", name, c, q)
		}
	}
}

func (f shareThread) pageOf(t *testing.T, item uuid.UUID) api.CommentPage {
	t.Helper()
	limit := 100
	res, err := f.routes.GetFeedComments(asUser(t.Context(), f.member()), api.GetFeedCommentsRequestObject{
		Id: item, Params: api.GetFeedCommentsParams{Limit: &limit},
	})
	if err != nil {
		t.Fatal(err)
	}
	return api.CommentPage(res.(api.GetFeedComments200JSONResponse))
}

func TestThesisQuote_aPartialQuoteCutsByRunes(t *testing.T) {
	t.Parallel()
	f := newShareThread(t, "Naïve add — earnings next week.")
	start, end := 2, 3
	created, err := f.post2(t, f.thesis, thesisRequest(&start, &end))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.thesisOf(t, created).Text; got != "ï" {
		t.Fatalf("text = %q, want the one rune at offset 2", got)
	}
	from := 12
	created, err = f.post2(t, f.thesis, thesisRequest(&from, nil))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.thesisOf(t, created).Text; got != "earnings next week." {
		t.Fatalf("text = %q, want the thesis from rune 12 on", got)
	}
}

func TestThesisQuote_isRefusedOffAProposalOrWithoutAThesisOrWithABadRange(t *testing.T) {
	t.Parallel()
	f := newShareThread(t, "Small add.")
	trade := f.commentRoutes.item(t, feed.KindTrade)
	_, err := f.post2(t, trade, thesisRequest(nil, nil))
	reasonIs(t, err, "no_thesis")
	neg, big, zero := -1, 11, 0
	for name, req := range map[string]*api.CreateCommentRequest{
		"negative start": thesisRequest(&neg, nil), "end past the thesis": thesisRequest(nil, &big),
		"start at end": thesisRequest(&zero, &zero), "start past the end": thesisRequest(&big, nil),
	} {
		_, err = f.post2(t, f.thesis, req)
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: error = %v, want invalid_input", name, err)
		}
	}
	if comments, attachments := f.rowsOf(t, f.thesis); comments != 0 || attachments != 0 {
		t.Fatalf("rows = %d comments, %d attachments, want none", comments, attachments)
	}
}

func TestThesisQuote_isRefusedOnAProposalWithoutAThesis(t *testing.T) {
	t.Parallel()
	f := newShareThread(t, "")
	_, err := f.post2(t, f.thesis, thesisRequest(nil, nil))
	reasonIs(t, err, "no_thesis")
}

func (f shareThread) rowsOf(t *testing.T, item uuid.UUID) (comments, attachments int) {
	t.Helper()
	err := f.pool.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM feed_comments WHERE feed_object_id = $1),
		(SELECT count(*) FROM comment_attachments WHERE feed_object_id = $1)`, item).Scan(&comments, &attachments)
	if err != nil {
		t.Fatal(err)
	}
	return comments, attachments
}

func TestStock_returnsTheLivePriceAndTheDayChange(t *testing.T) {
	t.Parallel()
	f := newShareThread(t, "Small add.")
	aapl := marketfake.AAPLx()
	created, err := f.post2(t, f.thesis, stockRequest(aapl.ID.UUID()))
	if err != nil {
		t.Fatal(err)
	}
	if created.BodyDisplay != "Shared a stock" || created.Body != nil {
		t.Fatalf("created = %+v, want body_display Shared a stock", created)
	}
	for name, c := range map[string]api.Comment{"created": created, "listed": f.pageOf(t, f.thesis).Items[0].Comment} {
		wantAAPL(t, name, stockOf(t, c))
	}
}

func wantAAPL(t *testing.T, name string, s api.StockAttachment) {
	t.Helper()
	aapl := marketfake.AAPLx()
	if s.Kind != api.StockAttachmentKindStock || s.AssetId != aapl.ID.UUID() || s.Symbol != aapl.Symbol ||
		s.Name != aapl.DisplayName || s.LogoUrl != aapl.LogoURL || s.IsPreIpo {
		t.Errorf("%s stock = %+v, want the AAPL snapshot", name, s)
	}
	if s.PriceMicros == nil || *s.PriceMicros != 180_000_000 || s.ChangeBps == nil || *s.ChangeBps != -55 {
		t.Errorf("%s stock = %+v, want 180000000 and down 55 bps", name, s)
	}
}

func TestStock_aPreIPOStockHasAnEstimateAndNoChange(t *testing.T) {
	t.Parallel()
	f := newShareThread(t, "Small add.")
	spacex := marketfake.SPACEX()
	created, err := f.post2(t, f.thesis, stockRequest(spacex.ID.UUID()))
	if err != nil {
		t.Fatal(err)
	}
	s := stockOf(t, f.pageOf(t, f.thesis).Items[0].Comment)
	if !s.IsPreIpo || s.PriceMicros == nil || *s.PriceMicros != 95_000_000 || s.ChangeBps != nil {
		t.Fatalf("stock = %+v, want the estimate and change_bps null", s)
	}
	if got := stockOf(t, created); got.ChangeBps != nil || !got.IsPreIpo {
		t.Fatalf("created stock = %+v, want change_bps null", got)
	}
}

func TestStock_aMissingEarlierPriceOrAZeroOneLeavesTheChangeNull(t *testing.T) {
	t.Parallel()
	f := newShareThread(t, "Small add.")
	f.prices.latest[marketfake.TSLAx().ID] = price(250_000_000)
	for _, asset := range []market.Asset{marketfake.TSLAx(), marketfake.JPSTx()} {
		created, err := f.post2(t, f.thesis, stockRequest(asset.ID.UUID()))
		if err != nil {
			t.Fatal(err)
		}
		if s := stockOf(t, created); s.ChangeBps != nil || s.PriceMicros == nil {
			t.Errorf("%s stock = %+v, want a price and no change", asset.Symbol, s)
		}
	}
}

func TestStock_aPageOfTenStockRepliesMakesOneBatchedPriceCall(t *testing.T) {
	t.Parallel()
	f := newShareThread(t, "Small add.")
	for range 10 {
		if _, err := f.post2(t, f.thesis, stockRequest(marketfake.AAPLx().ID.UUID())); err != nil {
			t.Fatal(err)
		}
		f.clock.Advance(time.Second)
	}
	f.prices.reset()
	if got := len(f.pageOf(t, f.thesis).Items); got != 10 {
		t.Fatalf("page has %d threads, want 10", got)
	}
	if latest, earlier := f.prices.calls(); latest != 1 || earlier != 1 {
		t.Fatalf("LatestPrices %d times and PricesAsOf %d times, want once each", latest, earlier)
	}
}

func TestStock_refusesAnUnknownAssetAndWritesNothing(t *testing.T) {
	t.Parallel()
	f := newShareThread(t, "Small add.")
	_, err := f.post2(t, f.thesis, stockRequest(f.gen.NewV7()))
	reasonIs(t, err, "asset_not_found")
	if comments, attachments := f.rowsOf(t, f.thesis); comments != 0 || attachments != 0 {
		t.Fatalf("rows = %d comments, %d attachments, want none", comments, attachments)
	}
}

func TestStock_providerFailuresFailTheCallAndAGoneAssetKeepsItsSnapshot(t *testing.T) {
	t.Parallel()
	f := newShareThread(t, "Small add.")
	aapl := marketfake.AAPLx().ID.UUID()
	if _, err := f.post2(t, f.thesis, stockRequest(aapl)); err != nil {
		t.Fatal(err)
	}
	down := errs.New(errs.CodeInternal, "test.Prices")
	f.prices.latestErr = down
	if _, err := f.post2(t, f.thesis, stockRequest(aapl)); err == nil {
		t.Fatal("create with LatestPrices failing = nil error")
	}
	f.prices.latestErr, f.prices.earlierErr = nil, down
	if _, err := f.post2(t, f.thesis, stockRequest(aapl)); err == nil {
		t.Fatal("create with PricesAsOf failing = nil error")
	}
	limit := 10
	_, err := f.routes.GetFeedComments(asUser(t.Context(), f.member()), api.GetFeedCommentsRequestObject{
		Id: f.thesis, Params: api.GetFeedCommentsParams{Limit: &limit},
	})
	if err == nil {
		t.Fatal("list with PricesAsOf failing = nil error")
	}
	f.prices.earlierErr = nil
	f.catalog.mode.Store(catalogBroken)
	if _, err := f.post2(t, f.thesis, stockRequest(aapl)); err == nil {
		t.Fatal("create with the catalog failing = nil error")
	}
	f.catalog.mode.Store(catalogHidden)
	s := stockOf(t, f.pageOf(t, f.thesis).Items[0].Comment)
	if s.Symbol != marketfake.AAPLx().Symbol || s.PriceMicros != nil || s.ChangeBps != nil {
		t.Fatalf("stock = %+v, want the snapshot and a null price", s)
	}
}

func TestAttachment_aMalformedUnionIsRefused(t *testing.T) {
	t.Parallel()
	f := newShareThread(t, "Small add.")
	var bad api.CommentAttachmentInput
	if err := bad.UnmarshalJSON([]byte(`{"kind":"gif","gif":5}`)); err != nil {
		t.Fatal(err)
	}
	_, err := f.post2(t, f.thesis, &api.CreateCommentRequest{Attachment: &bad})
	reasonIs(t, err, "invalid_attachment")
}
