package multichain_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestPendingAnswersNotImplementedForEveryMethod(t *testing.T) {
	t.Parallel()
	p := mc.Pending{Ticket: "#4969"}
	ctx := context.Background()
	calls := map[string]func() error{
		"Transfer":           func() error { _, err := p.Transfer(ctx, "w", mc.TransferSpec{}, "k"); return err },
		"Action":             func() error { _, err := p.Action(ctx, "w", "a"); return err },
		"CreateAppEVMWallet": func() error { _, err := p.CreateAppEVMWallet(ctx, "k"); return err },
		"SignEVMTransaction": func() error {
			_, err := p.SignEVMTransaction(ctx, "w", mc.EVMTransaction{}, true)
			return err
		},
		"SendEVMTransaction": func() error {
			_, err := p.SendEVMTransaction(ctx, "w", mc.EVMTransaction{}, true)
			return err
		},
		"CreateAutomation": func() error { _, err := p.CreateAutomation(ctx); return err },
		"Attach":           func() error { return p.Attach(ctx, "w", "dest") },
		"Executions":       func() error { _, err := p.Executions(ctx, "w", ""); return err },
		"Reindex":          func() error { return p.Reindex(ctx, "w", chain.NetworkBase) },
		"Quote": func() error {
			_, err := p.Quote(ctx, mc.Listing{}, mc.SideBuy, money.Micros{})
			return err
		},
		"Best":   func() error { _, err := p.Best(ctx, mc.StockID{}, mc.SideBuy, money.Micros{}); return err },
		"Prices": func() error { _, err := p.Prices(ctx, nil); return err },
	}
	for name, call := range calls {
		if err := call(); errs.CodeOf(err) != errs.CodeNotImplemented {
			t.Errorf("%s error = %v, want not_implemented", name, err)
		}
	}
}
