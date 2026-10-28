package social_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func (f commentRoutes) proposalThread(t *testing.T) (uuid.UUID, ids.ProposalID) {
	t.Helper()
	item := f.item(t, feed.KindProposal)
	var ref uuid.UUID
	err := f.pool.QueryRow(t.Context(),
		`SELECT ref_id FROM feed_objects WHERE id = $1 AND ref_type = 'proposals'`, item).Scan(&ref)
	if err != nil {
		t.Fatal(err)
	}
	return item, ids.ProposalIDFrom(ref)
}

func TestReplyCounts_CountsEveryLiveReplyInOneRead(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	author := f.cabal.Members[0].ID
	busy, busyID := f.proposalThread(t)
	quiet, quietID := f.proposalThread(t)
	_, silentID := f.proposalThread(t)
	var top uuid.UUID
	for range 4 {
		top = f.say(t, busy, author, "top", nil).Id
	}
	f.say(t, busy, author, "nested", &top)
	f.say(t, busy, author, "nested again", &top)
	f.say(t, quiet, author, "one", nil)
	f.say(t, quiet, author, "two", nil)
	elsewhere := f.item(t, feed.KindTrade)
	f.say(t, elsewhere, author, "a trade reply", nil)

	reads := social.New(module.Deps{Pool: f.pool})
	counts, err := reads.ReplyCounts(t.Context(), []ids.ProposalID{busyID, quietID, silentID})
	if err != nil {
		t.Fatalf("ReplyCounts = %v", err)
	}
	if counts[busyID] != 6 || counts[quietID] != 2 || len(counts) != 2 {
		t.Errorf("ReplyCounts = %v, want 6 and 2 with the proposal that has no thread absent", counts)
	}
	if _, ok := counts[silentID]; ok {
		t.Errorf("ReplyCounts listed %s, which has no replies", silentID)
	}
}

func TestReplyCounts_SkipsDeletedAndRemovedReplies(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	author := f.cabal.Members[0].ID
	item, proposal := f.proposalThread(t)
	f.say(t, item, author, "kept", nil)
	own := f.say(t, item, author, "taken back", nil)
	removed := f.say(t, item, author, "against the rules", nil)
	if err := f.delete(t, own.Id, author); err != nil {
		t.Fatal(err)
	}
	if err := f.removeAs(asAdmin(t), removed.Id, "abuse"); err != nil {
		t.Fatal(err)
	}
	counts, err := social.New(module.Deps{Pool: f.pool}).ReplyCounts(t.Context(), []ids.ProposalID{proposal})
	if err != nil || counts[proposal] != 1 {
		t.Errorf("ReplyCounts = %v, %v, want 1 after one self-deleted and one removed reply", counts, err)
	}
}

func TestReplyCounts_FailsWhenTheReadFails(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	_, proposal := f.proposalThread(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := social.New(module.Deps{Pool: f.pool}).ReplyCounts(ctx, []ids.ProposalID{proposal})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("ReplyCounts on a cancelled context err = %v, want internal", err)
	}
}

func TestReplyCounts_NoProposalsReadsNothing(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	counts, err := social.New(module.Deps{Pool: f.pool}).ReplyCounts(t.Context(), nil)
	if err != nil || len(counts) != 0 {
		t.Errorf("ReplyCounts(nil) = %v, %v, want an empty map", counts, err)
	}
}
