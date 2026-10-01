package identity_test

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const aliceNo = "+14155550100"

func aliceX() *domain.XAccount { return &domain.XAccount{UserID: "x-alice", Username: "alice_on_x"} }

func (f *sessionFixture) signedIn(t *testing.T, privy app.PrivyUser) app.Me {
	t.Helper()
	f.privy.Seed(privy)
	me, err := f.open(t, privy.ID)
	if err != nil {
		t.Fatal(err)
	}
	return me
}

func (f *sessionFixture) storeLinks(
	t *testing.T,
	id ids.UserID,
	state domain.AuthState,
	phone string,
	x *domain.XAccount,
) {
	t.Helper()
	xID, xName := "", ""
	if x != nil {
		xID, xName = x.UserID, x.Username
	}
	var hash []byte
	if phone != "" {
		hash = portHash(phone)
	}
	if _, err := f.pool.Exec(t.Context(), `UPDATE users SET auth_state = $2, phone_e164 = NULLIF($3::text, ''),
		phone_hash = $4, phone_verified_at = CASE WHEN $3::text = '' THEN NULL ELSE now() END,
		x_user_id = NULLIF($5::text, ''), x_username = NULLIF($6::text, ''),
		x_linked_at = CASE WHEN $5::text = '' THEN NULL ELSE now() END WHERE id = $1`,
		id.UUID(), string(state), phone, hash, xID, xName); err != nil {
		t.Fatal(err)
	}
}

func (f *sessionFixture) authStateChanges(t *testing.T) []events.UserAuthStateChanged {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT payload FROM events WHERE type = $1 ORDER BY id`,
		string(events.TypeUserAuthStateChanged))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []events.UserAuthStateChanged
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			t.Fatal(err)
		}
		ev, err := events.Decode(events.TypeUserAuthStateChanged, 1, payload)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, ev.(events.UserAuthStateChanged))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (f *sessionFixture) linkColumns(t *testing.T) (phone string, hash []byte, xID string) {
	t.Helper()
	var p, x *string
	if err := f.pool.QueryRow(t.Context(), `SELECT phone_e164, phone_hash, x_user_id FROM users`).
		Scan(&p, &hash, &x); err != nil {
		t.Fatal(err)
	}
	if p != nil {
		phone = *p
	}
	if x != nil {
		xID = *x
	}
	return phone, hash, xID
}

func TestOpenSession_unlinkingThePhoneInPrivyMovesACompletedUserToAwaitingPhone(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	me := f.signedIn(t, app.PrivyUser{ID: alice, PhoneE164: aliceNo, X: aliceX()})
	f.storeLinks(t, me.ID, domain.AuthOnboardingCompleted, aliceNo, aliceX())
	f.privy.Seed(app.PrivyUser{ID: alice, Email: "alice@example.com", X: aliceX()})
	got, err := f.open(t, alice)
	if err != nil || got.AuthState != domain.AuthAwaitingPhone || got.PhoneLinked || got.XUsername != "alice_on_x" {
		t.Fatalf("Me = %+v, %v, want AWAITING_PHONE with no phone and X kept", got, err)
	}
	if phone, hash, x := f.linkColumns(t); phone != "" || hash != nil || x != "x-alice" {
		t.Fatalf("stored phone %q hash %x X %q, want the phone columns cleared and X kept", phone, hash, x)
	}
	want := events.UserAuthStateChanged{
		V:      1,
		UserID: me.ID.UUID(),
		From:   "ONBOARDING_COMPLETED",
		To:     "AWAITING_PHONE",
		Cause:  "unlink",
		At:     f.clock.Now(),
	}
	if changes := f.authStateChanges(t); len(changes) != 1 || changes[0] != want {
		t.Fatalf("user.auth_state_changed = %+v, want one %+v", changes, want)
	}
	if hints := f.hints.sent(); len(hints) != 2 || hints[1] != "user."+me.ID.String()+".me_changed" {
		t.Fatalf("hints = %v, want one for the creation and one for the change", hints)
	}
}

func TestOpenSession_unlinkingXMovesACompletedUserToAwaitingSocials(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	me := f.signedIn(t, app.PrivyUser{ID: alice, PhoneE164: aliceNo, X: aliceX()})
	f.storeLinks(t, me.ID, domain.AuthOnboardingCompleted, aliceNo, aliceX())
	f.privy.Seed(app.PrivyUser{ID: alice, PhoneE164: aliceNo})
	got, err := f.open(t, alice)
	if err != nil || got.AuthState != domain.AuthAwaitingSocials || !got.PhoneLinked || got.XUsername != "" {
		t.Fatalf("Me = %+v, %v, want AWAITING_SOCIALS with the phone kept and no X", got, err)
	}
	if phone, _, x := f.linkColumns(t); phone != aliceNo || x != "" {
		t.Fatalf("stored phone %q X %q, want the phone kept and the X columns cleared", phone, x)
	}
	if changes := f.authStateChanges(t); len(changes) != 1 || changes[0].Cause != "unlink" ||
		changes[0].To != "AWAITING_SOCIALS" {
		t.Fatalf("user.auth_state_changed = %+v, want one unlink to AWAITING_SOCIALS", changes)
	}
}

func TestOpenSession_aPhoneLinkedAfterOnboardingIsStoredWithItsHashAndMovesTheState(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	me := f.signedIn(t, app.PrivyUser{ID: alice, Email: "alice@example.com"})
	f.storeLinks(t, me.ID, domain.AuthAwaitingPhone, "", nil)
	f.privy.Seed(app.PrivyUser{ID: alice, Email: "alice@example.com", PhoneE164: aliceNo})
	got, err := f.open(t, alice)
	if err != nil || got.AuthState != domain.AuthAwaitingSocials || !got.PhoneLinked {
		t.Fatalf("Me = %+v, %v, want AWAITING_SOCIALS with the phone stored", got, err)
	}
	if phone, hash, _ := f.linkColumns(t); phone != aliceNo || !bytes.Equal(hash, portHash(aliceNo)) {
		t.Fatalf("stored phone %q hash %x, want the number and its SHA-256", phone, hash)
	}
	if changes := f.authStateChanges(t); len(changes) != 1 || changes[0].Cause != "link" ||
		changes[0].From != "AWAITING_PHONE" || changes[0].To != "AWAITING_SOCIALS" {
		t.Fatalf("user.auth_state_changed = %+v, want one link from AWAITING_PHONE to AWAITING_SOCIALS", changes)
	}
}

func TestOpenSession_bothAccountsLinkedAtOnceEachAppendTheirOwnChange(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	me := f.signedIn(t, app.PrivyUser{ID: alice, Email: "alice@example.com"})
	f.storeLinks(t, me.ID, domain.AuthAwaitingPhone, "", nil)
	f.privy.Seed(app.PrivyUser{ID: alice, Email: "alice@example.com", PhoneE164: aliceNo, X: aliceX()})
	got, err := f.open(t, alice)
	if err != nil || got.AuthState != domain.AuthOnboardingCompleted {
		t.Fatalf("Me = %+v, %v, want ONBOARDING_COMPLETED", got, err)
	}
	changes := f.authStateChanges(t)
	if len(changes) != 2 || changes[0].To != "AWAITING_SOCIALS" || changes[1].From != "AWAITING_SOCIALS" ||
		changes[1].To != "ONBOARDING_COMPLETED" {
		t.Fatalf(
			"user.auth_state_changed = %+v, want AWAITING_PHONE to AWAITING_SOCIALS to ONBOARDING_COMPLETED",
			changes,
		)
	}
}

func TestOpenSession_duringOnboardingANewLinkIsLeftToTheOnboardingCommands(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.signedIn(t, app.PrivyUser{ID: alice, PhoneE164: aliceNo, X: aliceX()})
	got, err := f.open(t, alice)
	if err != nil || got.AuthState != domain.AuthCreated || got.PhoneLinked || got.XUsername != "" {
		t.Fatalf("Me = %+v, %v, want CREATED with nothing stored", got, err)
	}
	if phone, _, x := f.linkColumns(
		t,
	); phone != "" || x != "" || len(f.authStateChanges(t)) != 0 ||
		len(f.hints.sent()) != 1 {
		t.Fatalf("stored phone %q X %q, %d changes, %d hints; want nothing stored or appended beyond the creation",
			phone, x, len(f.authStateChanges(t)), len(f.hints.sent()))
	}
}

func TestOpenSession_aNumberAnotherUserStoresIsLeftForTheNextSignIn(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	holder := testkit.SeedUser(t, f.pool, testkit.UserOpts{})
	if _, err := f.pool.Exec(t.Context(), `UPDATE users SET phone_e164 = $2, phone_hash = $3, x_user_id = 'x-alice'
		WHERE id = $1`, holder.ID.UUID(), aliceNo, portHash(aliceNo)); err != nil {
		t.Fatal(err)
	}
	me := f.signedIn(t, app.PrivyUser{ID: alice, Email: "alice@example.com"})
	f.storeLinks(t, me.ID, domain.AuthAwaitingPhone, "", nil)
	f.privy.Seed(app.PrivyUser{ID: alice, Email: "alice@example.com", PhoneE164: aliceNo, X: aliceX()})
	got, err := f.open(t, alice)
	if err != nil || got.AuthState != domain.AuthAwaitingPhone || got.PhoneLinked || got.XUsername != "" {
		t.Fatalf("Me = %+v, %v, want the account unchanged while another user holds the links", got, err)
	}
	if changes := f.authStateChanges(t); len(changes) != 0 {
		t.Fatalf("user.auth_state_changed = %+v, want none", changes)
	}
}

func TestOpenSession_aSecondSignInAfterALinkChangeChangesNothing(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	me := f.signedIn(t, app.PrivyUser{ID: alice, PhoneE164: aliceNo, X: aliceX()})
	f.storeLinks(t, me.ID, domain.AuthOnboardingCompleted, aliceNo, aliceX())
	f.privy.Seed(app.PrivyUser{ID: alice, Email: "alice@example.com", X: aliceX()})
	first, err := f.open(t, alice)
	if err != nil {
		t.Fatal(err)
	}
	hintsBefore := len(f.hints.sent())
	second, err := f.open(t, alice)
	if err != nil || second != first || len(f.authStateChanges(t)) != 1 || len(f.hints.sent()) != hintsBefore {
		t.Fatalf("second sign-in = %+v, %v with %d changes and %d hints, want the same account and nothing new",
			second, err, len(f.authStateChanges(t)), len(f.hints.sent()))
	}
}

func TestOpenSession_aLinkFailureRollsTheWholeSignInBack(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	me := f.signedIn(t, app.PrivyUser{ID: alice, PhoneE164: aliceNo, X: aliceX()})
	f.storeLinks(t, me.ID, domain.AuthOnboardingCompleted, aliceNo, aliceX())
	f.privy.Seed(app.PrivyUser{ID: alice, Email: "alice@example.com", X: aliceX()})
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE events RENAME TO events_gone`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.open(t, alice); err == nil {
		t.Fatal("sign-in succeeded without being able to append user.auth_state_changed")
	}
	phone, _, _ := f.linkColumns(t)
	var state string
	if err := f.pool.QueryRow(context.WithoutCancel(t.Context()), `SELECT auth_state FROM users`).
		Scan(&state); err != nil ||
		phone != aliceNo ||
		state != "ONBOARDING_COMPLETED" {
		t.Fatalf("after the failed sign-in: phone %q state %q, %v; want both untouched", phone, state, err)
	}
}

func TestOpenSession_aStateChangeReachesTheUsersStream(t *testing.T) {
	t.Parallel()
	conn := testkit.NATS(t).Conn
	hub, err := sse.NewHub(sse.NoMemberships{}, noop.NewMeterProvider())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	var running sync.WaitGroup
	running.Go(func() { hub.Run(ctx) })
	t.Cleanup(func() {
		cancel()
		running.Wait()
	})
	if err := conn.SubscribeHints(ctx, hub.Deliver); err != nil {
		t.Fatal(err)
	}
	f := newSessionFixture(t)
	f.handler = f.handlerFor(adapters.Users{}, conn)
	me := f.signedIn(t, app.PrivyUser{ID: alice, PhoneE164: aliceNo, X: aliceX()})
	f.storeLinks(t, me.ID, domain.AuthOnboardingCompleted, aliceNo, aliceX())
	stream, err := hub.Register(ctx, auth.Actor{Kind: auth.ActorUser, ID: me.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	f.privy.Seed(app.PrivyUser{ID: alice, Email: "alice@example.com", X: aliceX()})
	if _, err := f.open(t, alice); err != nil {
		t.Fatal(err)
	}
	want := sse.Hint{Key: sse.UserKey(me.ID), What: "me_changed"}
	var got []sse.Hint
	testkit.Eventually(t, func() bool {
		for {
			select {
			case hint := <-stream.Hints():
				got = append(got, hint)
			default:
				return slices.Contains(got, want)
			}
		}
	}, 10*time.Second)
}

type failingLinks struct {
	adapters.Users
	failing string
}

func (l failingLinks) HeldLinks(
	ctx context.Context, q sqlc.DBTX, id ids.UserID, claims domain.Claims, links domain.Links,
) (domain.Claims, error) {
	if l.failing == "HeldLinks" {
		return domain.Claims{}, errs.New(errs.CodeDBUnavailable, "test.HeldLinks")
	}
	return l.Users.HeldLinks(ctx, q, id, claims, links)
}

func (l failingLinks) ApplyLinks(
	ctx context.Context, q sqlc.DBTX, id ids.UserID, sync domain.LinkSync, at time.Time,
) error {
	if l.failing == "ApplyLinks" {
		return errs.New(errs.CodeDBUnavailable, "test.ApplyLinks")
	}
	return l.Users.ApplyLinks(ctx, q, id, sync, at)
}

func TestOpenSession_aFailureReadingOrWritingLinksRollsTheSignInBack(t *testing.T) {
	t.Parallel()
	for _, step := range []string{"HeldLinks", "ApplyLinks"} {
		t.Run(step, func(t *testing.T) {
			t.Parallel()
			f := newSessionFixture(t)
			me := f.signedIn(t, app.PrivyUser{ID: alice, Email: "alice@example.com"})
			f.storeLinks(t, me.ID, domain.AuthAwaitingPhone, "", nil)
			f.privy.Seed(app.PrivyUser{ID: alice, Email: "alice@example.com", PhoneE164: aliceNo})
			f.handler = f.handlerWith(adapters.Users{}, failingLinks{failing: step}, f.hints)
			if _, err := f.open(t, alice); errs.CodeOf(err) != errs.CodeDBUnavailable {
				t.Fatalf("sign-in = %v, want db_unavailable", err)
			}
			if phone, _, _ := f.linkColumns(t); phone != "" || len(f.authStateChanges(t)) != 0 {
				t.Fatalf("stored phone %q, %d changes; want nothing written", phone, len(f.authStateChanges(t)))
			}
		})
	}
}
