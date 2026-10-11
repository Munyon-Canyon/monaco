package uniswap

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

//go:embed testdata/*.json
var recordings embed.FS

type Request struct {
	Type              string   `json:"type"`
	Amount            string   `json:"amount"`
	TokenInChainID    int64    `json:"tokenInChainId"`
	TokenOutChainID   int64    `json:"tokenOutChainId"`
	TokenIn           string   `json:"tokenIn"`
	TokenOut          string   `json:"tokenOut"`
	Swapper           string   `json:"swapper"`
	RoutingPreference string   `json:"routingPreference"`
	Protocols         []string `json:"protocols"`
	APIKey            string   `json:"-"`
}

type key struct{ tokenIn, tokenOut, amount string }

func keyOf(tokenIn, tokenOut, amount string) key {
	return key{strings.ToLower(tokenIn), strings.ToLower(tokenOut), amount}
}

type recording struct {
	Request Request         `json:"request"`
	Status  int             `json:"status"`
	Body    json.RawMessage `json:"body"`
}

type Server struct {
	replies  map[key]recording
	mu       sync.Mutex
	failures []int
	requests []Request
}

func New() *Server {
	s := &Server{replies: map[key]recording{}}
	paths, _ := fs.Glob(recordings, "testdata/*.json")
	for _, p := range paths {
		raw, _ := recordings.ReadFile(p)
		var r recording
		if err := json.Unmarshal(raw, &r); err != nil {
			panic(errs.Wrap(err, errs.CodeDecodeFailed, "uniswap.New", slog.String("recording", p)))
		}
		s.replies[keyOf(r.Request.TokenIn, r.Request.TokenOut, r.Request.Amount)] = r
	}
	return s
}

func (s *Server) FailNext(status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = append(s.failures, status)
}

func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/v1/quote" {
		http.NotFound(w, r)
		return
	}
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		reply(w, http.StatusBadRequest, json.RawMessage(`{"errorCode":"RequestValidationError"}`))
		return
	}
	req.APIKey = r.Header.Get("x-api-key")
	if status, failed := s.record(req); failed {
		reply(w, status, json.RawMessage(`{"errorCode":"`+http.StatusText(status)+`"}`))
		return
	}
	rec, ok := s.replies[keyOf(req.TokenIn, req.TokenOut, req.Amount)]
	if !ok {
		http.Error(w, "no recording for "+req.TokenIn+" -> "+req.TokenOut+" "+req.Amount, http.StatusNotImplemented)
		return
	}
	reply(w, rec.Status, rec.Body)
}

func (s *Server) record(req Request) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, req)
	if len(s.failures) == 0 {
		return 0, false
	}
	status := s.failures[0]
	s.failures = s.failures[1:]
	return status, true
}

func reply(w http.ResponseWriter, status int, body json.RawMessage) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
