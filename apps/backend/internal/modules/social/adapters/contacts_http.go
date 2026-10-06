package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (h HTTP) PostMeContactsMatch(
	ctx context.Context, req api.PostMeContactsMatchRequestObject,
) (api.PostMeContactsMatchResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	var hashes []string
	if req.Body != nil {
		hashes = req.Body.Hashes
	}
	if err := h.Match.Handle(ctx, app.MatchContacts{User: me, Hashes: hashes}); err != nil {
		return nil, err
	}
	page, err := h.contactPage(ctx, me, nil, app.ContactPageDefault)
	if err != nil {
		return nil, err
	}
	return api.PostMeContactsMatch200JSONResponse(page), nil
}

func (h HTTP) GetMeContactsMatches(
	ctx context.Context, req api.GetMeContactsMatchesRequestObject,
) (api.GetMeContactsMatchesResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	limit := app.ContactPageDefault
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	var after *domain.Keyset
	if req.Params.Cursor != nil && *req.Params.Cursor != "" {
		parsed, err := domain.ParseKeyset(*req.Params.Cursor)
		if err != nil {
			return nil, err
		}
		after = &parsed
	}
	page, err := h.contactPage(ctx, me, after, limit)
	if err != nil {
		return nil, err
	}
	return api.GetMeContactsMatches200JSONResponse(page), nil
}

func (h HTTP) contactPage(
	ctx context.Context, me ids.UserID, after *domain.Keyset, limit int,
) (api.ContactMatchPage, error) {
	page, err := app.ListContactMatches(ctx, h.Reads, h.Users, app.ContactMatchQuery{
		User: me, After: after, Limit: limit,
	})
	if err != nil {
		return api.ContactMatchPage{}, err
	}
	return wireContactPage(page), nil
}

func wireContactPage(page app.ContactMatchPage) api.ContactMatchPage {
	items := make([]api.ContactMatch, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, api.ContactMatch{
			UserId: item.UserID.UUID(), Handle: item.Handle, DisplayName: item.DisplayName,
			PhotoUrl: optionalWireText(item.PhotoURL), FollowedByMe: item.FollowedByMe,
		})
	}
	var next *string
	if page.Next != nil {
		encoded := page.Next.Encode()
		next = &encoded
	}
	return api.ContactMatchPage{Items: items, NextCursor: next}
}
