package analytics_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type verifier func(context.Context, string) (auth.Actor, error)

func (v verifier) Verify(ctx context.Context, raw string) (auth.Actor, error) { return v(ctx, raw) }

const viewerToken = "viewer"

func dashboardHandler(t *testing.T, pool *pgxpool.Pool, mount func(api.Mount)) http.Handler {
	t.Helper()
	clk := testkit.NewClock(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	v := verifier(func(_ context.Context, raw string) (auth.Actor, error) {
		if raw != viewerToken {
			return auth.Actor{}, errs.New(errs.CodeAdminForbidden, "test")
		}
		return auth.Actor{Kind: auth.ActorAdmin, ID: "019cc330-1111-7000-8000-000000000002", Role: "viewer"}, nil
	})
	h, err := httpx.Handler(httpx.Deps{
		Logger:        observability.NewLogger(config.Config{Env: config.EnvTest}, io.Discard),
		Tracer:        noop.NewTracerProvider(),
		Clock:         clk,
		IDs:           testkit.NewIDs(1),
		MaxBodyBytes:  1 << 20,
		Idempotency:   db.NewIdempotencyStore(pool, clk),
		Verifier:      v,
		AdminVerifier: v,
	}, mount, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return testkit.HTTP(t, h)
}

func productHandler(t *testing.T, pool *pgxpool.Pool) http.Handler {
	t.Helper()
	return dashboardHandler(t, pool, analytics.New(module.Deps{Pool: pool, Config: testkit.Config()}).Mount)
}

func moneyGet(t *testing.T, h http.Handler, token string, q url.Values) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/admin/dashboards/money?"+q.Encode(), nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func window(from, to, bucket string) url.Values {
	return url.Values{"from": {from}, "to": {to}, "bucket": {bucket}}
}

func decode(t *testing.T, w *httptest.ResponseRecorder, out any) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body)
	}
	if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
		t.Fatalf("decode %s: %v", w.Body, err)
	}
}

func problemCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", w.Body, err)
	}
	return body.Code
}

type ledgerEntry struct {
	account string
	asset   string
	amount  int64
}

type ledgerSeed struct {
	t    *testing.T
	pool *pgxpool.Pool
	ids  *testkit.IDs
}

func (s ledgerSeed) userTxn(kind, status string, at time.Time, entries ...ledgerEntry) {
	s.t.Helper()
	id := s.ids.NewV7()
	if _, err := s.pool.Exec(s.t.Context(),
		`INSERT INTO user_txns (id, user_id, kind, status, created_at) VALUES ($1, $2, $3, $4, $5)`,
		id, s.ids.NewV7(), kind, status, at); err != nil {
		s.t.Fatal(err)
	}
	for seq, e := range entries {
		if _, err := s.pool.Exec(s.t.Context(),
			`INSERT INTO user_txn_entries (txn_id, seq, account, asset, amount) VALUES ($1, $2, $3, $4, $5)`,
			id, seq, e.account, e.asset, e.amount); err != nil {
			s.t.Fatal(err)
		}
	}
}

func (s ledgerSeed) cabalTxn(kind, status string, at time.Time, entries ...ledgerEntry) {
	s.t.Helper()
	id := s.ids.NewV7()
	if _, err := s.pool.Exec(s.t.Context(),
		`INSERT INTO cabal_txns (id, cabal_id, kind, status, created_at, seq) VALUES ($1, $2, $3, $4, $5, 1)`,
		id, s.ids.NewV7(), kind, status, at); err != nil {
		s.t.Fatal(err)
	}
	for seq, e := range entries {
		if _, err := s.pool.Exec(s.t.Context(),
			`INSERT INTO cabal_txn_entries (txn_id, seq, account, asset, amount) VALUES ($1, $2, $3, $4, $5)`,
			id, seq, e.account, e.asset, e.amount); err != nil {
			s.t.Fatal(err)
		}
	}
}

func (s ledgerSeed) swap(status string, at time.Time, usdcToVenue int64) {
	s.t.Helper()
	usdc, stock := string(testkit.USDCMint), "XSTOCKMINT"
	s.cabalTxn("swap", status, at,
		ledgerEntry{"treasury", usdc, -usdcToVenue}, ledgerEntry{"venue", usdc, usdcToVenue},
		ledgerEntry{"venue", stock, -7}, ledgerEntry{"treasury", stock, 7})
}

func (s ledgerSeed) cashOut(status string, at time.Time, paid int64) {
	s.t.Helper()
	usdc := string(testkit.USDCMint)
	s.cabalTxn("cash_out", status, at, ledgerEntry{"treasury", usdc, -paid}, ledgerEntry{"members", usdc, paid})
}

func (s ledgerSeed) wallet(kind, status string, at time.Time, delta int64) {
	s.t.Helper()
	usdc := string(testkit.USDCMint)
	counter := "external"
	if kind == "fund" {
		counter = "cabal"
	}
	s.userTxn(kind, status, at, ledgerEntry{"wallet", usdc, delta}, ledgerEntry{counter, usdc, -delta})
}

func (s ledgerSeed) fundMember(at time.Time, micros uint64) {
	s.t.Helper()
	gen := ids.Real{}
	testkit.NewLedgerFor(s.t, s.pool, gen, at).FundMember(
		s.t.Context(), ids.NewUserID(gen), ids.CabalIDFrom(gen.NewV7()), money.MicrosFromUint64(micros))
}
