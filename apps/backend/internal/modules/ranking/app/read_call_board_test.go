package app_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type rcbRow struct {
	period   string
	rank     int
	name     string
	right    int
	resolved int
}

func rcbID(name string) uuid.UUID {
	return uuid.NewSHA1(uuid.Nil, []byte("read-call-board/"+name))
}

func rcbUser(name string) ids.UserID { return ids.UserIDFrom(rcbID(name)) }

func rcbSeed(t *testing.T, pool dbsqlc.DBTX, board, kind string, rows []rcbRow) {
	t.Helper()
	run := ids.Real{}.NewV7()
	at := boardNow()
	starts := map[string]string{"week": "2026-10-26", "month": "2026-10-01", "all": "1970-01-01"}
	wire := make([]map[string]any, len(rows))
	for i, r := range rows {
		entry := map[string]any{
			"board":               board,
			"period":              r.period,
			"period_start":        starts[r.period],
			"subject_id":          rcbID(r.name),
			"subject_kind":        kind,
			"subject_name":        r.name,
			"subject_handle":      nil,
			"subject_picture_url": nil,
			"subject_created_at":  at.Add(-time.Duration(i+1) * time.Hour).Format(time.RFC3339Nano),
			"rank":                nil,
			"right_count":         r.right,
			"resolved":            r.resolved,
			"voted":               r.resolved,
			"eligible":            r.resolved,
			"rate_bps":            nil,
			"turnout_bps":         10_000,
			"baseline_rank":       nil,
			"movement":            1,
			"movement_kind":       "up",
			"last_call":           nil,
			"run_id":              run,
			"computed_at":         at.Format(time.RFC3339Nano),
		}
		if r.rank > 0 {
			entry["rank"], entry["rate_bps"] = r.rank, r.right*10_000/r.resolved
		}
		if i == 0 {
			entry["last_call"] = map[string]any{
				"proposal_id": rcbID("proposal"), "kind": "buy", "symbol": "NVDA", "right": true,
			}
		}
		if i == 1 {
			entry["last_call"] = "not an object"
		}
		wire[i] = entry
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	q := sqlc.New(pool)
	if err := q.InsertCallBoardEntries(t.Context(), raw); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertCallBoardRun(t.Context(), sqlc.InsertCallBoardRunParams{
		RunID: run, StartedAt: at, FinishedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
}

func rcbPeople(period string, count int) []rcbRow {
	rows := make([]rcbRow, 0, count+3)
	for rank := 1; rank <= count; rank++ {
		name := fmt.Sprintf("P%02d", rank)
		if rank == 8 {
			name = "Andres"
		}
		rows = append(rows, rcbRow{period: period, rank: rank, name: name, right: 10 - rank/3, resolved: 10})
	}
	rows = append(rows, rcbRow{period: period, name: "Rosa", right: 1, resolved: 2})
	return rows
}

func rcbWeekPeople() []rcbRow {
	rows := rcbPeople("week", 6)
	return append(rows, rcbRow{period: "week", name: "Andres", right: 2, resolved: 3})
}

func rcbCabals(period string, unranked ...string) []rcbRow {
	names := []string{"Pilates", "Titans", "AC", "Degens", "Quant", "Moon", "Value", "Macro"}
	rows := make([]rcbRow, 0, len(names))
	rank := 0
	for _, name := range names {
		row := rcbRow{period: period, name: name, right: 9 - rank/2, resolved: 10}
		if slices.Contains(unranked, name) {
			row.right, row.resolved = 1, 2
			rows = append(rows, row)
			continue
		}
		rank++
		row.rank = rank
		rows = append(rows, row)
	}
	return rows
}

var errBoom = errs.New(errs.CodeInternal, "test.boom")

type rcbReads struct {
	q      *sqlc.Queries
	calls  *atomic.Int32
	failAt int32
}

func (r rcbReads) fails() bool {
	n := r.calls.Add(1)
	return r.failAt != 0 && n == r.failAt
}

func (r rcbReads) CallBoardLastRun(ctx context.Context) (sqlc.CallBoardRun, error) {
	if r.fails() {
		return sqlc.CallBoardRun{}, errBoom
	}
	return r.q.CallBoardLastRun(ctx)
}

func (r rcbReads) CallBoardTop(ctx context.Context, a sqlc.CallBoardTopParams) ([]sqlc.CallBoardEntry, error) {
	if r.fails() {
		return nil, errBoom
	}
	return r.q.CallBoardTop(ctx, a)
}

func (r rcbReads) CallBoardCount(ctx context.Context, a sqlc.CallBoardCountParams) (int32, error) {
	if r.fails() {
		return 0, errBoom
	}
	return r.q.CallBoardCount(ctx, a)
}

func (r rcbReads) CallBoardRow(ctx context.Context, a sqlc.CallBoardRowParams) (sqlc.CallBoardEntry, error) {
	if r.fails() {
		return sqlc.CallBoardEntry{}, errBoom
	}
	return r.q.CallBoardRow(ctx, a)
}

func (r rcbReads) CallBoardAt(ctx context.Context, a sqlc.CallBoardAtParams) (sqlc.CallBoardEntry, error) {
	if r.fails() {
		return sqlc.CallBoardEntry{}, errBoom
	}
	return r.q.CallBoardAt(ctx, a)
}

func (r rcbReads) CallBoardWindow(ctx context.Context, a sqlc.CallBoardWindowParams) ([]sqlc.CallBoardEntry, error) {
	if r.fails() {
		return nil, errBoom
	}
	return r.q.CallBoardWindow(ctx, a)
}

func (r rcbReads) CallBoardRanked(ctx context.Context, a sqlc.CallBoardRankedParams) ([]sqlc.CallBoardEntry, error) {
	if r.fails() {
		return nil, errBoom
	}
	return r.q.CallBoardRanked(ctx, a)
}

func (r rcbReads) CallBoardOf(ctx context.Context, a sqlc.CallBoardOfParams) ([]sqlc.CallBoardEntry, error) {
	if r.fails() {
		return nil, errBoom
	}
	return r.q.CallBoardOf(ctx, a)
}

func (r rcbReads) CallBoardUnranked(
	ctx context.Context, a sqlc.CallBoardUnrankedParams,
) ([]sqlc.CallBoardEntry, error) {
	if r.fails() {
		return nil, errBoom
	}
	return r.q.CallBoardUnranked(ctx, a)
}

type rcbPorts struct {
	following []ids.UserID
	cabals    []ids.CabalID
	cards     map[ids.UserID]identity.UserCard
	symbols   []string
	failing   string
}

func (p rcbPorts) fail(port string) error {
	if p.failing == port {
		return errBoom
	}
	return nil
}

func (p rcbPorts) FollowingIDs(context.Context, ids.UserID) ([]ids.UserID, error) {
	return p.following, p.fail("follows")
}

func (p rcbPorts) CabalsOf(context.Context, ids.UserID) ([]ids.CabalID, error) {
	return p.cabals, p.fail("members")
}

func (p rcbPorts) UsersByID(context.Context, []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	return p.cards, p.fail("users")
}

func (p rcbPorts) PendingSymbols(
	_ context.Context, users []ids.UserID, _ *ids.CabalID, _ domain.Period,
) (map[ids.UserID][]string, error) {
	return map[ids.UserID][]string{users[0]: p.symbols}, p.fail("pending")
}

type rcbHeads struct{ failing bool }

func (h rcbHeads) Load(
	ctx context.Context, _ app.CallBoardHeadKey, load func(context.Context) (app.CallBoardHead, error),
) (app.CallBoardHead, error) {
	if h.failing {
		return app.CallBoardHead{}, errBoom
	}
	return load(ctx)
}

type rcbCase struct {
	name   string
	query  app.CallBoardQuery
	ports  rcbPorts
	noHead bool
	uses   []string
}

func (c rcbCase) read(pool dbsqlc.DBTX, failAt int32, failing string) (int, error) {
	calls := &atomic.Int32{}
	ports := c.ports
	ports.failing = failing
	reads := rcbReads{q: sqlc.New(pool), calls: calls, failAt: failAt}
	read := app.ReadCallBoard{
		Runs: reads, Ranks: reads, Subjects: reads,
		Pending: ports, Users: ports, Follows: ports, Members: ports, Clock: clock.Real{},
	}
	if !c.noHead {
		read.Heads = rcbHeads{failing: failing == "heads"}
	}
	_, err := read.Run(context.Background(), c.query)
	return int(calls.Load()), err
}

func (c rcbCase) everyStepReportsItsFailure(t *testing.T, pool dbsqlc.DBTX) {
	t.Helper()
	queries, err := c.read(pool, 0, "")
	if err != nil || queries == 0 {
		t.Fatalf("read = %d queries, %v, want it to succeed", queries, err)
	}
	for at := 1; at <= queries; at++ {
		if _, err := c.read(pool, int32(at), ""); err == nil {
			t.Errorf("read with query %d of %d failing = nil, want the error", at, queries)
		}
	}
	for _, port := range c.uses {
		if _, err := c.read(pool, 0, port); err == nil {
			t.Errorf("read with the %s port failing = nil, want the error", port)
		}
	}
}

func rcbQuery(viewer, board string, period domain.PeriodKind, friends bool) app.CallBoardQuery {
	return app.CallBoardQuery{
		Viewer: rcbUser(viewer), Board: domain.Board(board), Period: period, Friends: friends,
	}
}

func rcbCursor(run uuid.UUID, after int) string {
	return base64.RawURLEncoding.EncodeToString(fmt.Appendf(nil, "%s:%d", run, after))
}

func rcbFollowing(names ...string) []ids.UserID {
	out := make([]ids.UserID, len(names))
	for i, n := range names {
		out[i] = rcbUser(n)
	}
	return out
}

func rcbCabalIDs(names ...string) []ids.CabalID {
	out := make([]ids.CabalID, len(names))
	for i, n := range names {
		out[i] = ids.CabalIDFrom(rcbID(n))
	}
	return out
}

func rcbFixture(t *testing.T) (dbsqlc.DBTX, uuid.UUID) {
	t.Helper()
	pool := testkit.DB(t)
	rcbSeed(t, pool, "people", "person", append(rcbPeople("month", 24), rcbWeekPeople()...))
	rcbSeed(t, pool, "people", "person", rcbPeople("all", 5))
	rcbSeed(t, pool, "cabals", "cabal", append(rcbCabals("month"), rcbCabals("all", "Titans", "Degens")...))
	run, err := sqlc.New(pool).CallBoardLastRun(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return pool, run.RunID
}

func rcbCases() []rcbCase {
	ghost := map[ids.UserID]identity.UserCard{rcbUser("Ghost"): {ID: rcbUser("Ghost"), Handle: "ghost"}}
	month, week := domain.PeriodMonth, domain.PeriodWeek
	return []rcbCase{
		{name: "people ranked", query: rcbQuery("Andres", "people", month, false), uses: []string{"heads"}},
		{name: "people ranked without a head cache", query: rcbQuery("Andres", "people", month, false), noHead: true},
		{name: "people on the podium", query: rcbQuery("P02", "people", month, false)},
		{name: "people at the bottom", query: rcbQuery("P24", "people", month, false)},
		{
			name:  "people unranked",
			query: rcbQuery("Andres", "people", week, false),
			ports: rcbPorts{
				following: rcbFollowing("Rosa"),
				symbols:   []string{"SPACEX"},
			},
			uses: []string{"follows", "pending"},
		},
		{
			name: "people without a row", query: rcbQuery("Ghost", "people", month, false),
			ports: rcbPorts{cards: ghost}, uses: []string{"follows", "users"},
		},
		{
			name: "people without a row or a card", query: rcbQuery("Ghost", "people", month, false),
			uses: []string{"follows", "users"},
		},
		{
			name: "people unranked over all time", query: rcbQuery("Ghost", "people", domain.PeriodAll, false),
			ports: rcbPorts{cards: ghost}, uses: []string{"follows", "users", "pending"},
		},
		{
			name: "friends ranked", query: rcbQuery("Andres", "people", month, true),
			ports: rcbPorts{following: rcbFollowing("P01", "P02", "P03", "P04", "P09")}, uses: []string{"follows"},
		},
		{
			name: "friends unranked with a row", query: rcbQuery("Andres", "people", week, true),
			ports: rcbPorts{following: rcbFollowing("P01", "Rosa")}, uses: []string{"follows", "pending"},
		},
		{
			name: "friends without a row", query: rcbQuery("Ghost", "people", month, true),
			ports: rcbPorts{following: rcbFollowing("P01"), cards: ghost}, uses: []string{"follows", "users"},
		},
	}
}

func rcbCabalCases() []rcbCase {
	month, week := domain.PeriodMonth, domain.PeriodWeek
	return []rcbCase{
		{
			name: "cabals ranked above the podium", query: rcbQuery("Andres", "cabals", month, false),
			ports: rcbPorts{cabals: rcbCabalIDs("Quant", "Macro")}, uses: []string{"members"},
		},
		{
			name: "cabals ranked first", query: rcbQuery("Andres", "cabals", month, false),
			ports: rcbPorts{cabals: rcbCabalIDs("Pilates")}, uses: []string{"members"},
		},
		{
			name: "cabals unranked", query: rcbQuery("Andres", "cabals", domain.PeriodAll, false),
			ports: rcbPorts{cabals: rcbCabalIDs("Titans", "Degens")}, uses: []string{"members"},
		},
		{
			name: "cabals with none of mine on the board", query: rcbQuery("Andres", "cabals", month, false),
			ports: rcbPorts{cabals: rcbCabalIDs("Stranger")}, uses: []string{"members"},
		},
		{
			name: "cabals over a period with no rows", query: rcbQuery("Andres", "cabals", week, false),
			ports: rcbPorts{cabals: rcbCabalIDs("Titans")}, uses: []string{"members"},
		},
		{name: "cabals with no cabal", query: rcbQuery("Andres", "cabals", month, false), uses: []string{"members"}},
	}
}

func TestReadCallBoard_everyStepReportsItsFailure(t *testing.T) {
	t.Parallel()
	pool, _ := rcbFixture(t)
	for _, c := range append(rcbCases(), rcbCabalCases()...) {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			c.everyStepReportsItsFailure(t, pool)
		})
	}
}

func TestReadCallBoard_everyPageReportsItsFailure(t *testing.T) {
	t.Parallel()
	pool, run := rcbFixture(t)
	month := domain.PeriodMonth
	for _, c := range []rcbCase{
		{name: "people", query: rcbQuery("Andres", "people", month, false)},
		{
			name: "friends", query: rcbQuery("Andres", "people", month, true),
			ports: rcbPorts{following: rcbFollowing("P01", "P02", "P03", "P04", "P09", "P10", "P11")},
		},
		{
			name: "cabals", query: rcbQuery("Andres", "cabals", month, false),
			ports: rcbPorts{cabals: rcbCabalIDs("Titans")},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			for _, after := range []int{0, 5, 24} {
				c.query.Cursor = rcbCursor(run, after)
				if queries, err := c.read(pool, 0, ""); err != nil {
					t.Fatalf("page after %d = %d queries, %v", after, queries, err)
				}
			}
			c.query.Cursor = rcbCursor(run, 2)
			c.everyStepReportsItsFailure(t, pool)
		})
	}
}
