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

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/authn"
	privyadapter "github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/identityapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/systemapi"
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
	v, err := authn.New(cfg, f.clock, privyadapter.Users{Client: privyClient(t, cfg, f.clock)}, f.pool)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (f verifierFixture) privyToken(sub string) string {
	return fakes.PrivyAccessToken(privyAppID, sub, f.clock.Now(), time.Hour)
}

func (f verifierFixture) privyActor(u testkit.SeededUser, standing auth.Standing) auth.Actor {
	return auth.Actor{
		Kind: auth.ActorUser, ID: u.ID.String(), Standing: standing,
		SessionID: "session-" + u.PrivyUserID, IssuedAt: f.clock.Now().Unix(),
	}
}

func (f verifierFixture) devToken(t *testing.T, sub string) string {
	t.Helper()
	dev, err := auth.NewDevVerifier(verifierConfig(config.EnvLocal), f.clock)
	if err != nil {
		t.Fatal(err)
	}
	return dev.Mint(sub, f.clock.Now().Add(time.Hour))
}

func (f verifierFixture) signed(t *testing.T, key *ecdsa.PrivateKey, header, sub string) string {
	t.Helper()
	claims, err := json.Marshal(map[string]any{
		"iss": "privy.io", "aud": privyAppID, "sub": sub, "sid": "session-" + sub,
		"iat": f.clock.Now().Unix(), "exp": f.clock.Now().Add(time.Hour).Unix(),
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
	devSession := func(a auth.Actor) auth.Actor {
		a.SessionID, a.IssuedAt = "dev-"+a.ID, f.clock.Now().Unix()
		return a
	}
	known := auth.Actor{Kind: auth.ActorUser, ID: user.ID.String(), Standing: auth.StandingActive}
	signedIn := verdict{actor: devSession(known), queries: 1}
	privySignedIn := verdict{actor: f.privyActor(user, auth.StandingActive), queries: 1}
	strangerID := testkit.NewIDs(1).NewV7().String()
	stranger := verdict{actor: devSession(auth.Actor{
		Kind: auth.ActorUser, ID: strangerID, Standing: auth.StandingActive,
	})}
	suspended := testkit.SeedUser(t, f.pool, testkit.UserOpts{AccountStatus: "suspended"})
	banned := testkit.SeedUser(t, f.pool, testkit.UserOpts{AccountStatus: "banned"})
	deleted := testkit.SeedUser(t, f.pool, testkit.UserOpts{AccountStatus: "deleted"})
	standing := func(u testkit.SeededUser, s auth.Standing) verdict {
		return verdict{actor: devSession(auth.Actor{Kind: auth.ActorUser, ID: u.ID.String(), Standing: s}), queries: 1}
	}
	marked := func(v verdict) verdict {
		v.actor.DevToken, v.actor.SessionID, v.actor.IssuedAt = true, "", 0
		return v
	}
	noSession := verdict{code: errs.CodeSessionRequired, queries: 1}
	refused := verdict{code: errs.CodeUnauthorized}
	sub := user.PrivyUserID
	dev, prod, staging, test := config.EnvLocal, config.EnvProduction, config.EnvStaging, config.EnvTest
	for name, tc := range map[string]struct {
		env   config.Env
		token string
		want  verdict
	}{
		"privy token for a known user": {dev, f.privyToken(sub), privySignedIn},
		"privy token in production":    {prod, f.privyToken(sub), privySignedIn},
		"unknown sub":                  {dev, f.privyToken("did:privy:unknown"), noSession},
		"expired": {
			dev, fakes.PrivyAccessToken(privyAppID, sub, f.clock.Now().Add(-2*time.Hour), time.Hour), refused,
		},
		"signed by another key": {dev, f.signed(t, other, es256JWT, sub), refused},
		"wrong alg": {
			dev, f.signed(t, fakes.PrivyTokenKey(), `{"alg":"ES384","typ":"JWT"}`, sub), refused,
		},
		"alg none":                                    {dev, unsigned(`{"alg":"none","typ":"JWT"}`, sub), refused},
		"not a jwt":                                   {dev, "opaque-token", refused},
		"dev token for a known user":                  {dev, f.devToken(t, user.ID.String()), signedIn},
		"dev token for a known user in test":          {test, f.devToken(t, user.ID.String()), signedIn},
		"dev token for a known user in staging":       {staging, f.devToken(t, user.ID.String()), marked(signedIn)},
		"dev token for a suspended user":              {dev, f.devToken(t, suspended.ID.String()), standing(suspended, auth.StandingSuspended)},
		"dev token for a banned user":                 {staging, f.devToken(t, banned.ID.String()), marked(standing(banned, auth.StandingBanned))},
		"dev token for a deleted user":                {test, f.devToken(t, deleted.ID.String()), standing(deleted, auth.StandingDeleted)},
		"dev token for an unknown user":               {dev, f.devToken(t, strangerID), noSession},
		"dev token for an unknown user in staging":    {staging, f.devToken(t, strangerID), noSession},
		"dev token for an unknown user in test":       {test, f.devToken(t, strangerID), verdict{actor: stranger.actor, queries: 1}},
		"dev token with a subject that is no user id": {dev, f.devToken(t, "u-dev"), refused},
		"dev token in production":                     {prod, f.devToken(t, user.ID.String()), refused},
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
		if got != (verdict{actor: f.privyActor(user, want), queries: 1}) {
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
	if got := f.verify(t, v, f.devToken(t, testkit.NewIDs(1).NewV7().String())); got.code != errs.CodeDBUnavailable {
		t.Fatalf("Verify of a dev token on a closed pool = %+v, want db_unavailable", got)
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
	if _, err := v.VerifyPrivy(t.Context(), f.devToken(t, "u-dev")); errs.CodeOf(err) != errs.CodeUnauthorized {
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

func TestNewVerifier_refusesAMissingPrivyVerificationKeyInEveryEnv(t *testing.T) {
	t.Parallel()
	f := newVerifierFixture(t)
	for _, env := range []config.Env{config.EnvLocal, config.EnvStaging, config.EnvProduction} {
		cfg := verifierConfig(env)
		cfg.Privy.VerificationKey = ""
		v, err := identity.NewVerifier(module.Deps{Config: cfg, Clock: f.clock, Pool: f.pool})
		if v != nil || errs.CodeOf(err) != errs.CodeInvalidInput || !strings.HasPrefix(err.Error(), "privy.New") {
			t.Fatalf("%s: NewVerifier = %v, %v, want invalid_input from privy.New", env, v, err)
		}
	}
}

type actorProbe struct {
	mu   sync.Mutex
	seen []auth.Actor
}

func (p *actorProbe) record(ctx context.Context) {
	a, _ := auth.ActorFrom(ctx)
	p.mu.Lock()
	p.seen = append(p.seen, a)
	p.mu.Unlock()
}

type identityProbe struct {
	api.StrictServerInterface
	*actorProbe
}

func (p identityProbe) GetMe(ctx context.Context, _ api.GetMeRequestObject) (api.GetMeResponseObject, error) {
	p.record(ctx)
	return nil, errs.New(errs.CodeNotFound, "actorProbe.GetMe")
}

type systemProbe struct {
	systemapi.StrictServerInterface
	*actorProbe
}

func (systemProbe) PostSystemPing(context.Context, systemapi.PostSystemPingRequestObject) (
	systemapi.PostSystemPingResponseObject, error,
) {
	return nil, errs.New(errs.CodeNotFound, "actorProbe.PostSystemPing")
}

func (p systemProbe) GetSystemPing(ctx context.Context, _ systemapi.GetSystemPingRequestObject) (
	systemapi.GetSystemPingResponseObject, error,
) {
	p.record(ctx)
	return nil, errs.New(errs.CodeNotFound, "actorProbe.GetSystemPing")
}

func TestVerifier_behindTheAuthMiddlewarePutsTheStandingInContextAndLogsNoToken(t *testing.T) {
	t.Parallel()
	f := newVerifierFixture(t)
	banned := testkit.SeedUser(t, f.pool, testkit.UserOpts{AccountStatus: "banned"})
	active := testkit.SeedUser(t, f.pool, testkit.UserOpts{})
	logs := &testkit.Logs{}
	logger := observability.NewLogger(config.Config{Env: config.EnvTest}, logs)
	probe := &actorProbe{}
	h, err := httpx.HandlerFor(httpx.Deps{
		Logger:       logger,
		Tracer:       tracenoop.NewTracerProvider(),
		Clock:        f.clock,
		IDs:          testkit.NewIDs(1),
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(f.pool, f.clock),
		Verifier:     f.verifier(t, config.EnvTest),
	}, func(m apibase.Mount) {
		api.Mount(identityProbe{actorProbe: probe}, m)
		systemapi.Mount(systemProbe{actorProbe: probe}, m)
	}, specContract(t))
	if err != nil {
		t.Fatal(err)
	}
	expired := fakes.PrivyAccessToken(privyAppID, banned.PrivyUserID, f.clock.Now().Add(-2*time.Hour), time.Hour)
	ping := "/v1/system/pings/" + testkit.NewIDs(2).NewV7().String()
	requests := []struct {
		method, path, token string
		status              int
	}{
		{http.MethodGet, "/v1/me", f.privyToken(banned.PrivyUserID), http.StatusNotFound},
		{http.MethodGet, ping, f.privyToken(banned.PrivyUserID), http.StatusForbidden},
		{http.MethodGet, ping, f.devToken(t, active.ID.String()), http.StatusNotFound},
		{http.MethodGet, ping, f.devToken(t, banned.ID.String()), http.StatusForbidden},
		{http.MethodGet, ping, f.devToken(t, "u-dev"), http.StatusUnauthorized},
		{http.MethodGet, ping, f.privyToken("did:privy:unknown"), http.StatusUnauthorized},
		{http.MethodGet, ping, unsigned(`{"alg":"none"}`, banned.PrivyUserID), http.StatusUnauthorized},
		{http.MethodGet, ping, expired, http.StatusUnauthorized},
	}
	for _, r := range requests {
		req := httptest.NewRequestWithContext(t.Context(), r.method, r.path, nil)
		req.Header.Set("Authorization", "Bearer "+r.token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != r.status {
			t.Fatalf("%s %s = %d, want %d: %s", r.method, r.path, rec.Code, r.status, rec.Body.String())
		}
	}
	want := []auth.Actor{
		f.privyActor(banned, auth.StandingBanned),
		{
			Kind: auth.ActorUser, ID: active.ID.String(), Standing: auth.StandingActive,
			SessionID: "dev-" + active.ID.String(), IssuedAt: f.clock.Now().Unix(),
		},
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
