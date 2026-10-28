package funding_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/fundingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type stubQuotes struct {
	quote  app.TransferQuote
	err    error
	wallet string
	spec   multichain.TransferSpec
	calls  int
}

func (s *stubQuotes) QuoteTransfer(
	_ context.Context, wallet string, spec multichain.TransferSpec,
) (app.TransferQuote, error) {
	s.calls++
	s.wallet, s.spec = wallet, spec
	return s.quote, s.err
}

type stubBalances struct {
	available uint64
	err       error
}

func (s stubBalances) ForDisplay(context.Context, ids.UserID) (port.Balance, error) {
	return port.Balance{AvailableMicros: money.MicrosFromUint64(s.available)}, s.err
}

const previewAvailable = 500_000_000

func previewHTTP(quotes *stubQuotes, wallets signingWallet) (adapters.HTTP, context.Context) {
	return previewWith(quotes, wallets, stubBalances{available: previewAvailable}, nil)
}

func previewWith(
	quotes *stubQuotes, wallets signingWallet, balances stubBalances, fees *app.BridgeFees,
) (adapters.HTTP, context.Context) {
	user := ids.UserIDFrom(ids.Real{}.NewV7())
	ctx := auth.WithActor(context.Background(), auth.Actor{Kind: auth.ActorUser, ID: user.String()})
	return adapters.HTTP{Preview: app.PreviewWithdrawalHandler{
		Wallets: wallets, Quotes: quotes, Balances: balances, Fees: fees,
	}}, ctx
}

func preview(ctx context.Context, h adapters.HTTP, body api.WithdrawPreviewRequest) (api.WithdrawalPreview, error) {
	resp, err := h.PreviewWithdrawal(ctx, api.PreviewWithdrawalRequestObject{Body: &body})
	got, _ := resp.(api.PreviewWithdrawal200JSONResponse)
	return api.WithdrawalPreview(got), err
}

func evmPreviewBody() api.WithdrawPreviewRequest {
	kind, network := api.WithdrawPreviewRequestDestinationKindEvm, api.WithdrawPreviewRequestNetworkBase
	amount, to := "10000000", evmTo
	return api.WithdrawPreviewRequest{AmountMicros: &amount, DestinationKind: &kind, Network: &network, ToAddress: &to}
}

func networkBody(network api.WithdrawPreviewRequestNetwork, amount string) api.WithdrawPreviewRequest {
	kind := api.WithdrawPreviewRequestDestinationKindSolana
	if network != api.WithdrawPreviewRequestNetworkSolana {
		kind = api.WithdrawPreviewRequestDestinationKindEvm
	}
	return api.WithdrawPreviewRequest{AmountMicros: &amount, DestinationKind: &kind, Network: &network}
}

func str(v string) *string { return &v }

type previewView struct {
	Available, Fee, Max, Receive, Over, Min string
	FeeKnown, BelowMin, Expires             bool
	Mode                                    api.WithdrawalPreviewFeeMode
	Arrival                                 int
}

func deref(v *string) string {
	if v == nil {
		return "null"
	}
	return *v
}

func view(p api.WithdrawalPreview) previewView {
	return previewView{
		Available: p.AvailableMicros, Fee: deref(p.FeeMicros), Max: p.MaxAmountMicros,
		Receive: p.ReceiveMicros, Over: deref(p.OverMicros), Min: p.MinMicros,
		FeeKnown: p.FeeKnown, BelowMin: p.BelowMin, Expires: p.ExpiresAt != nil, Mode: p.FeeMode,
		Arrival: p.ArrivalSeconds,
	}
}

func TestPreviewWithdrawal_EVMReturnsPrivysEstimate(t *testing.T) {
	t.Parallel()
	expires := clock.Real{}.Now().Add(30 * time.Second).UTC().Truncate(time.Second)
	quotes := &stubQuotes{
		quote: app.TransferQuote{ReceiveMicros: money.MicrosFromUint64(9_992_500), ExpiresAt: expires},
	}
	h, ctx := previewHTTP(quotes, signingWallet{wallet: chain.Wallet{ID: "member-wallet"}})
	got, err := preview(ctx, h, evmPreviewBody())
	want := previewView{
		Available: "500000000", Fee: "7500", Max: "500000000", Receive: "9992500", Over: "0", Min: "1000000",
		FeeKnown: true, Expires: true, Mode: api.Included, Arrival: 120,
	}
	if err != nil || view(got) != want || !got.ExpiresAt.Equal(expires) {
		t.Fatalf("preview = %+v, %v, want %+v", got, err, want)
	}
	spec := multichain.TransferSpec{
		From: chain.NetworkSolana, To: chain.NetworkBase, Destination: evmTo,
		AmountMicros: money.MicrosFromUint64(10_000_000),
	}
	if quotes.wallet != "member-wallet" || quotes.spec != spec {
		t.Fatalf("quoted %q %+v, want member-wallet %+v", quotes.wallet, quotes.spec, spec)
	}
}

func TestPreviewWithdrawal_SolanaReceivesTheWholeAmountWithoutAQuote(t *testing.T) {
	t.Parallel()
	quotes := &stubQuotes{err: errs.New(errs.CodePrivyUnavailable, "test")}
	h, ctx := previewHTTP(quotes, signingWallet{err: errs.New(errs.CodeInternal, "test")})
	body := api.WithdrawPreviewRequest{AmountMicros: str("2000000"), ToAddress: str(string(withdrawTo))}
	got, err := preview(ctx, h, body)
	want := previewView{
		Available: "500000000", Fee: "0", Max: "500000000", Receive: "2000000", Over: "0", Min: "1000000",
		FeeKnown: true, Mode: api.OnTop, Arrival: 60,
	}
	if err != nil || view(got) != want || quotes.calls != 0 {
		t.Fatalf("preview = %+v, %v, quotes %d, want %+v", got, err, quotes.calls, want)
	}
}

func TestPreviewWithdrawal_EmptyKeypadAnswersTheFeeAndTheMax(t *testing.T) {
	t.Parallel()
	quotes := &stubQuotes{err: errs.New(errs.CodePrivyUnavailable, "test")}
	h, ctx := previewHTTP(quotes, signingWallet{err: errs.New(errs.CodeInternal, "test")})
	solana, kind := api.WithdrawPreviewRequestNetworkSolana, api.WithdrawPreviewRequestDestinationKindSolana
	want := previewView{
		Available: "500000000", Fee: "0", Max: "500000000", Receive: "0", Over: "null", Min: "1000000",
		FeeKnown: true, Mode: api.OnTop, Arrival: 60,
	}
	for name, body := range map[string]api.WithdrawPreviewRequest{
		"network and kind": {Network: &solana, DestinationKind: &kind},
		"no fields":        {},
	} {
		got, err := preview(ctx, h, body)
		if err != nil || view(got) != want || quotes.calls != 0 {
			t.Fatalf("%s: preview = %+v, %v, quotes %d, want %+v", name, got, err, quotes.calls, want)
		}
	}
}

func TestPreviewWithdrawal_LocalStatesNeedNoQuote(t *testing.T) {
	t.Parallel()
	for network, fee := range map[api.WithdrawPreviewRequestNetwork]string{
		api.WithdrawPreviewRequestNetworkSolana: "0", api.WithdrawPreviewRequestNetworkBase: "null",
		api.WithdrawPreviewRequestNetworkArbitrum: "null", api.WithdrawPreviewRequestNetworkEthereum: "null",
	} {
		t.Run(string(network), func(t *testing.T) {
			t.Parallel()
			quotes := &stubQuotes{err: errs.New(errs.CodePrivyUnavailable, "test")}
			h, ctx := previewHTTP(quotes, signingWallet{err: errs.New(errs.CodeInternal, "test")})
			small, err := preview(ctx, h, networkBody(network, "500000"))
			wantSmall := previewView{BelowMin: true, Receive: "0", Over: "null", Fee: fee}
			if got := view(small); err != nil || !sameLocalState(got, wantSmall) {
				t.Fatalf("below the minimum = %+v, %v", small, err)
			}
			over, err := preview(ctx, h, networkBody(network, "650000000"))
			wantOver := previewView{Receive: "0", Over: "150000000", Fee: fee}
			if got := view(over); err != nil || !sameLocalState(got, wantOver) || got.Max != "500000000" {
				t.Fatalf("over the max = %+v, %v", over, err)
			}
			if quotes.calls != 0 {
				t.Fatalf("%d bridge quotes for a locally decided state", quotes.calls)
			}
		})
	}
}

func sameLocalState(got, want previewView) bool {
	return got.BelowMin == want.BelowMin && got.Receive == want.Receive && got.Over == want.Over &&
		got.Fee == want.Fee && got.FeeKnown == (want.Fee != "null")
}

func TestPreviewWithdrawal_EVMWithoutAnAddressOrAnAmountSkipsTheQuote(t *testing.T) {
	t.Parallel()
	quotes := &stubQuotes{err: errs.New(errs.CodePrivyUnavailable, "test")}
	h, ctx := previewHTTP(quotes, signingWallet{err: errs.New(errs.CodeInternal, "test")})
	noAmount := evmPreviewBody()
	noAmount.AmountMicros = nil
	for name, body := range map[string]api.WithdrawPreviewRequest{
		"no address": networkBody(api.WithdrawPreviewRequestNetworkBase, "10000000"),
		"no amount":  noAmount,
	} {
		got, err := preview(ctx, h, body)
		if v := view(got); err != nil || v.Receive != "0" || v.FeeKnown || v.Fee != "null" || v.Expires ||
			v.Mode != api.Included || v.Max != "500000000" {
			t.Fatalf("%s: preview = %+v, %v", name, got, err)
		}
	}
	if quotes.calls != 0 {
		t.Fatalf("%d bridge quotes without an amount and an address", quotes.calls)
	}
}

func evmFee(ctx context.Context, h adapters.HTTP, network api.WithdrawPreviewRequestNetwork) string {
	body := networkBody(network, "")
	body.AmountMicros = nil
	got, _ := preview(ctx, h, body)
	return deref(got.FeeMicros)
}

func TestPreviewWithdrawal_EVMFeeIsRememberedForFiveMinutes(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(clock.Real{}.Now())
	quotes := &stubQuotes{quote: app.TransferQuote{
		ReceiveMicros: money.MicrosFromUint64(9_992_500), ExpiresAt: clk.Now().Add(30 * time.Second),
	}}
	h, ctx := previewWith(quotes, signingWallet{}, stubBalances{available: previewAvailable}, app.NewBridgeFees(clk))
	base, arbitrum := api.WithdrawPreviewRequestNetworkBase, api.WithdrawPreviewRequestNetworkArbitrum
	if fee := evmFee(ctx, h, base); fee != "null" {
		t.Fatalf("fee before any quote = %s", fee)
	}
	if _, err := preview(ctx, h, evmPreviewBody()); err != nil {
		t.Fatalf("quote: %v", err)
	}
	if fee := evmFee(ctx, h, base); fee != "7500" {
		t.Fatalf("cached fee = %s, want 7500", fee)
	}
	if fee := evmFee(ctx, h, arbitrum); fee != "null" {
		t.Fatalf("another network's fee = %s", fee)
	}
	over, err := preview(ctx, h, networkBody(base, "650000000"))
	if v := view(over); err != nil || v.Fee != "7500" || v.Over != "150000000" {
		t.Fatalf("over with a cached fee = %+v, %v", over, err)
	}
	clk.Advance(5 * time.Minute)
	if fee := evmFee(ctx, h, base); fee != "null" {
		t.Fatalf("fee after five minutes = %s", fee)
	}
}

func TestPreviewWithdrawal_QuoteLargerThanTheAmountMeansNoFee(t *testing.T) {
	t.Parallel()
	quotes := &stubQuotes{quote: app.TransferQuote{
		ReceiveMicros: money.MicrosFromUint64(10_000_001), ExpiresAt: clock.Real{}.Now().Add(time.Minute),
	}}
	h, ctx := previewHTTP(quotes, signingWallet{})
	got, err := preview(ctx, h, evmPreviewBody())
	if v := view(got); err != nil || v.Fee != "0" {
		t.Fatalf("preview = %+v, %v", got, err)
	}
}

func TestPreviewWithdrawal_OnTopMaxLeavesRoomForTheFee(t *testing.T) {
	t.Parallel()
	h, ctx := previewWith(&stubQuotes{}, signingWallet{}, stubBalances{available: 0}, nil)
	got, err := preview(ctx, h, networkBody(api.WithdrawPreviewRequestNetworkSolana, "2000000"))
	if v := view(got); err != nil || v.Max != "0" || v.Over != "2000000" || v.Receive != "0" {
		t.Fatalf("no cash = %+v, %v", got, err)
	}
}

func TestPreviewWithdrawal_Refusals(t *testing.T) {
	t.Parallel()
	bad := evmPreviewBody()
	bad.ToAddress = str(string(withdrawTo))
	notANumber := api.WithdrawPreviewRequest{AmountMicros: str("1.5")}
	for name, c := range map[string]struct {
		quotes   *stubQuotes
		wallets  signingWallet
		balances stubBalances
		body     api.WithdrawPreviewRequest
		anon     bool
		want     errs.Code
	}{
		"no caller":   {&stubQuotes{}, signingWallet{}, stubBalances{}, evmPreviewBody(), true, errs.CodeUnauthorized},
		"bad address": {&stubQuotes{}, signingWallet{}, stubBalances{}, bad, false, errs.CodeInvalidAddress},
		"bad amount":  {&stubQuotes{}, signingWallet{}, stubBalances{}, notANumber, false, errs.CodeInvalidInput},
		"balance read": {
			&stubQuotes{},
			signingWallet{},
			stubBalances{err: errs.New(errs.CodeInternal, "test")},
			evmPreviewBody(), false, errs.CodeInternal,
		},
		"wallet lookup": {
			&stubQuotes{},
			signingWallet{err: errs.New(errs.CodeInternal, "test")},
			stubBalances{available: previewAvailable},
			evmPreviewBody(), false, errs.CodeInternal,
		},
		"privy quote": {
			&stubQuotes{err: errs.New(errs.CodePrivyUnavailable, "test")},
			signingWallet{},
			stubBalances{available: previewAvailable},
			evmPreviewBody(), false, errs.CodePrivyUnavailable,
		},
		"bridge says insufficient": {
			&stubQuotes{err: errs.New(errs.CodeInsufficientFunds, "test")},
			signingWallet{},
			stubBalances{available: previewAvailable},
			evmPreviewBody(), false, errs.CodeInsufficientFunds,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, ctx := previewWith(c.quotes, c.wallets, c.balances, nil)
			if c.anon {
				ctx = context.Background()
			}
			if _, err := preview(ctx, h, c.body); errs.CodeOf(err) != c.want {
				t.Fatalf("preview err = %v, want %s", err, c.want)
			}
		})
	}
}
