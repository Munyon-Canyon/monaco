package app

import (
	"context"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type SearchUser struct {
	ID          ids.UserID
	Handle      string
	DisplayName string
	PhotoURL    string
}

type searchUsersReader interface {
	SearchUsers(context.Context, sqlc.SearchUsersParams) ([]sqlc.SearchUsersRow, error)
}

func SearchUsers(ctx context.Context, q sqlc.DBTX, caller ids.UserID, raw string) ([]SearchUser, error) {
	return searchUsers(ctx, sqlc.New(q), caller, raw)
}

func searchUsers(ctx context.Context, reader searchUsersReader, caller ids.UserID, raw string) ([]SearchUser, error) {
	query, err := searchQuery(raw)
	if err != nil {
		return nil, err
	}
	rows, err := reader.SearchUsers(ctx, sqlc.SearchUsersParams{
		Caller: caller.UUID(), Prefix: query + "%", Contains: "%" + query + "%",
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "identity.SearchUsers")
	}
	users := make([]SearchUser, 0, len(rows))
	for _, row := range rows {
		users = append(users, SearchUser{
			ID:          ids.UserIDFrom(row.ID),
			Handle:      row.Handle.String,
			DisplayName: row.DisplayName,
			PhotoURL:    row.PhotoUrl.String,
		})
	}
	return users, nil
}

func searchQuery(raw string) (string, error) {
	query := strings.TrimSpace(raw)
	query = strings.TrimPrefix(query, "@")
	query = strings.ToLower(query)
	if len(query) < 2 || len(query) > 50 {
		return "", errs.New(errs.CodeInvalidInput, "identity.SearchUsers")
	}
	return strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(query), nil
}
