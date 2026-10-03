package app

import (
	"context"
	"encoding/base64"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	userTxnDefault = 30
	userTxnMax     = 100
)

type ListUserTxns struct {
	UserID ids.UserID
	Limit  int
	Cursor string
}

type UserTxnCabal struct {
	ID   ids.CabalID
	Name string
}

type UserTxnView struct {
	ID          uuid.UUID
	Kind        domain.UserTxnKind
	Status      domain.TxnStatus
	USDCMicros  int64
	Cabal       *UserTxnCabal
	TxSignature *string
	CreatedAt   time.Time
}

type UserTxnPage struct {
	Items      []UserTxnView
	NextCursor string
}

type UserTxnReads struct {
	q      *sqlc.Queries
	cabals CabalViews
	usdc   domain.Asset
}

type CabalViews interface {
	Cabals(context.Context, []ids.CabalID) (map[ids.CabalID]cabalport.CabalView, error)
}

func NewUserTxnReads(db sqlc.DBTX, cabals CabalViews, usdc domain.Asset) *UserTxnReads {
	return &UserTxnReads{q: sqlc.New(db), cabals: cabals, usdc: usdc}
}

func (r *UserTxnReads) List(ctx context.Context, req ListUserTxns) (UserTxnPage, error) {
	limit, cursor, err := parseUserTxnPage(req)
	if err != nil {
		return UserTxnPage{}, err
	}
	rows, err := r.q.ListUserTxns(ctx, sqlc.ListUserTxnsParams{
		UserID: req.UserID.UUID(), UsdcAsset: string(r.usdc), HasCursor: cursor.ok,
		CursorAt: cursor.at, CursorID: cursor.id, RowLimit: limit + 1,
	})
	if err != nil {
		return UserTxnPage{}, err
	}
	page := UserTxnPage{Items: []UserTxnView{}}
	if len(rows) > int(limit) {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		page.NextCursor = encodeUserTxnCursor(last.CreatedAt, last.ID)
	}
	cabals, err := r.cabalsFor(ctx, rows)
	if err != nil {
		return UserTxnPage{}, err
	}
	for _, row := range rows {
		page.Items = append(page.Items, userTxnView(row, cabals))
	}
	return page, nil
}

func (r *UserTxnReads) cabalsFor(
	ctx context.Context, rows []sqlc.ListUserTxnsRow,
) (map[ids.CabalID]cabalport.CabalView, error) {
	idsByCabal := map[ids.CabalID]struct{}{}
	for _, row := range rows {
		if row.CabalID.Valid {
			idsByCabal[ids.CabalIDFrom(row.CabalID.Bytes)] = struct{}{}
		}
	}
	if len(idsByCabal) == 0 {
		return map[ids.CabalID]cabalport.CabalView{}, nil
	}
	cabalIDs := make([]ids.CabalID, 0, len(idsByCabal))
	for id := range idsByCabal {
		cabalIDs = append(cabalIDs, id)
	}
	return r.cabals.Cabals(ctx, cabalIDs)
}

func userTxnView(row sqlc.ListUserTxnsRow, cabals map[ids.CabalID]cabalport.CabalView) UserTxnView {
	v := UserTxnView{
		ID: row.ID, Kind: domain.UserTxnKind(row.Kind), Status: domain.TxnStatus(row.Status),
		USDCMicros: row.Amount, CreatedAt: row.CreatedAt.UTC(),
	}
	if row.CabalID.Valid {
		id := ids.CabalIDFrom(row.CabalID.Bytes)
		if cabal, ok := cabals[id]; ok {
			v.Cabal = &UserTxnCabal{ID: id, Name: cabal.Name}
		}
	}
	if row.TxSignature.Valid {
		v.TxSignature = &row.TxSignature.String
	}
	return v
}

type userTxnCursor struct {
	ok bool
	at time.Time
	id uuid.UUID
}

func parseUserTxnPage(req ListUserTxns) (int32, userTxnCursor, error) {
	const op = "treasury.ListUserTxns"
	limit := req.Limit
	if limit == 0 {
		limit = userTxnDefault
	}
	if limit < 1 || limit > userTxnMax {
		return 0, userTxnCursor{}, errs.New(errs.CodeInvalidInput, op, slog.String("field", "limit"))
	}
	if req.Cursor == "" {
		return int32(limit), userTxnCursor{}, nil
	}
	body, err := base64.RawURLEncoding.DecodeString(req.Cursor)
	if err != nil {
		return 0, userTxnCursor{}, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	micros, id, _ := strings.Cut(string(body), ":")
	at, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return 0, userTxnCursor{}, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return 0, userTxnCursor{}, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	return int32(limit), userTxnCursor{ok: true, at: time.UnixMicro(at).UTC(), id: parsed}, nil
}

func encodeUserTxnCursor(at time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(at.UnixMicro(), 10) + ":" + id.String()))
}
