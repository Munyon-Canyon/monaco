package social_test

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const publishFailedLog = "social.chat_publish_failed"

func jsonOf(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func (f chatFixture) channel() string { return "cabal:" + f.cabal.ID.String() }

func (f chatFixture) loggedCtx(t *testing.T, user ids.UserID) (context.Context, *testkit.Logs) {
	t.Helper()
	logs := &testkit.Logs{}
	logger := observability.NewLogger(config.Config{Env: config.EnvTest}, logs)
	return observability.WithLogger(f.as(t, user), logger), logs
}

func TestPostChatMessage_publishesTheStoredMessageOnTheCabalChannel(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	got := f.mustSend(t, f.member(0), "gm frens", nil)
	want := []testkit.RealtimePublish{{
		Channel: f.channel(), Name: app.EventMessageCreated,
		Data: jsonOf(t, map[string]any{"id": got.ID, "body": "gm frens"}),
	}}
	if published := f.rt.Published(); !reflect.DeepEqual(published, want) {
		t.Fatalf("published = %+v, want %+v", published, want)
	}
}

func TestPostChatMessage_aReplyAlsoPublishesTheParentsThreadUpdate(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	top := f.mustSend(t, f.member(0), "top", nil)
	f.clock.Advance(time.Minute)
	reply := f.mustSend(t, f.member(1), "reply", &domain.Reply{Parent: top.ID})
	published := f.rt.Published()
	if len(published) != 3 {
		t.Fatalf("published = %+v, want the top message, then the reply and the thread update", published)
	}
	thread := app.ThreadUpdated{MessageID: top.ID, ReplyCount: 1, LastReplyAt: reply.CreatedAt}
	wantReply := testkit.RealtimePublish{
		Channel: f.channel(), Name: app.EventMessageCreated,
		Data: jsonOf(t, map[string]any{"id": reply.ID, "body": "reply"}),
	}
	wantThread := testkit.RealtimePublish{Channel: f.channel(), Name: app.EventThreadUpdated, Data: jsonOf(t, thread)}
	if !reflect.DeepEqual(published[1:], []testkit.RealtimePublish{wantReply, wantThread}) {
		t.Fatalf("published = %+v, want %+v then %+v", published[1:], wantReply, wantThread)
	}
}

type commitProbe struct {
	f    chatFixture
	seen *[]int
}

func (p commitProbe) Publish(ctx context.Context, _ string, _ string, _ any) error {
	var n int
	if err := p.f.pool.QueryRow(ctx, `SELECT count(*) FROM cabal_messages WHERE cabal_id = $1`, p.f.cabal.ID.UUID()).
		Scan(&n); err != nil {
		return err
	}
	*p.seen = append(*p.seen, n)
	return nil
}

func (commitProbe) TokenRequest(context.Context, ids.UserID, []string, time.Duration) (app.TokenRequest, error) {
	return app.TokenRequest{}, nil
}

func TestPostChatMessage_publishesOnlyAfterTheRowIsCommitted(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	var seen []int
	deps := f.deps
	deps.Publish = app.NewChatPublisher(commitProbe{f: f, seen: &seen}, plainWire)
	body, err := domain.ParseChatBody("gm")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.NewPostChatMessageHandler(deps).Handle(f.as(t, f.member(0)), app.PostChatMessage{
		CabalID: f.cabal.ID, Author: f.member(0), Body: body,
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(seen, []int{1}) {
		t.Fatalf("rows visible to another connection at publish time = %v, want [1]", seen)
	}
}

func TestPostChatMessage_aRolledBackPostPublishesNothing(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	missing := &domain.Reply{Parent: testkit.NewIDs(9).NewV7()}
	if _, err := f.send(t, f.member(0), "orphan", missing); errs.CodeOf(err) != errs.CodeChatParentNotFound {
		t.Fatalf("send = %v, want chat_parent_not_found", err)
	}
	if published := f.rt.Published(); len(published) != 0 {
		t.Fatalf("published = %+v after a rolled-back post, want none", published)
	}
}

func TestPostChatMessage_aFailedPublishStillStoresAndReturnsTheMessageAndLogsIt(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	f.rt.Fail(errs.New(errs.CodeUpstreamUnavailable, "test.ably"))
	ctx, logs := f.loggedCtx(t, f.member(0))
	body, err := domain.ParseChatBody("gm")
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.post.Handle(ctx, app.PostChatMessage{CabalID: f.cabal.ID, Author: f.member(0), Body: body})
	if err != nil || got.Body != "gm" {
		t.Fatalf("Handle = %+v, %v, want the stored message and no error", got, err)
	}
	if n := f.count(t, `SELECT count(*) FROM cabal_messages WHERE id = $1`, got.ID); n != 1 {
		t.Fatalf("%d rows stored, want 1", n)
	}
	out := string(logs.Bytes())
	for _, want := range []string{publishFailedLog, got.ID.String(), app.EventMessageCreated, "upstream_unavailable"} {
		if strings.Count(out, publishFailedLog) != 1 || !strings.Contains(out, want) {
			t.Fatalf("logs = %s, want one %s naming %q", out, publishFailedLog, want)
		}
	}
}

func TestPostChatMessage_aFailedMessagePublishDoesNotStopTheThreadUpdate(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	top := f.mustSend(t, f.member(0), "top", nil)
	f.rt.FailOnce(errs.New(errs.CodeUpstreamUnavailable, "test.ably"))
	f.mustSend(t, f.member(1), "reply", &domain.Reply{Parent: top.ID})
	published := f.rt.Published()
	if len(published) != 2 || published[1].Name != app.EventThreadUpdated {
		t.Fatalf("published = %+v, want the top message then only the thread update", published)
	}
}

func TestPostChatMessage_aMessageThatCannotBeWiredIsStoredLoggedAndNotPublished(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	deps := f.deps
	deps.Publish = app.NewChatPublisher(f.rt, func(context.Context, app.ChatMessage) (any, error) {
		return nil, errs.New(errs.CodeInternal, "test.wire")
	})
	ctx, logs := f.loggedCtx(t, f.member(0))
	body, err := domain.ParseChatBody("gm")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.NewPostChatMessageHandler(deps).Handle(ctx, app.PostChatMessage{
		CabalID: f.cabal.ID, Author: f.member(0), Body: body,
	}); err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(logs.Bytes(), []byte(publishFailedLog)); n != 1 || len(f.rt.Published()) != 0 {
		t.Fatalf("logs = %s, published = %+v, want one %s and nothing published",
			logs.Bytes(), f.rt.Published(), publishFailedLog)
	}
}

func TestDeleteChatMessage_publishesTheDeletionOnceAndOnlyWhenItHappens(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	m := f.mustSend(t, f.member(0), "oops", nil)
	cmd := app.DeleteChatMessage{CabalID: f.cabal.ID, MessageID: m.ID, Caller: f.member(1)}
	if err := f.del.Handle(f.as(t, f.member(1)), cmd); errs.CodeOf(err) != errs.CodeChatMessageNotOwned {
		t.Fatalf("delete by a stranger = %v, want chat_message_not_owned", err)
	}
	cmd.Caller = f.member(0)
	for range 2 {
		if err := f.del.Handle(f.as(t, f.member(0)), cmd); err != nil {
			t.Fatal(err)
		}
	}
	published := f.rt.Published()
	want := testkit.RealtimePublish{
		Channel: f.channel(), Name: app.EventMessageDeleted, Data: jsonOf(t, map[string]any{"id": m.ID}),
	}
	if len(published) != 2 || !reflect.DeepEqual(published[1], want) {
		t.Fatalf("published = %+v, want the post then one %+v", published, want)
	}
}

func TestDeleteChatMessage_aFailedPublishStillDeletesAndLogsIt(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	m := f.mustSend(t, f.member(0), "oops", nil)
	f.rt.Fail(errs.New(errs.CodeUpstreamUnavailable, "test.ably"))
	ctx, logs := f.loggedCtx(t, f.member(0))
	if err := f.del.Handle(ctx, app.DeleteChatMessage{
		CabalID: f.cabal.ID, MessageID: m.ID, Caller: f.member(0),
	}); err != nil {
		t.Fatal(err)
	}
	if n := f.count(t, `SELECT count(*) FROM cabal_messages WHERE id = $1 AND deleted_at IS NOT NULL`, m.ID); n != 1 {
		t.Fatalf("%d rows deleted, want 1", n)
	}
	out := string(logs.Bytes())
	if strings.Count(out, publishFailedLog) != 1 || !strings.Contains(out, app.EventMessageDeleted) {
		t.Fatalf("logs = %s, want one %s for %s", out, publishFailedLog, app.EventMessageDeleted)
	}
}
