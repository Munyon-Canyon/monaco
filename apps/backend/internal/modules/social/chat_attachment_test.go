package social_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/feedtest"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type chatAttach struct {
	chatRoutes
	catalog *shareCatalog
}

func newChatAttach(t *testing.T) chatAttach {
	t.Helper()
	f := newChatRoutes(t)
	cat := &shareCatalog{Catalog: marketfake.NewCatalog(marketfake.AAPLx(), marketfake.SPACEX())}
	prices := &countingPrices{
		latest: map[market.AssetID]market.Price{
			marketfake.AAPLx().ID: price(180_000_000), marketfake.SPACEX().ID: price(95_000_000),
		},
		earlier: map[market.AssetID]market.Price{
			marketfake.AAPLx().ID: price(181_000_000), marketfake.SPACEX().ID: price(90_000_000),
		},
	}
	deps := module.Deps{Pool: f.pool, UoW: f.deps.UoW, IDs: f.deps.IDs, Clock: f.clock}
	f.routes = social.HTTPOf(social.New(
		deps, social.WithUsers(f.users), social.WithRealtime(f.rt), social.WithAssets(cat), social.WithPrices(prices),
	))
	return chatAttach{chatRoutes: f, catalog: cat}
}

func (f chatAttach) proposal(t *testing.T, cabal ids.CabalID, thesis string) uuid.UUID {
	t.Helper()
	ref := f.deps.IDs.NewV7()
	feedtest.Item(t, f.pool, f.clock, f.deps.IDs, func(it *feed.Item) {
		it.Kind, it.CabalID, it.ActorID, it.RefID, it.Body = feed.KindProposal, cabal, f.cabal.Creator.ID, ref, thesis
		it.Payload = feed.Payload{
			CabalName: "Alpha Cabal", ActorName: "Ada", Symbol: "AAPLx", AssetName: "Apple", Action: feed.ActionBuy,
			USDCMicros: money.MicrosFromUint64(250_000_000),
		}
	})
	return ref
}

func chatGif() *api.ChatAttachmentInput {
	var u api.ChatAttachmentInput
	still := gifMedia("200_s.gif")
	still.Url += "?cid=abc"
	if err := u.FromGifAttachmentInput(api.GifAttachmentInput{
		Kind: api.GifAttachmentInputKindGif,
		Gif: api.GifInput{
			Id: gifID, Title: "happy dance", Still: still, Preview: gifMedia("200w.mp4"),
			Original: gifMedia("giphy.gif"),
		},
	}); err != nil {
		panic(err)
	}
	return &u
}

func chatStock(asset uuid.UUID) *api.ChatAttachmentInput {
	var u api.ChatAttachmentInput
	if err := u.FromStockAttachmentInput(
		api.StockAttachmentInput{Kind: api.StockAttachmentInputKindStock, AssetId: asset},
	); err != nil {
		panic(err)
	}
	return &u
}

func chatThesis(proposal uuid.UUID, start, end *int) *api.ChatAttachmentInput {
	var u api.ChatAttachmentInput
	if err := u.FromChatThesisQuoteAttachmentInput(api.ChatThesisQuoteAttachmentInput{
		Kind: api.ChatThesisQuoteAttachmentInputKindThesisQuote, ProposalId: proposal, Start: start, End: end,
	}); err != nil {
		panic(err)
	}
	return &u
}

func (f chatAttach) stored(t *testing.T) int {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM chat_message_attachments`)
}

func wantAttachmentBody(t *testing.T, name string, m api.ChatMessage, body string) {
	t.Helper()
	if m.Body == nil || *m.Body != body || m.Attachment == nil {
		t.Fatalf("%s = %+v, want body %q and an attachment", name, m, body)
	}
}

func TestChatAttachment_aGifAloneReadsAsGIFEverywhere(t *testing.T) {
	t.Parallel()
	f := newChatAttach(t)
	created := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Attachment: chatGif()})
	listed, err := f.channel(t, f.member(0), api.GetChatMessagesParams{})
	if err != nil || len(listed) != 1 {
		t.Fatalf("channel = %+v, %v", listed, err)
	}
	for name, m := range map[string]api.ChatMessage{"created": created, "listed": listed[0]} {
		wantAttachmentBody(t, name, m, "GIF")
		gif, err := m.Attachment.AsGifAttachment()
		if err != nil || gif.Id != gifID || gif.Still != gifMedia("200_s.gif") {
			t.Fatalf("%s gif = %+v, %v, want the posted one without its query string", name, gif, err)
		}
	}
	if f.stored(t) != 1 || f.posted(t) != 1 {
		t.Fatalf("attachments = %d, events = %d, want 1 and 1", f.stored(t), f.posted(t))
	}
}

func TestChatAttachment_aTextMessageKeepsItsTextBesideTheAttachment(t *testing.T) {
	t.Parallel()
	f := newChatAttach(t)
	created := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: ptr("lol"), Attachment: chatGif()})
	wantAttachmentBody(t, "created", created, "lol")
	blank := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: ptr("  "), Attachment: chatGif()})
	wantAttachmentBody(t, "blank text", blank, "GIF")
	plain := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: ptr("plain")})
	if plain.Attachment != nil {
		t.Fatalf("plain = %+v, want no attachment", plain)
	}
}

func TestChatAttachment_aStockCarriesTheLivePriceInChannelAndThread(t *testing.T) {
	t.Parallel()
	f := newChatAttach(t)
	top := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Attachment: chatStock(marketfake.AAPLx().ID.UUID())})
	reply := f.mustPost(t, f.member(0), api.PostChatMessageRequest{
		Attachment: chatStock(marketfake.SPACEX().ID.UUID()), ParentId: &top.Id,
	})
	listed, err := f.channel(t, f.member(0), api.GetChatMessagesParams{})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := f.thread(t, f.member(0), top.Id, api.GetChatThreadParams{})
	if err != nil {
		t.Fatal(err)
	}
	wantAttachmentBody(t, "created", top, "Shared a stock")
	wantAttachmentBody(t, "reply", reply, "Shared a stock")
	for name, m := range map[string]api.ChatMessage{"listed": listed[0], "parent": thread.Parent} {
		wantAttachmentBody(t, name, m, "Shared a stock")
		stock, err := m.Attachment.AsStockAttachment()
		if err != nil || stock.Symbol != marketfake.AAPLx().Symbol || stock.PriceMicros == nil ||
			*stock.PriceMicros != 180_000_000 {
			t.Fatalf("%s stock = %+v, %v, want AAPLx at 180000000", name, stock, err)
		}
	}
	replied, err := thread.Replies[0].Attachment.AsStockAttachment()
	if err != nil || replied.Symbol != marketfake.SPACEX().Symbol {
		t.Fatalf("reply stock = %+v, %v", replied, err)
	}
}

func TestChatAttachment_aGoneAssetKeepsItsSnapshotWithNoPrice(t *testing.T) {
	t.Parallel()
	f := newChatAttach(t)
	f.mustPost(t, f.member(0), api.PostChatMessageRequest{Attachment: chatStock(marketfake.AAPLx().ID.UUID())})
	f.catalog.mode.Store(catalogHidden)
	listed, err := f.channel(t, f.member(0), api.GetChatMessagesParams{})
	if err != nil {
		t.Fatal(err)
	}
	stock, err := listed[0].Attachment.AsStockAttachment()
	if err != nil || stock.Symbol != marketfake.AAPLx().Symbol || stock.PriceMicros != nil {
		t.Fatalf("stock = %+v, %v, want the snapshot and a null price", stock, err)
	}
	f.catalog.mode.Store(catalogBroken)
	if _, err := f.channel(t, f.member(0), api.GetChatMessagesParams{}); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("channel error = %v, want internal", err)
	}
}

func TestChatAttachment_anUnknownAssetWritesNothing(t *testing.T) {
	t.Parallel()
	f := newChatAttach(t)
	_, err := f.postAs(t, f.member(0), api.PostChatMessageRequest{Attachment: chatStock(f.deps.IDs.NewV7())})
	reasonIs(t, err, "asset_not_found")
	f.catalog.mode.Store(catalogBroken)
	_, err = f.postAs(t, f.member(0), api.PostChatMessageRequest{
		Attachment: chatStock(marketfake.AAPLx().ID.UUID()),
	})
	wantCode(t, err, errs.CodeInternal)
	if f.count(t, `SELECT count(*) FROM cabal_messages`) != 0 || f.stored(t) != 0 {
		t.Fatal("a refused attachment wrote rows")
	}
}

func TestChatAttachment_aThesisQuoteIsCutByTheServer(t *testing.T) {
	t.Parallel()
	f := newChatAttach(t)
	ref := f.proposal(t, f.cabal.ID, "Small add to the position.")
	whole := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Attachment: chatThesis(ref, nil, nil)})
	wantAttachmentBody(t, "whole", whole, "Quoted the thesis")
	part := f.mustPost(t, f.member(0), api.PostChatMessageRequest{
		Body: ptr("this"), Attachment: chatThesis(ref, ptr(0), ptr(9)),
	})
	wantAttachmentBody(t, "part", part, "this")
	listed, err := f.channel(t, f.member(0), api.GetChatMessagesParams{})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"part": "Small add", "whole": "Small add to the position."} {
		m := listed[0]
		if name == "whole" {
			m = listed[1]
		}
		q, err := m.Attachment.AsThesisQuoteAttachment()
		if err != nil || q.Text != want || q.ProposerName != "Ada" || q.ProposalId != ref {
			t.Fatalf("%s quote = %+v, %v, want %q by Ada", name, q, err, want)
		}
	}
}

func TestChatAttachment_aThesisQuoteIsRefusedWithItsReason(t *testing.T) {
	t.Parallel()
	f := newChatAttach(t)
	ref := f.proposal(t, f.cabal.ID, "Small add.")
	cases := map[string]struct {
		att    *api.ChatAttachmentInput
		reason string
	}{
		"other cabal":    {chatThesis(f.proposal(t, f.other.ID, "Not yours."), nil, nil), "no_thesis"},
		"no thesis":      {chatThesis(f.proposal(t, f.cabal.ID, ""), nil, nil), "no_thesis"},
		"unknown":        {chatThesis(f.deps.IDs.NewV7(), nil, nil), "no_thesis"},
		"nil proposal":   {chatThesis(uuid.Nil, nil, nil), "no_proposal"},
		"past the end":   {chatThesis(ref, nil, ptr(99)), "invalid_range"},
		"empty range":    {chatThesis(ref, ptr(3), ptr(3)), "invalid_range"},
		"negative start": {chatThesis(ref, ptr(-1), nil), "invalid_range"},
	}
	for name, c := range cases {
		_, err := f.postAs(t, f.member(0), api.PostChatMessageRequest{Attachment: c.att})
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: error = %v, want invalid_input", name, err)
			continue
		}
		reasonIs(t, err, c.reason)
	}
	if f.count(t, `SELECT count(*) FROM cabal_messages`) != 0 {
		t.Fatal("a refused quote wrote a message")
	}
}

func TestChatAttachment_refusesBadShapes(t *testing.T) {
	t.Parallel()
	f := newChatAttach(t)
	var photo api.ChatAttachmentInput
	if err := photo.UnmarshalJSON([]byte(`{"kind":"photo"}`)); err != nil {
		t.Fatal(err)
	}
	var oneOption api.ChatAttachmentInput
	if err := oneOption.UnmarshalJSON([]byte(`{"kind":"poll","question":"q","options":["a"]}`)); err != nil {
		t.Fatal(err)
	}
	var badGif api.ChatAttachmentInput
	if err := badGif.UnmarshalJSON([]byte(`{"kind":"gif","gif":{"id":"!"}}`)); err != nil {
		t.Fatal(err)
	}
	var mangled api.ChatAttachmentInput
	if err := mangled.UnmarshalJSON([]byte(`{"kind":"gif","gif":"x"}`)); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		req  api.PostChatMessageRequest
		code errs.Code
	}{
		"mangled gif":  {api.PostChatMessageRequest{Attachment: &mangled}, errs.CodeInvalidInput},
		"empty":        {api.PostChatMessageRequest{}, errs.CodeChatBodyInvalid},
		"blank":        {api.PostChatMessageRequest{Body: ptr("  ")}, errs.CodeChatBodyInvalid},
		"unknown kind": {api.PostChatMessageRequest{Attachment: &photo}, errs.CodeInvalidInput},
		"one option":   {api.PostChatMessageRequest{Attachment: &oneOption}, errs.CodeInvalidInput},
		"bad gif":      {api.PostChatMessageRequest{Attachment: &badGif}, errs.CodeInvalidInput},
	} {
		if _, err := f.postAs(t, f.member(0), c.req); errs.CodeOf(err) != c.code {
			t.Errorf("%s: error = %v, want %s", name, err, c.code)
		}
	}
	if _, err := f.postAs(t, f.outsider, api.PostChatMessageRequest{Attachment: chatGif()}); errs.CodeOf(err) !=
		errs.CodeNotCabalMember {
		t.Fatalf("outsider error = %v, want not_cabal_member", err)
	}
	if f.stored(t) != 0 || f.posted(t) != 0 {
		t.Fatal("a refused post wrote rows or events")
	}
}

func TestChatAttachment_aDeletedMessageReturnsNoAttachment(t *testing.T) {
	t.Parallel()
	f := newChatAttach(t)
	top := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Attachment: chatGif()})
	f.mustPost(t, f.member(1), api.PostChatMessageRequest{Body: ptr("re"), ParentId: &top.Id})
	if err := f.remove(t, f.member(0), top.Id); err != nil {
		t.Fatal(err)
	}
	listed, err := f.channel(t, f.member(0), api.GetChatMessagesParams{})
	if err != nil || len(listed) != 1 {
		t.Fatalf("channel = %+v, %v", listed, err)
	}
	if got := listed[0]; !got.Deleted || got.Attachment != nil || got.Body != nil {
		t.Fatalf("deleted message = %+v, want no body and no attachment", got)
	}
}

func TestChatAttachment_aPageLoadsItsAttachmentsInOneQuery(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	counter := &queryCounter{}
	counting := derivedPool(t, f.pool, func(c *pgxpool.Config) { c.ConnConfig.Tracer = counter })
	top := f.mustSend(t, f.member(0), "plain", nil)
	msgs := []app.ChatMessage{{ID: top.ID}, {ID: f.deps.IDs.NewV7()}, {ID: f.deps.IDs.NewV7(), Deleted: true}}
	att := &domain.Attachment{Kind: domain.AttachmentGIF, GIF: &domain.GifAttachment{ID: gifID}}
	_, err := f.pool.Exec(t.Context(),
		`INSERT INTO chat_message_attachments (message_id, cabal_id, kind, payload) VALUES ($1, $2, 'gif', $3)`,
		top.ID, f.cabal.ID.UUID(), att.Payload(f.now))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.LoadChatAttachments(t.Context(), counting, nil, msgs); err != nil {
		t.Fatal(err)
	}
	if counter.n.Load() != 1 || msgs[0].Attachment == nil || msgs[1].Attachment != nil || msgs[2].Attachment != nil {
		t.Fatalf("queries = %d, attachments = %+v, want 1 and only the first", counter.n.Load(), msgs)
	}
	if err := app.LoadChatAttachments(t.Context(), counting, nil, msgs[2:]); err != nil || counter.n.Load() != 1 {
		t.Fatalf("a page of deleted messages queried: %v, %d", err, counter.n.Load())
	}
}

type queryCounter struct{ n atomic.Int32 }

func (c *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.n.Add(1)
	return ctx
}

func (*queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func derivedPool(t *testing.T, from *pgxpool.Pool, edit func(*pgxpool.Config)) *pgxpool.Pool {
	t.Helper()
	cfg := from.Config().Copy()
	edit(cfg)
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func downPool(t *testing.T, from *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	return derivedPool(t, from, func(c *pgxpool.Config) {
		c.ConnConfig.Host, c.ConnConfig.Port = "127.0.0.1", 1
		c.ConnConfig.ConnectTimeout = time.Second
	})
}

func TestChatAttachment_storeFailuresSurfaceAsInternal(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	if err := app.LoadChatAttachments(
		t.Context(),
		downPool(t, f.pool),
		nil,
		[]app.ChatMessage{{ID: f.deps.IDs.NewV7()}},
	); errs.CodeOf(
		err,
	) !=
		errs.CodeInternal {
		t.Fatalf("LoadChatAttachments error = %v, want internal", err)
	}
	deps := f.deps
	deps.Reads = downPool(t, f.pool)
	thesis := domain.Attachment{
		Kind:   domain.AttachmentThesisQuote,
		Thesis: &domain.ThesisQuote{ProposalID: f.deps.IDs.NewV7()},
	}
	_, err := app.NewPostChatMessageHandler(deps).Handle(f.as(t, f.member(0)), app.PostChatMessage{
		CabalID: f.cabal.ID, Author: f.member(0), Input: domain.ChatInput{Attachment: &thesis},
	})
	wantCode(t, err, errs.CodeInternal)
	odd := domain.Attachment{Kind: "photo"}
	_, err = f.post.Handle(f.as(t, f.member(0)), app.PostChatMessage{
		CabalID: f.cabal.ID, Author: f.member(0), Input: domain.ChatInput{Attachment: &odd},
	})
	wantCode(t, err, errs.CodeInternal)
	if f.count(t, `SELECT count(*) FROM cabal_messages`) != 0 {
		t.Fatal("a failed attachment insert left its message")
	}
}

func TestChatAttachment_aMixedPageAndACorruptRowStillRender(t *testing.T) {
	t.Parallel()
	f := newChatAttach(t)
	f.mustPost(t, f.member(0), api.PostChatMessageRequest{Attachment: chatStock(marketfake.AAPLx().ID.UUID())})
	f.mustPost(t, f.member(0), api.PostChatMessageRequest{Attachment: chatGif()})
	broken := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: ptr("keep"), Attachment: chatGif()})
	if _, err := f.pool.Exec(t.Context(),
		`UPDATE chat_message_attachments SET payload = '"x"' WHERE message_id = $1`, broken.Id); err != nil {
		t.Fatal(err)
	}
	listed, err := f.channel(t, f.member(0), api.GetChatMessagesParams{})
	if err != nil || len(listed) != 3 {
		t.Fatalf("channel = %+v, %v", listed, err)
	}
	if got := listed[0]; got.Attachment != nil || got.Body == nil || *got.Body != "keep" {
		t.Fatalf("corrupt row message = %+v, want its text and no attachment", got)
	}
	wantAttachmentBody(t, "gif", listed[1], "GIF")
	stock, err := listed[2].Attachment.AsStockAttachment()
	if err != nil || stock.PriceMicros == nil {
		t.Fatalf("stock = %+v, %v, want a live price", stock, err)
	}
}
