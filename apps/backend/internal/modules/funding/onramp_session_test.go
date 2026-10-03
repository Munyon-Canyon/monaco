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

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	fundPage = "https://fund.example/fund"
	usdcMint = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
)

type onrampFixture struct {
	ids      *testkit.IDs
	pool     *pgxpool.Pool
	clock    *testkit.Clock
	user     testkit.SeededUser
	create   *app.CreateOnrampSessionHandler
	exchange *app.ExchangeOnrampTokenHandler
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
		exchange: app.NewExchangeOnrampTokenHandler(uow, c, identity.New(module.Deps{Pool: pool}).Queries(),
			usdcMint),
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

func TestExchangeOnrampToken_opensTheSessionOnceAndReturnsTheWallet(t *testing.T) {
	t.Parallel()
	f := newOnrampFixture(t)
	suggested := money.MicrosFromUint64(25_000_000)
	created, token := f.start(t, &suggested)
	got, err := f.exchange.Handle(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	want := app.OnrampExchange{
		SessionID: created.ID, WalletAddress: f.user.Address, SuggestedAmount: &suggested, USDCMint: usdcMint,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("exchange = %+v, want %+v", got, want)
	}
	assertOpenedAt(t, f.pool, created.ID, f.clock.Now())
	if _, err := f.exchange.Handle(t.Context(), token); errs.CodeOf(err) != errs.CodeOnrampLinkInvalid {
		t.Fatalf("second exchange = %v, want onramp_link_invalid", err)
	}
	evs := onrampEvents(t, f.pool)
	if len(evs) != 2 || *evs[1].From != "created" || evs[1].To != "opened" || evs[1].SessionID != created.ID {
		t.Fatalf("events = %+v, want created then one opened", evs)
	}
	assertLastActor(t, f.pool, "user:"+f.user.ID.String())
}

func assertOpenedAt(t *testing.T, pool *pgxpool.Pool, id uuid.UUID, at time.Time) {
	t.Helper()
	var status string
	var opened time.Time
	if err := pool.QueryRow(t.Context(), `SELECT status, opened_at FROM onramp_sessions WHERE id = $1`,
		id).Scan(&status, &opened); err != nil {
		t.Fatal(err)
	}
	if status != "opened" || !opened.Equal(at) {
		t.Fatalf("row = %s %s, want opened at %s", status, opened, at)
	}
}

func assertLastActor(t *testing.T, pool *pgxpool.Pool, want string) {
	t.Helper()
	var actor string
	if err := pool.QueryRow(t.Context(), `SELECT actor_type || ':' || actor_id FROM events
		WHERE type = $1 ORDER BY id DESC LIMIT 1`, events.TypeOnrampStatusChanged).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	if actor != want {
		t.Fatalf("last event actor = %s, want %s", actor, want)
	}
}

func TestExchangeOnrampToken_withoutAnAmountReturnsNull(t *testing.T) {
	t.Parallel()
	f := newOnrampFixture(t)
	_, token := f.start(t, nil)
	got, err := f.exchange.Handle(t.Context(), token)
	if err != nil || got.SuggestedAmount != nil {
		t.Fatalf("exchange = %+v, %v, want a null amount", got, err)
	}
}

func TestExchangeOnrampToken_refusesAnExpiredOrUnknownTokenWithoutWriting(t *testing.T) {
	t.Parallel()
	t.Run("eleven minutes later", func(t *testing.T) {
		t.Parallel()
		f := newOnrampFixture(t)
		_, token := f.start(t, nil)
		f.clock.Advance(11 * time.Minute)
		if _, err := f.exchange.Handle(t.Context(), token); errs.CodeOf(err) != errs.CodeOnrampLinkExpired {
			t.Fatalf("exchange = %v, want onramp_link_expired", err)
		}
		assertOnlyCreated(t, f.pool)
	})
	t.Run("expired by the poller", func(t *testing.T) {
		t.Parallel()
		f := newOnrampFixture(t)
		_, token := f.start(t, nil)
		f.exec(t, `UPDATE onramp_sessions SET status = 'expired'`)
		if _, err := f.exchange.Handle(t.Context(), token); errs.CodeOf(err) != errs.CodeOnrampLinkExpired {
			t.Fatalf("exchange = %v, want onramp_link_expired", err)
		}
	})
	t.Run("unknown", func(t *testing.T) {
		t.Parallel()
		f := newOnrampFixture(t)
		f.start(t, nil)
		stranger := domain.NewOnrampToken([domain.OnrampTokenBytes]byte{1})
		if _, err := f.exchange.Handle(t.Context(), stranger); errs.CodeOf(err) != errs.CodeOnrampLinkInvalid {
			t.Fatalf("exchange = %v, want onramp_link_invalid", err)
		}
		assertOnlyCreated(t, f.pool)
	})
}

func assertOnlyCreated(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var status string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM onramp_sessions`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if evs := onrampEvents(t, pool); status != "created" || len(evs) != 1 {
		t.Fatalf("status %s with events %+v, want the session untouched", status, evs)
	}
}

func TestExchangeOnrampToken_rollsBackTheOpenWhenAStepFails(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup string
		want  errs.Code
	}{
		{"no member wallet", `DELETE FROM user_wallets`, errs.CodeUserNotFound},
		{
			"an amount past uint64", `UPDATE onramp_sessions SET suggested_amount_micros = 99999999999999999999`,
			errs.CodeInternal,
		},
		{"the event table is gone", `ALTER TABLE events RENAME TO events_gone`, errs.CodeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newOnrampFixture(t)
			_, token := f.start(t, nil)
			f.exec(t, tt.setup)
			if _, err := f.exchange.Handle(t.Context(), token); errs.CodeOf(err) != tt.want {
				t.Fatalf("exchange = %v, want %s", err, tt.want)
			}
			var status string
			if err := f.pool.QueryRow(t.Context(), `SELECT status FROM onramp_sessions`).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != "created" {
				t.Fatalf("status = %s, want created after the rollback", status)
			}
		})
	}
}

func TestExchangeOnrampToken_failsWhenTheSessionTableIsGone(t *testing.T) {
	t.Parallel()
	f := newOnrampFixture(t)
	_, token := f.start(t, nil)
	f.exec(t, `ALTER TABLE onramp_sessions RENAME TO onramp_sessions_gone`)
	if _, err := f.exchange.Handle(t.Context(), token); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("exchange = %v, want internal", err)
	}
}

func (f onrampFixture) exec(t *testing.T, sql string) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), sql); err != nil {
		t.Fatal(err)
	}
}
