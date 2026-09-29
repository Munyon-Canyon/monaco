package solana_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const (
	member   = chain.SolanaAddress("5kwEmpcR8Txq1b4bDazRm9j4cx8Qo2aiE53rYA1dCDDP")
	sender   = chain.SolanaAddress("F4nbnZw67wKUQGW46MSRwN23VAKQRhYUMEXt88GQwD9z")
	feeMint  = chain.SolanaAddress("FHZNBei86FjdpSzU786ECJuqEAVYyfqCctt4aXWZh91M")
	usdcMint = chain.SolanaAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	deposit  = chain.Signature(
		"2Wjpe2AeceTX6fouJ1gYZktJKLVfJZLrPP7QGp3XvmzCtjTua3GZ6np8nB8geG9G5AWARjQaUKgtCCcGdyAcoGcK",
	)
	olderSig = chain.Signature(
		"3SK1GyUJGQCUTv9BzTMEcDoHUQCyNhtDoymkw8nERTkHmaAsgdYxdy5y1uKkdDzPy4ZA6G5istv5vfVAdc9WCHAn",
	)
	hiddenPart = "rpc-key-5ecret"
)

func usdc() chain.Mint { return chain.Mint{Address: usdcMint, Decimals: 6} }

type sent struct {
	path   string
	query  string
	method string
	params []json.RawMessage
}

type upstream struct {
	handler   http.Handler
	transport error
	badBody   bool

	mu   sync.Mutex
	sent []sent
}

func (u *upstream) RoundTrip(r *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	var call struct {
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	_ = json.Unmarshal(body, &call)
	u.mu.Lock()
	u.sent = append(u.sent, sent{path: r.URL.Path, query: r.URL.RawQuery, method: call.Method, params: call.Params})
	u.mu.Unlock()
	if u.transport != nil {
		return nil, u.transport
	}
	rec := httptest.NewRecorder()
	u.handler.ServeHTTP(rec, r)
	if err := r.Context().Err(); err != nil {
		return nil, err
	}
	resp := rec.Result()
	if u.badBody {
		resp.Body = io.NopCloser(errReader{})
	}
	return resp, nil
}

func (u *upstream) requests() []sent {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]sent(nil), u.sent...)
}

func (u *upstream) methods() []string {
	reqs := u.requests()
	out := make([]string, 0, len(reqs))
	for _, s := range reqs {
		out = append(out, s.method)
	}
	return out
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errs.New(errs.CodeUpstreamUnavailable, "test.read")
}

func testConfig() config.Config {
	return config.Config{
		Solana:   config.Solana{RPCURL: "http://rpc.test/rpc?api-key=" + hiddenPart},
		Timeouts: config.Timeouts{RPC: 5 * time.Second},
	}
}

func client(u *upstream) *solana.Client {
	return solana.New(testConfig(), clock.Real{}, httpclient.WithTransport(u))
}

func overFakes(t *testing.T) (*solana.Client, *upstream, *fakes.Server) {
	t.Helper()
	srv := fakes.New()
	u := &upstream{handler: srv}
	return client(u), u, srv
}

func script(t *testing.T, srv *fakes.Server, step fakes.Step) {
	t.Helper()
	raw, err := json.Marshal(step)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/_script", bytes.NewReader(raw)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("script %s = %d %q", raw, rec.Code, rec.Body.String())
	}
}

func result(body string) *upstream {
	return replying(http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":`+body+`}`)
}

func replying(status int, body string) *upstream {
	return &upstream{handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})}
}

func wantCode(t *testing.T, err error, code errs.Code) {
	t.Helper()
	if errs.CodeOf(err) != code {
		t.Fatalf("err = %v, want %s", err, code)
	}
}
