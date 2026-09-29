package replay

import "testing"

func EmptyLedgerChecks(t *testing.T) {
	t.Helper()
	saved := ledgerChecks
	ledgerChecks = nil
	t.Cleanup(func() { ledgerChecks = saved })
}
