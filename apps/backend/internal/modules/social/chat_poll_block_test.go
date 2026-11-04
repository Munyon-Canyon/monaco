package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
)

func TestChatPoll_aBlockedPairCannotVoteEitherWay(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct{ blocker, blocked int }{
		"the author blocked the voter": {0, 1},
		"the voter blocked the author": {1, 0},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newChatPoller(t)
			m := f.poll(t)
			insertBlock(t, f.pool, f.deps.IDs.NewV7(), f.member(tt.blocker), f.member(tt.blocked))
			_, err := f.vote.Handle(f.as(t, f.member(1)), app.VoteChatPoll{
				CabalID: f.cabal.ID, MessageID: m.ID, Viewer: f.member(1), Position: 0,
			})
			wantCode(t, err, errs.CodeChatMessageNotFound)
			if rows := f.votes(t); rows != 0 {
				t.Fatalf("%d vote rows after a blocked vote, want 0", rows)
			}
			voted := string(events.TypeChatPollVoted)
			if n := f.count(t, `SELECT count(*) FROM events WHERE type = $1`, voted); n != 0 {
				t.Fatalf("%d vote events after a blocked vote, want 0", n)
			}
		})
	}
}

func TestChatPoll_aMemberOutsideTheBlockStillVotes(t *testing.T) {
	t.Parallel()
	f := newChatPoller(t)
	m := f.poll(t)
	insertBlock(t, f.pool, f.deps.IDs.NewV7(), f.member(0), f.member(1))
	got := f.cast(t, m.ID, f.member(2), 0)
	if got.Total != 1 {
		t.Fatalf("total = %d, want 1", got.Total)
	}
}

func TestChatPoll_aBlockedPairCanStillRetractAVote(t *testing.T) {
	t.Parallel()
	f := newChatPoller(t)
	m := f.poll(t)
	f.cast(t, m.ID, f.member(1), 0)
	insertBlock(t, f.pool, f.deps.IDs.NewV7(), f.member(0), f.member(1))
	got, err := f.unvote.Handle(f.as(t, f.member(1)), app.UnvoteChatPoll{
		CabalID: f.cabal.ID, MessageID: m.ID, Viewer: f.member(1),
	})
	if err != nil || got.Total != 0 || f.votes(t) != 0 {
		t.Fatalf("unvote = %+v, %v, rows %d, want a retracted vote", got, err, f.votes(t))
	}
}
