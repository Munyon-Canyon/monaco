package jupiter_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const priceFixtures = "../../../testkit/fakes/testdata/fakes/jupiter/price/"

func fixtureMint(i int) jupiter.Mint {
	return jupiter.Mint{Address: fmt.Sprintf("Xs%02dPriceFixtureMint1111111111111111111111", i), Decimals: 8}
}

func fixtureBody(t *testing.T, file string) string {
	t.Helper()
	raw, err := fs.ReadFile(os.DirFS(priceFixtures), file)
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Body json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return string(f.Body)
}

func TestPrices_BatchesOf50(t *testing.T) {
	t.Parallel()
	first, second := fixtureBody(t, "v3.json"), fixtureBody(t, "v3/batch-2.json")
	u := &upstream{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := first
		if !strings.Contains(r.URL.Query().Get("ids"), fixtureMint(1).Address) {
			body = second
		}
		_, _ = w.Write([]byte(body))
	})}
	mints := make([]jupiter.Mint, 55)
	for i := range mints {
		mints[i] = fixtureMint(i + 1)
	}
	start := now()

	got, err := client(u).Prices(t.Context(), mints)
	if err != nil {
		t.Fatal(err)
	}
	wantBatches(t, u.requests(), 50, 5)
	if len(got) != 55 {
		t.Fatalf("got %d prices, want 55", len(got))
	}
	for i := 1; i <= 55; i++ {
		m := fixtureMint(i)
		want := money.MicrosFromUint64(uint64((10+7*i)*1_000_000 + (i*123457)%1_000_000))
		if p := got[m]; p.Mint != m || p.USDMicros != want || p.ObservedAt.Before(start) {
			t.Fatalf("price %d = %+v, want %v micros", i, p, want)
		}
	}
}

func wantBatches(t *testing.T, sent []sent, sizes ...int) {
	t.Helper()
	got := make([]int, 0, len(sent))
	for _, s := range sent {
		got = append(got, len(strings.Split(s.query.Get("ids"), ",")))
		if s.path != "/jupiter/price/v3" || s.apiKey != "test-key" {
			t.Fatalf("request = %+v, want GET /jupiter/price/v3 with the key", s)
		}
	}
	slices.Sort(got)
	slices.Sort(sizes)
	if !slices.Equal(got, sizes) {
		t.Fatalf("price calls carried %v mints, want %v", got, sizes)
	}
}

func TestPrices_missingOrNullMintIsAbsentNotZero(t *testing.T) {
	t.Parallel()
	a, b, c := fixtureMint(1), fixtureMint(2), fixtureMint(3)
	u := replying(http.StatusOK, `{"`+a.Address+`":{"usdPrice":"1.5"},"`+b.Address+`":null,"Other":{"usdPrice":9}}`)
	got, err := client(u).Prices(t.Context(), []jupiter.Mint{a, b, c})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[a].USDMicros != money.MicrosFromUint64(1_500_000) {
		t.Fatalf("Prices = %+v, want only %s at 1500000 micros", got, a.Address)
	}
	if _, ok := got[b]; ok {
		t.Fatalf("null mint %s is present", b.Address)
	}
}

func TestPrices_noMintsMakesNoCall(t *testing.T) {
	t.Parallel()
	u := replying(http.StatusOK, `{}`)
	got, err := client(u).Prices(t.Context(), nil)
	if err != nil || len(got) != 0 || len(u.requests()) != 0 {
		t.Fatalf("Prices(nil) = %v, %v after %d calls", got, err, len(u.requests()))
	}
}

func TestPrices_failures(t *testing.T) {
	t.Parallel()
	m := fixtureMint(1)
	for name, tc := range map[string]struct {
		u    *upstream
		want errs.Code
	}{
		"unauthorized": {replying(http.StatusUnauthorized, `Unauthorized`), errs.CodeJupiterRejected},
		"server error": {replying(http.StatusInternalServerError, ``), errs.CodeJupiterUnavailable},
		"not json":     {replying(http.StatusOK, `{"`), errs.CodeDecodeFailed},
		"negative":     {replying(http.StatusOK, `{"`+m.Address+`":{"usdPrice":-1}}`), errs.CodeDecodeFailed},
		"no price":     {replying(http.StatusOK, `{"`+m.Address+`":{}}`), errs.CodeDecodeFailed},
	} {
		_, err := client(tc.u).Prices(t.Context(), []jupiter.Mint{m})
		if errs.CodeOf(err) != tc.want {
			t.Fatalf("%s: err = %v, want %s", name, err, tc.want)
		}
	}
}

func TestPrices_oneFailedBatchFailsTheCall(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	u := &upstream{handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 2 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})}
	mints := make([]jupiter.Mint, 51)
	for i := range mints {
		mints[i] = fixtureMint(i + 1)
	}
	if _, err := client(u).Prices(t.Context(), mints); errs.CodeOf(err) != errs.CodeJupiterRejected {
		t.Fatalf("err = %v, want jupiter_rejected", err)
	}
}

func TestPrices_keepsTheBatchesThatAnsweredAndReturnsTheFailedBatchsCode(t *testing.T) {
	t.Parallel()
	first := fixtureBody(t, "v3.json")
	u := &upstream{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Query().Get("ids"), fixtureMint(51).Address) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(first))
	})}
	mints := make([]jupiter.Mint, 51)
	for i := range mints {
		mints[i] = fixtureMint(i + 1)
	}
	got, err := client(u).Prices(t.Context(), mints)
	if errs.CodeOf(err) != errs.CodeJupiterUnavailable || attr(err, "failed_batches") != "1" {
		t.Fatalf("err = %v, want jupiter_unavailable from one failed batch", err)
	}
	if _, ok := got[fixtureMint(51)]; ok || len(got) != 50 || got[fixtureMint(1)].USDMicros.Uint64() != 17_123_457 {
		t.Fatalf("Prices kept %d prices, want the 50 of the batch that answered", len(got))
	}
}

func TestPrices_cancelledCallAsksNothing(t *testing.T) {
	t.Parallel()
	u := replying(http.StatusOK, `{}`)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	got, err := client(u).Prices(ctx, []jupiter.Mint{fixtureMint(1)})
	if !errors.Is(err, context.Canceled) || got != nil || len(u.requests()) != 0 {
		t.Fatalf("Prices on a cancelled context = %v, %v after %d calls", got, err, len(u.requests()))
	}
}

func TestParsePrice_roundsDownToMicros(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]uint64{
		"0":                0,
		"1":                1_000_000,
		"212.3456789":      212_345_678,
		"0.0000009":        0,
		"1.2e-5":           12,
		"18446744073709.5": 18_446_744_073_709_500_000,
	} {
		got, ok := jupiter.ParsePrice(raw)
		if !ok || got.Uint64() != want {
			t.Fatalf("ParsePrice(%q) = %d, %v, want %d", raw, got.Uint64(), ok, want)
		}
	}
	for _, raw := range []string{"-1", "abc", "1e1000", "18446744073709.552", ""} {
		if _, ok := jupiter.ParsePrice(raw); ok {
			t.Fatalf("ParsePrice(%q) accepted", raw)
		}
	}
}

func FuzzParsePrice(f *testing.F) {
	for _, seed := range []string{"212.3456789", "0", "-0.5", "1.2e-7", "9e18", "1e-999", "NaN", "0x10", "1_0"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		got, ok := jupiter.ParsePrice(raw)
		if !ok {
			return
		}
		r, parsed := new(big.Rat).SetString(raw)
		if !parsed || r.Sign() < 0 {
			t.Fatalf("ParsePrice(%q) accepted a non-price", raw)
		}
		micros := new(big.Rat).SetUint64(got.Uint64())
		scaled := new(big.Rat).Mul(r, big.NewRat(1_000_000, 1))
		if micros.Cmp(scaled) > 0 || new(big.Rat).Add(micros, big.NewRat(1, 1)).Cmp(scaled) <= 0 {
			t.Fatalf("ParsePrice(%q) = %d micros, want %s rounded down", raw, got.Uint64(), scaled.FloatString(3))
		}
	})
}
