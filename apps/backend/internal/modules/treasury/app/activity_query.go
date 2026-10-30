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
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	activityDefault = 30
	activityMax     = 100
)

type Members interface {
	IsMember(ctx context.Context, id ids.CabalID, user ids.UserID) (bool, error)
}

type Users interface {
	UsersByID(ctx context.Context, userIDs []ids.UserID) (map[ids.UserID]identityport.UserCard, error)
}

type AssetName struct {
	Symbol       string
	Name         string
	Decimals     uint8
	MultiplierAt func(time.Time) (num, den int64)
}

type AssetNames interface {
	AssetNames(ctx context.Context) (map[domain.Asset]AssetName, error)
}

type ListActivity struct {
	CabalID ids.CabalID
	Caller  ids.UserID
	Limit   int
	Cursor  string
	Symbol  string
}

type Actor struct {
	UserID      ids.UserID
	Handle      string
	DisplayName string
}

type ActivityView struct {
	ID          uuid.UUID
	Kind        domain.ActivityKind
	Status      domain.ActivityStatus
	Asset       *AssetName
	USDCMicros  *int64
	Units       *int64
	Actor       *Actor
	TxSignature *string
	OccurredAt  time.Time
}

type ActivityPage struct {
	Items      []ActivityView
	NextCursor string
}

type ActivityReads struct {
	q        *sqlc.Queries
	members  Members
	users    Users
	assets   AssetNames
	listings ListingReader
}

func NewActivityReads(db sqlc.DBTX, members Members, users Users, assets AssetNames) *ActivityReads {
	return &ActivityReads{q: sqlc.New(db), members: members, users: users, assets: assets}
}

func (r *ActivityReads) List(ctx context.Context, req ListActivity) (ActivityPage, error) {
	const op = "treasury.ListActivity"
	limit, cursor, err := parseActivityPage(req)
	if err != nil {
		return ActivityPage{}, err
	}
	member, err := r.members.IsMember(ctx, req.CabalID, req.Caller)
	if err != nil {
		return ActivityPage{}, err
	}
	if !member {
		return ActivityPage{}, errs.New(errs.CodeNotCabalMember, op, slog.String("cabal_id", req.CabalID.String()))
	}
	params := sqlc.ListActivityRoutedParams{
		CabalID: req.CabalID.UUID(), HasCursor: cursor.ok, CursorAt: cursor.at, CursorID: cursor.id,
		RowLimit: limit + 1,
	}
	names, known, err := r.filter(ctx, req, &params)
	if err != nil {
		return ActivityPage{}, err
	}
	page := ActivityPage{Items: []ActivityView{}}
	if !known {
		return page, nil
	}
	rows, err := r.page(ctx, params, &page)
	if err != nil || len(rows) == 0 {
		return page, err
	}
	names, actors, err := r.lookups(ctx, rows, names)
	if err != nil {
		return ActivityPage{}, err
	}
	if names == nil {
		names = map[domain.Asset]AssetName{}
	}
	if err := r.listingNames(ctx, rows, names); err != nil {
		return ActivityPage{}, err
	}
	for _, row := range rows {
		page.Items = append(page.Items, activityView(row, names, actors))
	}
	return page, nil
}

func (r *ActivityReads) filter(
	ctx context.Context, req ListActivity, params *sqlc.ListActivityRoutedParams,
) (map[domain.Asset]AssetName, bool, error) {
	if req.Symbol == "" {
		return nil, true, nil
	}
	names, err := r.assets.AssetNames(ctx)
	if err != nil {
		return nil, false, err
	}
	mint, ok := mintOf(names, req.Symbol)
	if !ok {
		return names, false, nil
	}
	params.Assets, err = r.stockKeys(ctx, req.CabalID, mint)
	return names, err == nil, err
}

func (r *ActivityReads) page(
	ctx context.Context, params sqlc.ListActivityRoutedParams, page *ActivityPage,
) ([]sqlc.ListActivityRow, error) {
	routed, err := r.q.ListActivityRouted(ctx, params)
	if err != nil {
		return nil, err
	}
	rows := make([]sqlc.ListActivityRow, len(routed))
	for i, row := range routed {
		rows[i] = sqlc.ListActivityRow(row)
	}
	if limit := int(params.RowLimit) - 1; len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		page.NextCursor = encodeActivityCursor(last.OccurredAt, last.ID)
	}
	return rows, nil
}

func mintOf(names map[domain.Asset]AssetName, symbol string) (domain.Asset, bool) {
	for mint, name := range names {
		if name.Symbol == symbol {
			return mint, true
		}
	}
	return "", false
}

func (r *ActivityReads) lookups(
	ctx context.Context, rows []sqlc.ListActivityRow, names map[domain.Asset]AssetName,
) (map[domain.Asset]AssetName, map[ids.UserID]identityport.UserCard, error) {
	var users []ids.UserID
	for _, row := range rows {
		if row.Asset.Valid && names == nil {
			var err error
			if names, err = r.assets.AssetNames(ctx); err != nil {
				return nil, nil, err
			}
		}
		if row.ActorUserID.Valid {
			users = append(users, ids.UserIDFrom(row.ActorUserID.Bytes))
		}
	}
	if len(users) == 0 {
		return names, nil, nil
	}
	cards, err := r.users.UsersByID(ctx, users)
	return names, cards, err
}

func activityView(
	row sqlc.ListActivityRow, names map[domain.Asset]AssetName, cards map[ids.UserID]identityport.UserCard,
) ActivityView {
	v := ActivityView{
		ID: row.ID, Kind: domain.ActivityKind(row.Kind), Status: domain.ActivityStatus(row.Status),
		OccurredAt: row.OccurredAt.UTC(),
	}
	if name, ok := names[domain.Asset(row.Asset.String)]; ok && row.Asset.Valid {
		v.Asset = &name
	}
	if n, err := row.UsdcMicros.Int64Value(); err == nil && n.Valid {
		v.USDCMicros = &n.Int64
	}
	if n, err := row.Units.Int64Value(); err == nil && n.Valid {
		v.Units = &n.Int64
	}
	if row.TxSignature.Valid {
		v.TxSignature = &row.TxSignature.String
	}
	if card, ok := cards[ids.UserIDFrom(row.ActorUserID.Bytes)]; ok && row.ActorUserID.Valid {
		v.Actor = &Actor{UserID: card.ID, Handle: card.Handle, DisplayName: card.DisplayName}
	}
	return v
}

type activityCursor struct {
	ok bool
	at time.Time
	id uuid.UUID
}

func parseActivityPage(req ListActivity) (int32, activityCursor, error) {
	const op = "treasury.ListActivity"
	limit := req.Limit
	if limit == 0 {
		limit = activityDefault
	}
	if limit < 1 || limit > activityMax {
		return 0, activityCursor{}, errs.New(errs.CodeInvalidInput, op, slog.String("field", "limit"))
	}
	if req.Cursor == "" {
		return int32(limit), activityCursor{}, nil
	}
	body, err := base64.RawURLEncoding.DecodeString(req.Cursor)
	if err != nil {
		return 0, activityCursor{}, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	micros, id, _ := strings.Cut(string(body), ":")
	at, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return 0, activityCursor{}, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return 0, activityCursor{}, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	return int32(limit), activityCursor{ok: true, at: time.UnixMicro(at).UTC(), id: parsed}, nil
}

func encodeActivityCursor(at time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(at.UnixMicro(), 10) + ":" + id.String()))
}

func (r *ActivityReads) stockKeys(ctx context.Context, cabalID ids.CabalID, mint domain.Asset) ([]string, error) {
	keys, _, err := stockListings(ctx, r.q, r.listings, cabalID, mint)
	return keys, err
}

func stockListings(
	ctx context.Context, q *sqlc.Queries, listings ListingReader, cabalID ids.CabalID, mint domain.Asset,
) ([]string, map[string]ListingQuote, error) {
	keys := []string{string(mint)}
	if listings == nil {
		return keys, nil, nil
	}
	listed, err := q.CabalActivityListingKeys(ctx, cabalID.UUID())
	if err != nil || len(listed) == 0 {
		return keys, nil, err
	}
	quotes, err := listings(ctx, listed, time.Time{})
	if err != nil {
		return nil, nil, err
	}
	matched := map[string]ListingQuote{}
	for _, key := range listed {
		if quote, ok := quotes[key]; ok && quote.Asset != nil && quote.Asset.Mint == string(mint) {
			keys = append(keys, key)
			matched[key] = quote
		}
	}
	return keys, matched, nil
}
