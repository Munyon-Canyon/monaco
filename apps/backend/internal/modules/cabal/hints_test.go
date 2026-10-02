package cabal_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
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
