package treasury_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/treasuryapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestFundHTTP_answersSubmitted(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	res, err := adapters.HTTP{Fund: h.handler}.FundCabal(h.actorCtx(), api.FundCabalRequestObject{
		Id: h.cabal.UUID(), Body: &api.FundRequest{AmountMicros: "5000000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	accepted, ok := res.(api.FundCabal202JSONResponse)
	if !ok || accepted.Status != api.FundTransferStatus(domain.FundSubmitted) {
		t.Fatalf("FundCabal = %#v, want 202 submitted", res)
	}
	if got := h.transfer(t, accepted.TransferId); got.AmountMicros != "5000000" {
		t.Fatalf("transfer = %+v, want 5000000", got)
	}
}

func TestFundHTTP_refusals(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	srv := adapters.HTTP{Fund: h.handler}
	for _, body := range []*api.FundRequest{nil, {AmountMicros: "999999"}, {AmountMicros: "1.5"}} {
		_, err := srv.FundCabal(h.actorCtx(), api.FundCabalRequestObject{Id: h.cabal.UUID(), Body: body})
		wantCode(t, err, errs.CodeInvalidInput)
	}
	h.stubs.member = false
	_, err := srv.FundCabal(h.actorCtx(), api.FundCabalRequestObject{
		Id: h.cabal.UUID(), Body: &api.FundRequest{AmountMicros: "5000000"},
	})
	wantCode(t, err, errs.CodeNotCabalMember)
	_, err = srv.FundCabal(h.ctx(), api.FundCabalRequestObject{Id: h.cabal.UUID()})
	wantCode(t, err, errs.CodeUnauthorized)
}

func TestTransfers_misconfiguredRelayerIsInternal(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	transfers := adapters.NewTransfers(f.cfg, f.clock)
	_, err := transfers.Build(f.ctx(), relayer.TransferSpec{Amount: money.NewBaseUnits(1, 6)})
	wantCode(t, err, errs.CodeInternal)
	wantCode(t, transfers.Broadcast(f.ctx(), relayer.SignedTx{}), errs.CodeInternal)
	cfg := f.cfg
	cfg.Privy = config.Privy{
		AppID: "app-fixture", AppSecret: "privy-app-5ecret", BaseURL: "http://127.0.0.1:1/privy",
		VerificationKey: fakes.PrivyVerificationKey(), AuthorizationPrivateKey: fakes.PrivyAuthorizationKeyConfig(),
		AuthorizationKeyID: fakes.PrivyAuthorizationKeyID,
	}
	cfg.Solana.RPCURL = "http://127.0.0.1:1/rpc/"
	cfg.Timeouts = config.Timeouts{RPC: time.Second, Privy: time.Second}
	_, err = adapters.NewTransfers(cfg, f.clock).Build(f.ctx(), relayer.TransferSpec{})
	wantCode(t, err, errs.CodeInternal)
	cfg.Relayer.PrivateKey = chain.EncodeBase58(fakes.FixtureKey("treasury-transfers"))
	transfers = adapters.NewTransfers(cfg, f.clock)
	_, err = transfers.Build(f.ctx(), relayer.TransferSpec{})
	wantCode(t, err, errs.CodeInvalidAddress)
	if err := transfers.Broadcast(f.ctx(), relayer.SignedTx{Bytes: []byte{1}}); err == nil {
		t.Fatal("Broadcast to an unreachable RPC = nil error")
	}
}
