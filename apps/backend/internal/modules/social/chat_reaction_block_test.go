package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
)

func TestChatReactions_aBlockedPairCannotReactEitherWay(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct{ blocker, blocked int }{
		"the author blocked the reactor": {0, 1},
		"the reactor blocked the author": {1, 0},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newChatReactor(t)
			m := f.mustSend(t, f.member(0), "gm", nil)
			insertBlock(t, f.pool, f.deps.IDs.NewV7(), f.member(tt.blocker), f.member(tt.blocked))
			_, err := f.react.Handle(f.as(t, f.member(1)), f.cmd(m.ID, f.member(1), laugh))
			wantCode(t, err, errs.CodeChatMessageNotFound)
			if rows := f.rows(t); rows != 0 {
				t.Fatalf("%d rows after a blocked react, want 0", rows)
			}
			if n := f.eventCount(t, events.TypeChatReactionAdded); n != 0 {
				t.Fatalf("%d reaction events after a blocked react, want 0", n)
			}
		})
	}
}

func TestChatReactions_aMemberOutsideTheBlockStillReacts(t *testing.T) {
	t.Parallel()
	f := newChatReactor(t)
	m := f.mustSend(t, f.member(0), "gm", nil)
	insertBlock(t, f.pool, f.deps.IDs.NewV7(), f.member(0), f.member(1))
	got := f.add(t, m.ID, f.member(2), laugh)
	if got.Total != 1 {
		t.Fatalf("total = %d, want 1", got.Total)
	}
}

func TestChatReactions_aBlockedPairCanStillUnreact(t *testing.T) {
	t.Parallel()
	f := newChatReactor(t)
	m := f.mustSend(t, f.member(0), "gm", nil)
	f.add(t, m.ID, f.member(1), laugh)
	insertBlock(t, f.pool, f.deps.IDs.NewV7(), f.member(0), f.member(1))
	if _, err := f.unreact.Handle(f.as(t, f.member(1)), f.cmd(m.ID, f.member(1), laugh)); err != nil {
		t.Fatal(err)
	}
	if rows := f.rows(t); rows != 0 {
		t.Fatalf("%d rows after unreact, want 0", rows)
	}
}
