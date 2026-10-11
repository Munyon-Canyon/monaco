package funding_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const evmTo = "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"

type evmTransfer struct {
	wallet string
	spec   multichain.TransferSpec
	key    string
}

type privyActions struct {
	mu          sync.Mutex
	calls       []evmTransfer
	created     map[string]multichain.ActionID
	transferErr error
	status      multichain.ActionStatus
}

func (p *privyActions) Transfer(
	_ context.Context, wallet string, spec multichain.TransferSpec, key string,
) (multichain.ActionID, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, evmTransfer{wallet: wallet, spec: spec, key: key})
	if p.transferErr != nil {
		return "", p.transferErr
	}
	if p.created == nil {
		p.created = map[string]multichain.ActionID{}
	}
	if _, ok := p.created[key]; !ok {
		p.created[key] = multichain.ActionID(fmt.Sprintf("privy-action-%d", len(p.created)+1))
	}
	return p.created[key], nil
}

func (p *privyActions) Action(_ context.Context, _ string, id multichain.ActionID) (multichain.ActionStatus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.status
	s.ID = id
	return s, nil
}

func (p *privyActions) actionsCreated() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.created)
}

func newEVMFixture(t *testing.T, actions *privyActions) *withdrawFixture {
	t.Helper()
	return newWithdrawFixture(t, 5_000_000, func(d *app.WithdrawDeps) { d.Actions = actions })
}

func (f *withdrawFixture) withdrawEVM(ctx context.Context) (app.WithdrawResult, error) {
	request, err := domain.ParseWithdrawalRequest(domain.RawWithdrawal{
		Amount: "2000000", Kind: "evm", Network: "base", To: evmTo,
	})
	if err != nil {
		return app.WithdrawResult{}, err
	}
	return f.handler.Handle(ctx, app.Withdraw{ID: ids.Real{}.NewV7(), UserID: f.user.ID, Request: request})
}

func (f *withdrawFixture) evmPoller(actions *privyActions) *app.WithdrawalPoller {
	return app.NewWithdrawalPoller(app.WithdrawalPollerDeps{
		UoW: f.uow, Reads: f.pool, Clock: f.clock, Chain: status(solana.StateNotFound, false, 0),
		Transfers: func() (app.Transfers, error) { return f.transfers, nil }, Hints: f.hints,
		UnsentAge: withdrawalUnsentAge, Actions: actions, TopUp: f.topUps,
		Wallets: signingWallet{wallet: chain.Wallet{ID: f.user.PrivyWalletID, Address: f.user.Address}},
	})
}

type evmRow struct {
	status, failCode, kind, network, to, evm, action, signature string
}

func (f *withdrawFixture) evmRow(t *testing.T) evmRow {
	t.Helper()
	var r evmRow
	if err := f.pool.QueryRow(t.Context(), `SELECT status, COALESCE(fail_code, ''), destination_kind, network,
		to_address, COALESCE(evm_address, ''), COALESCE(privy_action_id, ''), COALESCE(tx_signature, '')
		FROM withdrawals`).Scan(&r.status, &r.failCode, &r.kind, &r.network, &r.to, &r.evm, &r.action,
		&r.signature); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestWithdrawEVM_SendsOnePrivyTransferAndStoresTheAction(t *testing.T) {
	t.Parallel()
	actions := &privyActions{}
	f := newEVMFixture(t, actions)
	got, err := f.withdrawEVM(f.ctx(t.Context()))
	if err != nil || got.Status != domain.WithdrawalSubmitted || got.TxSignature != "" {
		t.Fatalf("Handle = %+v, %v", got, err)
	}
	want := evmTransfer{wallet: f.user.PrivyWalletID, key: got.ID.String(), spec: multichain.TransferSpec{
		From: chain.NetworkSolana, To: chain.NetworkBase, Destination: evmTo,
		AmountMicros: money.MicrosFromUint64(withdrawMicros),
	}}
	if len(actions.calls) != 1 || actions.calls[0] != want {
		t.Fatalf("transfers = %+v, want %+v", actions.calls, want)
	}
	if row := f.evmRow(t); row != (evmRow{
		status: "submitted", kind: "evm", network: "base", to: evmTo, evm: evmTo,
		action: "privy-action-1",
	}) {
		t.Fatalf("row = %+v", row)
	}
	if len(f.transfers.builds) != 0 || len(f.transfers.sent) != 0 {
		t.Fatalf("an EVM withdrawal used the Solana relayer: %+v", f.transfers.builds)
	}
	submitted := f.submittedEvents(t)
	if len(submitted) != 1 || submitted[0].ToAddress != evmTo || submitted[0].TxSignature != "" {
		t.Fatalf("withdrawal.submitted = %+v", submitted)
	}
	f.assertInFlight(t, "2000000")
}

func TestWithdrawEVM_RefusedTransferReleasesTheHold(t *testing.T) {
	t.Parallel()
	for _, code := range []errs.Code{errs.CodeInvalidInput, errs.CodeNotFound} {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()
			actions := &privyActions{transferErr: errs.New(code, "privy.Transfer")}
			f := newEVMFixture(t, actions)
			if _, err := f.withdrawEVM(f.ctx(t.Context())); errs.CodeOf(err) != code {
				t.Fatalf("Handle err = %v, want %s", err, code)
			}
			if row := f.evmRow(t); row.status != "failed" || row.failCode != string(code) || row.action != "" {
				t.Fatalf("row = %+v", row)
			}
			if len(f.submittedEvents(t)) != 0 || len(f.payloads(t, events.TypeWithdrawalFailed)) != 0 {
				t.Fatal("a refused transfer appended withdrawal events")
			}
			f.assertInFlight(t, "0")
		})
	}
}

func TestWithdrawEVM_UnknownTransferOutcomeRecoversWithTheSameKey(t *testing.T) {
	t.Parallel()
	actions := &privyActions{transferErr: errs.New(errs.CodePrivyUnavailable, "privy.Transfer")}
	f := newEVMFixture(t, actions)
	if _, err := f.withdrawEVM(f.ctx(t.Context())); errs.CodeOf(err) != errs.CodePrivyUnavailable {
		t.Fatalf("Handle err = %v, want privy_unavailable", err)
	}
	if row := f.evmRow(t); row.status != "created" {
		t.Fatalf("row = %+v", row)
	}
	f.assertInFlight(t, "2000000")
	actions.transferErr = nil
	p := f.evmPoller(actions)
	if changed, err := f.tick(t, p); err != nil || changed != 0 {
		t.Fatalf("Tick before the unsent age = %d, %v", changed, err)
	}
	f.clock.Advance(withdrawalUnsentAge + time.Second)
	if changed, err := f.tick(t, p); err != nil || changed != 1 {
		t.Fatalf("Tick = %d, %v", changed, err)
	}
	assertOneAction(t, f, actions)
}

func TestWithdrawEVM_RecoveryRefusedFailsUnsent(t *testing.T) {
	t.Parallel()
	actions := &privyActions{transferErr: errs.New(errs.CodePrivyUnavailable, "privy.Transfer")}
	f := newEVMFixture(t, actions)
	if _, err := f.withdrawEVM(f.ctx(t.Context())); err == nil {
		t.Fatal("Handle succeeded")
	}
	actions.transferErr = errs.New(errs.CodeInvalidInput, "privy.Transfer")
	f.clock.Advance(withdrawalUnsentAge + time.Second)
	if changed, err := f.tick(t, f.evmPoller(actions)); err != nil || changed != 1 {
		t.Fatalf("Tick = %d, %v", changed, err)
	}
	f.assertNotSent(t)
}

func TestWithdrawEVM_RecoveryStillUnavailableWaits(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodePrivyUnavailable, "privy.Transfer")
	actions := &privyActions{transferErr: boom}
	f := newEVMFixture(t, actions)
	if _, err := f.withdrawEVM(f.ctx(t.Context())); err == nil {
		t.Fatal("Handle succeeded")
	}
	f.clock.Advance(withdrawalUnsentAge + time.Second)
	if _, err := f.tick(t, f.evmPoller(actions)); !errors.Is(err, boom) {
		t.Fatalf("Tick = %v, want %v", err, boom)
	}
	if row := f.evmRow(t); row.status != "created" {
		t.Fatalf("row = %+v", row)
	}
}

func assertOneAction(t *testing.T, f *withdrawFixture, actions *privyActions) {
	t.Helper()
	if row := f.evmRow(t); row.status != "submitted" || row.action != "privy-action-1" {
		t.Fatalf("row = %+v", row)
	}
	if actions.actionsCreated() != 1 || len(actions.calls) != 2 || actions.calls[0].key != actions.calls[1].key {
		t.Fatalf("transfers = %+v, want one action under one idempotency key", actions.calls)
	}
	if n := len(f.submittedEvents(t)); n != 1 {
		t.Fatalf("withdrawal.submitted = %d", n)
	}
}

func TestWithdraw_BadEVMDestinationIsRefusedBeforeAnyHold(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		body string
		want errs.Code
	}{
		"bad checksum": {`{"amount_micros":"2000000","destination_kind":"evm","network":"base",` +
			`"to_address":"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAeD"}`, errs.CodeInvalidAddress},
		"solana address": {`{"amount_micros":"2000000","destination_kind":"evm","network":"base",` +
			`"to_address":"` + string(withdrawTo) + `"}`, errs.CodeInvalidAddress},
		"no network": {
			`{"amount_micros":"2000000","destination_kind":"evm","to_address":"` + evmTo + `"}`,
			errs.CodeInvalidNetwork,
		},
		"solana kind on base": {
			`{"amount_micros":"2000000","network":"base","to_address":"` + evmTo + `"}`,
			errs.CodeInvalidNetwork,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := flow15Scenario(t)
			member := flows.SeedWithdrawer(s)
			s.Given(scenario.AsSeededUser("member", member.ID)).
				When(scenario.Post("/v1/me/withdrawals", c.body)).
				Then(scenario.ExpectProblem(c.want), scenario.ExpectEvents(events.TypeWithdrawalSubmitted, 0))
			var rows int
			if err := s.DB().
				QueryRow(t.Context(), `SELECT count(*) FROM withdrawals`).
				Scan(&rows); err != nil ||
				rows != 0 {
				t.Fatalf("withdrawals = %d, %v, want none", rows, err)
			}
		})
	}
}
