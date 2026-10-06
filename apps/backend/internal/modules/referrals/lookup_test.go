package referrals_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/referralsapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const kaiPhoto = "https://img.example/kai.png"

type answer struct {
	status                   int
	contentType, cache, body string
}

func answerOf(rec *httptest.ResponseRecorder) answer {
	return answer{rec.Code, rec.Header().Get("Content-Type"), rec.Header().Get("Cache-Control"), rec.Body.String()}
}

func (s server) lookup(t *testing.T, code string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/referrals/"+code, nil)
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

func TestReferralLookup_namesTheReferrerOfARandomCodeOrAnUnlockedHandle(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct {
		owner owner
		input string
		photo any
	}{
		"random code": {
			owner{handle: "kaicenat", code: "k7m4qx2p", name: "Kai", photo: kaiPhoto}, "k7m4qx2p", kaiPhoto,
		},
		"uppercase random code": {
			owner{handle: "kaicenat", code: "k7m4qx2p", name: "Kai", photo: kaiPhoto}, "K7M4QX2P", kaiPhoto,
		},
		"unlocked handle": {
			owner{handle: "kaicenat", unlocked: true, name: "Kai", photo: kaiPhoto}, "KaiCenat", kaiPhoto,
		},
		"referrer without a photo": {
			owner{handle: "kaicenat", code: "k7m4qx2p", name: "Kai"}, "k7m4qx2p", nil,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := newServer(t)
			id := seedOwner(t, s.pool, tt.owner)
			got := answerOf(s.lookup(t, tt.input))
			var body map[string]any
			if err := json.Unmarshal([]byte(got.body), &body); err != nil {
				t.Fatalf("GET %s body %q: %v", tt.input, got.body, err)
			}
			want := map[string]any{"referrer": map[string]any{
				"user_id": id.String(), "display_name": "Kai", "photo_url": tt.photo, "handle": "kaicenat",
			}}
			if got.status != http.StatusOK || got.cache != "public, max-age=300" || !reflect.DeepEqual(body, want) {
				t.Fatalf("GET %s = %+v, want 200 %v with Cache-Control public, max-age=300", tt.input, got, want)
			}
		})
	}
}

func TestReferralLookup_UnknownAndLockedLookIdentical(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedOwner(t, s.pool, owner{handle: "lockedkai", name: "Kai", photo: kaiPhoto})
	seedOwner(t, s.pool, owner{handle: "bannedkai", unlocked: true, status: "banned", code: "bbbbbbbb", name: "Kai"})
	seedOwner(t, s.pool, owner{handle: "pausedkai", unlocked: true, status: "suspended", code: "cccccccc", name: "Kai"})
	seedOwner(t, s.pool, owner{handle: "deletedkai", unlocked: true, status: "deleted", code: "dddddddd", name: "Kai"})
	miss := answerOf(s.lookup(t, "k7m4qx2q"))
	if want := (answer{http.StatusOK, "application/json", "public, max-age=300", "{\"referrer\":null}\n"}); miss != want {
		t.Fatalf("GET an unknown code = %+v, want %+v", miss, want)
	}
	for name, code := range map[string]string{
		"unknown handle":      "nobodykai",
		"locked handle":       "lockedkai",
		"banned handle":       "bannedkai",
		"banned random code":  "bbbbbbbb",
		"suspended handle":    "pausedkai",
		"suspended code":      "cccccccc",
		"deleted handle":      "deletedkai",
		"deleted random code": "dddddddd",
		"not a code":          "kai%20cenat!",
	} {
		if got := answerOf(s.lookup(t, code)); got != miss {
			t.Errorf("%s: GET %s = %+v, want what an unknown code gets: %+v", name, code, got, miss)
		}
	}
}

func TestReferralLookup_aFailedReadIsAnErrorNotACachedNull(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	if _, err := s.pool.Exec(t.Context(), `ALTER TABLE referral_codes RENAME TO referral_codes_gone`); err != nil {
		t.Fatal(err)
	}
	rec := s.lookup(t, "k7m4qx2p")
	if rec.Code != http.StatusInternalServerError || rec.Header().Get("Content-Type") != "application/problem+json" ||
		rec.Header().Get("Cache-Control") != "" {
		t.Fatalf("GET with the code table gone = %+v, want an uncached 500 problem", answerOf(rec))
	}
}

func TestPostMeReferral_returnsTheReferrersPhotoOrNull(t *testing.T) {
	t.Parallel()
	for name, photo := range map[string]string{"with a photo": kaiPhoto, "without a photo": ""} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pool := testkit.DB(t)
			clock := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
			generator := testkit.NewIDs(96)
			referrer, caller := mustUser(t, generator.NewV7().String()), mustUser(t, generator.NewV7().String())
			if _, err := pool.Exec(t.Context(),
				`INSERT INTO referral_codes (code, user_id, created_at) VALUES ('k7m4qx2p', $1, now())`,
				referrer.UUID(),
			); err != nil {
				t.Fatal(err)
			}
			users := fakes.NewIdentity([]identity.UserCard{
				{
					ID: referrer, AccountStatus: identity.AccountActive,
					DisplayName: "Kai", Handle: "kaicenat", PhotoURL: photo,
				},
				{ID: caller, AccountStatus: identity.AccountActive, CreatedAt: clock.Now()},
			}, nil)
			resolver := app.Resolver{Reads: pool, Users: users}
			h := adapters.HTTP{
				Codes: resolver,
				Attach: app.NewAttachReferralHandler(app.AttachReferralDeps{
					UoW: db.New(pool, generator, clock), Resolver: resolver, Users: users, IDs: generator, Clock: clock,
				}),
			}
			ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: caller.String()})
			got, err := h.PostMeReferral(ctx, api.PostMeReferralRequestObject{
				Body: &api.PostMeReferralJSONRequestBody{Code: "k7m4qx2p", Source: api.Manual},
			})
			created, ok := got.(api.PostMeReferral201JSONResponse)
			if err != nil || !ok {
				t.Fatalf("PostMeReferral = %T, %v, want 201", got, err)
			}
			if photo == "" && created.Referrer.PhotoUrl != nil ||
				photo != "" && (created.Referrer.PhotoUrl == nil || *created.Referrer.PhotoUrl != photo) {
				t.Fatalf("PostMeReferral photo_url = %v, want %q or null", created.Referrer.PhotoUrl, photo)
			}
		})
	}
}
