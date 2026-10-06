package fakes

import (
	"cmp"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

//go:embed all:testdata/fakes
var fixtures embed.FS

type Action string

const (
	ActionSucceed Action = "succeed"
	ActionFail    Action = "fail"
	ActionDelay   Action = "delay"
	ActionHang    Action = "hang"
)

type Step struct {
	Route   string            `json:"route"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Query   map[string]string `json:"query,omitempty"`
	Action  Action            `json:"action"`
	Status  int               `json:"status,omitempty"`
	Body    json.RawMessage   `json:"body,omitempty"`
	Delay   string            `json:"delay,omitempty"`
	Times   int               `json:"times,omitempty"`
	Fixture string            `json:"fixture,omitempty"`
	Reset   bool              `json:"reset,omitempty"`
}

type fixture struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

type scripted struct {
	action  Action
	status  int
	body    []byte
	delay   time.Duration
	left    int
	fixture string
	method  string
	headers map[string]string
	query   map[string]string
}

type fieldError string

func (f fieldError) Error() string { return "invalid step field: " + string(f) }

type Server struct {
	mux          *http.ServeMux
	live         *http.ServeMux
	fixtures     map[string]fixture
	mu           sync.Mutex
	scripts      map[string][]*scripted
	upstreams    []string
	wallets      map[string]privyWallet
	createdUsers map[string]privyCreatedUser
	objects      map[string]storedObject
	posthog      []PostHogCapture
	ably         []AblyPublish
	balances     map[string]map[string]tokenBalance
	swaps        map[string]SetSwap
	orders       map[string]swapOrder
	orderCount   uint64
	landed       map[string]landedSwap
	blockhashes  map[string]bool
	unexecuted   map[string]bool
	nextUser     int
}

func New() *Server { return newFrom(fixtures, "testdata/fakes") }

func newFrom(fsys fs.FS, root string) *Server {
	s := &Server{
		mux:          http.NewServeMux(),
		live:         http.NewServeMux(),
		fixtures:     loadFixtures(fsys, root),
		scripts:      map[string][]*scripted{},
		wallets:      map[string]privyWallet{},
		createdUsers: map[string]privyCreatedUser{},
		objects:      map[string]storedObject{},
		balances:     map[string]map[string]tokenBalance{},
		swaps:        map[string]SetSwap{},
		orders:       map[string]swapOrder{},
		landed:       map[string]landedSwap{},
		blockhashes:  map[string]bool{},
		unexecuted:   map[string]bool{},
		upstreams:    upstreamsIn(fsys, root),
	}
	s.mux.HandleFunc("POST /_script", s.script)
	s.mux.HandleFunc("POST /_balance", s.setBalance)
	s.mux.HandleFunc("POST /_swap", s.setSwap)
	s.mux.HandleFunc("POST /_wallet", s.setWallet)
	s.live.HandleFunc("POST /rpc/sendTransaction", sendTransaction)
	s.live.HandleFunc("GET "+orderRoute, s.jupiterOrder)
	s.live.HandleFunc("POST /rpc/getSignatureStatuses", s.signatureStatuses)
	s.live.HandleFunc("POST /rpc/isBlockhashValid", s.blockhashValid)
	s.live.HandleFunc("POST /rpc/getTransaction", s.swapTransaction)
	s.live.HandleFunc("POST "+executeRoute, s.jupiterExecute)
	s.live.HandleFunc("GET /privy/v1/users/{id}", s.privyUser)
	s.live.HandleFunc("POST /privy/v1/users", s.privyCreateUser)
	s.live.HandleFunc("GET /privy/v1/wallets", s.privyWallets)
	s.live.HandleFunc("POST /privy/v1/wallets", s.privyCreateWallet)
	s.live.HandleFunc("POST /privy/v1/wallets/{id}/rpc", s.privySign)
	s.live.HandleFunc("POST /apns/3/device/{token}", s.apnsPush)
	s.live.HandleFunc("POST /posthog/batch/", s.posthogBatch)
	s.live.HandleFunc("POST /ably/channels/{name}/messages", s.ablyPublish)
	s.mountStorage()
	for _, name := range s.upstreams {
		replay := s.replay(name)
		upstream := http.NewServeMux()
		upstream.HandleFunc("/", replay)
		s.mux.Handle("/"+name+"/", http.StripPrefix("/"+name, upstream))
		s.mux.HandleFunc("/"+name, func(w http.ResponseWriter, r *http.Request) {
			r = r.Clone(r.Context())
			r.URL.Path, r.URL.RawPath = "/", ""
			replay(w, r)
		})
	}
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) script(w http.ResponseWriter, r *http.Request) {
	var step Step
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&step); err != nil {
		http.Error(w, "decode: "+err.Error(), http.StatusBadRequest)
		return
	}
	sc, err := parse(step, s.upstreams)
	if err == nil && !s.replayable(step) {
		err = fieldError("fixture")
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if step.Reset {
		s.scripts[step.Route] = nil
	}
	s.scripts[step.Route] = append(s.scripts[step.Route], sc)
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func parse(step Step, upstreams []string) (*scripted, error) {
	upstream, _, _ := strings.Cut(strings.TrimPrefix(step.Route, "/"), "/")
	if !strings.HasPrefix(step.Route, "/"+upstream+"/") || !slices.Contains(upstreams, upstream) {
		return nil, fieldError("route")
	}
	if step.Times < 0 {
		return nil, fieldError("times")
	}
	sc := &scripted{
		action: step.Action, status: step.Status, body: step.Body, left: max(step.Times, 1), fixture: step.Fixture,
		method: step.Method, headers: step.Headers, query: step.Query,
	}
	switch step.Action {
	case ActionSucceed, ActionHang:
	case ActionFail:
		if step.Status < http.StatusBadRequest || step.Status > 599 {
			return nil, fieldError("status")
		}
	case ActionDelay:
		d, err := time.ParseDuration(step.Delay)
		if err != nil || d <= 0 {
			return nil, fieldError("delay")
		}
		sc.delay = d
	default:
		return nil, fieldError("action")
	}
	return sc, nil
}

func (s *Server) replayable(step Step) bool {
	if step.Fixture == "" {
		return true
	}
	upstream, _, _ := strings.Cut(strings.TrimPrefix(step.Route, "/"), "/")
	_, ok := s.fixtures[step.Fixture]
	return ok && step.Action == ActionSucceed && strings.HasPrefix(step.Fixture, "/"+upstream+"/")
}

func (s *Server) next(route string, r *http.Request) scripted {
	s.mu.Lock()
	defer s.mu.Unlock()
	queue := s.scripts[route]
	for i, candidate := range queue {
		if !candidate.matches(r) {
			continue
		}
		step := *candidate
		candidate.left--
		if candidate.left == 0 {
			s.scripts[route] = append(queue[:i], queue[i+1:]...)
		}
		return step
	}
	return scripted{action: ActionSucceed}
}

func (s scripted) matches(r *http.Request) bool {
	if s.method != "" && s.method != r.Method {
		return false
	}
	for key, want := range s.headers {
		if r.Header.Get(key) != want {
			return false
		}
	}
	for key, want := range s.query {
		if r.URL.Query().Get(key) != want {
			return false
		}
	}
	return true
}

func (s scripted) write(w http.ResponseWriter, status int) {
	if len(s.body) > 0 {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	_, _ = w.Write(s.body)
}

func (s *Server) replay(upstream string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		route, keys := routeOf(upstream, r)
		step := s.next(route, r)
		switch step.action {
		case ActionHang:
			if r.Body != nil {
				_, _ = io.Copy(io.Discard, r.Body)
			}
			<-r.Context().Done()
			return
		case ActionFail:
			step.write(w, step.status)
			return
		case ActionDelay:
			t := time.NewTimer(step.delay)
			defer t.Stop()
			select {
			case <-r.Context().Done():
				return
			case <-t.C:
			}
		case ActionSucceed:
		}
		if len(step.body) > 0 {
			step.write(w, cmp.Or(step.status, http.StatusOK))
			return
		}
		if step.fixture != "" {
			keys = []string{step.fixture}
		} else if live := liveRequest(r, storageOr(route, upstream, r)); s.isLive(live) {
			s.live.ServeHTTP(w, live)
			return
		}
		if s.answerRPC(w, r, route, keys, step.fixture != "") {
			return
		}
		s.serveFixture(w, keys)
	}
}

func (s *Server) multipleAccounts(w http.ResponseWriter, r *http.Request) bool {
	var call rpcCall
	if json.NewDecoder(r.Body).Decode(&call) != nil || len(call.Params) == 0 {
		return false
	}
	var mints []string
	if json.Unmarshal(call.Params[0], &mints) != nil {
		return false
	}
	values := make([]json.RawMessage, len(mints))
	for i, mint := range mints {
		fixture, ok := s.fixtures["/rpc/getAccountInfo/"+mint]
		if !ok {
			continue
		}
		var reply struct {
			Result struct {
				Value json.RawMessage `json:"value"`
			} `json:"result"`
		}
		if json.Unmarshal(fixture.Body, &reply) != nil {
			return false
		}
		values[i] = reply.Result.Value
	}
	rpcReply(w, call.ID, map[string]any{"value": values}, nil)
	return true
}

func (s *Server) serveFixture(w http.ResponseWriter, keys []string) {
	for _, key := range keys {
		f, ok := s.fixtures[key]
		if !ok {
			continue
		}
		for k, v := range f.Headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(f.Status)
		_, _ = w.Write(f.Body)
		return
	}
	http.Error(w, "no fixture for "+keys[len(keys)-1], http.StatusNotImplemented)
}

func (s *Server) isLive(r *http.Request) bool {
	_, pattern := s.live.Handler(r)
	return pattern != ""
}

func liveRequest(r *http.Request, route string) *http.Request {
	c := r.Clone(r.Context())
	c.URL.Path, c.URL.RawPath = route, ""
	return c
}

func upstreamsIn(fsys fs.FS, root string) []string {
	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		panic(errs.Wrap(err, errs.CodeDecodeFailed, "fakes.upstreamsIn", slog.String("root", root)))
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

func loadFixtures(fsys fs.FS, root string) map[string]fixture {
	out := map[string]fixture{}
	err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		var f fixture
		raw, err := fs.ReadFile(fsys, p)
		if err == nil {
			err = json.Unmarshal(raw, &f)
		}
		if err != nil {
			return errs.Wrap(err, errs.CodeDecodeFailed, "fakes.loadFixtures", slog.String("fixture", p))
		}
		out[strings.TrimSuffix(strings.TrimPrefix(p, root), ".json")] = f
		return nil
	})
	if err != nil {
		panic(err)
	}
	return out
}
