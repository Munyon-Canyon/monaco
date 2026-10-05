package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
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
	q           *sqlc.Queries
	cabals      CabalViews
	withdrawals fundingport.Withdrawals
	usdc        domain.Asset
}

type CabalViews interface {
	Cabals(context.Context, []ids.CabalID) (map[ids.CabalID]cabalport.CabalView, error)
}

func NewUserTxnReads(
	db sqlc.DBTX, cabals CabalViews, withdrawals fundingport.Withdrawals, usdc domain.Asset,
) *UserTxnReads {
	return &UserTxnReads{q: sqlc.New(db), cabals: cabals, withdrawals: withdrawals, usdc: usdc}
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
	withdrawalPage := fundingport.WithdrawalPage{Limit: limit + 1}
	if cursor.ok {
		withdrawalPage.Before = &fundingport.WithdrawalCursor{At: cursor.at, ID: cursor.id}
	}
	open, err := r.withdrawals.OpenWithdrawals(ctx, req.UserID, withdrawalPage)
	if err != nil {
		return UserTxnPage{}, err
	}
	views := make([]UserTxnView, 0, len(rows)+len(open))
	for _, row := range rows {
		views = append(views, ledgerTxnView(row))
	}
	for _, w := range open {
		views = append(views, withdrawalTxnView(w))
	}
	slices.SortFunc(views, newestFirst)
	page := UserTxnPage{}
	if len(views) > int(limit) {
		views = views[:limit]
		last := views[len(views)-1]
		page.NextCursor = encodeUserTxnCursor(last.CreatedAt, last.ID)
	}
	if err := r.nameCabals(ctx, views); err != nil {
		return UserTxnPage{}, err
	}
	page.Items = views
	return page, nil
}

func ledgerTxnView(row sqlc.ListUserTxnsRow) UserTxnView {
	v := UserTxnView{
		ID: row.ID, Kind: domain.UserTxnKind(row.Kind), Status: domain.TxnStatus(row.Status),
		USDCMicros: row.Amount, CreatedAt: row.CreatedAt.UTC(),
	}
	if row.CabalID.Valid {
		v.Cabal = &UserTxnCabal{ID: ids.CabalIDFrom(row.CabalID.Bytes)}
	}
	if row.TxSignature.Valid {
		v.TxSignature = &row.TxSignature.String
	}
	return v
}

func withdrawalTxnView(w fundingport.OpenWithdrawal) UserTxnView {
	status := domain.TxnPending
	if w.Failed {
		status = domain.TxnFailed
	}
	return UserTxnView{
		ID: w.ID, Kind: domain.UserWithdrawal, Status: status, USDCMicros: w.Delta.Int64(),
		TxSignature: w.TxSignature, CreatedAt: w.CreatedAt.UTC(),
	}
}

func newestFirst(a, b UserTxnView) int {
	if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
		return c
	}
	return bytes.Compare(b.ID[:], a.ID[:])
}

func (r *UserTxnReads) nameCabals(ctx context.Context, views []UserTxnView) error {
	idsByCabal := map[ids.CabalID]struct{}{}
	for _, v := range views {
		if v.Cabal != nil {
			idsByCabal[v.Cabal.ID] = struct{}{}
		}
	}
	if len(idsByCabal) == 0 {
		return nil
	}
	cabals, err := r.cabals.Cabals(ctx, slices.Collect(maps.Keys(idsByCabal)))
	if err != nil {
		return err
	}
	for i, v := range views {
		if v.Cabal == nil {
			continue
		}
		if cabal, ok := cabals[v.Cabal.ID]; ok {
			views[i].Cabal.Name = cabal.Name
		} else {
			views[i].Cabal = nil
		}
	}
	return nil
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
