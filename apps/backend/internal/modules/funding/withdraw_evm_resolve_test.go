package funding_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	evmSourceSig = chain.Signature(
		"5h6xBEauJ3PK6SWCZ1PGjBvj8vDdWG3KpwATGy1ARAXFSDwt8GFXM7W5Ncn16wmqokgpiKRLuS83KUxyZyv2sUYv")
	evmBaseHash = "0x2c9f6f0b1a3e0c4d1f5b6a7e8d9c0b1a2f3e4d5c6b7a8998877665544332211a"
)

func TestWithdrawEVM_SucceededConfirmsWithTheSolanaSignature(t *testing.T) {
	t.Parallel()
	actions := &privyActions{
		status: multichain.ActionStatus{State: multichain.ActionSucceeded, Steps: []multichain.ActionStep{
			{Network: chain.NetworkSolana, TxHash: string(evmSourceSig), State: multichain.StepConfirmed},
			{Network: chain.NetworkBase, TxHash: evmBaseHash, State: multichain.StepConfirmed},
		}},
	}
	f := newEVMFixture(t, actions)
	got, err := f.withdrawEVM(f.ctx(t.Context()))
	if err != nil {
		t.Fatal(err)
	}
	p := f.evmPoller(actions)
	if changed, err := f.tick(t, p); err != nil || changed != 1 {
		t.Fatalf("Tick = %d, %v", changed, err)
	}
	if changed, err := f.tick(t, p); err != nil || changed != 0 {
		t.Fatalf("second Tick = %d, %v", changed, err)
	}
	if row := f.evmRow(t); row.status != "confirmed" || row.signature != string(evmSourceSig) {
		t.Fatalf("row = %+v", row)
	}
	confirmed := f.payloads(t, events.TypeWithdrawalConfirmed)
	var e events.WithdrawalConfirmed
	if len(confirmed) != 1 || json.Unmarshal(confirmed[0], &e) != nil {
		t.Fatalf("withdrawal.confirmed = %d", len(confirmed))
	}
	want := events.WithdrawalConfirmed{
		V: 1, WithdrawalID: got.ID, UserID: f.user.ID.UUID(), AmountMicros: money.MicrosFromUint64(withdrawMicros),
		ToAddress: evmTo, TxSignature: evmSourceSig,
	}
	if e != want || len(actions.calls) != 1 {
		t.Fatalf("confirmed = %+v, want %+v, transfers = %d", e, want, len(actions.calls))
	}
	f.assertInFlight(t, "0")
}

func TestWithdrawEVM_EndsWithoutAConfirmedStepReleaseTheHold(t *testing.T) {
	t.Parallel()
	for state, code := range map[multichain.ActionState]string{
		multichain.ActionRejected: domain.WithdrawalTransferRejected,
		multichain.ActionFailed:   domain.WithdrawalTransferFailed,
	} {
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			actions := &privyActions{status: multichain.ActionStatus{State: state, Steps: []multichain.ActionStep{
				{Network: chain.NetworkSolana, State: multichain.StepFailed},
			}}}
			f := newEVMFixture(t, actions)
			got, err := f.withdrawEVM(f.ctx(t.Context()))
			if err != nil {
				t.Fatal(err)
			}
			if changed, err := f.tick(t, f.evmPoller(actions)); err != nil || changed != 1 {
				t.Fatalf("Tick = %d, %v", changed, err)
			}
			f.assertFailed(t, got, code)
			f.assertInFlight(t, "0")
			if len(actions.calls) != 1 {
				t.Fatalf("transfers = %d, want 1", len(actions.calls))
			}
		})
	}
}

func TestWithdrawEVM_FailedAfterTheSolanaStepKeepsTheHoldAndNeverRetries(t *testing.T) {
	t.Parallel()
	actions := &privyActions{
		status: multichain.ActionStatus{State: multichain.ActionFailed, Steps: []multichain.ActionStep{
			{Network: chain.NetworkSolana, TxHash: string(evmSourceSig), State: multichain.StepConfirmed},
			{Network: chain.NetworkBase, State: multichain.StepFailed},
		}},
	}
	f := newEVMFixture(t, actions)
	if _, err := f.withdrawEVM(f.ctx(t.Context())); err != nil {
		t.Fatal(err)
	}
	p := f.evmPoller(actions)
	for range 3 {
		f.clock.Advance(withdrawalUnsentAge + time.Second)
		if changed, err := f.tick(t, p); err != nil || changed != 0 {
			t.Fatalf("Tick = %d, %v", changed, err)
		}
	}
	if row := f.evmRow(t); row.status != "submitted" || row.signature != string(evmSourceSig) {
		t.Fatalf("row = %+v", row)
	}
	if len(actions.calls) != 1 || len(f.payloads(t, events.TypeWithdrawalFailed)) != 0 {
		t.Fatalf(
			"transfers = %d failed events = %d",
			len(actions.calls),
			len(f.payloads(t, events.TypeWithdrawalFailed)),
		)
	}
	f.assertInFlight(t, "2000000")
	landed, err := (app.WithdrawalOutflows{Reads: f.pool}).Submitted(t.Context(), f.user.ID)
	if err != nil || landed[evmSourceSig].String() != "2000000" {
		t.Fatalf("submitted = %v, %v, want the Solana step so the balance does not count it twice", landed, err)
	}
}

func TestWithdrawEVM_PendingWaits(t *testing.T) {
	t.Parallel()
	actions := &privyActions{status: multichain.ActionStatus{State: multichain.ActionPending}}
	f := newEVMFixture(t, actions)
	if _, err := f.withdrawEVM(f.ctx(t.Context())); err != nil {
		t.Fatal(err)
	}
	if changed, err := f.tick(t, f.evmPoller(actions)); err != nil || changed != 0 {
		t.Fatalf("Tick = %d, %v", changed, err)
	}
	if row := f.evmRow(t); row.status != "submitted" || row.signature != "" {
		t.Fatalf("row = %+v", row)
	}
	landed, err := (app.WithdrawalOutflows{Reads: f.pool}).Submitted(t.Context(), f.user.ID)
	if err != nil || len(landed) != 0 {
		t.Fatalf("submitted = %v, %v, want none before the Solana step", landed, err)
	}
}
