package solana_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestMintConfig_classicSPLHasNoFee(t *testing.T) {
	t.Parallel()
	c, _, _ := overFakes(t)
	got, err := c.MintConfig(t.Context(), usdcMint)
	want := solana.MintConfig{Mint: usdc(), TokenProgram: chain.SPLProgram, MaxFee: money.NewBaseUnits(0, 6)}
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
		TransferFeeBps: 50, MaxFee: money.NewBaseUnits(5_000_000, 8),
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

func byMethod(body func(method string) string) *upstream {
	return &upstream{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&call)
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":`+body(call.Method)+`}`)
	})}
}
