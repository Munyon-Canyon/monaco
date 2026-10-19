package notify_test

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const bothOn = `{"decision_requests":true,"replies_and_mentions":true}`

const preferencesPath = "/v1/me/notification-preferences"

type namedKind struct {
	*crowd
	name string
}

func (k namedKind) Name() string { return k.name }

type queryCounter struct{ preferences atomic.Int32 }

func (c *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.Contains(d.SQL, "FROM notification_preferences") {
		c.preferences.Add(1)
	}
	return ctx
}

func (*queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (s server) getPreferences(t *testing.T, user ids.UserID) *httptest.ResponseRecorder {
	t.Helper()
	return s.do(t, http.MethodGet, preferencesPath, user, "", "")
}

func wantPreferences(t *testing.T, step string, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	wantCode(t, step, rec, http.StatusOK)
	var got, exp map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("%s body %s: %v", step, rec.Body, err)
	}
	if err := json.Unmarshal([]byte(want), &exp); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(exp) || got["decision_requests"] != exp["decision_requests"] ||
		got["replies_and_mentions"] != exp["replies_and_mentions"] {
		t.Fatalf("%s = %s, want %s", step, rec.Body, want)
	}
}

func (s server) putPreferences(t *testing.T, user ids.UserID, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	return s.do(t, http.MethodPut, preferencesPath, user, key, body)
}

func TestPreferences_aMemberWithNoRowsHasBothOn(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	user := s.user(t, "active")
	wantPreferences(t, "GET", s.getPreferences(t, user), `{"decision_requests":true,"replies_and_mentions":true}`)
}

func TestPreferences_aPutPersistsAndASecondPutOverwrites(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	user, other := s.user(t, "active"), s.user(t, "active")

	first := `{"decision_requests":false,"replies_and_mentions":true}`
	wantPreferences(t, "first PUT", s.putPreferences(t, user, "k1", first), first)
	wantPreferences(t, "GET", s.getPreferences(t, user), first)
	second := `{"decision_requests":true,"replies_and_mentions":false}`
	wantPreferences(t, "second PUT", s.putPreferences(t, user, "k2", second), second)
	wantPreferences(t, "GET again", s.getPreferences(t, user), second)
	wantPreferences(t, "another member's GET", s.getPreferences(t, other),
		`{"decision_requests":true,"replies_and_mentions":true}`)
}

func TestPreferences_refusesABodyWithAMissingOrExtraFieldAndADeletedAccount(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	active, deleted := s.user(t, "active"), s.user(t, "deleted")
	for name, tc := range map[string]struct {
		user ids.UserID
		body string
		want int
	}{
		"missing decision_requests":    {active, `{"replies_and_mentions":true}`, http.StatusBadRequest},
		"missing replies_and_mentions": {active, `{"decision_requests":true}`, http.StatusBadRequest},
		"extra field":                  {active, bothOn[:len(bothOn)-1] + `,"nudges":true}`, http.StatusBadRequest},
		"deleted account":              {deleted, bothOn, http.StatusUnauthorized},
	} {
		wantCode(t, name, s.putPreferences(t, tc.user, "k-"+name, tc.body), tc.want)
	}
	wantCode(t, "deleted account GET", s.getPreferences(t, deleted), http.StatusUnauthorized)
	wantPreferences(t, "GET after refusals", s.getPreferences(t, active),
		`{"decision_requests":true,"replies_and_mentions":true}`)
}

func (r *pushRig) preference(t *testing.T, user ids.UserID, c domain.Category, enabled bool) {
	t.Helper()
	r.exec(t, `INSERT INTO notification_preferences (user_id, category, enabled, updated_at) VALUES ($1, $2, $3, $4)`,
		user.UUID(), string(c), enabled, r.clock.Now())
}

func (r *pushRig) wantKinds(t *testing.T, want ...string) {
	t.Helper()
	rows, err := r.pool.Query(t.Context(), `SELECT user_id || '/' || kind FROM notifications`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var row string
		if err := rows.Scan(&row); err != nil {
			t.Fatal(err)
		}
		got = append(got, row)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("notifications = %v, want %v", got, want)
	}
}

func TestPush_aMemberWithDecisionRequestsOffStillGetsDeposits(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	quiet, loud := r.user(t, "active"), r.user(t, "active")
	r.preference(t, quiet, domain.CategoryDecisionRequests, false)
	d, e := r.trigger(t, opsActor, quiet)
	to := []ids.UserID{quiet, loud}

	err := r.handle(t, r.sender, d, e,
		namedKind{&crowd{users: to}, "proposal_created"}, namedKind{&crowd{users: to}, "deposit_credited"})

	wantVerdict(t, err, "", errs.VerdictAck)
	r.wantKinds(t,
		quiet.String()+"/deposit_credited", loud.String()+"/deposit_credited", loud.String()+"/proposal_created")
}

func TestPush_aMemberWithRepliesAndMentionsOffStillGetsProposals(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	quiet := r.user(t, "active")
	r.preference(t, quiet, domain.CategoryRepliesAndMentions, false)
	d, e := r.trigger(t, opsActor, quiet)
	to := []ids.UserID{quiet}

	err := r.handle(t, r.sender, d, e,
		namedKind{&crowd{users: to}, "comment_reply"}, namedKind{&crowd{users: to}, "chat_mention"},
		namedKind{&crowd{users: to}, "proposal_created"})

	wantVerdict(t, err, "", errs.VerdictAck)
	r.wantKinds(t, quiet.String()+"/proposal_created")
}

func TestPush_anEnabledRowDoesNotSilenceAnything(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	user := r.user(t, "active")
	r.preference(t, user, domain.CategoryDecisionRequests, true)
	d, e := r.trigger(t, opsActor, user)

	err := r.handle(t, r.sender, d, e, namedKind{&crowd{users: []ids.UserID{user}}, "proposal_created"})

	wantVerdict(t, err, "", errs.VerdictAck)
	r.wantKinds(t, user.String()+"/proposal_created")
}

func TestPush_readsPreferencesOnceForAllRecipientsAndKinds(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	counter := &queryCounter{}
	cfg := r.pool.Config().Copy()
	cfg.ConnConfig.Tracer = counter
	traced, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(traced.Close)
	r.uow = db.New(traced, r.ids, r.clock)
	a, b, c := r.user(t, "active"), r.user(t, "active"), r.user(t, "active")
	d, e := r.trigger(t, opsActor, a)

	err = r.handle(t, r.sender, d, e,
		namedKind{&crowd{users: []ids.UserID{a, b, c}}, "proposal_created"},
		namedKind{&crowd{users: []ids.UserID{b, c, a}}, "comment_reply"},
		namedKind{&crowd{users: []ids.UserID{c}}, "deposit_credited"})

	wantVerdict(t, err, "", errs.VerdictAck)
	if got := counter.preferences.Load(); got != 1 {
		t.Fatalf("preference reads = %d, want one for the whole event", got)
	}
}

func kindNames(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir("app")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(gotoken.NewFileSet(), filepath.Join("app", entry.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			if name, ok := kindName(decl); ok {
				names = append(names, name)
			}
		}
	}
	return names
}

func kindName(decl ast.Decl) (string, bool) {
	fn, ok := decl.(*ast.FuncDecl)
	if !ok || fn.Recv == nil || fn.Name.Name != "Name" || fn.Body == nil || len(fn.Body.List) != 1 {
		return "", false
	}
	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return "", false
	}
	lit, ok := ret.Results[0].(*ast.BasicLit)
	if !ok || lit.Kind != gotoken.STRING {
		return "", false
	}
	name, err := strconv.Unquote(lit.Value)
	return name, err == nil
}

func TestCategoryOf_classifiesEveryKindExactlyOnce(t *testing.T) {
	t.Parallel()
	decisions := []string{
		"proposal_created", "proposal_passed", "trade_filled", "trade_failed", "trade_blocked",
		"cabal_paused", "cabal_resumed",
	}
	replies := []string{"comment_reply", "chat_mention", "chat_thread_reply"}
	always := domain.AlwaysOnKinds()
	names := kindNames(t)
	known := len(decisions) + len(replies) + len(always)
	if len(names) != known {
		t.Fatalf("app/ has %d kind names, the buckets hold %d: %v", len(names), known, names)
	}
	for _, name := range names {
		_, categorised := domain.CategoryOf(name)
		if inAlways := slices.Contains(always, name); categorised == inAlways {
			t.Errorf("kind %q is categorised = %v and always on = %v, want exactly one", name, categorised, inAlways)
		}
	}
	for want, kinds := range map[domain.Category][]string{
		domain.CategoryDecisionRequests: decisions, domain.CategoryRepliesAndMentions: replies,
	} {
		for _, name := range kinds {
			if got, ok := domain.CategoryOf(name); !ok || got != want || !slices.Contains(names, name) {
				t.Errorf("kind %q is in %q (%v), want %q and a Name() in app/", name, got, ok, want)
			}
		}
	}
}
