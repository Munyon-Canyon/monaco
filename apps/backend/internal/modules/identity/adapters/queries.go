package adapters

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Postgres struct {
	q *sqlc.Queries
}

var _ port.Queries = Postgres{}

func NewQueries(db sqlc.DBTX) Postgres { return Postgres{q: sqlc.New(db)} }

func (r Postgres) UsersByID(ctx context.Context, userIDs []ids.UserID) (map[ids.UserID]port.UserCard, error) {
	const op = "identity.UsersByID"
	if err := sized(op, len(userIDs), 0, port.MaxUsersByID); err != nil {
		return nil, err
	}
	asked := make(map[uuid.UUID]ids.UserID, len(userIDs))
	raw := make([]uuid.UUID, len(userIDs))
	for i, id := range userIDs {
		raw[i] = id.UUID()
		asked[raw[i]] = id
	}
	rows, err := r.q.UserCardsByID(ctx, raw)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	cards := make(map[ids.UserID]port.UserCard, len(rows))
	for _, row := range rows {
		id := asked[row.ID]
		cards[id] = cardOf(id, row)
	}
	return cards, nil
}

func (r Postgres) UserByHandle(ctx context.Context, handle string) (port.UserCard, error) {
	const op = "identity.UserByHandle"
	key, ok := handleKey(handle)
	if !ok {
		return port.UserCard{}, errs.New(errs.CodeUserNotFound, op)
	}
	row, err := r.q.UserCardByHandle(ctx, key)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return port.UserCard{}, errs.New(errs.CodeUserNotFound, op)
	case err != nil:
		return port.UserCard{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	id, err := userID(op, row.ID)
	if err != nil {
		return port.UserCard{}, err
	}
	return cardOf(id, sqlc.UserCardsByIDRow(row)), nil
}

func (r Postgres) UserIDsByHandles(ctx context.Context, handles []string) (map[string]ids.UserID, error) {
	const op = "identity.UserIDsByHandles"
	if err := sized(op, len(handles), 0, port.MaxHandles); err != nil {
		return nil, err
	}
	stored := make(map[string]string, len(handles))
	keys := make([]string, 0, len(handles))
	for _, h := range handles {
		if key, ok := handleKey(h); ok {
			stored[h] = key
			keys = append(keys, key)
		}
	}
	rows, err := r.q.UserIDsByHandles(ctx, keys)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	byKey, err := idsByKey(op, rows, func(row sqlc.UserIDsByHandlesRow) (string, uuid.UUID) {
		return row.Handle.String, row.ID
	})
	if err != nil {
		return nil, err
	}
	found := make(map[string]ids.UserID, len(stored))
	for h, key := range stored {
		if id, ok := byKey[key]; ok {
			found[h] = id
		}
	}
	return found, nil
}

func (r Postgres) UsersByPhoneHashes(ctx context.Context, hashes [][]byte) (map[string]ids.UserID, error) {
	const op = "identity.UsersByPhoneHashes"
	if err := sized(op, len(hashes), 0, port.MaxPhoneHashes); err != nil {
		return nil, err
	}
	rows, err := r.q.UserIDsByPhoneHashes(ctx, hashes)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	return idsByKey(op, rows, func(row sqlc.UserIDsByPhoneHashesRow) (string, uuid.UUID) {
		return hex.EncodeToString(row.PhoneHash), row.ID
	})
}

func (r Postgres) UsersByXUserIDs(ctx context.Context, xUserIDs []string) (map[string]ids.UserID, error) {
	const op = "identity.UsersByXUserIDs"
	if err := sized(op, len(xUserIDs), 0, port.MaxXUserIDs); err != nil {
		return nil, err
	}
	rows, err := r.q.UserIDsByXUserIDs(ctx, xUserIDs)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	return idsByKey(op, rows, func(row sqlc.UserIDsByXUserIDsRow) (string, uuid.UUID) {
		return row.XUserID.String, row.ID
	})
}

func (r Postgres) MemberWallet(ctx context.Context, id ids.UserID) (port.MemberWallet, error) {
	const op = "identity.MemberWallet"
	row, err := r.q.MemberWalletByUserID(ctx, id.UUID())
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return port.MemberWallet{}, errs.New(errs.CodeUserNotFound, op)
	case err != nil:
		return port.MemberWallet{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return walletOf(id, row.PrivyWalletID, row.Address), nil
}

func (r Postgres) MemberWallets(ctx context.Context, after ids.UserID, limit int) ([]port.MemberWallet, error) {
	const op = "identity.MemberWallets"
	if err := sized(op, limit, 1, port.MaxWalletPage); err != nil {
		return nil, err
	}
	rows, err := r.q.MemberWalletsAfter(ctx, sqlc.MemberWalletsAfterParams{After: after.UUID(), PageSize: int64(limit)})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	page := make([]port.MemberWallet, 0, len(rows))
	for _, row := range rows {
		id, err := userID(op, row.UserID)
		if err != nil {
			return nil, err
		}
		page = append(page, walletOf(id, row.PrivyWalletID, row.Address))
	}
	return page, nil
}

func sized(op string, n, lo, hi int) error {
	if n >= lo && n <= hi {
		return nil
	}
	return errs.New(errs.CodeInvalidInput, op, slog.Int("size", n), slog.Int("min", lo), slog.Int("max", hi))
}

func handleKey(raw string) (string, bool) {
	h, err := domain.ParseHandle(raw)
	return h.String(), err == nil
}

func userID(op string, raw uuid.UUID) (ids.UserID, error) {
	id, err := ids.ParseUserID(raw.String())
	if err != nil {
		return ids.UserID{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	return id, nil
}

func idsByKey[R any](op string, rows []R, keyed func(R) (string, uuid.UUID)) (map[string]ids.UserID, error) {
	found := make(map[string]ids.UserID, len(rows))
	for _, row := range rows {
		key, raw := keyed(row)
		id, err := userID(op, raw)
		if err != nil {
			return nil, err
		}
		found[key] = id
	}
	return found, nil
}

func cardOf(id ids.UserID, row sqlc.UserCardsByIDRow) port.UserCard {
	card := port.UserCard{
		ID: id, AuthState: domain.AuthState(row.AuthState), AccountStatus: domain.AccountStatus(row.AccountStatus),
		PhoneVerified: row.PhoneVerified, XLinked: row.XLinked, CreatedAt: row.CreatedAt.UTC(), Deleted: row.Deleted,
	}
	if row.FirstDepositAt.Valid {
		at := row.FirstDepositAt.Time.UTC()
		card.FirstDepositAt = &at
	}
	if !row.Deleted {
		card.Handle, card.PhotoURL = row.Handle.String, row.PhotoUrl.String
		card.DisplayName = cmp.Or(row.DisplayName, row.Handle.String)
	}
	return card
}

func walletOf(id ids.UserID, privyWalletID, address string) port.MemberWallet {
	return port.MemberWallet{UserID: id, PrivyWalletID: privyWalletID, Address: chain.SolanaAddress(address)}
}
