package trading_test

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/evmcalls"
)

const (
	liveOwner   = "0xf15262a7ee9d20fa690185d58e0eb2aad475bf32"
	liveToken   = "0xb2000000000000000000002d0ba3164cc74f58b7"
	liveUSDCEIP = "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"
)

func liveSwap() string {
	return evmcalls.Hex(evmcalls.Honest(liveOwner, baseCash, liveToken, 9_992_500, 995_000))
}

func liveSpec(t *testing.T) app.EVMSwapSpec {
	t.Helper()
	return app.EVMSwapSpec{
		Network: chain.NetworkBase, Owner: evmAddress(t, liveOwner), TokenIn: evmAddress(t, baseCash),
		TokenOut: evmAddress(t, liveToken), AmountIn: 9_992_500, MinOut: 990_000,
	}
}

func liveCheckApproval() string {
	unlimited := strings.Repeat("f", 64)
	return `{"requestId":"r1","approval":{"to":"` + liveUSDCEIP + `","from":"` + liveOwner +
		`","data":"0x095ea7b3` + word(permit2) + unlimited + `","value":"0x00","chainId":8453},"cancel":null}`
}

func liveQuote(t *testing.T, out, minAmount, permitData, permitTx string) string {
	t.Helper()
	recipient := evmAddress(t, liveOwner).Checksum()
	return `{"requestId":"r2","routing":"CLASSIC","isTokenApprovalApplicable":true,"permitData":` + permitData +
		`,"permitTransaction":` + permitTx + `,"quote":{"quoteId":"q1","chainId":8453,"swapper":"` + recipient +
		`","input":{"token":"` + liveUSDCEIP + `","amount":"9992500"},"output":{"token":"` + liveToken +
		`","amount":"` + out + `","recipient":"` + recipient + `"},"aggregatedOutputs":[{"amount":"` + out +
		`","token":"` + liveToken + `","recipient":"` + recipient + `","bps":10000,"minAmount":"` + minAmount +
		`"}]}}`
}

const (
	liveRouter = "0xd6145b2d3f379919e8cdeda7b97e37c4b2ca9c40"
	maxUint160 = "1461501637330902918203684832716283019655932542975"
)

func livePermitCall() string {
	return abiCall("87517c45", word(baseCash), word(liveRouter), word(maxUint160), word(expiresIn(30*24*time.Hour)))
}

func livePermitTx() string {
	return `{"to":"0x000000000022D473030F116dDEE9F6B43aC78BA3","from":"` + liveOwner + `","data":"` + livePermitCall() +
		`","value":"0x00","chainId":8453,"gasLimit":"55407"}`
}

func TestEVMVenue_buildsItsOwnExactApprovalInsteadOfTheUnlimitedOneAndAsksForThePermitAsATransaction(t *testing.T) {
	t.Parallel()
	e := newVenueEnv(t)
	e.script(
		liveCheckApproval(),
		liveQuote(t, "1000000", "1000000", "null", livePermitTx()),
		liveQuote(
			t,
			"1000100",
			"990200",
			"null",
			livePermitTx(),
		),
		swapReply(evmTxJSON(liveRouter, liveSwap(), "0x0")),
	)

	plan, err := e.venue.Build(t.Context(), liveSpec(t))

	if err != nil || len(plan.Approvals) != 2 || plan.MinOut != 990_200 {
		t.Fatalf("plan = %+v, %v, want our approval, the permit and the EIP-55 recipient's floor", plan, err)
	}
	exact, _ := hex.DecodeString("095ea7b3" + word(permit2) + word("9992500"))
	got := plan.Approvals[0]
	if got.To != evmAddress(t, baseCash) || string(got.Data) != string(exact) || got.Value.Cmp(new(big.Int)) != 0 {
		t.Fatalf("approval = %+v, want approve(Permit2, 9992500) on USDC, never the unlimited calldata", got)
	}
	ours, _ := hex.DecodeString("87517c45" + word(baseCash) + word(liveRouter) + word("9992500") +
		word(expiresIn(30*time.Minute)))
	theirs, _ := hex.DecodeString(strings.TrimPrefix(livePermitCall(), "0x"))
	permit := plan.Approvals[1]
	if permit.To != evmAddress(t, permit2) || string(permit.Data) != string(ours) ||
		string(permit.Data) == string(theirs) {
		t.Fatalf("permit = %+v, want approve(USDC, the router, 9992500, now+30m) on Permit2, never the API's", permit)
	}
	for _, i := range []int{1, 2} {
		if e.uniswap.calls[i].body["generatePermitAsTransaction"] != true {
			t.Fatalf("quote %d body = %+v, want generatePermitAsTransaction", i, e.uniswap.calls[i].body)
		}
	}
}

func TestEVMVenue_refusesAPermitItCanOnlySign(t *testing.T) {
	t.Parallel()
	e := newVenueEnv(t)
	permitData := `{"domain":{"name":"Permit2","chainId":8453},"types":{},"values":{}}`
	e.script(liveCheckApproval(), liveQuote(t, "1000000", "1000000", permitData, "null"),
		liveQuote(t, "1000100", "990200", permitData, "null"), swapReply(evmTxJSON(liveRouter, liveSwap(), "0x0")))

	_, err := e.venue.Build(t.Context(), liveSpec(t))

	if errs.CodeOf(err) != errs.CodeDecodeFailed ||
		!strings.Contains(fmt.Sprint(errs.Detail(err)), "permit_data_without_permit_transaction") {
		t.Fatalf("Build err = %v, want decode_failed for a signature-only permit", err)
	}
}
