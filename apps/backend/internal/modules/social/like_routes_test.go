package social_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	baseapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFeedLikes_RoutesMountBesideEveryFeedRouteAndAnswerThroughTheRouter(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withSocial())
	scenario.SeededUser("kai", "active")(s)
	scenario.SeededUser("lee", "active")(s)
	item, missing := ids.Real{}.NewV7(), ids.Real{}.NewV7()
	if _, err := s.DB().Exec(s.Context(), `INSERT INTO feed_objects
		(id, kind, ref_type, ref_id, title, payload, created_at, updated_at)
		VALUES ($1, 'trade', 'swaps', $2, 'seeded', '{}', now(), now())`, item, ids.Real{}.NewV7()); err != nil {
		t.Fatal(err)
	}
	like, comments, one := "/v1/feed/likes/"+item.String(), "/v1/feed/"+item.String()+"/comments", "/v1/feed/"+item.String()
	s.Given(scenario.AsUser("kai")).
		When(
			scenario.Put(like, ""), scenario.ExpectStatus(http.StatusNoContent),
			scenario.Put(like, ""), scenario.ExpectStatus(http.StatusNoContent),
			scenario.Get(one), scenario.ExpectStatus(http.StatusOK),
			itemLikes(1, true),
			scenario.Get(comments), scenario.ExpectStatus(http.StatusOK),
			scenario.AsUser("lee"),
			scenario.Get("/v1/feed"), scenario.ExpectStatus(http.StatusOK),
			scenario.Get(one), itemLikes(1, false),
			scenario.AsUser("kai"),
			scenario.Delete(like), scenario.ExpectStatus(http.StatusNoContent),
			scenario.Delete(like), scenario.ExpectStatus(http.StatusNoContent),
			scenario.Get(one), itemLikes(0, false),
			scenario.Delete("/v1/feed/comments/"+missing.String()), scenario.ExpectProblem(errs.CodeCommentNotFound),
			scenario.Put("/v1/feed/likes/"+missing.String(), ""), scenario.ExpectProblem(errs.CodeFeedItemNotFound),
			scenario.Delete("/v1/feed/likes/"+missing.String()), scenario.ExpectProblem(errs.CodeFeedItemNotFound),
		)
}

func itemLikes(count int32, mine bool) scenario.Step {
	return scenario.ExpectField("item", func(s *scenario.Scenario, raw json.RawMessage) {
		s.Helper()
		var item struct {
			LikeCount     int32           `json:"like_count"`
			LikedByMe     bool            `json:"liked_by_me"`
			Reactions     json.RawMessage `json:"reactions"`
			ReactionCount int32           `json:"reaction_count"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			s.Fatalf("decode the item: %v", err)
		}
		if item.LikeCount != count || item.LikedByMe != mine {
			s.Fatalf("like_count %d, liked_by_me %v, want %d and %v", item.LikeCount, item.LikedByMe, count, mine)
		}
		var reactions []baseapi.Reaction
		if err := json.Unmarshal(item.Reactions, &reactions); err != nil || reactions == nil {
			s.Fatalf("reactions %s, %v, want an array", item.Reactions, err)
		}
		want := []baseapi.Reaction{}
		if count > 0 {
			want = []baseapi.Reaction{{Emoji: redHeart, Count: count, Mine: mine}}
		}
		if !slices.Equal(reactions, want) || item.ReactionCount != count {
			s.Fatalf("reactions %s, reaction_count %d, want %v and %d", item.Reactions, item.ReactionCount, want, count)
		}
	})
}
