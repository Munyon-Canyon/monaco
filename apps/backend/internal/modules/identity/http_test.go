package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/authn"
	privyadapter "github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/privy"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/ratelimit"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

type httpFixture struct {
	portFixture
	handler  http.Handler
	verifier *auth.DevVerifier
	privy    *privyfake.Users
	wallets  *privyfake.Wallets
	photos   *photoStore
}

type photoStore struct {
	url                      string
	err                      error
	puts                     int
	bucket, key, contentType string
	body                     []byte
}

func (s *photoStore) Put(_ context.Context, key, contentType string, body []byte) (string, error) {
	s.puts++
	s.bucket, s.key, s.contentType = "avatars", key, contentType
	s.body = append([]byte(nil), body...)
	return s.url, s.err
}

func (*photoStore) DeleteAll(context.Context, ids.UserID) error { return nil }

func newHTTPFixture(t *testing.T) httpFixture {
	t.Helper()
	f := newPortFixture(t)
	clk := testkit.NewClock(f.now)
	verifier, err := auth.NewDevVerifier(
		config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: devKey}}, clk)
	if err != nil {
		t.Fatal(err)
	}
	fakeUsers, fakeWallets := &privyfake.Users{}, &privyfake.Wallets{}
	photos := &photoStore{url: "https://img.example/photo.png"}
	limit, err := ratelimit.Load(openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	limiter, err := ratelimit.New(f.pool, clk, noop.NewMeterProvider())
	if err != nil {
		t.Fatal(err)
	}
	var routes httpx.Routes
	identity.New(
		module.Deps{Pool: f.pool, UoW: db.New(f.pool, f.ids, clk), IDs: f.ids, Clock: clk},
		identity.WithPrivy(fakeUsers, fakeWallets),
		identity.WithPhotoStore(photos),
	).Routes(&routes)
	h, err := httpx.Handler(httpx.Deps{
		Logger:       observability.NewLogger(config.Config{Env: config.EnvTest}, io.Discard),
		Tracer:       tracenoop.NewTracerProvider(),
		Clock:        clk,
		IDs:          f.ids,
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(f.pool, clk),
		Verifier:     verifier,
		RateLimit:    ratelimit.Middleware(limiter, limit, httpx.ActorKey, false),
	}, routes, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return httpFixture{
		portFixture: f, handler: testkit.HTTP(t, h), verifier: verifier, privy: fakeUsers, wallets: fakeWallets,
		photos: photos,
	}
}

func (f httpFixture) uploadPhoto(t *testing.T, user ids.UserID, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	return f.uploadPhotoKey(t, user, body, "photo-1")
}

func (f httpFixture) uploadPhotoKey(
	t *testing.T, user ids.UserID, body []byte, key string,
) *httptest.ResponseRecorder {
	t.Helper()
	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	part, err := writer.CreateFormFile("photo", "photo.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/me/profile-photo", &form)
	req.Header.Set("Authorization", "Bearer "+f.verifier.Mint(user.String(), f.now.Add(time.Hour)))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func (f httpFixture) uploadPhotoPart(
	t *testing.T, user ids.UserID, body []byte, filename, contentType, key string,
) *httptest.ResponseRecorder {
	t.Helper()
	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="photo"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/me/profile-photo", &form)
	req.Header.Set("Authorization", "Bearer "+f.verifier.Mint(user.String(), f.now.Add(time.Hour)))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func (f httpFixture) updateProfile(t *testing.T, user ids.UserID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPatch, "/v1/me", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.verifier.Mint(user.String(), f.now.Add(time.Hour)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "profile-1")
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func (f httpFixture) getMe(t *testing.T, user ids.UserID) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/me", nil)
	if user != (ids.UserID{}) {
		req.Header.Set("Authorization", "Bearer "+f.verifier.Mint(user.String(), f.now.Add(time.Hour)))
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func (f httpFixture) openSession(t *testing.T, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/auth/session", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func decodeMe(t *testing.T, rec *httptest.ResponseRecorder) api.Me {
	t.Helper()
	var me api.Me
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return me
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) api.Problem {
	t.Helper()
	var p api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return p
}

func TestGetMe_overHTTPServesTheAccount(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.seed(t, portSeed{
		handle: "kaicenat", name: "Kai Cenat", photo: "https://img.example/kai.png", authState: "AWAITING_SOCIALS",
		phoneHash: portHash("kai"), phoneVerified: true, wallet: true,
	})
	changed := f.now.Add(-48 * time.Hour)
	_, err := f.pool.Exec(t.Context(),
		`UPDATE users SET x_username = 'kai_on_x', handle_changed_at = $2 WHERE id = $1`, u.ID.UUID(), changed)
	if err != nil {
		t.Fatal(err)
	}
	rec := f.getMe(t, u.ID)
	handle, photo, x := "kaicenat", "https://img.example/kai.png", "kai_on_x"
	changeable := changed.Add(domain.HandleChangeInterval)
	want := api.Me{
		Id: u.ID.UUID(), Handle: &handle, DisplayName: "Kai Cenat", PhotoUrl: &photo,
		AuthState: api.AuthState(domain.AuthAwaitingSocials), AccountStatus: api.AccountStatus(domain.AccountActive),
		MemberWalletAddress: string(u.Address), PhoneLinked: true, XUsername: &x, HandleChangeableAt: &changeable,
		CreatedAt: f.created(),
	}
	got := decodeMe(t, rec)
	if rec.Code != http.StatusOK || !reflect.DeepEqual(normalized(got), normalized(want)) {
		t.Fatalf("GET /v1/me = %d %s, want %+v", rec.Code, rec.Body, want)
	}
}

func TestFlow23a_UploadProfilePhoto_OK(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.seed(t, portSeed{handle: "kai", name: "Kai", wallet: true})
	rec := f.uploadPhoto(t, u.ID, []byte("\x89PNG\r\n\x1a\nphoto"))
	if rec.Code != http.StatusOK {
		t.Fatalf("upload = %d %s", rec.Code, rec.Body)
	}
	if got := decodeMe(t, rec).PhotoUrl; got == nil || *got != "https://img.example/photo.png" {
		t.Fatalf("photo_url = %v", got)
	}
	if got := profileEvents(t, f); len(got) != 1 || got[0].PhotoURL != "https://img.example/photo.png" ||
		!reflect.DeepEqual(got[0].Fields, []string{"photo"}) {
		t.Fatalf("profile events = %+v", got)
	}
}

func TestFlow23a_UploadProfilePhoto_usesSniffedPNGMetadata(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.seed(t, portSeed{handle: "kai", name: "Kai", wallet: true})
	if rec := f.uploadPhotoPart(
		t, u.ID, []byte("\x89PNG\r\n\x1a\nphoto"), "avatar.jpg", "image/jpeg", "photo-png",
	); rec.Code != http.StatusOK {
		t.Fatalf("upload = %d %s", rec.Code, rec.Body)
	}
	if f.photos.bucket != "avatars" || f.photos.contentType != "image/png" ||
		!strings.HasPrefix(f.photos.key, u.ID.String()+"/") || !strings.HasSuffix(f.photos.key, ".png") {
		t.Fatalf("Put = bucket %q key %q content type %q", f.photos.bucket, f.photos.key, f.photos.contentType)
	}
}

func TestFlow23_UpdateProfile_OK(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.seed(t, portSeed{handle: "kai", name: "Kai", wallet: true})
	rec := f.updateProfile(t, u.ID, `{"display_name":"  Kai   Q "}`)
	if rec.Code != http.StatusOK || decodeMe(t, rec).DisplayName != "Kai Q" {
		t.Fatalf("update = %d %s", rec.Code, rec.Body)
	}
}

func TestFlow23_UpdateProfile_DisplayNameInvalid(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.seed(t, portSeed{handle: "kai", name: "Kai", wallet: true})
	rec := f.updateProfile(t, u.ID, `{"display_name":"   "}`)
	if rec.Code != http.StatusBadRequest || decodeProblem(t, rec).Code != api.DisplayNameInvalid {
		t.Fatalf("update = %d %s", rec.Code, rec.Body)
	}
}

func TestFlow23a_UploadProfilePhoto_PhotoInvalid(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.seed(t, portSeed{handle: "kai", name: "Kai", wallet: true})
	rec := f.uploadPhoto(t, u.ID, []byte("GIF89a"))
	if rec.Code != http.StatusBadRequest || decodeProblem(t, rec).Code != api.PhotoInvalid {
		t.Fatalf("upload = %d %s", rec.Code, rec.Body)
	}
}

func TestProfileAdapters_rejectMissingActorAndBody(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{}
	if _, err := h.PatchMe(t.Context(), api.PatchMeRequestObject{}); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("PatchMe without actor = %v", err)
	}
	if _, err := h.PostProfilePhoto(
		t.Context(), api.PostProfilePhotoRequestObject{},
	); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("PostProfilePhoto without actor = %v", err)
	}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: testkit.NewIDs(7).NewV7().String()})
	if _, err := h.PatchMe(ctx, api.PatchMeRequestObject{}); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("PatchMe without body = %v", err)
	}
	if _, err := h.PostProfilePhoto(
		ctx, api.PostProfilePhotoRequestObject{},
	); errs.CodeOf(err) != errs.CodePhotoInvalid {
		t.Fatalf("PostProfilePhoto without body = %v", err)
	}
}

func TestFlow23a_UploadProfilePhoto_acceptsEverySupportedFormat(t *testing.T) {
	t.Parallel()
	for name, body := range map[string][]byte{
		"jpeg": {0xff, 0xd8, 0xff, 0xe0, 0, 16, 'J', 'F', 'I', 'F', 0, 1},
		"png":  {0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'},
		"webp": {'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P', 'V', 'P', '8', ' '},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newHTTPFixture(t)
			u := f.seed(t, portSeed{handle: "kai", name: "Kai", wallet: true})
			rec := f.uploadPhoto(t, u.ID, body)
			if rec.Code != http.StatusOK {
				t.Fatalf("upload = %d %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestFlow23a_UploadProfilePhoto_StorageUnavailable(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	f.photos.err = errs.New(errs.CodeUpstreamUnavailable, "test.photoStore")
	u := f.seed(t, portSeed{handle: "kai", name: "Kai", photo: "https://img.example/old.png", wallet: true})
	rec := f.uploadPhoto(t, u.ID, []byte("\x89PNG\r\n\x1a\nphoto"))
	if rec.Code != http.StatusServiceUnavailable || decodeProblem(t, rec).Code != api.StorageUnavailable {
		t.Fatalf("upload = %d %s", rec.Code, rec.Body)
	}
	if got := decodeMe(t, f.getMe(t, u.ID)).PhotoUrl; got == nil || *got != "https://img.example/old.png" {
		t.Fatalf("photo_url after failed upload = %v", got)
	}
	if got := profileEvents(t, f); len(got) != 0 {
		t.Fatalf("profile events = %+v", got)
	}
}

func profileEvents(t *testing.T, f httpFixture) []events.UserProfileUpdated {
	t.Helper()
	rows, err := f.pool.Query(
		t.Context(), `SELECT payload FROM events WHERE type = $1 ORDER BY id`, string(events.TypeUserProfileUpdated),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []events.UserProfileUpdated
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			t.Fatal(err)
		}
		decoded, err := events.Decode(events.TypeUserProfileUpdated, 1, payload)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, decoded.(events.UserProfileUpdated))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestUploadProfilePhotoHandler_rejectsInvalidInputAndStorageFailure(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	id := f.newID(t)
	if _, err := (app.UploadProfilePhotoHandler{}).Handle(
		t.Context(), id, "image/png", "", []byte("x"),
	); errs.CodeOf(err) != errs.CodePhotoInvalid {
		t.Fatalf("invalid upload = %v", err)
	}
	h := app.UploadProfilePhotoHandler{
		IDs: f.ids, Store: &photoStore{err: errs.New(errs.CodeUpstreamUnavailable, "test")},
	}
	_, err := h.Handle(t.Context(), id, "image/png", "png", []byte("x"))
	if errs.CodeOf(err) != errs.CodeStorageUnavailable {
		t.Fatalf("failed upload = %v", err)
	}
}

func TestProfileUpdates_surfaceDatabaseFailures(t *testing.T) {
	t.Parallel()
	for name, request := range map[string]func(httpFixture, ids.UserID) *httptest.ResponseRecorder{
		"display name": func(f httpFixture, id ids.UserID) *httptest.ResponseRecorder {
			return f.updateProfile(t, id, `{"display_name":"Kai Q"}`)
		},
		"photo": func(f httpFixture, id ids.UserID) *httptest.ResponseRecorder {
			return f.uploadPhoto(t, id, []byte("\x89PNG\r\n\x1a\nphoto"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newHTTPFixture(t)
			u := f.seed(t, portSeed{handle: "kai", name: "Kai", wallet: true})
			failProfileWrite(t, f, "users")
			if rec := request(f, u.ID); rec.Code != http.StatusInternalServerError {
				t.Fatalf("update = %d %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestProfileUpdates_surfaceEventFailures(t *testing.T) {
	t.Parallel()
	for name, request := range map[string]func(httpFixture, ids.UserID) *httptest.ResponseRecorder{
		"display name": func(f httpFixture, id ids.UserID) *httptest.ResponseRecorder {
			return f.updateProfile(t, id, `{"display_name":"Kai Q"}`)
		},
		"photo": func(f httpFixture, id ids.UserID) *httptest.ResponseRecorder {
			return f.uploadPhoto(t, id, []byte("\x89PNG\r\n\x1a\nphoto"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newHTTPFixture(t)
			u := f.seed(t, portSeed{handle: "kai", name: "Kai", wallet: true})
			failProfileWrite(t, f, "events")
			if rec := request(f, u.ID); rec.Code != http.StatusInternalServerError {
				t.Fatalf("update = %d %s", rec.Code, rec.Body)
			}
		})
	}
}

func failProfileWrite(t *testing.T, f httpFixture, table string) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), `
CREATE FUNCTION fail_profile_write() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'profile write failure'; END $$;
CREATE TRIGGER fail_profile_write BEFORE INSERT OR UPDATE ON `+table+`
FOR EACH ROW EXECUTE FUNCTION fail_profile_write()`); err != nil {
		t.Fatal(err)
	}
}

func TestFlow23a_UploadProfilePhoto_RateLimited(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.seed(t, portSeed{handle: "kai", name: "Kai", wallet: true})
	for i := range 3 {
		key := "photo-rate-" + strconv.Itoa(i)
		if rec := f.uploadPhotoKey(
			t, u.ID, []byte("\x89PNG\r\n\x1a\nphoto"), key,
		); rec.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d = %d", i, rec.Code)
		}
	}
	rec := f.uploadPhotoKey(t, u.ID, []byte("\x89PNG\r\n\x1a\nphoto"), "photo-rate-4")
	if rec.Code != http.StatusTooManyRequests || decodeProblem(t, rec).Code != api.RateLimited {
		t.Fatalf("fourth request = %d %s", rec.Code, rec.Body)
	}
}

func TestFlow23a_UploadProfilePhoto_usesTheOperationBodyLimit(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.seed(t, portSeed{handle: "kai", name: "Kai", wallet: true})
	valid := make([]byte, 3<<20/2)
	copy(valid, []byte("\x89PNG\r\n\x1a\n"))
	if rec := f.uploadPhotoKey(t, u.ID, valid, "photo-large-ok"); rec.Code != http.StatusOK {
		t.Fatalf("1.5 MiB upload = %d %s", rec.Code, rec.Body)
	}
	tooLarge := make([]byte, 2<<20+1)
	copy(tooLarge, []byte("\x89PNG\r\n\x1a\n"))
	before := f.photos.puts
	rec := f.uploadPhotoKey(t, u.ID, tooLarge, "photo-large-reject")
	if rec.Code < http.StatusBadRequest || rec.Code >= http.StatusInternalServerError {
		t.Fatalf("over-limit upload = %d %s", rec.Code, rec.Body)
	}
	if f.photos.puts != before {
		t.Fatalf("Put calls = %d, want %d", f.photos.puts, before)
	}
}

func normalized(me api.Me) api.Me {
	me.CreatedAt = me.CreatedAt.UTC()
	if me.HandleChangeableAt != nil {
		at := me.HandleChangeableAt.UTC()
		me.HandleChangeableAt = &at
	}
	return me
}

func TestGetMe_overHTTPOmitsWhatIsUnset(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	bare := f.seed(t, portSeed{wallet: true})
	rec := f.getMe(t, bare.ID)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &fields); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/me for a new user = %d %s, %v", rec.Code, rec.Body, err)
	}
	for _, unset := range []string{"handle", "photo_url", "x_username", "handle_changeable_at", "email", "phone"} {
		if _, ok := fields[unset]; ok {
			t.Fatalf("GET /v1/me for a new user carries %q: %s", unset, rec.Body)
		}
	}
	if string(fields["display_name"]) != `""` || string(fields["phone_linked"]) != "false" ||
		string(fields["auth_state"]) != `"CREATED"` {
		t.Fatalf("GET /v1/me for a new user = %s, want an empty display name, no phone and CREATED", rec.Body)
	}
}

func TestGetMe_overHTTPRefusesWhoIsNotASignedInUser(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	for name, tc := range map[string]struct {
		user   ids.UserID
		status int
		code   api.ErrorCode
	}{
		"no token":                 {ids.UserID{}, http.StatusUnauthorized, api.Unauthorized},
		"token without an account": {f.newID(t), http.StatusNotFound, api.UserNotFound},
	} {
		rec := f.getMe(t, tc.user)
		if got := decodeProblem(t, rec); rec.Code != tc.status || got.Code != tc.code {
			t.Errorf("%s: GET /v1/me = %d %s, want %d %s", name, rec.Code, rec.Body, tc.status, tc.code)
		}
	}
}

func TestHTTP_getMeRefusesCallersThatAreNotAUser(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{}
	for name, tc := range map[string]struct {
		actor *auth.Actor
		want  errs.Code
	}{
		"no actor":    {nil, errs.CodeUnauthorized},
		"agent":       {&auth.Actor{Kind: auth.ActorAgent, ID: "a1"}, errs.CodeForbidden},
		"admin":       {&auth.Actor{Kind: auth.ActorAdmin, ID: "a2"}, errs.CodeForbidden},
		"bad user id": {&auth.Actor{Kind: auth.ActorUser, ID: "u1"}, errs.CodeUnauthorized},
	} {
		ctx := t.Context()
		if tc.actor != nil {
			ctx = auth.WithActor(ctx, *tc.actor)
		}
		if _, err := h.GetMe(ctx, api.GetMeRequestObject{}); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: GetMe err = %v, want %s", name, err, tc.want)
		}
	}
}

func TestPostAuthSession_opensASessionWithNeitherAnIdempotencyKeyNorAnActor(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	f.privy.Seed(app.PrivyUser{ID: alice, PhoneE164: "+14155550100", Email: "alice@example.com"})
	rec := f.openSession(t, "Bearer "+string(alice))
	me := decodeMe(t, rec)
	if rec.Code != http.StatusOK || me.AuthState != api.AuthState(domain.AuthCreated) ||
		me.AccountStatus != api.AccountStatus(domain.AccountActive) || me.PhoneLinked || me.MemberWalletAddress == "" {
		t.Fatalf("POST /v1/auth/session = %d %s, want a CREATED active account with its wallet", rec.Code, rec.Body)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("alice@example.com")) ||
		bytes.Contains(rec.Body.Bytes(), []byte("4155550100")) {
		t.Fatalf("the response carries the email or the phone: %s", rec.Body)
	}
}

func TestPostAuthSession_aSecondCallAndGetMeReturnTheSameAccount(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	f.privy.Seed(app.PrivyUser{ID: alice, PhoneE164: "+14155550100"})
	first := f.openSession(t, "Bearer "+string(alice))
	again := f.openSession(t, "bearer "+string(alice))
	if first.Code != http.StatusOK || again.Code != http.StatusOK || again.Body.String() != first.Body.String() {
		t.Fatalf("POST twice = %d %s then %d %s, want the same account", first.Code, first.Body, again.Code, again.Body)
	}
	user, err := ids.ParseUserID(decodeMe(t, first).Id.String())
	if err != nil {
		t.Fatal(err)
	}
	if read := f.getMe(t, user); read.Code != http.StatusOK || read.Body.String() != first.Body.String() {
		t.Fatalf("GET /v1/me = %d %s, want the account the session returned", read.Code, read.Body)
	}
}

func TestPostAuthSession_refusesWhatItCannotSignIn(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		setup         func(f httpFixture)
		authorization string
		status        int
		code          api.ErrorCode
	}{
		"no header":     {func(httpFixture) {}, "", http.StatusUnauthorized, api.Unauthorized},
		"not a bearer":  {func(httpFixture) {}, "Basic YWxpY2U6eA==", http.StatusUnauthorized, api.Unauthorized},
		"empty bearer":  {func(httpFixture) {}, "Bearer ", http.StatusUnauthorized, api.Unauthorized},
		"unknown token": {func(httpFixture) {}, "Bearer nobody", http.StatusUnauthorized, api.Unauthorized},
		"no login method": {func(f httpFixture) {
			f.privy.Seed(app.PrivyUser{ID: alice, X: &domain.XAccount{UserID: "1", Username: "a"}})
		}, "Bearer " + string(alice), http.StatusForbidden, api.LoginMethodNotAllowed},
		"privy down": {func(f httpFixture) {
			f.privy.Seed(app.PrivyUser{ID: alice, PhoneE164: "+14155550100"})
			f.privy.Fail("User", errs.New(errs.CodePrivyUnavailable, "test"))
		}, "Bearer " + string(alice), http.StatusServiceUnavailable, api.PrivyUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newHTTPFixture(t)
			tc.setup(f)
			rec := f.openSession(t, tc.authorization)
			if got := decodeProblem(t, rec); rec.Code != tc.status || got.Code != tc.code {
				t.Fatalf("POST /v1/auth/session = %d %s, want %d %s", rec.Code, rec.Body, tc.status, tc.code)
			}
		})
	}
}

type openPing struct{}

func (openPing) PostSystemPing(context.Context, api.PostSystemPingRequestObject) (
	api.PostSystemPingResponseObject, error,
) {
	return api.PostSystemPing201JSONResponse{}, nil
}

func (openPing) GetSystemPing(context.Context, api.GetSystemPingRequestObject) (
	api.GetSystemPingResponseObject, error,
) {
	return api.GetSystemPing200JSONResponse{}, nil
}

func TestAccountStanding_changesTheNextResponseWithoutARestart(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	clk := testkit.NewClock(f.now)
	cfg := privyConfig()
	cfg.Env, cfg.Auth.DevTokenKey = config.EnvTest, devKey
	verifier, err := authn.New(cfg, clk, privyadapter.Users{Client: privy.New(cfg, clk)}, f.pool)
	if err != nil {
		t.Fatal(err)
	}
	var routes httpx.Routes
	identity.New(module.Deps{
		Config: cfg, Pool: f.pool, UoW: db.New(f.pool, f.ids, clk), IDs: f.ids, Clock: clk,
	}, identity.WithPrivy(&privyfake.Users{}, &privyfake.Wallets{})).Routes(&routes)
	routes.SystemRoutes = openPing{}
	handler, err := httpx.Handler(httpx.Deps{
		Logger: observability.NewLogger(config.Config{Env: config.EnvTest}, io.Discard),
		Tracer: tracenoop.NewTracerProvider(), Clock: clk, IDs: f.ids, MaxBodyBytes: 1 << 20,
		Idempotency: db.NewIdempotencyStore(f.pool, clk), Verifier: verifier,
	}, routes, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	user := testkit.SeedUser(t, f.pool, testkit.UserOpts{AccountStatus: "banned", WithWallet: true})
	token := fakes.PrivyAccessToken(privyAppID, user.PrivyUserID, clk.Now(), time.Hour)
	ping := "/v1/system/pings/" + testkit.NewIDs(9).NewV7().String()
	for _, step := range []standingStep{
		{method: http.MethodGet, path: "/v1/me", status: http.StatusOK},
		{method: http.MethodPost, path: "/v1/system/pings", body: `{"note":"out"}`, status: http.StatusForbidden, code: api.AccountBanned},
		{next: "suspended", method: http.MethodGet, path: ping, status: http.StatusOK},
		{method: http.MethodPost, path: "/v1/system/pings", body: `{"note":"out"}`, status: http.StatusForbidden, code: api.AccountSuspended},
		{next: "deleted", method: http.MethodGet, path: "/v1/me", status: http.StatusForbidden, code: api.AccountDeleted},
		{next: "active", method: http.MethodGet, path: ping, status: http.StatusOK},
	} {
		if step.next != "" {
			setAccountStatus(t, f, user.ID, step.next)
		}
		rec := callStanding(t, handler, token, step)
		got := api.ErrorCode("")
		if step.code != "" {
			got = decodeProblem(t, rec).Code
		}
		if rec.Code != step.status || got != step.code {
			t.Fatalf("%s %s after %q = %d %s, want %d %s",
				step.method, step.path, step.next, rec.Code, rec.Body, step.status, step.code)
		}
	}
}

type standingStep struct {
	next         string
	method, path string
	body         string
	status       int
	code         api.ErrorCode
}

func callStanding(t *testing.T, handler http.Handler, token string, step standingStep) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), step.method, step.path, strings.NewReader(step.body))
	req.Header.Set("Authorization", "Bearer "+token)
	if step.method == http.MethodPost {
		req.Header.Set("Idempotency-Key", "standing")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func setAccountStatus(t *testing.T, f portFixture, id ids.UserID, status string) {
	t.Helper()
	var deleted *time.Time
	if status == "deleted" {
		at := f.now
		deleted = &at
	}
	if _, err := f.pool.Exec(t.Context(),
		`UPDATE users SET account_status = $2, deleted_at = $3 WHERE id = $1`, id.UUID(), status, deleted); err != nil {
		t.Fatal(err)
	}
}
