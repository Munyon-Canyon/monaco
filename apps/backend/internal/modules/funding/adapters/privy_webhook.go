package adapters

import (
	"context"
	"io"
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	PrivyWebhookRoute    = "POST /v1/webhooks/privy"
	privyWebhookActor    = "system:webhook.privy"
	privyWebhookMaxBytes = 64 << 10
)

type WebhookVerifier interface {
	VerifyWebhook(header http.Header, body []byte) (privy.WebhookEvent, error)
}

type TreasuryLookup interface {
	CabalFor(ctx context.Context, addr chain.SolanaAddress) (ids.CabalID, bool, error)
}

type ExternalDepositDetector interface {
	Handle(ctx context.Context, cmd app.DetectExternalDeposit) (app.DetectResult, error)
}

type PrivyWebhook struct {
	Verifier   WebhookVerifier
	Treasuries TreasuryLookup
	Detect     ExternalDepositDetector
	Problem    func(http.ResponseWriter, *http.Request, error)
}

func (h PrivyWebhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h.serve(r); err != nil {
		h.Problem(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h PrivyWebhook) serve(r *http.Request) error {
	const op = "funding.PrivyWebhook"
	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, privyWebhookMaxBytes))
	if err != nil {
		return errs.Wrap(err, errs.CodeUnauthorized, op)
	}
	event, err := h.Verifier.VerifyWebhook(r.Header, body)
	if err != nil {
		return errs.Wrap(err, errs.CodeUnauthorized, op)
	}
	if event.Deposit == nil {
		return nil
	}
	ctx := observability.WithActor(r.Context(), privyWebhookActor)
	cabalID, ok, err := h.Treasuries.CabalFor(ctx, event.Deposit.Recipient)
	if err != nil || !ok {
		return err
	}
	_, err = h.Detect.Handle(ctx, app.DetectExternalDeposit{
		Signature: event.Deposit.Signature, CabalID: cabalID, Treasury: event.Deposit.Recipient,
		Source: domain.SourceWebhook,
	})
	return err
}
