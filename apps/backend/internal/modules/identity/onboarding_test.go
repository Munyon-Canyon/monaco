package identity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/identityapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const onboardNo = "+14155550142"

func (f httpFixture) onboardingUser(t *testing.T, handle string, privy app.PrivyUser) testkit.SeededUser {
	t.Helper()
	u := f.seed(t, portSeed{handle: handle, wallet: true})
	privy.ID = app.PrivyUserID(u.PrivyUserID)
	f.privy.Seed(privy)
	return u
}

func (f httpFixture) onboard(t *testing.T, user ids.UserID, step, body, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/v1/me/onboarding/"+step, strings.NewReader(body),
	)
	req.Header.Set("Authorization", "Bearer "+f.verifier.Mint(user.String(), f.now.Add(time.Hour)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func (f httpFixture) authSteps(t *testing.T, user ids.UserID) []events.UserAuthStateChanged {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT payload FROM events WHERE type = $1 AND payload->>'user_id' = $2
		ORDER BY id`, string(events.TypeUserAuthStateChanged), user.String())
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
		step := ev.(events.UserAuthStateChanged)
		out = append(out, events.UserAuthStateChanged{From: step.From, To: step.To, Cause: step.Cause})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (f httpFixture) storedPhone(t *testing.T, user ids.UserID) (string, []byte, bool) {
	t.Helper()
	var phone *string
	var hash []byte
	var verified bool
	if err := f.pool.QueryRow(t.Context(), `SELECT phone_e164, phone_hash, phone_verified_at IS NOT NULL
		FROM users WHERE id = $1`, user.UUID()).Scan(&phone, &hash, &verified); err != nil {
		t.Fatal(err)
	}
	if phone == nil {
		return "", hash, verified
	}
	return *phone, hash, verified
}

func (f httpFixture) wantStoredPhone(t *testing.T, user ids.UserID, sum []byte) {
	t.Helper()
	if phone, hash, verified := f.storedPhone(t, user); phone != onboardNo || !bytes.Equal(hash, sum) || !verified {
		t.Fatalf("stored %q %x verified=%v, want %s, its sha256 and a verified time", phone, hash, verified, onboardNo)
	}
}

func wantProblem(t *testing.T, rec *httptest.ResponseRecorder, code apibase.ErrorCode) {
	t.Helper()
	if got := decodeProblem(t, rec); got.Code != code {
		t.Fatalf("problem = %d %s, want %s", rec.Code, rec.Body, code)
	}
}

func assertNoPII(t *testing.T, logs []byte, values ...string) {
	t.Helper()
	for _, v := range values {
		if bytes.Contains(logs, []byte(v)) {
			t.Fatalf("logs carry %q:\n%s", v, logs)
		}
	}
}

func TestLinkPhone_storesThePrivyPhoneNotTheBodyAndMovesToAwaitingSocialsOnce(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.onboardingUser(t, "phone_one", app.PrivyUser{PhoneE164: onboardNo})
	rec := f.onboard(t, u.ID, "phone", `{"phone":"+15550000000"}`, "p1")
	if rec.Code != http.StatusOK {
		t.Fatalf("link = %d %s", rec.Code, rec.Body)
	}
	if me := decodeMe(t, rec); me.AuthState != api.AWAITINGSOCIALS || !me.PhoneLinked {
		t.Fatalf("Me = %+v, want AWAITING_SOCIALS with the phone linked", me)
	}
	sum := sha256.Sum256([]byte(onboardNo))
	f.wantStoredPhone(t, u.ID, sum[:])
	again := f.onboard(t, u.ID, "phone", ``, "p2")
	if again.Code != http.StatusOK || decodeMe(t, again).AuthState != api.AWAITINGSOCIALS {
		t.Fatalf("again = %d %s", again.Code, again.Body)
	}
	want := []events.UserAuthStateChanged{{From: "CREATED", To: "AWAITING_SOCIALS", Cause: "onboarding"}}
	if got := f.authSteps(t, u.ID); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("auth steps = %+v, want %+v", got, want)
	}
	if got := f.hints.sent(); len(got) != 1 || got[0] != "user."+u.ID.String()+".me_changed" {
		t.Fatalf("hints = %v, want one me_changed", got)
	}
	assertNoPII(t, f.logs.Bytes(), onboardNo, hex.EncodeToString(sum[:]), "+15550000000")
}

func TestLinkPhone_refusesWithoutAHandleBeforeAskingPrivy(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.seed(t, portSeed{wallet: true})
	wantProblem(t, f.onboard(t, u.ID, "phone", ``, "p1"), apibase.HandleRequired)
}

func TestLinkPhone_refusesWhenPrivyHasNoPhone(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.onboardingUser(t, "phone_none", app.PrivyUser{Email: "none@example.com"})
	wantProblem(t, f.onboard(t, u.ID, "phone", ``, "p1"), apibase.PhoneNotLinked)
	if got := f.authSteps(t, u.ID); len(got) != 0 {
		t.Fatalf("auth steps = %+v, want none", got)
	}
}

func TestLinkPhone_aPrivyOutageIsPrivyUnavailable(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.onboardingUser(t, "phone_down", app.PrivyUser{PhoneE164: onboardNo})
	f.privy.FailOnce("User", errs.New(errs.CodePrivyUnavailable, "test.privy"))
	wantProblem(t, f.onboard(t, u.ID, "phone", ``, "p1"), apibase.PrivyUnavailable)
}

func TestLinkPhone_aNumberAnotherUserHoldsIsPhoneNotLinkedAndLoggedWithoutTheNumber(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	sum := sha256.Sum256([]byte(onboardNo))
	f.seed(t, portSeed{phoneHash: sum[:], phoneVerified: true})
	u := f.onboardingUser(t, "phone_clash", app.PrivyUser{PhoneE164: onboardNo})
	wantProblem(t, f.onboard(t, u.ID, "phone", ``, "p1"), apibase.PhoneNotLinked)
	logs := f.logs.Bytes()
	if !bytes.Contains(logs, []byte("identity.phone.conflict")) {
		t.Fatalf("logs lack identity.phone.conflict:\n%s", logs)
	}
	assertNoPII(t, logs, onboardNo, hex.EncodeToString(sum[:]))
	if phone, _, _ := f.storedPhone(t, u.ID); phone != "" {
		t.Fatalf("stored phone %q, want none", phone)
	}
}

func TestOnboardAdapter_passesOtherUniqueViolationsThrough(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	f.seed(t, portSeed{xUserID: "x-held"})
	u := f.seed(t, portSeed{})
	if _, err := f.pool.Exec(t.Context(), `UPDATE users SET x_username = 'held' WHERE x_user_id = 'x-held';
		CREATE UNIQUE INDEX test_unique_x_username ON users (x_username)`); err != nil {
		t.Fatal(err)
	}
	x := &domain.XAccount{UserID: "x-free", Username: "held"}
	sync := domain.LinkSync{X: domain.Write[*domain.XAccount]{Changed: true, Value: x}}
	err := adapters.Users{}.Onboard(t.Context(), f.pool, u.ID, sync, f.now)
	if err == nil || errs.CodeOf(err) == errs.CodeXNotLinked {
		t.Fatalf("Onboard = %v, want the raw unique violation", err)
	}
	_, err = adapters.Users{}.LockByID(t.Context(), f.pool, ids.UserID{})
	if errs.CodeOf(err) != errs.CodeUserNotFound {
		t.Fatalf("LockByID = %v, want user_not_found", err)
	}
}

type failingOnboardUsers struct {
	adapters.Users
	fail string
}

func (u failingOnboardUsers) FindByID(ctx context.Context, q sqlc.DBTX, id ids.UserID) (domain.User, error) {
	if u.fail == "find" {
		return domain.User{}, errs.New(errs.CodeInternal, "test.find")
	}
	return u.Users.FindByID(ctx, q, id)
}

func (u failingOnboardUsers) LockByID(ctx context.Context, q sqlc.DBTX, id ids.UserID) (domain.User, error) {
	if u.fail == "lock" {
		return domain.User{}, errs.New(errs.CodeInternal, "test.lock")
	}
	return u.Users.LockByID(ctx, q, id)
}

func (u failingOnboardUsers) Onboard(
	ctx context.Context, q sqlc.DBTX, id ids.UserID, sync domain.LinkSync, at time.Time,
) error {
	if u.fail == "write" {
		return errs.New(errs.CodeInternal, "test.write")
	}
	return u.Users.Onboard(ctx, q, id, sync, at)
}

func TestLinkPhone_aFailureAtAnyStepWritesNothing(t *testing.T) {
	t.Parallel()
	for _, fail := range []string{"find", "lock", "write", "event"} {
		t.Run(fail, func(t *testing.T) {
			t.Parallel()
			f := newHTTPFixture(t)
			u := f.onboardingUser(t, "fail_"+fail, app.PrivyUser{PhoneE164: onboardNo})
			if fail == "event" {
				const reject = `CREATE OR REPLACE FUNCTION reject_onboard_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'event'; END $$; CREATE TRIGGER reject_onboard_event BEFORE INSERT ON events FOR EACH ROW EXECUTE FUNCTION reject_onboard_event()`
				if _, err := f.pool.Exec(t.Context(), reject); err != nil {
					t.Fatal(err)
				}
			}
			h := app.NewOnboarding(app.OnboardingDeps{
				UoW: db.New(f.pool, f.ids, testkit.NewClock(f.now)), Reads: f.pool,
				Users: failingOnboardUsers{fail: fail}, Privy: f.privy, Clock: testkit.NewClock(f.now), Hints: f.hints,
			})
			if _, err := h.LinkPhone(t.Context(), app.LinkPhone{UserID: u.ID}); err == nil {
				t.Fatal("LinkPhone = nil error")
			}
			if phone, _, _ := f.storedPhone(t, u.ID); phone != "" {
				t.Fatalf("stored phone %q after a %s failure", phone, fail)
			}
		})
	}
}

func TestOnboard_refusesWhatItCannotFold(t *testing.T) {
	t.Parallel()
	ready := domain.User{Handle: "fold_one", AuthState: domain.AuthCreated}
	if _, err := domain.Onboard(ready, domain.PhoneUnlinked, domain.Links{}); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("unlink = %v, want internal", err)
	}
	broken := domain.User{Handle: "fold_two", AuthState: "BOGUS"}
	_, err := domain.Onboard(broken, domain.PhoneVerified, domain.Links{Phone: onboardNo})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("unknown state = %v, want internal", err)
	}
}

func TestHTTP_onboardingRefusesCallersThatAreNotAUser(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{}
	agent := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorAgent, ID: "a1"})
	_, phone := h.PostOnboardingPhone(agent, api.PostOnboardingPhoneRequestObject{})
	_, socials := h.PostOnboardingSocials(agent, api.PostOnboardingSocialsRequestObject{})
	_, skip := h.PostOnboardingSkip(agent, api.PostOnboardingSkipRequestObject{})
	for name, err := range map[string]error{"phone": phone, "socials": socials, "skip": skip} {
		if errs.CodeOf(err) != errs.CodeForbidden {
			t.Errorf("%s err = %v, want forbidden", name, err)
		}
	}
	user := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: testkit.NewIDs(9).NewV7().String()})
	_, err := h.PostOnboardingSkip(user, api.PostOnboardingSkipRequestObject{})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("PostOnboardingSkip with no body err = %v, want invalid_input", err)
	}
}

const onboardX = "x-onboard-1"

func xUser() *domain.XAccount { return &domain.XAccount{UserID: onboardX, Username: "onboard_on_x"} }

func (f httpFixture) wantState(t *testing.T, rec *httptest.ResponseRecorder, want api.AuthState) api.Me {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	me := decodeMe(t, rec)
	if me.AuthState != want {
		t.Fatalf("auth_state = %s, want %s", me.AuthState, want)
	}
	return me
}

func (f httpFixture) wantSteps(t *testing.T, user ids.UserID, want ...events.UserAuthStateChanged) {
	t.Helper()
	got := f.authSteps(t, user)
	if len(got) != len(want) {
		t.Fatalf("auth steps = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("auth steps = %+v, want %+v", got, want)
		}
	}
}

func onboardStep(from, to string) events.UserAuthStateChanged {
	return events.UserAuthStateChanged{From: from, To: to, Cause: "onboarding"}
}

func TestLinkSocials_storesThePrivyXAccountOnce(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.onboardingUser(t, "x_one", app.PrivyUser{Email: "x@example.com", X: xUser()})
	me := f.wantState(t, f.onboard(t, u.ID, "socials", `{"x_user_id":"spoof"}`, "x1"), api.AWAITINGPHONE)
	if me.XUsername == nil || *me.XUsername != "onboard_on_x" {
		t.Fatalf("x_username = %v, want onboard_on_x", me.XUsername)
	}
	f.wantState(t, f.onboard(t, u.ID, "socials", ``, "x2"), api.AWAITINGPHONE)
	f.wantSteps(t, u.ID, onboardStep("CREATED", "AWAITING_PHONE"))
	var stored string
	if err := f.pool.QueryRow(t.Context(), `SELECT x_user_id FROM users WHERE id = $1`, u.ID.UUID()).
		Scan(&stored); err != nil || stored != onboardX {
		t.Fatalf("x_user_id = %q, %v, want %s", stored, err, onboardX)
	}
	assertNoPII(t, f.logs.Bytes(), onboardX, "spoof")
}

func TestLinkSocials_refusesWhatItCannotLink(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	none := f.seed(t, portSeed{wallet: true})
	wantProblem(t, f.onboard(t, none.ID, "socials", ``, "x1"), apibase.HandleRequired)
	bare := f.onboardingUser(t, "x_bare", app.PrivyUser{Email: "bare@example.com"})
	wantProblem(t, f.onboard(t, bare.ID, "socials", ``, "x1"), apibase.XNotLinked)
	f.seed(t, portSeed{xUserID: onboardX})
	clash := f.onboardingUser(t, "x_clash", app.PrivyUser{Email: "clash@example.com", X: xUser()})
	wantProblem(t, f.onboard(t, clash.ID, "socials", ``, "x1"), apibase.XNotLinked)
	logs := f.logs.Bytes()
	if !bytes.Contains(logs, []byte("identity.x.conflict")) {
		t.Fatalf("logs lack identity.x.conflict:\n%s", logs)
	}
	assertNoPII(t, logs, onboardX)
	f.wantSteps(t, bare.ID)
	f.wantSteps(t, clash.ID)
}

func TestSkipOnboardingStep_skipsThePhoneOnceAndTheSocialsWithoutAMove(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.onboardingUser(t, "skip_one", app.PrivyUser{})
	f.wantState(t, f.onboard(t, u.ID, "skip", `{"step":"phone"}`, "s1"), api.AWAITINGPHONE)
	f.wantState(t, f.onboard(t, u.ID, "skip", `{"step":"phone"}`, "s2"), api.AWAITINGPHONE)
	f.wantState(t, f.onboard(t, u.ID, "skip", `{"step":"socials"}`, "s3"), api.AWAITINGPHONE)
	f.wantSteps(t, u.ID, onboardStep("CREATED", "AWAITING_PHONE"))
	wantProblem(t, f.onboard(t, u.ID, "skip", `{"step":"email"}`, "s4"), apibase.InvalidInput)
	none := f.seed(t, portSeed{wallet: true})
	wantProblem(t, f.onboard(t, none.ID, "skip", `{"step":"socials"}`, "s1"), apibase.HandleRequired)
	h := app.NewOnboarding(app.OnboardingDeps{Reads: f.pool, Users: adapters.Users{}})
	_, err := h.Skip(t.Context(), app.SkipOnboardingStep{UserID: u.ID, Step: "email"})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Skip(email) = %v, want invalid_input", err)
	}
}

func TestOnboarding_eachPathEndsWhereTheAuthStateMachineSays(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	both := app.PrivyUser{PhoneE164: onboardNo, X: xUser()}
	skipThenX := f.onboardingUser(t, "path_one", both)
	f.wantState(t, f.onboard(t, skipThenX.ID, "skip", `{"step":"phone"}`, "a1"), api.AWAITINGPHONE)
	f.wantState(t, f.onboard(t, skipThenX.ID, "socials", ``, "a2"), api.AWAITINGPHONE)
	f.wantSteps(t, skipThenX.ID, onboardStep("CREATED", "AWAITING_PHONE"))

	phoneThenSkip := f.onboardingUser(t, "path_two", app.PrivyUser{PhoneE164: "+14155550143"})
	f.wantState(t, f.onboard(t, phoneThenSkip.ID, "phone", ``, "b1"), api.AWAITINGSOCIALS)
	me := f.wantState(t, f.onboard(t, phoneThenSkip.ID, "skip", `{"step":"socials"}`, "b2"), api.AWAITINGSOCIALS)
	if !me.PhoneLinked {
		t.Fatal("skipping socials cleared the phone")
	}
	f.wantSteps(t, phoneThenSkip.ID, onboardStep("CREATED", "AWAITING_SOCIALS"))

	phoneThenX := f.onboardingUser(t, "path_three", app.PrivyUser{PhoneE164: "+14155550144", X: &domain.XAccount{
		UserID: "x-onboard-3", Username: "three_on_x",
	}})
	f.wantState(t, f.onboard(t, phoneThenX.ID, "phone", ``, "c1"), api.AWAITINGSOCIALS)
	f.wantState(t, f.onboard(t, phoneThenX.ID, "socials", ``, "c2"), api.ONBOARDINGCOMPLETED)
	f.wantSteps(t, phoneThenX.ID,
		onboardStep("CREATED", "AWAITING_SOCIALS"), onboardStep("AWAITING_SOCIALS", "ONBOARDING_COMPLETED"))
}
