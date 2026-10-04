package scenario

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type T interface {
	Helper()
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Context() context.Context
	Cleanup(f func())
}

type Stage string

const (
	StageGiven Stage = "given"
	StageWhen  Stage = "when"
	StageThen  Stage = "then"
)

type Exchange struct {
	Method, Path, IdempotencyKey string
	Request, Response            []byte
	Status                       int
	Started                      time.Time
	Took                         time.Duration
}

type backend struct {
	baseURL       string
	clientIP      string
	client        *http.Client
	bus           *bus.Conn
	note          *notifier
	pool          *pgxpool.Pool
	mint          func(id ids.UserID) string
	privyToken    func(sub string) string
	script        func(ctx context.Context, t T, step fakes.Step)
	newUserID     func() (ids.UserID, error)
	enter         func(stage Stage)
	exchanged     func(e Exchange)
	events        func(t T, typ events.Type, actors []string) []string
	eventPayloads func(t T, typ events.Type) [][]byte
	awaitHandled  func(t T, typ events.Type, eventIDs []string)
	published     func(t T, typ events.Type, eventIDs []string) uint64
	hold          func()
	crashAt       func(t T, point faultpoint.Name)
	seed          func(t T, name string) []testkit.Seeded
	lines         func(from int) ([]string, <-chan struct{})
	tick          func(t T, poller string) (stop func())
	coreSubscribe func(t T, subject string) <-chan []byte
	tickCrash     func(t T, poller string, point faultpoint.Name)
	flow          string
	trigger       string
	restart       func(t T)
	faults        int
}

type Scenario struct {
	t        T
	app      *backend
	users    map[string]*user
	actor    *user
	last     *response
	remember map[string]string
	keys     int
	ticks    map[string]tick
	marks    map[string]int
	core     map[string]<-chan []byte
}

type user struct {
	id     ids.UserID
	token  string
	stream *stream
}

type response struct {
	req    request
	status int
	header http.Header
	body   []byte
}

type request struct {
	method, path, body, key, token, contentType string
}

type Step func(s *Scenario)

type Option func(*options)

type options struct {
	modules    []func(module.Deps) module.Module
	logs       io.Writer
	contract   *httpx.Contract
	fakes      *fakes.Server
	privyAppID string
	wrap       func(http.Handler) http.Handler
}

func WithPrivy(upstreams *fakes.Server, appID string) Option {
	return func(o *options) { o.fakes, o.privyAppID = upstreams, appID }
}

func WithRequestMiddleware(wrap func(http.Handler) http.Handler) Option {
	return func(o *options) { o.wrap = wrap }
}

func WithContract(c *httpx.Contract) Option {
	return func(o *options) { o.contract = c }
}

func LoadContract(spec []byte) (*httpx.Contract, error) {
	c, err := httpx.LoadContract(spec)
	if err != nil {
		return nil, err
	}
	if err := c.Document().Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("the spec is not a valid OpenAPI document: %w", err)
	}
	return c, nil
}

func WithLogs(w io.Writer) Option {
	return func(o *options) { o.logs = w }
}

func WithModules(mods ...func(module.Deps) module.Module) Option {
	return func(o *options) { o.modules = append(o.modules, mods...) }
}

func New(t *testing.T, opts ...Option) *Scenario {
	t.Helper()
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return newScenario(t, start(t, o).backend())
}

func newScenario(t T, b *backend) *Scenario {
	return &Scenario{
		t:        t,
		app:      b,
		users:    map[string]*user{},
		remember: map[string]string{},
		ticks:    map[string]tick{},
		marks:    map[string]int{},
		core:     map[string]<-chan []byte{},
	}
}

func (s *Scenario) Given(steps ...Step) *Scenario { return s.run(StageGiven, steps) }

func (s *Scenario) When(steps ...Step) *Scenario { return s.run(StageWhen, steps) }

func (s *Scenario) Then(steps ...Step) *Scenario { return s.run(StageThen, steps) }

func (s *Scenario) run(stage Stage, steps []Step) *Scenario {
	s.t.Helper()
	s.app.enter(stage)
	for _, step := range steps {
		step(s)
	}
	return s
}

func (s *Scenario) Faults() int { return s.app.faults }

func (s *Scenario) DB() *pgxpool.Pool { return s.app.pool }

func (s *Scenario) Context() context.Context { return s.t.Context() }

func (s *Scenario) Bus() *bus.Conn { return s.app.bus }

func (s *Scenario) Helper() { s.t.Helper() }

func (s *Scenario) Fatalf(format string, args ...any) {
	s.t.Helper()
	s.t.Fatalf(format, args...)
}

func (s *Scenario) Recall(name string) string { return s.remember[name] }

func (s *Scenario) ActorID() ids.UserID {
	s.t.Helper()
	if s.actor == nil {
		s.t.Fatal("scenario: no actor")
	}
	return s.actor.id
}

func (s *Scenario) user(name string) *user {
	s.t.Helper()
	if u, ok := s.users[name]; ok {
		return u
	}
	id, err := s.app.newUserID()
	if err != nil {
		s.t.Fatalf("scenario: user %s: %v", name, err)
	}
	return s.addUser(name, id)
}

func (s *Scenario) addUser(name string, id ids.UserID) *user {
	s.t.Helper()
	u := &user{id: id, token: s.app.mint(id)}
	u.stream = openStream(s.t, s.app, u.token)
	s.users[name] = u
	return u
}

func (s *Scenario) actors() []string {
	out := make([]string, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u.id.String())
	}
	return out
}

func (s *Scenario) path(p string) string {
	for name, v := range s.remember {
		p = strings.ReplaceAll(p, "{"+name+"}", v)
	}
	return p
}

func (s *Scenario) send(req request) {
	s.t.Helper()
	r, err := http.NewRequestWithContext(s.t.Context(), req.method, s.app.baseURL+req.path,
		strings.NewReader(req.body))
	if err != nil {
		s.t.Fatalf("scenario: %s %s: %v", req.method, req.path, err)
	}
	if req.contentType == "" {
		req.contentType = "application/json"
	}
	r.Header.Set("Content-Type", req.contentType)
	if req.token != "" {
		r.Header.Set("Authorization", "Bearer "+req.token)
	}
	if req.key != "" {
		r.Header.Set("Idempotency-Key", req.key)
	}
	if s.app.clientIP != "" {
		r.Header.Set("X-Forwarded-For", s.app.clientIP)
	}
	if s.app.flow != "" && matchesTrigger(s.app.trigger, req.method, req.path) {
		r.Header.Set(httpx.FlowHeader, s.app.flow)
	}
	started := time.Now()
	resp, err := s.app.client.Do(r)
	if err != nil {
		s.t.Fatalf("scenario: %s %s: %v", req.method, req.path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.t.Fatalf("scenario: %s %s: %v", req.method, req.path, err)
	}
	s.app.exchanged(Exchange{
		Method:         req.method,
		Path:           req.path,
		IdempotencyKey: req.key,
		Request:        []byte(req.body),
		Response:       body,
		Status:         resp.StatusCode,
		Started:        started,
		Took:           time.Since(started),
	})
	s.last = &response{req: req, status: resp.StatusCode, header: resp.Header, body: body}
	if s.isFaultpoint() {
		s.app.faults++
		if s.app.restart != nil {
			s.app.restart(s.t)
			s.app.restart = nil
			s.reopenStreams()
		}
	}
}

func (s *Scenario) reopenStreams() {
	for _, u := range s.users {
		u.stream = openStream(s.t, s.app, u.token)
	}
}

func (s *Scenario) isFaultpoint() bool {
	var p api.Problem
	return s.last.status == http.StatusServiceUnavailable && json.Unmarshal(s.last.body, &p) == nil &&
		p.Code == api.ErrorCode("faultpoint")
}

func matchesTrigger(trigger, method, path string) bool {
	wantMethod, wantPath, ok := strings.Cut(trigger, " ")
	if !ok || wantMethod != method {
		return false
	}
	wantParts, gotParts := strings.Split(wantPath, "/"), strings.Split(path, "/")
	if len(wantParts) != len(gotParts) {
		return false
	}
	for i, want := range wantParts {
		if strings.HasPrefix(want, "{") && strings.HasSuffix(want, "}") {
			continue
		}
		if want != gotParts[i] {
			return false
		}
	}
	return true
}

func (s *Scenario) response() *response {
	s.t.Helper()
	if s.last == nil {
		s.t.Fatal("scenario: no request has been sent")
	}
	return s.last
}

func (s *Scenario) field(name string) json.RawMessage {
	s.t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(s.response().body, &fields); err != nil {
		s.t.Fatalf("scenario: response body %s: %v", s.last.body, err)
	}
	v, ok := fields[name]
	if !ok {
		s.t.Fatalf("scenario: response body %s has no field %q", s.last.body, name)
	}
	return v
}

func (s *Scenario) nextKey() string {
	s.keys++
	return "scenario-" + strconv.Itoa(s.keys)
}

func equalJSON(a, b []byte) bool {
	var x, y any
	return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil &&
		bytes.Equal(mustMarshal(x), mustMarshal(y))
}

func mustMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
