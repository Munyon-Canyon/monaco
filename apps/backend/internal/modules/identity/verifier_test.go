package identity_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/authn"
	privyadapter "github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const (
	devKey   = "dev-fixture-key"
	es256JWT = `{"alg":"ES256","typ":"JWT"}`
)

type verifierFixture struct {
	pool   *pgxpool.Pool
	tracer *db.CountingTracer
	clock  *testkit.Clock
}

func newVerifierFixture(t *testing.T) verifierFixture {
	t.Helper()
	return verifierFixture{pool: testkit.DB(t), clock: testkit.NewClock(clock.Real{}.Now())}.traced(t)
}

func (f verifierFixture) traced(t *testing.T) verifierFixture {
	t.Helper()
	tracer := &db.CountingTracer{}
	cfg := f.pool.Config()
	cfg.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return verifierFixture{pool: pool, tracer: tracer, clock: f.clock}
}

func verifierConfig(env config.Env) config.Config {
	cfg := privyConfig()
	cfg.Env, cfg.Auth.DevTokenKey = env, devKey
	return cfg
}

func (f verifierFixture) verifier(t *testing.T, env config.Env) *authn.Verifier {
	t.Helper()
	cfg := verifierConfig(env)
	v, err := authn.New(cfg, f.clock, privyadapter.Users{Client: privy.New(cfg, f.clock)}, f.pool)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (f verifierFixture) privyToken(sub string) string {
	return fakes.PrivyAccessToken(privyAppID, sub, f.clock.Now(), time.Hour)
}

func (f verifierFixture) devToken(t *testing.T) string {
	t.Helper()
	dev, err := auth.NewDevVerifier(verifierConfig(config.EnvLocal), f.clock)
	if err != nil {
		t.Fatal(err)
	}
	return dev.Mint("u-dev", f.clock.Now().Add(time.Hour))
}

func (f verifierFixture) signed(t *testing.T, key *ecdsa.PrivateKey, header, sub string) string {
	t.Helper()
	claims, err := json.Marshal(map[string]any{
		"iss": "privy.io", "aud": privyAppID, "sub": sub, "exp": f.clock.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return fakes.SignES256(key, header, string(claims))
}

func unsigned(header, sub string) string {
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(header)) + "." + enc.EncodeToString([]byte(`{"sub":"`+sub+`"}`)) + "."
}

type verdict struct {
	actor   auth.Actor
	code    errs.Code
	queries int64
}

func (f verifierFixture) verify(t *testing.T, v *authn.Verifier, token string) verdict {
	t.Helper()
	f.tracer.Reset()
	actor, err := v.Verify(t.Context(), token)
	got := verdict{actor: actor, queries: f.tracer.Queries()}
	if err != nil {
		got.code = errs.CodeOf(err)
	}
	return got
}

func TestVerifier_routesEachTokenByAlgorithmAndLooksTheUserUpOnce(t *testing.T) {
	t.Parallel()
	f := newVerifierFixture(t)
	user := testkit.SeedUser(t, f.pool, testkit.UserOpts{})
	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyedWithPublicKey, err := auth.NewDevVerifier(
		config.Config{Env: config.EnvLocal, Auth: config.Auth{DevTokenKey: fakes.PrivyVerificationKey()}}, f.clock)
	if err != nil {
		t.Fatal(err)
	}
	known := auth.Actor{Kind: auth.ActorUser, ID: user.ID.String(), Standing: auth.StandingActive}
	signedIn := verdict{actor: known, queries: 1}
	devUser := verdict{actor: auth.Actor{Kind: auth.ActorUser, ID: "u-dev", Standing: auth.StandingActive}}
	noSession := verdict{code: errs.CodeSessionRequired, queries: 1}
	refused := verdict{code: errs.CodeUnauthorized}
	sub := user.PrivyUserID
	dev, prod := config.EnvLocal, config.EnvProduction
	for name, tc := range map[string]struct {
		env   config.Env
		token string
		want  verdict
	}{
		"privy token for a known user": {dev, f.privyToken(sub), signedIn},
		"privy token in production":    {prod, f.privyToken(sub), signedIn},
		"unknown sub":                  {dev, f.privyToken("did:privy:unknown"), noSession},
		"expired": {
			dev, fakes.PrivyAccessToken(privyAppID, sub, f.clock.Now().Add(-2*time.Hour), time.Hour), refused,
		},
		"signed by another key": {dev, f.signed(t, other, es256JWT, sub), refused},
		"wrong alg": {
			dev, f.signed(t, fakes.PrivyTokenKey(), `{"alg":"ES384","typ":"JWT"}`, sub), refused,
		},
		"alg none":                {dev, unsigned(`{"alg":"none","typ":"JWT"}`, sub), refused},
		"not a jwt":               {dev, "opaque-token", refused},
		"dev token":               {dev, f.devToken(t), devUser},
		"dev token in production": {prod, f.devToken(t), refused},
		"HS256 keyed with the privy public key": {
			dev, keyedWithPublicKey.Mint(sub, f.clock.Now().Add(time.Hour)), refused,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			own := f.traced(t)
			got := own.verify(t, own.verifier(t, tc.env), tc.token)
			t.Logf("%s: %+v", tc.env, got)
			if got != tc.want {
				t.Fatalf("Verify = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestVerifier_actorCarriesTheAccountStatusAsItsStanding(t *testing.T) {
	t.Parallel()
	f := newVerifierFixture(t)
	v := f.verifier(t, config.EnvTest)
	for status, want := range map[string]auth.Standing{
		"active": auth.StandingActive, "suspended": auth.StandingSuspended,
		"banned": auth.StandingBanned, "deleted": auth.StandingDeleted,
	} {
		user := testkit.SeedUser(t, f.pool, testkit.UserOpts{AccountStatus: status})
		got := f.verify(t, v, f.privyToken(user.PrivyUserID))
		if got != (verdict{actor: auth.Actor{Kind: auth.ActorUser, ID: user.ID.String(), Standing: want}, queries: 1}) {
			t.Fatalf("%s user: Verify = %+v, want standing %s", status, got, want)
		}
	}
}

func TestVerifier_aDatabaseFailureIsUnavailableNotUnauthorized(t *testing.T) {
	t.Parallel()
	f := newVerifierFixture(t)
	v := f.verifier(t, config.EnvTest)
	f.pool.Close()
	if got := f.verify(t, v, f.privyToken("did:privy:anyone")); got.code != errs.CodeDBUnavailable {
		t.Fatalf("Verify on a closed pool = %+v, want db_unavailable", got)
	}
}

func TestVerifier_verifyPrivyNamesTheSubjectWithoutAUserRow(t *testing.T) {
	t.Parallel()
	f := newVerifierFixture(t)
	v := f.verifier(t, config.EnvTest)
	id, err := v.VerifyPrivy(t.Context(), f.privyToken("did:privy:first-sign-in"))
	if err != nil || id != "did:privy:first-sign-in" || f.tracer.Queries() != 0 {
		t.Fatalf("VerifyPrivy = %q, %v after %d queries", id, err, f.tracer.Queries())
	}
	if _, err := v.VerifyPrivy(t.Context(), f.devToken(t)); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("VerifyPrivy(dev token) = %v, want unauthorized", err)
	}
}

func TestNewVerifier_needsADevTokenKeyOutsideProductionOnly(t *testing.T) {
	t.Parallel()
	f := newVerifierFixture(t)
	for env, want := range map[config.Env]errs.Code{
		config.EnvLocal: errs.CodeInvalidInput, config.EnvStaging: errs.CodeInvalidInput, config.EnvProduction: "",
	} {
		cfg := verifierConfig(env)
		cfg.Auth.DevTokenKey = ""
		v, err := identity.NewVerifier(module.Deps{Config: cfg, Clock: f.clock, Pool: f.pool})
		if want == "" && (err != nil || v == nil) || want != "" && (v != nil || errs.CodeOf(err) != want) {
			t.Fatalf("%s: NewVerifier = %v, %v, want %q", env, v, err, want)
		}
	}
}

type actorProbe struct {
	mu   sync.Mutex
	seen []auth.Actor
}

func (p *actorProbe) PostSystemPing(context.Context, api.PostSystemPingRequestObject) (
	api.PostSystemPingResponseObject, error,
) {
	return nil, errs.New(errs.CodeNotFound, "actorProbe.PostSystemPing")
}

func (p *actorProbe) GetSystemPing(ctx context.Context, _ api.GetSystemPingRequestObject) (
	api.GetSystemPingResponseObject, error,
) {
	a, _ := auth.ActorFrom(ctx)
	p.mu.Lock()
	p.seen = append(p.seen, a)
	p.mu.Unlock()
	return nil, errs.New(errs.CodeNotFound, "actorProbe.GetSystemPing")
}

func TestVerifier_behindTheAuthMiddlewarePutsTheStandingInContextAndLogsNoToken(t *testing.T) {
	t.Parallel()
	f := newVerifierFixture(t)
	banned := testkit.SeedUser(t, f.pool, testkit.UserOpts{AccountStatus: "banned"})
	logs := &testkit.Logs{}
	logger := observability.NewLogger(config.Config{Env: config.EnvTest}, logs)
	probe := &actorProbe{}
	h, err := httpx.Handler(httpx.Deps{
		Logger:       logger,
		Tracer:       tracenoop.NewTracerProvider(),
		Clock:        f.clock,
		IDs:          testkit.NewIDs(1),
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(f.pool, f.clock),
		Verifier:     f.verifier(t, config.EnvTest),
	}, httpx.Routes{SystemRoutes: probe}, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	expired := fakes.PrivyAccessToken(privyAppID, banned.PrivyUserID, f.clock.Now().Add(-2*time.Hour), time.Hour)
	requests := []struct {
		token  string
		status int
	}{
		{f.privyToken(banned.PrivyUserID), http.StatusNotFound},
		{f.devToken(t), http.StatusNotFound},
		{f.privyToken("did:privy:unknown"), http.StatusUnauthorized},
		{unsigned(`{"alg":"none"}`, banned.PrivyUserID), http.StatusUnauthorized},
		{expired, http.StatusUnauthorized},
	}
	for _, r := range requests {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"/v1/system/pings/"+testkit.NewIDs(2).NewV7().String(), nil)
		req.Header.Set("Authorization", "Bearer "+r.token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != r.status {
			t.Fatalf("status = %d, want %d: %s", rec.Code, r.status, rec.Body.String())
		}
	}
	want := []auth.Actor{
		{Kind: auth.ActorUser, ID: banned.ID.String(), Standing: auth.StandingBanned},
		{Kind: auth.ActorUser, ID: "u-dev", Standing: auth.StandingActive},
	}
	if !reflect.DeepEqual(probe.seen, want) {
		t.Fatalf("handler saw actors %+v, want %+v", probe.seen, want)
	}
	tokens := make([]string, len(requests))
	for i, r := range requests {
		tokens[i] = r.token
	}
	expectNoCredentials(t, string(logs.Bytes()), tokens)
}

func expectNoCredentials(t *testing.T, raw string, tokens []string) {
	t.Helper()
	if strings.Count(raw, "\n") < len(tokens) {
		t.Fatalf("want a log line per request, got %q", raw)
	}
	secrets := []string{"Bearer ", "Authorization"}
	for _, token := range tokens {
		secrets = append(append(secrets, token), strings.Split(token, ".")[1:]...)
	}
	for _, secret := range secrets {
		if secret != "" && strings.Contains(raw, secret) {
			t.Fatalf("logs carry %q:\n%s", secret, raw)
		}
	}
}
