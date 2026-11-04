package social_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func momentFilledAt() time.Time { return time.Date(2026, 9, 30, 14, 30, 0, 0, time.UTC) }

func (f chatRoutes) writer() *app.WriteChatMomentHandler {
	deps := module.Deps{Pool: f.pool, UoW: f.deps.UoW, IDs: f.deps.IDs, Clock: f.clock}
	return social.New(deps, social.WithUsers(f.users), social.WithRealtime(f.rt)).WriteChatMoment()
}

func (f chatRoutes) writeMoment(t *testing.T, m domain.ChatMoment, key string) (app.ChatMessage, error) {
	t.Helper()
	return f.writer().Handle(t.Context(), app.WriteChatMoment{
		CabalID: f.cabal.ID, Moment: m, At: f.clock.Now(), DedupeKey: key,
	})
}

func (f chatRoutes) mustWriteJoined(t *testing.T, key string) app.ChatMessage {
	t.Helper()
	joined, err := domain.NewMemberJoinedMoment(f.member(0))
	if err != nil {
		t.Fatal(err)
	}
	m, err := f.writeMoment(t, joined, key)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func (f chatRoutes) momentsOn() api.GetChatMessagesParams {
	on := true
	return api.GetChatMessagesParams{Moments: &on}
}

func TestWriteChatMoment_writesOncePerDedupeKeyAndPublishesOnce(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	first := f.mustWriteJoined(t, "joined:1")
	again := f.mustWriteJoined(t, "joined:1")
	if first.ID != again.ID || first.Kind != domain.ChatKindSystem || again.Moment == nil {
		t.Fatalf("first = %+v, again = %+v: a replay must return the stored row", first, again)
	}
	if n := f.count(t, `SELECT count(*) FROM cabal_messages WHERE kind = 'system'`); n != 1 {
		t.Fatalf("%d system rows, want 1", n)
	}
	if n := f.count(
		t,
		`SELECT count(*) FROM cabal_messages WHERE kind = 'system' AND author_id IS NULL AND body IS NULL`,
	); n != 1 {
		t.Fatalf("the system row has an author or a body")
	}
	published := f.rt.Published()
	if len(published) != 1 || published[0].Name != app.EventMomentCreated {
		t.Fatalf("published = %+v, want one moment.created", published)
	}
	if f.posted(t) != 0 {
		t.Fatalf("a system row appended a chat.message_posted event")
	}
	f.mustWriteJoined(t, "joined:2")
	if n := f.count(t, `SELECT count(*) FROM cabal_messages WHERE kind = 'system'`); n != 2 {
		t.Fatalf("%d system rows after a second key, want 2", n)
	}
}

func TestWriteChatMoment_refusesIncompleteCommands(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	created := domain.NewCabalCreatedMoment()
	cmds := map[string]app.WriteChatMoment{
		"no moment": {CabalID: f.cabal.ID, At: momentFilledAt(), DedupeKey: "k"},
		"no cabal":  {Moment: created, At: momentFilledAt(), DedupeKey: "k"},
		"no time":   {CabalID: f.cabal.ID, Moment: created, DedupeKey: "k"},
		"no dedupe": {CabalID: f.cabal.ID, Moment: created, At: momentFilledAt()},
	}
	for name, cmd := range cmds {
		if _, err := f.writer().Handle(t.Context(), cmd); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: err = %v, want invalid_input", name, err)
		}
	}
	if n := f.count(t, `SELECT count(*) FROM cabal_messages`); n != 0 {
		t.Fatalf("%d rows written by refused commands", n)
	}
}

func TestWriteChatMoment_aDatabaseFailureIsInternal(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	if _, err := f.pool.Exec(
		t.Context(),
		`ALTER TABLE cabal_messages RENAME COLUMN dedupe_key TO dedupe_gone`,
	); err != nil {
		t.Fatal(err)
	}
	_, err := f.writeMoment(t, domain.NewCabalCreatedMoment(), "k")
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err = %v, want internal", err)
	}
}

func TestChatShapeCheck_rejectsEveryBadCombination(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	top := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: ptr("hi")})
	cabal, author := f.cabal.ID.UUID(), f.member(0).UUID()
	cases := map[string]struct {
		query string
		args  []any
	}{
		"system with an author": {`INSERT INTO cabal_messages (id, cabal_id, author_id, kind, moment, created_at)
			VALUES (gen_random_uuid(), $1, $2, 'system', '{"kind":"cabal_created"}', now())`, []any{cabal, author}},
		"system with a body": {`INSERT INTO cabal_messages (id, cabal_id, body, kind, moment, created_at)
			VALUES (gen_random_uuid(), $1, 'hi', 'system', '{"kind":"cabal_created"}', now())`, []any{cabal}},
		"system with a parent": {`INSERT INTO cabal_messages (id, cabal_id, parent_id, kind, moment, created_at)
			VALUES (gen_random_uuid(), $1, $2, 'system', '{"kind":"cabal_created"}', now())`, []any{cabal, top.Id}},
		"system without a moment": {`INSERT INTO cabal_messages (id, cabal_id, kind, created_at)
			VALUES (gen_random_uuid(), $1, 'system', now())`, []any{cabal}},
		"user without an author": {`INSERT INTO cabal_messages (id, cabal_id, body, created_at)
			VALUES (gen_random_uuid(), $1, 'hi', now())`, []any{cabal}},
		"user with a moment": {`INSERT INTO cabal_messages (id, cabal_id, author_id, body, moment, created_at)
			VALUES (gen_random_uuid(), $1, $2, 'hi', '{"kind":"cabal_created"}', now())`, []any{cabal, author}},
		"proposal without a proposal": {`INSERT INTO cabal_messages (id, cabal_id, author_id, kind, created_at)
			VALUES (gen_random_uuid(), $1, $2, 'proposal', now())`, []any{cabal, author}},
		"unknown kind": {`INSERT INTO cabal_messages (id, cabal_id, author_id, body, kind, created_at)
			VALUES (gen_random_uuid(), $1, $2, 'hi', 'bot', now())`, []any{cabal, author}},
	}
	for name, c := range cases {
		if _, err := f.pool.Exec(t.Context(), c.query, c.args...); err == nil {
			t.Errorf("%s: the database accepted the row", name)
		}
	}
}

func TestGetChatMessages_hidesSystemRowsUnlessMomentsIsSet(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: ptr("gm")})
	f.mustWriteJoined(t, "joined:1")
	got, err := f.channel(t, f.member(1), api.GetChatMessagesParams{})
	if err != nil || len(got) != 1 || got[0].Kind != api.ChatMessageKindUser || got[0].Moment != nil {
		t.Fatalf("default read = %+v, %v: want the user message only", got, err)
	}
	got, err = f.channel(t, f.member(1), f.momentsOn())
	if err != nil || len(got) != 2 {
		t.Fatalf("moments read = %+v, %v: want both rows", got, err)
	}
}

func bareOf(row api.ChatMessage) bool {
	return row.Body == nil && row.Author.Id == uuid.Nil && row.Author.Handle == nil && row.Author.DisplayName == ""
}

func TestGetChatMessages_aSystemRowHasNoAuthorAndNoBody(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	moment := f.mustWriteJoined(t, "joined:1")
	got, err := f.channel(t, f.member(1), f.momentsOn())
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	row := got[0]
	if row.Id != moment.ID || row.Kind != api.ChatMessageKindSystem || row.Moment == nil || !bareOf(row) {
		t.Fatalf("system row = %+v", row)
	}
	joined, err := row.Moment.AsMemberJoinedMoment()
	if err != nil || joined.Kind != api.MemberJoined || joined.Member.Id != f.member(0).UUID() {
		t.Fatalf("moment = %+v, %v", joined, err)
	}
}

func TestGetChatMessages_memberJoinedShowsTheCurrentName(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	f.mustWriteJoined(t, "joined:1")
	got, err := f.channel(t, f.member(1), f.momentsOn())
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	joined, _ := got[0].Moment.AsMemberJoinedMoment()
	if joined.Member.DisplayName != "Kai" || joined.Member.Handle == nil || *joined.Member.Handle != "kai" ||
		joined.Member.PhotoUrl == nil {
		t.Fatalf("member = %+v, want Kai's current card", joined.Member)
	}
}

func TestGetChatMessages_tradeFilledReachesTheWire(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	asset, swap := f.deps.IDs.NewV7(), f.deps.IDs.NewV7()
	trade, err := domain.NewTradeFilledMoment(domain.TradeFilled{
		Side: domain.TradeSell, Symbol: "NVDA", AssetID: asset, PriceMicros: 196_750_000, USDCMicros: 300_000_000,
		FilledAt: momentFilledAt(), SwapID: swap,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.writeMoment(t, trade, "swap"); err != nil {
		t.Fatal(err)
	}
	got, err := f.channel(t, f.member(1), f.momentsOn())
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	filled, err := got[0].Moment.AsTradeFilledMoment()
	want := api.TradeFilledMoment{
		Kind: api.TradeFilled, Side: api.TradeFilledMomentSideSell, Symbol: "NVDA", AssetId: asset,
		PriceMicros: 196_750_000, UsdcMicros: 300_000_000, FilledAt: momentFilledAt(), SwapId: swap,
	}
	if err != nil || !reflect.DeepEqual(filled, want) {
		t.Fatalf("trade = %+v, %v, want %+v", filled, err, want)
	}
}

func TestGetChatMessages_cabalCreatedReachesTheWire(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	if _, err := f.writeMoment(t, domain.NewCabalCreatedMoment(), "created"); err != nil {
		t.Fatal(err)
	}
	got, err := f.channel(t, f.member(1), f.momentsOn())
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	if created, err := got[0].Moment.AsCabalCreatedMoment(); err != nil || created.Kind != api.CabalCreated {
		t.Fatalf("created = %+v, %v", created, err)
	}
}

func TestGetChatMessages_skipsAMomentKindItDoesNotKnow(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO cabal_messages (id, cabal_id, kind, moment, created_at)
		VALUES (gen_random_uuid(), $1, 'system', '{"kind":"cabal_renamed"}', now())`, f.cabal.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	f.mustWriteJoined(t, "joined:1")
	got, err := f.channel(t, f.member(1), f.momentsOn())
	if err != nil || len(got) != 1 || got[0].Kind != api.ChatMessageKindSystem {
		t.Fatalf("read = %+v, %v: want only the known moment", got, err)
	}
}

func TestGetChatThread_aSystemRowIsOnlyAThreadWithMoments(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	moment := f.mustWriteJoined(t, "joined:1")
	if _, err := f.thread(
		t,
		f.member(1),
		moment.ID,
		api.GetChatThreadParams{},
	); errs.CodeOf(
		err,
	) != errs.CodeChatMessageNotFound {
		t.Fatalf("default thread err = %v, want chat_message_not_found", err)
	}
	on := true
	got, err := f.thread(t, f.member(1), moment.ID, api.GetChatThreadParams{Moments: &on})
	if err != nil || got.Parent.Kind != api.ChatMessageKindSystem || len(got.Replies) != 0 {
		t.Fatalf("moments thread = %+v, %v", got, err)
	}
	unknown := f.deps.IDs.NewV7()
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO cabal_messages (id, cabal_id, kind, moment, created_at)
		VALUES ($1, $2, 'system', '{"kind":"cabal_renamed"}', now())`, unknown, f.cabal.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.thread(t, f.member(1), unknown,
		api.GetChatThreadParams{Moments: &on}); errs.CodeOf(err) != errs.CodeChatMessageNotFound {
		t.Fatalf("unknown moment thread err = %v, want chat_message_not_found", err)
	}
}

func TestChatMessage_aMemberCannotDeleteOrReportOrSeeSeenByOfASystemRow(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	moment := f.mustWriteJoined(t, "joined:1")
	err := f.remove(t, f.member(0), moment.ID)
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("delete err = %v, want invalid_input", err)
	}
	if n := f.count(t, `SELECT count(*) FROM cabal_messages WHERE deleted_at IS NOT NULL`); n != 0 {
		t.Fatal("the system row was deleted")
	}
	if _, err := f.seenBy(t, f.member(1), moment.ID); errs.CodeOf(err) != errs.CodeChatMessageNotFound {
		t.Fatalf("seen-by err = %v, want chat_message_not_found", err)
	}
	if err := f.removeAs(asAdmin(t), moment.ID, "spam"); err != nil {
		t.Fatalf("admin remove: %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM cabal_messages WHERE deleted_at IS NOT NULL`); n != 1 {
		t.Fatal("the admin removal did not delete the system row")
	}
}

func TestCreateReport_refusesASystemRow(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	system := f.gen.NewV7()
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO cabal_messages (id, cabal_id, kind, moment, created_at)
		VALUES ($1, $2, 'system', '{"kind":"cabal_created"}', now())`, system, f.cabal.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	_, err := f.file(t, f.reporter, domain.ReportMessage, system)
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("err = %v, want invalid_input", err)
	}
}

func TestChatMomentWire_publishesTheWiredMessage(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	moment := f.mustWriteJoined(t, "joined:1")
	published := f.rt.Published()
	if len(published) != 1 {
		t.Fatalf("published = %+v", published)
	}
	var wire api.ChatMessage
	if err := json.Unmarshal(published[0].Data, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Id != moment.ID || wire.Kind != api.ChatMessageKindSystem || wire.Moment == nil {
		t.Fatalf("wire = %+v", wire)
	}
}

func TestChatMomentWire_aFailedPublishStillStoresTheRow(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	failing := func(context.Context, app.ChatMessage) (any, error) { return nil, errs.New(errs.CodeInternal, "test") }
	deps := f.deps
	deps.Publish = app.NewChatPublisher(f.rt, failing)
	_, err := app.NewWriteChatMomentHandler(deps).Handle(t.Context(), app.WriteChatMoment{
		CabalID: f.cabal.ID, Moment: domain.NewCabalCreatedMoment(), At: momentFilledAt(), DedupeKey: "k",
	})
	if err != nil || len(f.rt.Published()) != 0 {
		t.Fatalf("err = %v, published = %+v: a failed wire must not fail the write", err, f.rt.Published())
	}
}

func TestPostChatMessage_refusesAReplyToASystemRow(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	moment := f.mustWriteJoined(t, "joined:1")
	normal, err := f.send(t, f.member(0), "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	before := f.count(t, `SELECT count(*) FROM cabal_messages`)
	_, err = f.send(t, f.member(1), "reply", &domain.Reply{Parent: moment.ID})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("err = %v, want invalid_input", err)
	}
	var reason string
	for _, attr := range errs.Detail(err) {
		if attr.Key == "reason" {
			reason = attr.Value.String()
		}
	}
	if reason != domain.ReasonSystemMessage {
		t.Fatalf("reason = %q, want %q", reason, domain.ReasonSystemMessage)
	}
	if after := f.count(t, `SELECT count(*) FROM cabal_messages`); after != before {
		t.Fatalf("rows = %d, want %d: the refused reply wrote a row", after, before)
	}
	if _, err := f.send(t, f.member(1), "reply", &domain.Reply{Parent: normal.ID}); err != nil {
		t.Fatalf("reply to a user row: %v", err)
	}
}
