package fakes

import (
	"embed"
	"encoding/json"
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
	Route  string          `json:"route"`
	Action Action          `json:"action"`
	Status int             `json:"status,omitempty"`
	Body   json.RawMessage `json:"body,omitempty"`
	Delay  string          `json:"delay,omitempty"`
	Times  int             `json:"times,omitempty"`
}

type fixture struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

type scripted struct {
	action Action
	status int
	body   []byte
	delay  time.Duration
	left   int
}

type fieldError string

func (f fieldError) Error() string { return "invalid step field: " + string(f) }

type Server struct {
	mux      *http.ServeMux
	fixtures map[string]fixture
	mu       sync.Mutex
	scripts  map[string][]*scripted
}

func upstreams() []string {
	return []string{"privy", "jupiter", "rpc", "helius", "xstocks", "apns", "ably"}
}

func New() *Server { return newFrom(fixtures, "testdata/fakes") }

func newFrom(fsys fs.FS, root string) *Server {
	s := &Server{mux: http.NewServeMux(), fixtures: loadFixtures(fsys, root), scripts: map[string][]*scripted{}}
	s.mux.HandleFunc("POST /_script", s.script)
	for _, name := range upstreams() {
		upstream := http.NewServeMux()
		upstream.HandleFunc("/", s.replay(name))
		s.mux.Handle("/"+name+"/", http.StripPrefix("/"+name, upstream))
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
	sc, err := parse(step)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.scripts[step.Route] = append(s.scripts[step.Route], sc)
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func parse(step Step) (*scripted, error) {
	upstream, _, _ := strings.Cut(strings.TrimPrefix(step.Route, "/"), "/")
	if !strings.HasPrefix(step.Route, "/"+upstream+"/") || !slices.Contains(upstreams(), upstream) {
		return nil, fieldError("route")
	}
	if step.Times < 0 {
		return nil, fieldError("times")
	}
	sc := &scripted{action: step.Action, status: step.Status, body: step.Body, left: max(step.Times, 1)}
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

func (s *Server) next(route string) scripted {
	s.mu.Lock()
	defer s.mu.Unlock()
	queue := s.scripts[route]
	if len(queue) == 0 {
		return scripted{action: ActionSucceed}
	}
	head := *queue[0]
	queue[0].left--
	if queue[0].left == 0 {
		s.scripts[route] = queue[1:]
	}
	return head
}

func (s *Server) replay(upstream string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		route := "/" + upstream + r.URL.Path
		step := s.next(route)
		switch step.action {
		case ActionHang:
			<-r.Context().Done()
			return
		case ActionFail:
			if len(step.body) > 0 {
				w.Header().Set("Content-Type", "application/json")
			}
			w.WriteHeader(step.status)
			_, _ = w.Write(step.body)
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
		f, ok := s.fixtures[route]
		if !ok {
			http.Error(w, "no fixture for "+route, http.StatusNotImplemented)
			return
		}
		for k, v := range f.Headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(f.Status)
		_, _ = w.Write(f.Body)
	}
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
