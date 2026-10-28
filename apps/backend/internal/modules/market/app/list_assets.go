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
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	listDefault = 20
	listMax     = 50
)

type Filter string

const (
	FilterAll     Filter = "all"
	FilterPopular Filter = "popular"
	FilterPreIPO  Filter = "pre_ipo"
	FilterMovers  Filter = "movers"

	FilterInCabals Filter = "in_cabals"
)

type ListRequest struct {
	Q      string
	Filter Filter
	Limit  int
	Cursor string
}

type Summary struct {
	Asset     domain.Asset
	Session   domain.SessionInfo
	Priced    bool
	Price     domain.Sample
	Change    *int32
	Sparkline []money.Micros
	Volume    *Volume

	CabalCount *int
	Profile    *domain.Profile
}

type Page struct {
	Items      []Summary
	NextCursor string
}

type Lister interface {
	Handle(context.Context, ListRequest) (Page, error)
}

type catalogReader interface {
	ListAssetsBySymbol(context.Context, sqlc.ListAssetsBySymbolParams) ([]sqlc.Asset, error)
	ListAssetsByRank(context.Context, sqlc.ListAssetsByRankParams) ([]sqlc.Asset, error)
	NewestSamples(context.Context, sqlc.NewestSamplesParams) ([]sqlc.NewestSamplesRow, error)
	FirstSamplesSince(context.Context, sqlc.FirstSamplesSinceParams) ([]sqlc.FirstSamplesSinceRow, error)
	SparklineCloses(context.Context, sqlc.SparklineClosesParams) ([]sqlc.SparklineClosesRow, error)
}

var _ Lister = (*ListAssets)(nil)

var _ catalogReader = (*sqlc.Queries)(nil)

type assetLister interface {
	ListAll(context.Context) ([]domain.Asset, error)
}

type ListAssets struct {
	read     catalogReader
	volumes  volumeReader
	clock    clock.Clock
	catalog  assetLister
	hot      []HotMints
	ranked   moversBook
	holders  HolderCounts
	held     holderBook
	profiles preIPOReader
}

func NewListAssets(db sqlc.DBTX, c clock.Clock, hot ...HotMints) *ListAssets {
	queries := sqlc.New(db)
	return &ListAssets{
		read: queries, volumes: queries, profiles: queries, clock: c, catalog: NewCatalog(db), hot: hot,
	}
}

type listQuery struct {
	q      string
	filter Filter
	limit  int32
	cursor pageCursor
}

type pageCursor struct {
	ok     bool
	move   int32
	symbol string
	rank   int16
	id     uuid.UUID
}

func (l *ListAssets) Handle(ctx context.Context, req ListRequest) (Page, error) {
	q, err := normalize(req)
	if err != nil {
		return Page{}, err
	}
	if q.filter == FilterMovers {
		items, next, moversErr := l.movers(ctx, q, l.clock.Now())
		if moversErr != nil {
			return Page{}, moversErr
		}
		return Page{Items: items, NextCursor: next}, nil
	}
	if q.filter == FilterInCabals {
		items, startersErr := l.starters(ctx, q, l.clock.Now())
		if startersErr != nil {
			return Page{}, startersErr
		}
		return Page{Items: items}, nil
	}
	rows, err := l.page(ctx, q)
	if err != nil {
		return Page{}, err
	}
	more := len(rows) > int(q.limit)
	if more {
		rows = rows[:q.limit]
	}
	assets, err := assetsOf(rows)
	if err != nil {
		return Page{}, err
	}
	if len(assets) == 0 {
		return Page{Items: []Summary{}}, nil
	}
	items, err := l.summaries(ctx, assets, l.clock.Now())
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: items}
	if more {
		page.NextCursor = encodeCursor(q.filter, assets[len(assets)-1])
	}
	return page, nil
}

func (l *ListAssets) page(ctx context.Context, q listQuery) ([]sqlc.Asset, error) {
	prefix, contains := like(q.q)
	if q.filter == FilterPopular {
		rows, err := l.read.ListAssetsByRank(ctx, sqlc.ListAssetsByRankParams{
			Q: q.q, Prefix: prefix, Contains: contains, HasCursor: q.cursor.ok,
			CursorRank: q.cursor.rank, CursorID: q.cursor.id, RowLimit: q.limit + 1,
		})
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeOf(err), "market.ListAssets")
		}
		return rows, nil
	}
	kind := ""
	if q.filter == FilterPreIPO {
		kind = string(domain.KindPreIPO)
	}
	rows, err := l.read.ListAssetsBySymbol(ctx, sqlc.ListAssetsBySymbolParams{
		Kind: kind, Q: q.q, Prefix: prefix, Contains: contains, HasCursor: q.cursor.ok,
		CursorSymbol: q.cursor.symbol, CursorID: q.cursor.id, RowLimit: q.limit + 1,
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "market.ListAssets")
	}
	return rows, nil
}

func normalize(req ListRequest) (listQuery, error) {
	filter, err := parseFilter(req.Filter)
	if err != nil {
		return listQuery{}, err
	}
	limit, err := normalizeLimit(req.Limit)
	if err != nil {
		return listQuery{}, err
	}
	if filter == FilterInCabals {
		if req.Cursor != "" {
			return listQuery{}, errs.New(errs.CodeInvalidInput, "market.cursor")
		}
		if req.Limit == 0 {
			limit = starterDefault
		}
	}
	cur, err := decodeCursor(filter, req.Cursor)
	if err != nil {
		return listQuery{}, err
	}
	return listQuery{q: strings.TrimSpace(req.Q), filter: filter, limit: limit, cursor: cur}, nil
}

func parseFilter(raw Filter) (Filter, error) {
	switch raw {
	case "", FilterAll:
		return FilterAll, nil
	case FilterPopular, FilterPreIPO, FilterMovers, FilterInCabals:
		return raw, nil
	default:
		return "", errs.New(errs.CodeInvalidInput, "market.ListAssets", slog.String("filter", string(raw)))
	}
}

func normalizeLimit(n int) (int32, error) {
	if n == 0 {
		return listDefault, nil
	}
	if n < 1 || n > listMax {
		return 0, errs.New(errs.CodeInvalidInput, "market.ListAssets", slog.Int("limit", n))
	}
	return int32(n), nil
}

func like(q string) (string, string) {
	esc := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(q)
	return esc + "%", "%" + esc + "%"
}

func encodeCursor(filter Filter, last domain.Asset) string {
	head := "s" + last.Symbol
	if filter == FilterPopular {
		head = "r" + strconv.FormatInt(int64(last.PopularRank), 10)
	}
	raw := head + "\x00" + last.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(filter Filter, raw string) (pageCursor, error) {
	if raw == "" {
		return pageCursor{}, nil
	}
	body, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return pageCursor{}, errs.Wrap(err, errs.CodeInvalidInput, "market.cursor")
	}
	head, id, ok := strings.Cut(string(body), "\x00")
	if !ok || len(head) < 2 {
		return pageCursor{}, errs.New(errs.CodeInvalidInput, "market.cursor")
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return pageCursor{}, errs.Wrap(err, errs.CodeInvalidInput, "market.cursor")
	}
	return cursorOf(filter, head, parsed)
}

func cursorOf(filter Filter, head string, id uuid.UUID) (pageCursor, error) {
	switch {
	case filter == FilterMovers && head[0] == 'm':
		return decodeMoverCursor(head, id)
	case filter == FilterPopular && head[0] == 'r':
		rank, err := strconv.ParseInt(head[1:], 10, 16)
		if err != nil {
			return pageCursor{}, errs.Wrap(err, errs.CodeInvalidInput, "market.cursor")
		}
		return pageCursor{ok: true, rank: int16(rank), id: id}, nil
	case filter != FilterPopular && filter != FilterMovers && head[0] == 's':
		return pageCursor{ok: true, symbol: head[1:], id: id}, nil
	}
	return pageCursor{}, errs.New(errs.CodeInvalidInput, "market.cursor")
}

func assetsOf(rows []sqlc.Asset) ([]domain.Asset, error) {
	out := make([]domain.Asset, len(rows))
	for i, row := range rows {
		asset, err := toAsset(row)
		if err != nil {
			return nil, err
		}
		out[i] = asset
	}
	return out, nil
}

func (l *ListAssets) summaries(ctx context.Context, assets []domain.Asset, now time.Time) ([]Summary, error) {
	sessions, err := sessionsFor(assets, now)
	if err != nil {
		return nil, err
	}
	loaded, err := l.load(ctx, assets, now, sessions[domain.KindEquity])
	if err != nil {
		return nil, err
	}
	profiles, err := loadProfiles(ctx, l.profiles, assets)
	if err != nil {
		return nil, err
	}
	out := make([]Summary, len(assets))
	for i, asset := range assets {
		out[i] = summary(asset, sessions[asset.Kind], loaded)
		if profile, ok := profiles[asset.CompanyKey]; ok && asset.Kind == domain.KindPreIPO {
			out[i].Profile = &profile
		}
	}
	return out, nil
}

func sessionsFor(assets []domain.Asset, now time.Time) (map[domain.Kind]domain.SessionInfo, error) {
	out := map[domain.Kind]domain.SessionInfo{}
	for _, asset := range assets {
		if _, ok := out[asset.Kind]; ok {
			continue
		}
		info, err := domain.Session(asset.Kind, now)
		if err != nil {
			return nil, err
		}
		out[asset.Kind] = info
	}
	return out, nil
}
