package social_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type mentionFixture struct {
	commentFixture
	people []ids.UserID
}

func newMentionFixture(t *testing.T, banned ...int) mentionFixture {
	t.Helper()
	f := newCommentFixture(t)
	seeded := testkit.NewCabal(t, f.pool, testkit.WithMembers(8))
	f.cabal = seeded
	cards := make([]identity.UserCard, 0, len(seeded.Members)+1)
	people := make([]ids.UserID, 0, len(seeded.Members))
	for i, member := range seeded.Members {
		card := identity.UserCard{ID: member.ID, Handle: fmt.Sprintf("user%d", i)}
		if slices.Contains(banned, i) {
			card.AccountStatus = "banned"
		}
		cards = append(cards, card)
		people = append(people, member.ID)
	}
	cards = append(cards, identity.UserCard{ID: f.outsider, Handle: "outsider"})
	f.users = fakes.NewIdentity(cards, nil)
	f.create = app.NewCreateCommentHandler(f.depsWith(f.users, cabal.New(module.Deps{Pool: f.pool}).Queries()))
	return mentionFixture{commentFixture: f, people: people}
}

func (f commentFixture) depsWith(users app.Users, members app.Members) app.CommentDeps {
	return app.CommentDeps{
		UoW: f.uow, Reads: f.pool, Members: members, Users: users, IDs: f.gen, Clock: f.clock, Hints: f.hints,
	}
}

func (f mentionFixture) mentioned(
	t *testing.T,
	item uuid.UUID,
	author ids.UserID,
	body string,
	parent uuid.UUID,
) []uuid.UUID {
	t.Helper()
	f.mustComment(t, item, author, body, parent)
	all := f.created(t)
	return all[len(all)-1].MentionedUserIDs
}

func wantMentioned(t *testing.T, got []uuid.UUID, want ...ids.UserID) {
	t.Helper()
	wantIDs := make([]uuid.UUID, 0, len(want))
	for _, id := range want {
		wantIDs = append(wantIDs, id.UUID())
	}
	if !slices.Equal(got, wantIDs) {
		t.Fatalf("mentioned_user_ids = %v, want %v", got, wantIDs)
	}
}

func TestCreateComment_aMentionNamesTheMemberOnTheEvent(t *testing.T) {
	t.Parallel()
	f := newMentionFixture(t)
	item := f.item(t, feed.KindProposal)

	got := f.mentioned(t, item, f.people[0], "Still out: @User1, Jordan?", uuid.Nil)

	wantMentioned(t, got, f.people[1])
}

func TestCreateComment_aHandleWithNoUserMentionsNobody(t *testing.T) {
	t.Parallel()
	f := newMentionFixture(t)
	item := f.item(t, feed.KindProposal)

	wantMentioned(t, f.mentioned(t, item, f.people[0], "Still out: @jordan?", uuid.Nil))
	wantMentioned(t, f.mentioned(t, item, f.people[0], "No handles here", uuid.Nil))
}

func TestCreateComment_aNonMemberMentionedInACabalThreadIsNotMentioned(t *testing.T) {
	t.Parallel()
	f := newMentionFixture(t)
	item := f.item(t, feed.KindProposal)

	wantMentioned(t, f.mentioned(t, item, f.people[0], "@outsider what do you think", uuid.Nil))
}

func TestCreateComment_aMentionOfTheAnsweredAuthorIsMentioned(t *testing.T) {
	t.Parallel()
	f := newMentionFixture(t)
	item := f.item(t, feed.KindProposal)
	parent := f.mustComment(t, item, f.people[1], "first", uuid.Nil)

	got := f.mentioned(t, item, f.people[0], "@user1 and @user1 again, and @user0", parent.ID)

	wantMentioned(t, got, f.people[1])
}

func TestCreateComment_blockedAndUnreadableUsersAreNotMentioned(t *testing.T) {
	t.Parallel()
	f := newMentionFixture(t, 5)
	item := f.item(t, feed.KindProposal)
	insertBlock(t, f.pool, f.gen.NewV7(), f.people[2], f.people[0])
	insertBlock(t, f.pool, f.gen.NewV7(), f.people[0], f.people[3])

	got := f.mentioned(t, item, f.people[0], "@user2 @user3 @user5 @user4", uuid.Nil)

	wantMentioned(t, got, f.people[4])
}

func TestCreateComment_sixHandlesMentionFive(t *testing.T) {
	t.Parallel()
	f := newMentionFixture(t)
	item := f.item(t, feed.KindProposal)

	got := f.mentioned(t, item, f.people[0], strings.Join(
		[]string{"@user1", "@user2", "@user3", "@user4", "@user5", "@user6", "@user7"}, " "), uuid.Nil)

	wantMentioned(t, got, f.people[1], f.people[2], f.people[3], f.people[4], f.people[5])
}

func TestCreateComment_anyoneWhoCanSeeAPostIsMentionedOnIt(t *testing.T) {
	t.Parallel()
	f := newMentionFixture(t)
	item := f.item(t, feed.KindTrade)
	if _, err := f.pool.Exec(t.Context(),
		`INSERT INTO feed_mutes (user_id, target_type, target_id, created_at) VALUES ($1, 'item', $2, now())`,
		f.people[2].UUID(), item.String()); err != nil {
		t.Fatal(err)
	}

	got := f.mentioned(t, item, f.people[0], "@outsider @user2 @user1", uuid.Nil)

	wantMentioned(t, got, f.outsider, f.people[1])
}

type membersFailingAfter struct {
	app.Members
	left int
}

func (m *membersFailingAfter) IsMember(ctx context.Context, id ids.CabalID, user ids.UserID) (bool, error) {
	if m.left == 0 {
		return false, errs.New(errs.CodeDBUnavailable, "test.members")
	}
	m.left--
	return m.Members.IsMember(ctx, id, user)
}

func TestCreateComment_aFailedMentionLookupFailsTheReply(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		kind  feed.Kind
		setup func(f mentionFixture) app.CommentDeps
	}{
		"the handle lookup": {feed.KindTrade, func(f mentionFixture) app.CommentDeps {
			f.users.Fail("UserIDsByHandles", errs.New(errs.CodeUpstreamUnavailable, "test"))
			return f.depsWith(f.users, cabal.New(module.Deps{Pool: f.pool}).Queries())
		}},
		"the card lookup": {feed.KindTrade, func(f mentionFixture) app.CommentDeps {
			f.users.Fail("UsersByID", errs.New(errs.CodeUpstreamUnavailable, "test"))
			return f.depsWith(f.users, cabal.New(module.Deps{Pool: f.pool}).Queries())
		}},
		"the membership lookup": {feed.KindProposal, func(f mentionFixture) app.CommentDeps {
			members := &membersFailingAfter{Members: cabal.New(module.Deps{Pool: f.pool}).Queries(), left: 1}
			return f.depsWith(f.users, members)
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newMentionFixture(t)
			item := f.item(t, tc.kind)
			f.create = app.NewCreateCommentHandler(tc.setup(f))

			_, err := f.comment(t, item, f.people[0], "@user1", uuid.Nil)

			if err == nil {
				t.Error("comment created, want a failure")
			}
			if stored, _ := f.count(t, item); stored != 0 {
				t.Errorf("stored %d comments, want none", stored)
			}
		})
	}
}
