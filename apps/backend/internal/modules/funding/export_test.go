package funding

import "github.com/monaco/monaco/apps/backend/internal/modules/funding/app"

func (m *Module) BuildTransfers() error {
	_, err := m.withdrawDeps(app.WalletReader{}).Transfers()
	return err
}
