package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
)

func TestBannedCabal_itsItemsCannotBeReadOrCommentedOn(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	item, kai := f.item(t, feed.KindTrade), f.cabal.Members[0].ID
	f.say(t, item, f.outsider, "before the ban", nil)
	if _, err := f.pool.Exec(t.Context(),
		`INSERT INTO feed_cabals (cabal_id, name, updated_at, hidden_at) VALUES ($1, 'Alpha Cabal', now(), now())
		 ON CONFLICT (cabal_id) DO UPDATE SET hidden_at = now()`, f.cabal.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	body := "after the ban"
	for _, viewer := range []struct {
		name string
		post func() error
		read func() error
		item func() error
	}{
		{
			"member",
			func() error {
				_, err := f.post(t, item, kai, &api.CreateCommentRequest{Body: &body})
				return err
			},
			func() error {
				_, err := f.routes.GetFeedComments(asUser(t.Context(), kai), api.GetFeedCommentsRequestObject{Id: item})
				return err
			},
			func() error {
				_, err := f.routes.GetFeedItem(asUser(t.Context(), kai), api.GetFeedItemRequestObject{Id: item})
				return err
			},
		},
		{
			"outsider",
			func() error {
				_, err := f.post(t, item, f.outsider, &api.CreateCommentRequest{Body: &body})
				return err
			},
			func() error {
				_, err := f.routes.GetFeedComments(asUser(t.Context(), f.outsider), api.GetFeedCommentsRequestObject{Id: item})
				return err
			},
			func() error {
				_, err := f.routes.GetFeedItem(asUser(t.Context(), f.outsider), api.GetFeedItemRequestObject{Id: item})
				return err
			},
		},
	} {
		wantCode(t, viewer.post(), errs.CodeFeedItemNotFound)
		wantCode(t, viewer.read(), errs.CodeFeedItemNotFound)
		wantCode(t, viewer.item(), errs.CodeFeedItemNotFound)
	}
}
