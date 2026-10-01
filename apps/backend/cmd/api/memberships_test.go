package main

import (
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func awaitHint(t *testing.T, sub *sse.Subscription) sse.Hint {
	t.Helper()
	var got sse.Hint
	testkit.Eventually(t, func() bool {
		select {
		case got = <-sub.Hints():
			return true
		default:
			return false
		}
	}, 5*time.Second)
	return got
}

func TestHubRegister_aPhoneInCabalADoesNotReceiveCabalBHints(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	pool := testkit.DB(t)
	a, other := testkit.NewCabal(t, pool), testkit.NewCabal(t, pool)
	hub, stop, err := startBackground(t.Context(), b.Conn, pool, db.New(pool, ids.Real{}, clock.Real{}),
		noop.NewMeterProvider(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stop(); err != nil {
			t.Error(err)
		}
	}()
	sub, err := hub.Register(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: a.Creator.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	hub.Deliver(t.Context(), "cabal."+other.ID.String()+".updated")
	hub.Deliver(t.Context(), "cabal."+a.ID.String()+".updated")
	if got, want := awaitHint(t, sub), (sse.Hint{Key: sse.CabalKey(a.ID), What: "updated"}); got != want {
		t.Fatalf("first hint = %+v, want %+v; cabal B's hint came first if it leaked", got, want)
	}
	select {
	case got := <-sub.Hints():
		t.Fatalf("unexpected hint %+v after cabal A's", got)
	default:
	}
}

func TestHubRegister_aPhoneInNoCabalReceivesNoCabalHints(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	stranger := testkit.SeedUser(t, pool, testkit.UserOpts{})
	hub, stop, err := startBackground(t.Context(), b.Conn, pool, db.New(pool, ids.Real{}, clock.Real{}),
		noop.NewMeterProvider(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stop(); err != nil {
			t.Error(err)
		}
	}()
	sub, err := hub.Register(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: stranger.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	hub.Deliver(t.Context(), "cabal."+c.ID.String()+".updated")
	hub.Deliver(t.Context(), "user."+stranger.ID.String()+".ping")
	if got, want := awaitHint(t, sub), (sse.Hint{Key: sse.UserKey(stranger.ID), What: "ping"}); got != want {
		t.Fatalf("first hint = %+v, want only the user's own %+v", got, want)
	}
}
