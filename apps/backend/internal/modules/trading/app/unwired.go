package app

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type (
	UnwiredCabals    struct{}
	UnwiredPauses    struct{}
	UnwiredProposals struct{}
)

func unwired(port string) error {
	return errs.New(errs.CodeUpstreamUnavailable, "trading.Unwired", slog.String("port", port))
}

func (UnwiredCabals) Status(context.Context, ids.CabalID) (cabalport.Status, error) {
	return "", unwired("Cabals.Status")
}

func (UnwiredCabals) SlippageBps(context.Context, ids.CabalID) (int32, error) {
	return 0, unwired("Cabals.SlippageBps")
}

func (UnwiredCabals) TreasuryWallet(context.Context, ids.CabalID) (cabalport.TreasuryWallet, error) {
	return cabalport.TreasuryWallet{}, unwired("Cabals.TreasuryWallet")
}

func (UnwiredPauses) IsPaused(context.Context, ids.CabalID) (fundingport.Pause, error) {
	return fundingport.Pause{}, unwired("Pauses.IsPaused")
}

func (UnwiredProposals) Status(context.Context, ids.ProposalID) (governanceport.Status, error) {
	return "", unwired("Proposals.Status")
}
