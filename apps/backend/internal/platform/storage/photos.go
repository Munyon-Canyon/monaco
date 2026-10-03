package storage

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const photoBucket = "avatars"

type ProfilePhotos struct{ Storage *Client }

func (s ProfilePhotos) Put(ctx context.Context, key, contentType string, body []byte) (string, error) {
	return s.Storage.Put(ctx, photoBucket, key, contentType, body)
}

func (s ProfilePhotos) DeleteAll(ctx context.Context, userID ids.UserID) error {
	return s.Storage.DeletePrefix(ctx, photoBucket, userID.String())
}
