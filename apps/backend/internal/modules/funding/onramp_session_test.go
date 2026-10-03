package funding_test

import (
	"context"
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const fundPage = "https://fund.example/fund"

type onrampFixture struct {
	ids    *testkit.IDs
	pool   *pgxpool.Pool
	clock  *testkit.Clock
	user   testkit.SeededUser
	create *app.CreateOnrampSessionHandler
}

func newOnrampFixture(t *testing.T) onrampFixture {
	t.Helper()
	pool := testkit.DB(t)
	c := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))
	uow := db.New(pool, testkit.NewIDs(testkit.RandSeed(t)), c)
	return onrampFixture{
		ids: testkit.NewIDs(
			testkit.RandSeed(t),
		),
		pool:   pool,
		clock:  c,
		user:   testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true}),
		create: app.NewCreateOnrampSessionHandler(uow, c, fundPage),
	}
}

func (f onrampFixture) asUser(t *testing.T) context.Context {
	t.Helper()
	return observability.WithActor(t.Context(), "user:"+f.user.ID.String())
}

func (f onrampFixture) start(t *testing.T, suggested *money.Micros) (app.OnrampSessionCreated, domain.OnrampToken) {
	t.Helper()
	created, err := f.create.Handle(f.asUser(t), app.CreateOnrampSession{
		ID: f.ids.NewV7(), UserID: f.user.ID, SuggestedAmount: suggested,
	})
	if err != nil {
		t.Fatal(err)
	}
	return created, tokenOf(t, created.URL)
}

func tokenOf(t *testing.T, raw string) domain.OnrampToken {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	token, err := domain.ParseOnrampToken(u.Query().Get("s"))
	if err != nil {
		t.Fatalf("URL %s carries no token: %v", raw, err)
	}
	return token
}

func onrampEvents(t *testing.T, pool *pgxpool.Pool) []events.OnrampStatusChanged {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT payload FROM events WHERE type = $1 ORDER BY id`,
		events.TypeOnrampStatusChanged)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []events.OnrampStatusChanged
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			t.Fatal(err)
		}
		var e events.OnrampStatusChanged
		if err := json.Unmarshal(payload, &e); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	return out
}

func TestCreateOnrampSession_storesOnlyTheTokenHashAndKeepsTheWalletOutOfTheURL(t *testing.T) {
	t.Parallel()
	f := newOnrampFixture(t)
	suggested := money.MicrosFromUint64(25_000_000)
	cabal := f.ids.NewV7()
	created, err := f.create.Handle(f.asUser(t), app.CreateOnrampSession{
		ID: f.ids.NewV7(), UserID: f.user.ID, SuggestedAmount: &suggested, CabalID: &cabal,
	})
	if err != nil {
		t.Fatal(err)
	}
	token := tokenOf(t, created.URL)
	if !strings.HasPrefix(created.URL, fundPage+"?s=") || strings.Contains(created.URL, string(f.user.Address)) {
		t.Fatalf("URL = %s, want the fund page with only the token", created.URL)
	}
	if want := f.clock.Now().Add(10 * time.Minute); !created.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %s, want %s", created.ExpiresAt, want)
	}
	assertCreatedRow(t, f.pool, created, token, cabal)
	assertNoRawToken(t, f.pool, token)
	got := onrampEvents(t, f.pool)
	if len(got) != 1 {
		t.Fatalf("events = %+v, want one", got)
	}
	want := events.OnrampStatusChanged{
		V: 1, SessionID: created.ID, UserID: f.user.ID.UUID(), To: "created", SuggestedAmountMicros: &suggested,
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Fatalf("event = %+v, want %+v", got[0], want)
	}
}

func assertCreatedRow(
	t *testing.T, pool *pgxpool.Pool, created app.OnrampSessionCreated, token domain.OnrampToken, cabal uuid.UUID,
) {
	t.Helper()
	var hash []byte
	var status, amount string
	var cabalID uuid.UUID
	var expires time.Time
	err := pool.QueryRow(t.Context(), `SELECT token_hash, status, suggested_amount_micros::text, cabal_id, expires_at
		FROM onramp_sessions WHERE id = $1`, created.ID).Scan(&hash, &status, &amount, &cabalID, &expires)
	if err != nil {
		t.Fatal(err)
	}
	if string(hash) != string(token.Hash()) || status != "created" || amount != "25000000" || cabalID != cabal ||
		!expires.Equal(created.ExpiresAt) {
		t.Fatalf("row = %x %s %s %s %s, want the hash, created, the amount, the cabal and the expiry",
			hash, status, amount, cabalID, expires)
	}
}

func assertNoRawToken(t *testing.T, pool *pgxpool.Pool, token domain.OnrampToken) {
	t.Helper()
	var leaks int
	err := pool.QueryRow(t.Context(), `SELECT count(*) FROM onramp_sessions s
		WHERE position($1::text in row_to_json(s)::text) > 0`, token.Encode()).Scan(&leaks)
	if err != nil {
		t.Fatal(err)
	}
	if leaks != 0 {
		t.Fatal("the raw token is stored")
	}
}

func TestCreateOnrampSession_withoutAnAmountStoresNull(t *testing.T) {
	t.Parallel()
	f := newOnrampFixture(t)
	created, _ := f.start(t, nil)
	var amount, cabal *string
	if err := f.pool.QueryRow(t.Context(), `SELECT suggested_amount_micros::text, cabal_id::text
		FROM onramp_sessions WHERE id = $1`, created.ID).Scan(&amount, &cabal); err != nil {
		t.Fatal(err)
	}
	if amount != nil || cabal != nil {
		t.Fatalf("amount, cabal = %v, %v, want null", amount, cabal)
	}
	if got := onrampEvents(t, f.pool); len(got) != 1 || got[0].SuggestedAmountMicros != nil {
		t.Fatalf("events = %+v, want a null suggested amount", got)
	}
}

func TestCreateOnrampSession_failsWithoutWritingWhenTheInsertFails(t *testing.T) {
	t.Parallel()
	f := newOnrampFixture(t)
	f.exec(t, `ALTER TABLE onramp_sessions RENAME TO onramp_sessions_gone`)
	_, err := f.create.Handle(f.asUser(t), app.CreateOnrampSession{ID: f.ids.NewV7(), UserID: f.user.ID})
	if err == nil {
		t.Fatal("Handle error = nil")
	}
	if got := onrampEvents(t, f.pool); len(got) != 0 {
		t.Fatalf("events = %+v, want none", got)
	}
}

func (f onrampFixture) exec(t *testing.T, sql string) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), sql); err != nil {
		t.Fatal(err)
	}
}
