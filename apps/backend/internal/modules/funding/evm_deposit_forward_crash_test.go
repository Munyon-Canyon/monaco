//go:build faultpoints

package funding_test

import (
	"strconv"
	"testing"

	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	fakemc "github.com/monaco/monaco/apps/backend/internal/testkit/fakes/multichain"
)

func TestEVMDepositForward_CrashAtEachCommitConvergesOnOneRowAndOneEventPerExecution(t *testing.T) {
	t.Parallel()
	page := []mc.Execution{execution("a", mc.ActionSucceeded), execution("b", mc.ActionPending)}
	for skip := range page {
		t.Run("commit "+strconv.Itoa(skip), func(t *testing.T) {
			t.Parallel()
			f := newForwardFixture(t)
			f.feed.Pages.Push(fakemc.Reply[mc.ExecutionPage]{Value: mc.ExecutionPage{Executions: page}})
			crashed := func() (p any) {
				defer func() { p = recover() }()
				_, _ = f.poller.Tick(faultpoint.ArmedAfter(f.ctx(t.Context()), faultpoint.BeforeCommit, skip))
				return nil
			}()
			if crashed != (faultpoint.Crash{Name: faultpoint.BeforeCommit}) {
				t.Fatalf("tick ended with %v, want a crash before commit", crashed)
			}
			f.tick(t, page...)
			f.tick(t, page...)
			if got := f.forwards(t, "pending"); got != 2 {
				t.Fatalf("pending rows = %d, want 2", got)
			}
			if got := f.forwarded(t); got != 1 {
				t.Fatalf("deposit.evm_forwarded events = %d, want 1", got)
			}
		})
	}
}
