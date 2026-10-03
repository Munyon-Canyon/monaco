package cabal_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type hintLog struct {
	keys    []string
	payload []byte
	called  bool
}

func (h *hintLog) PublishHint(_ context.Context, key string, payload []byte) {
	h.called = true
	h.keys = append(h.keys, key)
	h.payload = payload
}

func TestHints_publishesTheCreatorsCabalsAfterCommit(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	sent := &hintLog{}
	h := adapters.Hints{Publish: sent}
	ev := events.CabalCreated{V: 1, CabalID: f.ids.NewV7(), CreatorID: f.user.ID.UUID(), Name: "Friends pot"}
	err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		if handleErr := h.Handle(ctx, tx, ev, f.clock.Now()); handleErr != nil {
			return handleErr
		}
		return errs.New(errs.CodeInternal, "test.rollback")
	})
	if err == nil || sent.called {
		t.Fatalf("rolled back err=%v called=%v", err, sent.called)
	}
	for range 2 {
		if err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
			return h.Handle(ctx, tx, ev, f.clock.Now())
		}); err != nil {
			t.Fatal(err)
		}
	}
	want := "user." + f.user.ID.UUID().String() + ".cabals"
	if len(sent.keys) != 2 || sent.keys[0] != want || sent.keys[1] != want || sent.payload != nil {
		t.Fatalf("hints = %q payload %v, want two %q", sent.keys, sent.payload, want)
	}
}

func TestHints_publishMembershipAndRequestHintsAfterCommit(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	cabalID, user, request := f.ids.NewV7(), f.user.ID.UUID(), f.ids.NewV7()
	members, requests := "cabal."+cabalID.String()+".members", "cabal."+cabalID.String()+".access_requests"
	updated := "cabal." + cabalID.String() + ".updated"
	access := "user." + user.String() + ".cabal_access"
	for _, tt := range []struct {
		name   string
		handle func(context.Context, adapters.Hints, db.Tx) error
		want   []string
	}{
		{"member_joined", func(ctx context.Context, h adapters.Hints, tx db.Tx) error {
			return h.MemberJoined(ctx, tx, events.CabalMemberJoined{V: 1, CabalID: cabalID, UserID: user}, f.clock.Now())
		}, []string{members, access}},
		{"access_decided", func(ctx context.Context, h adapters.Hints, tx db.Tx) error {
			return h.AccessDecided(ctx, tx, events.CabalAccessDecided{
				V: 1, RequestID: request, CabalID: cabalID, UserID: user,
			}, f.clock.Now())
		}, []string{members, requests, access}},
		{"member_left", func(ctx context.Context, h adapters.Hints, tx db.Tx) error {
			return h.MemberLeft(ctx, tx, events.CabalMemberLeft{V: 1, CabalID: cabalID, UserID: user}, f.clock.Now())
		}, []string{members, "user." + user.String() + ".cabals", access}},
		{"access_requested", func(ctx context.Context, h adapters.Hints, tx db.Tx) error {
			return h.AccessRequested(ctx, tx, events.CabalAccessRequested{
				V: 1, RequestID: request, CabalID: cabalID, UserID: user,
			}, f.clock.Now())
		}, []string{requests}},
		{"updated", func(ctx context.Context, h adapters.Hints, tx db.Tx) error {
			return h.Updated(ctx, tx, events.CabalUpdated{
				V: 1, CabalID: cabalID, ActorID: user, Changes: events.CabalChanges{Name: ptr("Work pot")},
			}, f.clock.Now())
		}, []string{updated}},
	} {
		sent := &hintLog{}
		h := adapters.Hints{Publish: sent}
		err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
			if handleErr := tt.handle(ctx, h, tx); handleErr != nil {
				return handleErr
			}
			return errs.New(errs.CodeInternal, "test.rollback")
		})
		if err == nil || sent.called {
			t.Fatalf("%s: rolled back err=%v called=%v", tt.name, err, sent.called)
		}
		if err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
			return tt.handle(ctx, h, tx)
		}); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(sent.keys, tt.want) || sent.payload != nil {
			t.Errorf("%s: hints = %q, want %q", tt.name, sent.keys, tt.want)
		}
	}
}

type hubHints struct{ hub *sse.Hub }

func (h hubHints) PublishHint(ctx context.Context, key string, _ []byte) { h.hub.Deliver(ctx, key) }

type cabalsOf struct{ q cabal.Queries }

func (m cabalsOf) CabalIDs(ctx context.Context, user ids.UserID) ([]ids.CabalID, error) {
	return m.q.CabalsOf(ctx, user)
}

func hintsUntil(t *testing.T, sub *sse.Subscription, last sse.Hint) []sse.Hint {
	t.Helper()
	var got []sse.Hint
	testkit.Eventually(t, func() bool {
		for {
			select {
			case h := <-sub.Hints():
				if h == last {
					return true
				}
				got = append(got, h)
			default:
				return false
			}
		}
	}, 5*time.Second)
	return got
}

func TestHints_aLeaverStopsReceivingTheCabalsHints(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	leaver := c.Members[1].ID
	hub, err := sse.NewHub(cabalsOf{cabal.New(module.Deps{Pool: f.pool}).Queries()}, noop.NewMeterProvider())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	var running sync.WaitGroup
	running.Go(func() { hub.Run(ctx) })
	t.Cleanup(func() {
		cancel()
		running.Wait()
	})
	sub, err := hub.Register(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: leaver.String()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sub.Close)
	proposals := sse.Hint{Key: sse.CabalKey(c.ID), What: "proposals"}
	barrier := sse.Hint{Key: sse.UserKey(leaver), What: "barrier"}
	cabalKey, userKey := "cabal."+c.ID.String()+".proposals", "user."+leaver.String()+".barrier"
	hub.Deliver(t.Context(), cabalKey)
	hub.Deliver(t.Context(), userKey)
	if got := hintsUntil(t, sub, barrier); !slices.Contains(got, proposals) {
		t.Fatalf("hints before leaving = %+v, want %+v", got, proposals)
	}
	if err := f.leave(t.Context(), &treasuryStub{}, leaver, c.ID); err != nil {
		t.Fatal(err)
	}
	h := adapters.Hints{Publish: hubHints{hub}}
	left := f.lefts(t)
	if err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return h.MemberLeft(ctx, tx, left[0], f.clock.Now())
	}); err != nil {
		t.Fatal(err)
	}
	hub.Deliver(t.Context(), cabalKey)
	hub.Deliver(t.Context(), userKey)
	got := hintsUntil(t, sub, barrier)
	if !slices.Contains(got, sse.Hint{Key: sse.UserKey(leaver), What: sse.MembershipChanged}) ||
		slices.Contains(got, proposals) {
		t.Fatalf("hints after leaving = %+v, want the membership change and no cabal hint", got)
	}
}
