package social_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func (f chatFixture) memberLeft(t *testing.T, user ids.UserID) error {
	t.Helper()
	ctx := observability.WithEventID(t.Context(), ids.EventIDFrom(f.deps.IDs.NewV7()))
	return f.deps.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return adapters.ChatSeenCleanup{}.Handle(ctx, tx, events.CabalMemberLeft{
			V: 1, CabalID: f.cabal.ID.UUID(), UserID: user.UUID(),
		}, f.now)
	})
}

func (f chatFixture) seenRows(t *testing.T) int {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM chat_seen WHERE cabal_id = $1`, f.cabal.ID.UUID())
}

func TestChatSeenCleanup_isRegisteredOnItsOwnDurable(t *testing.T) {
	t.Parallel()
	for _, consumer := range social.New(module.Deps{}).Consumers() {
		if consumer.Durable == "social_chat_seen_cleanup" {
			if len(consumer.Handlers) != 1 || consumer.Handlers[0].Name != "social.chat_seen_cleanup" {
				t.Fatalf("handlers = %+v, want only social.chat_seen_cleanup", consumer.Handlers)
			}
			return
		}
	}
	t.Fatal("social_chat_seen_cleanup is not registered")
}

func TestChatSeenCleanup_deletesOnlyTheLeaversRowAndSurvivesRedelivery(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	f.mustMarkSeen(t, f.member(1))
	f.mustMarkSeen(t, f.member(2))
	for range 2 {
		if err := f.memberLeft(t, f.member(1)); err != nil {
			t.Fatal(err)
		}
	}
	var left int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM chat_seen WHERE user_id = $1`,
		f.member(2).UUID()).Scan(&left); err != nil || left != 1 || f.seenRows(t) != 1 {
		t.Fatalf("rows left = %d (%d in the cabal), %v; want only the other member's", left, f.seenRows(t), err)
	}
}

func TestChatSeenCleanup_aDatabaseFailureIsInternal(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE chat_seen`); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.memberLeft(t, f.member(1)), errs.CodeInternal)
}

func TestSeenCount_ExcludesAuthorAndLeftMembers(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	msg := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: "gm"})
	f.markAs(t, f.member(0))
	f.markAs(t, f.member(1))
	f.markAs(t, f.member(2))
	if err := f.memberLeft(t, f.member(2)); err != nil {
		t.Fatal(err)
	}
	got, err := f.seenBy(t, f.member(1), msg.Id)
	if err != nil || got.Count != 1 || len(got.Members) != 1 || got.Members[0].UserId != f.member(1).UUID() {
		t.Fatalf("seen by = %+v, %v; want only the member who stayed, not the author or the one who left", got, err)
	}
	page, err := f.channel(t, f.member(1), api.GetChatMessagesParams{})
	if err != nil || len(page) != 1 || page[0].SeenCount == nil || *page[0].SeenCount != 1 {
		t.Fatalf("channel = %+v, %v; want seen_count 1", page, err)
	}
}

func TestLeaveCabal_Ok_ChatSeenCleared(t *testing.T) {
	t.Parallel()
	const (
		founder = "did:privy:qa-f04-ok-creator"
		leaver  = "did:privy:qa-f04-ok-member"
		cabals  = "/v1/cabals"
		seen    = cabals + "/{cabal}/chat/seen"
	)
	seenRows := func(want int) func(*scenario.Scenario) bool {
		return func(s *scenario.Scenario) bool {
			var n int
			if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM chat_seen WHERE cabal_id = $1`,
				s.Recall("cabal")).Scan(&n); err != nil {
				s.Fatalf("count the seen rows: %v", err)
			}
			return n == want
		}
	}
	s := chatScenario(t)
	s.Given(
		scenario.SignIn(founder),
		scenario.Post(cabals, `{"name":"Friends pot","join_mode":"open","voter_mode":"all","threshold":"majority",`+
			`"proposal_expiry_seconds":86400}`),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.Remember("id", "cabal"),
		scenario.Post(seen, ""),
		scenario.ExpectStatus(http.StatusOK),
		scenario.SignIn(leaver),
		scenario.Post(cabals+"/{cabal}/members", ""),
		scenario.ExpectStatus(http.StatusOK),
		scenario.Post(seen, ""),
		scenario.ExpectStatus(http.StatusOK),
		scenario.Eventually("both watermarks stored", seenRows(2)),
	).When(
		scenario.Delete(cabals+"/{cabal}/members/me"),
		scenario.ExpectStatus(http.StatusNoContent),
	).Then(
		scenario.ExpectEvents(events.TypeCabalMemberLeft, 1),
		scenario.Eventually("the leaver's watermark is gone", seenRows(1)),
	)
}
