package jupiterprices_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/jupiterprices"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const priceRoute = "/jupiter/price/v3"

type upstream struct {
	fakes *fakes.Server
	fail  string
	calls atomic.Int32
}

func (u *upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u.calls.Add(1)
	if u.fail != "" && strings.Contains(r.URL.Query().Get("ids"), u.fail) {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	u.fakes.ServeHTTP(w, r)
}

func (u *upstream) script(t *testing.T, step fakes.Step) {
	t.Helper()
	raw, err := json.Marshal(step)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	u.fakes.ServeHTTP(
		rec,
		httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/_script", bytes.NewReader(raw)),
	)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("script %s = %d %q", raw, rec.Code, rec.Body.String())
	}
}

func source(t *testing.T, u *upstream) jupiterprices.Source {
	t.Helper()
	srv := httptest.NewServer(u)
	t.Cleanup(srv.Close)
	cfg := config.Config{
		Jupiter: config.Jupiter{
			SwapBaseURL: srv.URL + "/jupiter/swap/v2", PriceBaseURL: srv.URL + priceRoute, APIKey: "test-key",
		},
		Timeouts: config.Timeouts{JupiterQuote: 5 * time.Second, JupiterExecute: time.Minute},
	}
	return jupiterprices.New(jupiter.New(cfg, clock.Real{}))
}

func generatedMint(t *testing.T, i int) domain.Mint {
	t.Helper()
	key := sha256.Sum256([]byte("mint-" + strconv.Itoa(i)))
	m, err := domain.ParseMint(string(chain.AddressOf(key[:])))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSource_pricesTheMintsJupiterAnswersAndLeavesTheRestOut(t *testing.T) {
	t.Parallel()
	u := &upstream{fakes: fakes.New()}
	u.script(t, fakes.Step{Route: priceRoute, Action: fakes.ActionSucceed, Fixture: priceRoute + "/catalog"})
	aapl, tsla, jpst := marketfake.AAPLx().Mint, marketfake.TSLAx().Mint, marketfake.JPSTx().Mint
	got, err := source(t, u).Prices(t.Context(), []domain.Mint{aapl, tsla, jpst})
	if err != nil || len(got) != 2 || got[aapl].Uint64() != 254_371_234 || got[tsla].Uint64() != 436_120_500 {
		t.Fatalf("Prices = %v, %v, want AAPLx at 254.371234 and TSLAx at 436.1205 with JPSTx left out", got, err)
	}
}

func TestSource_sendsOneCallPerFiftyMints(t *testing.T) {
	t.Parallel()
	u := &upstream{fakes: fakes.New()}
	mints := make([]domain.Mint, 120)
	for i := range mints {
		mints[i] = generatedMint(t, i)
	}
	got, err := source(t, u).Prices(t.Context(), mints)
	if err != nil || len(got) != 0 || u.calls.Load() != 3 {
		t.Fatalf("Prices over 120 unlisted mints = %v, %v after %d calls, want nothing priced in 3 calls",
			got, err, u.calls.Load())
	}
}

func TestSource_keepsTheBatchThatAnsweredWhenAnotherFails(t *testing.T) {
	t.Parallel()
	mints := make([]domain.Mint, 0, 51)
	mints = append(mints, marketfake.AAPLx().Mint, marketfake.TSLAx().Mint)
	for i := range 49 {
		mints = append(mints, generatedMint(t, i))
	}
	u := &upstream{fakes: fakes.New(), fail: mints[50].String()}
	u.script(t, fakes.Step{Route: priceRoute, Action: fakes.ActionSucceed, Fixture: priceRoute + "/catalog"})
	got, err := source(t, u).Prices(t.Context(), mints)
	if errs.CodeOf(err) != errs.CodeJupiterUnavailable {
		t.Fatalf("err = %v, want the failed batch's jupiter_unavailable", err)
	}
	if len(got) != 2 || got[mints[0]].Uint64() != 254_371_234 || u.calls.Load() != 2 {
		t.Fatalf("Prices = %v after %d calls, want AAPLx and TSLAx from the batch that answered", got, u.calls.Load())
	}
}
