package funding_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/fundingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type stubQuotes struct {
	quote  app.TransferQuote
	err    error
	wallet string
	spec   multichain.TransferSpec
}

func (s *stubQuotes) QuoteTransfer(
	_ context.Context, wallet string, spec multichain.TransferSpec,
) (app.TransferQuote, error) {
	s.wallet, s.spec = wallet, spec
	return s.quote, s.err
}

func previewHTTP(quotes *stubQuotes, wallets signingWallet) (adapters.HTTP, context.Context) {
	user := ids.UserIDFrom(ids.Real{}.NewV7())
	ctx := auth.WithActor(context.Background(), auth.Actor{Kind: auth.ActorUser, ID: user.String()})
	return adapters.HTTP{Preview: app.PreviewWithdrawalHandler{Wallets: wallets, Quotes: quotes}}, ctx
}

func preview(ctx context.Context, h adapters.HTTP, body api.WithdrawRequest) (api.WithdrawalPreview, error) {
	resp, err := h.PreviewWithdrawal(ctx, api.PreviewWithdrawalRequestObject{Body: &body})
	got, _ := resp.(api.PreviewWithdrawal200JSONResponse)
	return api.WithdrawalPreview(got), err
}

func evmPreviewBody() api.WithdrawRequest {
	kind, network := api.WithdrawRequestDestinationKindEvm, api.WithdrawRequestNetworkBase
	return api.WithdrawRequest{AmountMicros: "10000000", DestinationKind: &kind, Network: &network, ToAddress: evmTo}
}

func TestPreviewWithdrawal_EVMReturnsPrivysEstimate(t *testing.T) {
	t.Parallel()
	expires := clock.Real{}.Now().Add(30 * time.Second).UTC().Truncate(time.Second)
	quotes := &stubQuotes{
		quote: app.TransferQuote{ReceiveMicros: money.MicrosFromUint64(9_992_500), ExpiresAt: expires},
	}
	h, ctx := previewHTTP(quotes, signingWallet{wallet: chain.Wallet{ID: "member-wallet"}})
	got, err := preview(ctx, h, evmPreviewBody())
	if err != nil || got.ReceiveMicros != "9992500" || got.ExpiresAt == nil || !got.ExpiresAt.Equal(expires) {
		t.Fatalf("preview = %+v, %v", got, err)
	}
	want := multichain.TransferSpec{
		From: chain.NetworkSolana, To: chain.NetworkBase, Destination: evmTo,
		AmountMicros: money.MicrosFromUint64(10_000_000),
	}
	if quotes.wallet != "member-wallet" || quotes.spec != want {
		t.Fatalf("quoted %q %+v, want member-wallet %+v", quotes.wallet, quotes.spec, want)
	}
}

func TestPreviewWithdrawal_SolanaReceivesTheWholeAmountWithoutAQuote(t *testing.T) {
	t.Parallel()
	quotes := &stubQuotes{err: errs.New(errs.CodePrivyUnavailable, "test")}
	h, ctx := previewHTTP(quotes, signingWallet{err: errs.New(errs.CodeInternal, "test")})
	got, err := preview(ctx, h, api.WithdrawRequest{AmountMicros: "2000000", ToAddress: string(withdrawTo)})
	if err != nil || got.ReceiveMicros != "2000000" || got.ExpiresAt != nil || quotes.wallet != "" {
		t.Fatalf("preview = %+v, %v, quoted %q", got, err, quotes.wallet)
	}
}

func TestPreviewWithdrawal_Refusals(t *testing.T) {
	t.Parallel()
	bad := evmPreviewBody()
	bad.ToAddress = string(withdrawTo)
	for name, c := range map[string]struct {
		quotes  *stubQuotes
		wallets signingWallet
		body    api.WithdrawRequest
		anon    bool
		want    errs.Code
	}{
		"no caller":     {&stubQuotes{}, signingWallet{}, evmPreviewBody(), true, errs.CodeUnauthorized},
		"bad address":   {&stubQuotes{}, signingWallet{}, bad, false, errs.CodeInvalidAddress},
		"wallet lookup": {&stubQuotes{}, signingWallet{err: errs.New(errs.CodeInternal, "test")}, evmPreviewBody(), false, errs.CodeInternal},
		"privy quote":   {&stubQuotes{err: errs.New(errs.CodePrivyUnavailable, "test")}, signingWallet{}, evmPreviewBody(), false, errs.CodePrivyUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, ctx := previewHTTP(c.quotes, c.wallets)
			if c.anon {
				ctx = context.Background()
			}
			if _, err := preview(ctx, h, c.body); errs.CodeOf(err) != c.want {
				t.Fatalf("preview err = %v, want %s", err, c.want)
			}
		})
	}
}
