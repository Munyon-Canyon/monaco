package referrals_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const clickPath = "/v1/referrals/clicks"

func postClick(t *testing.T, h http.Handler, header map[string]string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, clickPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func (s server) click(t *testing.T, key, code string) *httptest.ResponseRecorder {
	t.Helper()
	return postClick(t, s.handler, map[string]string{"Idempotency-Key": key}, fmt.Sprintf(`{"code":%q}`, code))
}

func (s server) tap(t *testing.T, key, code string) {
	t.Helper()
	if rec := s.click(t, key, code); rec.Code != http.StatusNoContent {
		t.Fatalf("POST a click for %q with key %s = %d %s, want 204", code, key, rec.Code, rec.Body)
	}
}

func clickRows(t *testing.T, pool *pgxpool.Pool) map[string]int {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT code || ' ' || day::text, clicks FROM referral_clicks`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]int{}
	for rows.Next() {
		var key string
		var clicks int
		if err := rows.Scan(&key, &clicks); err != nil {
			t.Fatal(err)
		}
		got[key] = clicks
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

func clickLogs(t *testing.T, logs *testkit.Logs) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.Lines(string(logs.Bytes())) {
		var fields map[string]any
		if err := json.Unmarshal([]byte(line), &fields); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		if fields["msg"] == observability.ReferralsClick.Name {
			out = append(out, fields)
		}
	}
	return out
}

func TestRecordReferralClick_IdempotentPerKey(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedOwner(t, s.pool, owner{handle: "kaicenat", code: "k7m4qx2p"})
	day := s.clock.Now().UTC().Format(time.DateOnly)
	for _, tap := range []struct {
		key  string
		want int
	}{{"tap-1", 1}, {"tap-1", 1}, {"tap-2", 2}} {
		rec := s.click(t, tap.key, "k7m4qx2p")
		if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
			t.Fatalf("POST a click with key %s = %d %q, want 204 with no body", tap.key, rec.Code, rec.Body)
		}
		want := map[string]int{"k7m4qx2p " + day: tap.want}
		if got := clickRows(t, s.pool); !reflect.DeepEqual(got, want) {
			t.Fatalf("after key %s the clicks are %v, want %v", tap.key, got, want)
		}
	}
	if got := len(clickLogs(t, s.logs)); got != 2 {
		t.Fatalf("%d referrals.click lines for 3 requests with 2 keys, want 2: a replay must not run the command", got)
	}
}

func TestRecordReferralClick_countsEachKnownCodeUnderItsLowercaseCode(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedOwner(t, s.pool, owner{handle: "kaicenat", unlocked: true, code: "k7m4qx2p"})
	day := s.clock.Now().UTC().Format(time.DateOnly)
	for i, code := range []string{"K7M4QX2P", "KaiCenat", " kaicenat "} {
		s.tap(t, fmt.Sprintf("tap-%d", i), code)
	}
	want := map[string]int{"k7m4qx2p " + day: 1, "kaicenat " + day: 2}
	if got := clickRows(t, s.pool); !reflect.DeepEqual(got, want) {
		t.Fatalf("clicks = %v, want %v", got, want)
	}
	lines := clickLogs(t, s.logs)
	kinds := make([]string, 0, len(lines))
	for _, line := range lines {
		kinds = append(kinds, fmt.Sprintf("%v %v", line["code_kind"], line["known"]))
	}
	if want := []string{"random true", "handle true", "handle true"}; !slices.Equal(kinds, want) {
		t.Fatalf("referrals.click lines say %v, want %v", kinds, want)
	}
	for _, code := range []string{"k7m4qx2p", "kaicenat"} {
		if strings.Contains(string(s.logs.Bytes()), code) {
			t.Errorf("a log line holds the code %s", code)
		}
	}
}

func TestRecordReferralClick_theDayRollsAtMidnightUTC(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedOwner(t, s.pool, owner{handle: "kaicenat", code: "k7m4qx2p"})
	midnight := s.clock.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
	westOfUTC := time.FixedZone("UTC-5", -5*60*60)
	s.clock.Set(midnight.Add(-time.Second).In(westOfUTC))
	s.tap(t, "tap-1", "k7m4qx2p")
	s.clock.Set(midnight.In(westOfUTC))
	s.tap(t, "tap-2", "k7m4qx2p")
	s.clock.Advance(time.Hour)
	s.tap(t, "tap-3", "k7m4qx2p")
	want := map[string]int{
		"k7m4qx2p " + midnight.AddDate(0, 0, -1).Format(time.DateOnly): 1,
		"k7m4qx2p " + midnight.Format(time.DateOnly):                   2,
	}
	if got := clickRows(t, s.pool); !reflect.DeepEqual(got, want) {
		t.Fatalf("clicks = %v, want %v", got, want)
	}
}

func TestRecordReferralClick_anUnknownCodeAnswers204AndCountsNothing(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedOwner(t, s.pool, owner{handle: "lockedkai"})
	seedOwner(t, s.pool, owner{handle: "bannedkai", unlocked: true, status: "banned", code: "gggggggg"})
	codes := []string{"k7m4qx2q", "nobodykai", "lockedkai", "bannedkai", "gggggggg"}
	for i, code := range codes {
		s.tap(t, fmt.Sprintf("tap-%d", i), code)
	}
	if got := clickRows(t, s.pool); len(got) != 0 {
		t.Fatalf("clicks = %v, want none", got)
	}
	lines := clickLogs(t, s.logs)
	if len(lines) != len(codes) {
		t.Fatalf("%d referrals.click lines for %d unknown codes, want one each", len(lines), len(codes))
	}
	for _, line := range lines {
		if line["code_kind"] != "unknown" || line["known"] != false {
			t.Fatalf("referrals.click line = %v, want code_kind unknown and known false", line)
		}
	}
	for _, code := range codes {
		if strings.Contains(string(s.logs.Bytes()), code) {
			t.Errorf("a log line holds the code %s", code)
		}
	}
}

func expectRateLimited(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	var problem apibase.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("body %q: %v", rec.Body, err)
	}
	retryAfter := rec.Header().Get("Retry-After")
	if rec.Code != http.StatusTooManyRequests || problem.Code != apibase.RateLimited || retryAfter == "" {
		t.Fatalf("answer = %d %s Retry-After %q, want 429 rate_limited", rec.Code, rec.Body, retryAfter)
	}
}

func TestRecordReferralClick_theEleventhBurstClickIsRateLimited(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedOwner(t, s.pool, owner{handle: "kaicenat", code: "k7m4qx2p"})
	for i := range 10 {
		s.tap(t, fmt.Sprintf("tap-%d", i), "k7m4qx2p")
	}
	expectRateLimited(t, s.click(t, "tap-10", "k7m4qx2p"))
	s.clock.Advance(2 * time.Second)
	expectRateLimited(t, s.click(t, "tap-10", "k7m4qx2p"))
	s.clock.Advance(time.Second)
	s.tap(t, "tap-10", "k7m4qx2p")
	day := s.clock.Now().UTC().Format(time.DateOnly)
	if got, want := clickRows(t, s.pool), map[string]int{"k7m4qx2p " + day: 11}; !reflect.DeepEqual(got, want) {
		t.Fatalf("clicks = %v, want %v: a refused tap must not count, and its key must stay free", got, want)
	}
	if strings.Contains(string(s.logs.Bytes()), "192.0.2.1") {
		t.Error("a log line holds the client address")
	}
}

func TestRecordReferralClick_answersPreflightForTheWebOriginOnly(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	for origin, allowed := range map[string]bool{"https://monacolabs.xyz": true, "https://evil.example": false} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, clickPath, nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
		req.Header.Set("Access-Control-Request-Headers", "content-type, idempotency-key")
		rec := httptest.NewRecorder()
		s.raw.ServeHTTP(rec, req)
		got := rec.Header().Get("Access-Control-Allow-Origin")
		if allowed && (rec.Code != http.StatusNoContent || got != origin ||
			!strings.Contains(rec.Header().Get("Access-Control-Allow-Headers"), "Idempotency-Key")) {
			t.Errorf("preflight from %s = %d %v, want 204 and Idempotency-Key", origin, rec.Code, rec.Header())
		}
		if !allowed && got != "" {
			t.Errorf("preflight from %s allowed the origin: %v", origin, rec.Header())
		}
	}
	seedOwner(t, s.pool, owner{handle: "kaicenat", code: "k7m4qx2p"})
	header := map[string]string{"Idempotency-Key": "tap-1", "Origin": "https://monacolabs.xyz"}
	rec := postClick(t, s.handler, header, `{"code":"k7m4qx2p"}`)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "https://monacolabs.xyz" {
		t.Fatalf("click from the web origin = %d %v, want 204 allowing the origin", rec.Code, rec.Header())
	}
}

func TestRecordReferralClick_refusesABodyTheContractRejects(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedOwner(t, s.pool, owner{handle: "kaicenat", code: "k7m4qx2p"})
	for name, body := range map[string]string{
		"no code":            `{}`,
		"an empty code":      `{"code":""}`,
		"a code of 41 bytes": `{"code":"` + strings.Repeat("a", 41) + `"}`,
		"an ip":              `{"code":"k7m4qx2p","ip":"203.0.113.9"}`,
	} {
		rec := postClick(t, s.handler, map[string]string{"Idempotency-Key": "tap-1"}, body)
		var problem apibase.Problem
		if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil ||
			rec.Code != http.StatusBadRequest || problem.Code != apibase.InvalidInput {
			t.Errorf("%s: POST = %d %s, want 400 invalid_input", name, rec.Code, rec.Body)
		}
	}
	if got := clickRows(t, s.pool); len(got) != 0 {
		t.Fatalf("clicks = %v, want none", got)
	}
	for _, leaked := range []string{"k7m4qx2p", "203.0.113.9"} {
		if strings.Contains(string(s.logs.Bytes()), leaked) {
			t.Errorf("a log line holds %s from a rejected body", leaked)
		}
	}
}

func TestRecordReferralClick_aFailedReadOrWriteIsAnUnloggedErrorAndTheSameKeyRetries(t *testing.T) {
	t.Parallel()
	for name, table := range map[string]string{"the code read": "referral_codes", "the count write": "referral_clicks"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := newServer(t)
			seedOwner(t, s.pool, owner{handle: "kaicenat", code: "k7m4qx2p"})
			rename := func(from, to string) {
				if _, err := s.pool.Exec(t.Context(), `ALTER TABLE `+from+` RENAME TO `+to); err != nil {
					t.Fatal(err)
				}
			}
			rename(table, table+"_gone")
			rec := s.click(t, "tap-1", "k7m4qx2p")
			if rec.Code != http.StatusInternalServerError ||
				rec.Header().Get("Content-Type") != "application/problem+json" {
				t.Fatalf("click with %s failing = %d %s, want a 500 problem", name, rec.Code, rec.Body)
			}
			if lines := clickLogs(t, s.logs); len(lines) != 0 {
				t.Fatalf("referrals.click lines = %v, want none after a failure", lines)
			}
			rename(table+"_gone", table)
			s.tap(t, "tap-1", "k7m4qx2p")
			want := map[string]int{"k7m4qx2p " + s.clock.Now().UTC().Format(time.DateOnly): 1}
			if got := clickRows(t, s.pool); !reflect.DeepEqual(got, want) {
				t.Fatalf("clicks after the retry = %v, want %v: a 5xx must release the key", got, want)
			}
		})
	}
}

func TestReferralClicks_NoIPColumnOrAttr(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	rows, err := pool.Query(t.Context(), `SELECT column_name FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'referral_clicks' ORDER BY column_name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"clicks", "code", "day"}; !slices.Equal(columns, want) {
		t.Fatalf("referral_clicks columns = %v, want %v", columns, want)
	}
	if required := observability.ReferralsClick.Required; slices.Contains(required, "ip") ||
		slices.Contains(required, "ip_hash") {
		t.Fatalf("referrals.click requires %v, want no ip or ip_hash", required)
	}
}
