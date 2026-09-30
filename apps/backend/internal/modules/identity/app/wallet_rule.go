package app

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type WalletRule struct {
	wallets       MemberWallets
	signerMissing metric.Int64Counter
}

func NewWalletRule(wallets MemberWallets, meters metric.MeterProvider) (*WalletRule, error) {
	counter, err := meters.Meter("github.com/monaco/monaco/apps/backend/internal/modules/identity").Int64Counter(
		"identity_wallet_signer_missing_total",
		metric.WithDescription("Member wallets reused at sign-in without the app signer."))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "identity.NewWalletRule")
	}
	return &WalletRule{wallets: wallets, signerMissing: counter}, nil
}

func (r *WalletRule) Resolve(ctx context.Context, user PrivyUserID, stored *domain.Wallet) (domain.Wallet, error) {
	if stored != nil {
		return *stored, nil
	}
	found, err := r.wallets.FindOrCreate(ctx, user)
	if err != nil {
		return domain.Wallet{}, err
	}
	if !found.HasAppSigner {
		observability.Degraded(ctx, observability.IdentityWalletSignerMissing,
			slog.String("wallet_id", found.PrivyWalletID))
		r.signerMissing.Add(ctx, 1)
	}
	return found.Wallet, nil
}
