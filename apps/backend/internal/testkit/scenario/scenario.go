package scenario

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

type Scenario struct {
	t        *testing.T
	app      *app
	users    map[string]*user
	actor    *user
	last     *response
	remember map[string]string
	keys     int
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
	method, path, body, key, token string
}

type Step func(s *Scenario)

type Option func(*options)

type options struct {
	modules []func(module.Deps) module.Module
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
	return &Scenario{t: t, app: start(t, o.modules), users: map[string]*user{}, remember: map[string]string{}}
}

func (s *Scenario) Given(steps ...Step) *Scenario { return s.run(steps) }

func (s *Scenario) When(steps ...Step) *Scenario { return s.run(steps) }

func (s *Scenario) Then(steps ...Step) *Scenario { return s.run(steps) }

func (s *Scenario) run(steps []Step) *Scenario {
	s.t.Helper()
	for _, step := range steps {
		step(s)
	}
	return s
}

func (s *Scenario) user(name string) *user {
	s.t.Helper()
	if u, ok := s.users[name]; ok {
		return u
	}
	id, err := ids.ParseUserID(s.app.ids.NewV7().String())
	if err != nil {
		s.t.Fatalf("scenario: user %s: %v", name, err)
	}
	return s.addUser(name, id)
}

func (s *Scenario) addUser(name string, id ids.UserID) *user {
	s.t.Helper()
	u := &user{id: id, token: s.app.verifier.Mint(id.String(), time.Now().Add(time.Hour))}
	u.stream = s.app.openStream(s.t, u.token)
	s.users[name] = u
	return u
}

func (s *Scenario) path(p string) string {
	for name, v := range s.remember {
		p = strings.ReplaceAll(p, "{"+name+"}", v)
	}
	return p
}

func (s *Scenario) send(req request) {
	s.t.Helper()
	r, err := http.NewRequestWithContext(s.t.Context(), req.method, s.app.server.URL+req.path,
		strings.NewReader(req.body))
	if err != nil {
		s.t.Fatalf("scenario: %s %s: %v", req.method, req.path, err)
	}
	r.Header.Set("Content-Type", "application/json")
	if req.token != "" {
		r.Header.Set("Authorization", "Bearer "+req.token)
	}
	if req.key != "" {
		r.Header.Set("Idempotency-Key", req.key)
	}
	resp, err := s.app.server.Client().Do(r)
	if err != nil {
		s.t.Fatalf("scenario: %s %s: %v", req.method, req.path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.t.Fatalf("scenario: %s %s: %v", req.method, req.path, err)
	}
	s.last = &response{req: req, status: resp.StatusCode, header: resp.Header, body: body}
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
