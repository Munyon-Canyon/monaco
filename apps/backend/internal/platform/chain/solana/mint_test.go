package solana_test

import (
	"encoding/json"
	"io"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const aaplx = chain.SolanaAddress("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")

func unscaled() solana.Multiplier { return solana.Multiplier{Num: 1, Den: 1} }

func TestMintConfig_classicSPLHasNoFee(t *testing.T) {
	t.Parallel()
	c, _, _ := overFakes(t)
	got, err := c.MintConfig(t.Context(), usdcMint)
	want := solana.MintConfig{
		Mint: usdc(), TokenProgram: chain.SPLProgram, MaxFee: money.NewBaseUnits(0, 6), UIMultiplier: unscaled(),
	}
	if err != nil || got != want {
		t.Fatalf("MintConfig = %+v, %v", got, err)
	}
}

func TestMintConfig_token2022ReadsTheFeeForTheCurrentEpoch(t *testing.T) {
	t.Parallel()
	c, u, srv := overFakes(t)
	got, err := c.MintConfig(t.Context(), feeMint)
	want := solana.MintConfig{
		Mint: chain.Mint{Address: feeMint, Decimals: 8}, TokenProgram: chain.SPL2022Program,
		TransferFeeBps: 50, MaxFee: money.NewBaseUnits(5_000_000, 8), UIMultiplier: unscaled(),
	}
	if err != nil || got != want {
		t.Fatalf("MintConfig = %+v, %v", got, err)
	}
	if n := len(u.requests()); n != 2 {
		t.Fatalf("%d calls, want getAccountInfo and getEpochInfo", n)
	}
	script(t, srv, fakes.Step{Route: "/rpc/getEpochInfo", Action: fakes.ActionFail, Status: 503, Times: 3})
	_, err = client(u).MintConfig(t.Context(), feeMint)
	wantCode(t, err, errs.CodeRPCUnavailable)
}

func TestMintConfig_olderFeeBeforeTheNewerEpoch(t *testing.T) {
	t.Parallel()
	body := func(method string) string {
		if method == "getEpochInfo" {
			return `{"epoch":10}`
		}
		return `{"value":{"owner":"` + string(
			chain.SPL2022Program,
		) + `","data":{"parsed":{"type":"mint","info":{"decimals":2,` +
			`"extensions":[{"extension":"transferFeeConfig","state":{"olderTransferFee":{"epoch":1,"maximumFee":9,` +
			`"transferFeeBasisPoints":25},"newerTransferFee":{"epoch":11,"maximumFee":99,"transferFeeBasisPoints":50}}}]}}}}}`
	}
	got, err := client(byMethod(body)).MintConfig(t.Context(), feeMint)
	if err != nil || got.TransferFeeBps != 25 || got.MaxFee != money.NewBaseUnits(9, 2) {
		t.Fatalf("MintConfig = %+v, %v", got, err)
	}
}

func TestMintConfig_newerFeeFromTheNewerEpochOn(t *testing.T) {
	t.Parallel()
	for epoch, want := range map[int]struct {
		bps uint16
		max uint64
	}{
		10: {25, 9},
		11: {50, 99},
		12: {50, 99},
	} {
		body := func(method string) string {
			if method == "getEpochInfo" {
				return `{"epoch":` + strconv.Itoa(epoch) + `}`
			}
			return `{"value":{"owner":"` + string(
				chain.SPL2022Program,
			) + `","data":{"parsed":{"type":"mint","info":{"decimals":2,` +
				`"extensions":[{"extension":"transferFeeConfig","state":{"olderTransferFee":{"epoch":1,"maximumFee":9,` +
				`"transferFeeBasisPoints":25},"newerTransferFee":{"epoch":11,"maximumFee":99,"transferFeeBasisPoints":50}}}]}}}}}`
		}
		got, err := client(byMethod(body)).MintConfig(t.Context(), feeMint)
		if err != nil || got.TransferFeeBps != want.bps || got.MaxFee != money.NewBaseUnits(want.max, 2) {
			t.Fatalf("epoch %d: MintConfig = %+v, %v; want %d bps and max fee %d", epoch, got, err, want.bps, want.max)
		}
	}
}

func TestMintConfig_cachesForAnHour(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(clock.Real{}.Now())
	u := &upstream{handler: fakes.New()}
	c := solana.New(testConfig(), clk, httpclient.WithTransport(u))
	for range 2 {
		if _, err := c.MintConfig(t.Context(), usdcMint); err != nil {
			t.Fatal(err)
		}
	}
	clk.Advance(59 * time.Minute)
	_, _ = c.MintConfig(t.Context(), usdcMint)
	if n := len(u.requests()); n != 1 {
		t.Fatalf("%d calls inside the hour, want 1", n)
	}
	clk.Advance(time.Minute)
	_, _ = c.MintConfig(t.Context(), usdcMint)
	if n := len(u.requests()); n != 2 {
		t.Fatalf("%d calls after the hour, want 2", n)
	}
}

func TestMintConfig_failures(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		body string
		want errs.Code
	}{
		"missing":       {`{"value":null}`, errs.CodeNotFound},
		"not a token":   {`{"value":{"owner":"` + string(chain.SystemProgram) + `","data":{"parsed":{"type":"mint"}}}}`, errs.CodeInvalidAddress},
		"token account": {`{"value":{"owner":"` + string(chain.SPLProgram) + `","data":{"parsed":{"type":"account"}}}}`, errs.CodeInvalidAddress},
		"bad fee state": {`{"value":{"owner":"` + string(chain.SPL2022Program) + `","data":{"parsed":{"type":"mint","info":{"extensions":[{"extension":"transferFeeConfig","state":[]}]}}}}}`, errs.CodeDecodeFailed},
	} {
		_, err := client(result(tc.body)).MintConfig(t.Context(), usdcMint)
		if errs.CodeOf(err) != tc.want {
			t.Fatalf("%s: err = %v, want %s", name, err, tc.want)
		}
	}
	_, err := client(replying(503, "")).MintConfig(t.Context(), usdcMint)
	wantCode(t, err, errs.CodeRPCUnavailable)
}

func scaledMint(multiplier, next string, nextFrom time.Time) string {
	return `{"value":{"owner":"` + string(chain.SPL2022Program) + `","data":{"parsed":{"type":"mint","info":{` +
		`"decimals":8,"extensions":[{"extension":"scaledUiAmountConfig","state":{"multiplier":"` + multiplier +
		`","newMultiplier":"` + next + `","newMultiplierEffectiveTimestamp":` +
		strconv.FormatInt(nextFrom.Unix(), 10) + `}}]}}}}}`
}

func TestMintConfig_xStocksReadTheScaledUIMultiplierInForce(t *testing.T) {
	t.Parallel()
	for mint, multiplier := range map[chain.SolanaAddress]solana.Multiplier{
		aaplx: {Num: 10_032_690_125_398_187, Den: 10_000_000_000_000_000},
		"XsDoVfqeBukxuZHWhdvWHBhgEHjGNst4MLodqsJHzoB": unscaled(),
		"XsCAXu7xTaZMG9b9KJhNWYapuvNjxPuE4SysZq8uvMq": {Num: 10_094_580_396_692_463, Den: 10_000_000_000_000_000},
	} {
		c, u, _ := overFakes(t)
		got, err := c.MintConfig(t.Context(), mint)
		want := solana.MintConfig{
			Mint: chain.Mint{Address: mint, Decimals: 8}, TokenProgram: chain.SPL2022Program,
			MaxFee: money.NewBaseUnits(0, 8), UIMultiplier: multiplier,
		}
		if err != nil || got != want {
			t.Fatalf("MintConfig(%s) = %+v, %v, want %+v, the multiplier whose effective time has passed", mint, got,
				err, want)
		}
		if methods := u.methods(); len(methods) != 1 || methods[0] != "getAccountInfo" {
			t.Fatalf("%s: calls = %v, want getAccountInfo only for a mint without a transfer fee", mint, methods)
		}
	}
}

func TestMintConfig_newMultiplierTakesOverAtItsEffectiveSecond(t *testing.T) {
	t.Parallel()
	start := clock.Real{}.Now().Truncate(time.Second)
	clk := testkit.NewClock(start)
	c := solana.New(testConfig(), clk, httpclient.WithTransport(result(scaledMint("1.5", "2", start.Add(time.Hour)))))
	got, err := c.MintConfig(t.Context(), usdcMint)
	if err != nil || got.UIMultiplier != (solana.Multiplier{Num: 3, Den: 2}) {
		t.Fatalf("an hour before the switch: MintConfig = %+v, %v, want the current 3/2", got, err)
	}
	clk.Advance(time.Hour)
	got, err = c.MintConfig(t.Context(), usdcMint)
	if err != nil || got.UIMultiplier != (solana.Multiplier{Num: 2, Den: 1}) {
		t.Fatalf("at the switch: MintConfig = %+v, %v, want the new 2/1", got, err)
	}
}

func TestMintConfig_readsTheMultiplierAsAnExactReducedFraction(t *testing.T) {
	t.Parallel()
	past := clock.Real{}.Now().Add(-time.Hour)
	for raw, want := range map[string]solana.Multiplier{
		"1":                     {Num: 1, Den: 1},
		"0.5":                   {Num: 1, Den: 2},
		"2.50":                  {Num: 5, Den: 2},
		"1.0070636829957968":    {Num: 629_414_801_872_373, Den: 625_000_000_000_000},
		"0.0000000000000000001": {Num: 1, Den: 10_000_000_000_000_000_000},
		"12345678901234567890":  {Num: 12_345_678_901_234_567_890, Den: 1},
	} {
		got, err := client(result(scaledMint("9", raw, past))).MintConfig(t.Context(), usdcMint)
		if err != nil || got.UIMultiplier != want {
			t.Fatalf("multiplier %q = %+v, %v, want %+v", raw, got.UIMultiplier, err, want)
		}
	}
}

func TestMintConfig_readsAnyDecimalMultiplierAsTheSameValueInLowestTerms(t *testing.T) {
	t.Parallel()
	ctx, past := t.Context(), clock.Real{}.Now().Add(-time.Hour)
	rapid.Check(t, func(rt *rapid.T) {
		digits := rapid.Uint64Range(1, math.MaxUint64).Draw(rt, "digits")
		scale := rapid.IntRange(0, 19).Draw(rt, "scale")
		padded := strconv.FormatUint(digits, 10)
		padded = strings.Repeat("0", max(scale+1-len(padded), 0)) + padded
		raw := padded
		if scale > 0 {
			raw = padded[:len(padded)-scale] + "." + padded[len(padded)-scale:]
		}
		got, err := client(result(scaledMint("9", raw, past))).MintConfig(ctx, usdcMint)
		if err != nil {
			rt.Fatalf("multiplier %q: %v", raw, err)
		}
		num, den := new(big.Int).SetUint64(got.UIMultiplier.Num), new(big.Int).SetUint64(got.UIMultiplier.Den)
		cross := new(big.Int).Mul(num, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil))
		if cross.Cmp(new(big.Int).Mul(new(big.Int).SetUint64(digits), den)) != 0 {
			rt.Fatalf("multiplier %q = %d/%d, not the same value", raw, num, den)
		}
		if new(big.Int).GCD(nil, nil, num, den).Cmp(big.NewInt(1)) != 0 {
			rt.Fatalf("multiplier %q = %d/%d, not in lowest terms", raw, num, den)
		}
	})
}

func TestMintConfig_refusesAMultiplierItCannotReadExactly(t *testing.T) {
	t.Parallel()
	past := clock.Real{}.Now().Add(-time.Hour)
	for _, raw := range []string{
		"", "0", "0.000", ".5", "-1", "+1", " 1", "1,5", "1.2.3", "1e-7", "NaN", "inf",
		"0.00000000000000000001", "18446744073709551616",
	} {
		_, err := client(result(scaledMint("1", raw, past))).MintConfig(t.Context(), usdcMint)
		if errs.CodeOf(err) != errs.CodeDecodeFailed {
			t.Fatalf("multiplier %q: err = %v, want decode_failed", raw, err)
		}
	}
	notAnObject := `{"value":{"owner":"` + string(chain.SPL2022Program) + `","data":{"parsed":{"type":"mint","info":{` +
		`"extensions":[{"extension":"scaledUiAmountConfig","state":[]}]}}}}}`
	_, err := client(result(notAnObject)).MintConfig(t.Context(), usdcMint)
	wantCode(t, err, errs.CodeDecodeFailed)
}

func byMethod(body func(method string) string) *upstream {
	return &upstream{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&call)
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":`+body(call.Method)+`}`)
	})}
}
