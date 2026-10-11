package adapters_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func TestWalletActions_refuseToRunWithoutPrivyConfig(t *testing.T) {
	t.Parallel()
	actions := adapters.MultichainWithdrawalsWalletActions(config.Config{}, clock.Real{})
	if _, err := actions.Transfer(
		t.Context(),
		"wallet",
		multichain.TransferSpec{},
		"key",
	); errs.CodeOf(
		err,
	) != errs.CodeInvalidInput {
		t.Errorf("Transfer err = %v, want invalid_input", err)
	}
	if _, err := actions.Action(t.Context(), "wallet", "action"); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Errorf("Action err = %v, want invalid_input", err)
	}
}
