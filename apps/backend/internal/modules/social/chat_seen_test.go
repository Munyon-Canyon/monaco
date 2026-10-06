package social_test

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f chatFixture) markSeen(t *testing.T, user ids.UserID) (time.Time, error) {
	t.Helper()
	return f.markSeenIn(f.as(t, user), user)
}

func (f chatFixture) markSeenIn(ctx context.Context, user ids.UserID) (time.Time, error) {
	return app.NewMarkChatSeenHandler(f.deps).Handle(ctx, app.MarkChatSeen{CabalID: f.cabal.ID, Caller: user})
}

func (f chatFixture) mustMarkSeen(t *testing.T, user ids.UserID) time.Time {
	t.Helper()
	at, err := f.markSeen(t, user)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func (f chatFixture) seenPublishes() []testkit.RealtimePublish {
	var out []testkit.RealtimePublish
	for _, p := range f.rt.Published() {
		if p.Name == app.EventSeenUpdated {
			out = append(out, p)
		}
	}
	return out
}

func TestMarkChatSeen_NeverBackwards(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	first := f.mustMarkSeen(t, f.member(0))
	if !first.Equal(f.now) {
		t.Fatalf("watermark = %v, want the clock's %v", first, f.now)
	}
	f.clock.Set(f.now.Add(-time.Hour))
	if got := f.mustMarkSeen(t, f.member(0)); !got.Equal(f.now) {
		t.Fatalf("watermark after the clock moved back = %v, want %v kept", got, f.now)
	}
	f.clock.Set(f.now.Add(time.Minute))
	if got := f.mustMarkSeen(t, f.member(0)); !got.Equal(f.now.Add(time.Minute)) {
		t.Fatalf("watermark after the clock moved on = %v, want %v", got, f.now.Add(time.Minute))
	}
	if n := f.count(t, `SELECT count(*) FROM events WHERE type LIKE 'chat.seen%'`); n != 0 {
		t.Fatalf("seen events = %d, want none", n)
	}
}

func TestMarkChatSeen_RequiresMembership(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	if _, err := f.markSeen(t, f.outsider); errs.CodeOf(err) != errs.CodeNotCabalMember {
		t.Fatalf("mark by an outsider = %v, want not_cabal_member", err)
	}
	if n := f.count(t, `SELECT count(*) FROM chat_seen`); n != 0 {
		t.Fatalf("chat_seen rows = %d, want none", n)
	}
}

func TestMarkChatSeen_PublishThrottle(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	msg := f.mustSend(t, f.member(0), "gm", nil)
	f.clock.Advance(time.Second)
	for range 3 {
		f.mustMarkSeen(t, f.member(1))
		f.clock.Advance(time.Second)
	}
	want := app.SeenUpdated{MessageID: msg.ID, Count: 1}
	if got := f.seenPublishes(); !reflect.DeepEqual(got, []testkit.RealtimePublish{
		{Channel: f.channel(), Name: app.EventSeenUpdated, Data: jsonOf(t, want)},
	}) {
		t.Fatalf("seen.updated after 3 calls in 3 s = %+v, want one with count 1", got)
	}
	f.clock.Advance(3 * time.Second)
	f.mustMarkSeen(t, f.member(1))
	if got := f.seenPublishes(); len(got) != 2 {
		t.Fatalf("seen.updated after a 4th call 6 s after the first = %d publishes, want 2", len(got))
	}
}

func TestMarkChatSeen_ThrottleIsPerMember(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	f.mustSend(t, f.member(0), "gm", nil)
	f.clock.Advance(time.Second)
	f.mustMarkSeen(t, f.member(1))
	f.mustMarkSeen(t, f.member(2))
	if got := f.seenPublishes(); len(got) != 2 {
		t.Fatalf("seen.updated for two members = %d publishes, want 2", len(got))
	}
}

func TestMarkChatSeen_NoChannelMessageNothingToPublish(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	f.mustMarkSeen(t, f.member(1))
	if got := f.seenPublishes(); len(got) != 0 {
		t.Fatalf("seen.updated in an empty chat = %+v, want none", got)
	}
	f.mustSend(t, f.member(0), "gm", nil)
	f.clock.Advance(time.Second)
	f.mustMarkSeen(t, f.member(1))
	if got := f.seenPublishes(); len(got) != 1 {
		t.Fatalf("seen.updated after a message = %d, want 1: an empty chat must not use up the throttle", len(got))
	}
}

func TestMarkChatSeen_aFailedPublishIsLoggedAndTheMarkStands(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	f.mustSend(t, f.member(0), "gm", nil)
	f.rt.FailOnce(errs.New(errs.CodeUpstreamUnavailable, "test.ably"))
	ctx, logs := f.loggedCtx(t, f.member(1))
	at, err := f.markSeenIn(ctx, f.member(1))
	if err != nil || !at.Equal(f.now) {
		t.Fatalf("mark = %v, %v, want the watermark and no error", at, err)
	}
	if n := bytes.Count(logs.Bytes(), []byte(publishFailedLog)); n != 1 {
		t.Fatalf("logs = %s, want one %s", logs.Bytes(), publishFailedLog)
	}
}

func TestMarkChatSeen_databaseFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, breaker string
		wantErr       bool
	}{
		{"the watermark write fails", `DROP TABLE chat_seen CASCADE`, true},
		{
			"the publish claim fails",
			`ALTER TABLE chat_seen ADD CONSTRAINT never_published CHECK (seen_published_at IS NULL) NOT VALID`,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newChatFixture(t)
			f.mustSend(t, f.member(0), "gm", nil)
			if _, err := f.pool.Exec(t.Context(), tt.breaker); err != nil {
				t.Fatal(err)
			}
			ctx, logs := f.loggedCtx(t, f.member(1))
			_, err := f.markSeenIn(ctx, f.member(1))
			if tt.wantErr {
				wantCode(t, err, errs.CodeInternal)
				return
			}
			if err != nil || bytes.Count(logs.Bytes(), []byte(publishFailedLog)) != 1 || len(f.seenPublishes()) != 0 {
				t.Fatalf("err = %v, logs = %s, want the mark kept, one %s and no publish",
					err, logs.Bytes(), publishFailedLog)
			}
		})
	}
}

func TestMarkChatSeen_route(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	req := api.MarkChatSeenRequestObject{Id: f.cabal.ID.UUID()}
	res, err := f.routes.MarkChatSeen(asUser(t.Context(), f.member(1)), req)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := res.(api.MarkChatSeen200JSONResponse); !ok || !got.LastSeenAt.Equal(f.now) {
		t.Fatalf("response = %#v, want 200 with the clock's watermark", res)
	}
	if _, err := f.routes.MarkChatSeen(t.Context(), req); err == nil {
		t.Fatal("unauthenticated mark succeeded")
	}
	_, err = f.routes.MarkChatSeen(asUser(t.Context(), f.outsider), req)
	if errs.CodeOf(err) != errs.CodeNotCabalMember {
		t.Fatalf("mark by an outsider = %v, want not_cabal_member", err)
	}
}
