package funding

import "github.com/monaco/monaco/apps/backend/internal/modules/funding/app"

func (m *Module) BuildTransfers() error {
	_, err := m.withdrawDeps(app.WalletReader{}).Transfers()
	return err
}

func (m *Module) BounceChain() lazyChain { return newLazyChain(m.deps.Config, m.deps.Clock) }
