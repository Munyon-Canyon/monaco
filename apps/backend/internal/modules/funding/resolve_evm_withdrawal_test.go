package funding_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type resolveRig struct {
	f       *withdrawFixture
	id      uuid.UUID
	logs    *testkit.Logs
	raw     func(cmd app.ResolveEVMWithdrawal) (app.EVMWithdrawalResolution, error)
	resolve func(cmd app.ResolveEVMWithdrawal) (app.EVMWithdrawalResolution, error)
}

func stuckEVM(
	t *testing.T, steps []multichain.ActionStep, state multichain.ActionState, chainState solana.Status,
) *resolveRig {
	t.Helper()
	actions := &privyActions{status: multichain.ActionStatus{State: state, Steps: steps}}
	f := newEVMFixture(t, actions)
	got, err := f.withdrawEVM(f.ctx(t.Context()))
	if err != nil {
		t.Fatal(err)
	}
	chainState.Signature = evmSourceSig
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	resolver := app.NewEVMWithdrawalResolver(app.EVMWithdrawalResolverDeps{
		UoW: f.uow, Reads: f.pool, Clock: f.clock, Chain: &stubStatuses{statuses: []solana.Status{chainState}},
		Wallets: signingWallet{wallet: chain.Wallet{ID: f.user.PrivyWalletID, Address: f.user.Address}},
		Actions: actions, Hints: f.hints,
	})
	return &resolveRig{
		f: f, id: got.ID, logs: logs,
		raw: func(cmd app.ResolveEVMWithdrawal) (app.EVMWithdrawalResolution, error) {
			return resolver.Handle(ctx, cmd)
		},
		resolve: func(cmd app.ResolveEVMWithdrawal) (app.EVMWithdrawalResolution, error) {
			cmd.ID, cmd.Actor, cmd.Reason = got.ID, "operator:ops-person", "support recovered the bridged funds"
			return resolver.Handle(ctx, cmd)
		},
	}
}

func solStep(state multichain.StepState) multichain.ActionStep {
	return multichain.ActionStep{Network: chain.NetworkSolana, TxHash: string(evmSourceSig), State: state}
}

func baseStep(state multichain.StepState) multichain.ActionStep {
	return multichain.ActionStep{Network: chain.NetworkBase, TxHash: evmBaseHash, State: state}
}

func chainStatus(state solana.State, failed bool) solana.Status {
	return solana.Status{State: state, Failed: failed}
}

func TestResolveEVMWithdrawal_FailedReleasesTheHoldWhenTheSolanaStepFailedOnChain(t *testing.T) {
	t.Parallel()
	r := stuckEVM(t, []multichain.ActionStep{solStep(multichain.StepFailed)}, multichain.ActionFailed,
		chainStatus(solana.StateFinalized, true))
	r.f.assertInFlight(t, "2000000")
	got, err := r.resolve(app.ResolveEVMWithdrawal{To: domain.WithdrawalFailed})
	if err != nil || got.Status != domain.WithdrawalFailed {
		t.Fatalf("resolve = %+v, %v", got, err)
	}
	if row := r.f.evmRow(t); row.status != "failed" || row.failCode != domain.WithdrawalSupportResolved {
		t.Fatalf("row = %+v, want failed with %s", row, domain.WithdrawalSupportResolved)
	}
	r.f.assertInFlight(t, "0")
	failed := r.f.payloads(t, events.TypeWithdrawalFailed)
	var e events.WithdrawalFailed
	if len(failed) != 1 || json.Unmarshal(failed[0], &e) != nil || e.Code != domain.WithdrawalSupportResolved {
		t.Fatalf("withdrawal.failed = %d events, last %+v", len(failed), e)
	}
	logged := string(r.logs.Bytes())
	for _, want := range []string{
		observability.FundingWithdrawalSupportResolved.Name, "operator:ops-person", "support recovered the bridged funds",
		"solana_chain_state",
	} {
		if !strings.Contains(logged, want) {
			t.Fatalf("log lacks %q:\n%s", want, logged)
		}
	}
}

func TestResolveEVMWithdrawal_FailedIsRefusedWhenTheChainShowsTheFundsLeft(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		steps  []multichain.ActionStep
		action multichain.ActionState
		chain  solana.Status
	}{
		"solana landed": {
			[]multichain.ActionStep{solStep(multichain.StepConfirmed)},
			multichain.ActionFailed,
			chainStatus(solana.StateFinalized, false),
		},
		"solana on chain": {
			[]multichain.ActionStep{solStep(multichain.StepPending)},
			multichain.ActionFailed,
			chainStatus(solana.StateFinalized, false),
		},
		"solana in flight": {
			[]multichain.ActionStep{solStep(multichain.StepPending)},
			multichain.ActionFailed,
			chainStatus(solana.StateProcessing, false),
		},
		"solana unseen": {
			[]multichain.ActionStep{solStep(multichain.StepPending)},
			multichain.ActionFailed,
			chainStatus(solana.StateNotFound, false),
		},
		"evm landed": {
			[]multichain.ActionStep{solStep(multichain.StepFailed), baseStep(multichain.StepConfirmed)},
			multichain.ActionFailed, chainStatus(solana.StateFinalized, true),
		},
		"evm pending": {
			[]multichain.ActionStep{solStep(multichain.StepFailed), baseStep(multichain.StepPending)},
			multichain.ActionFailed, chainStatus(solana.StateFinalized, true),
		},
		"action succeeded": {
			[]multichain.ActionStep{solStep(multichain.StepFailed)},
			multichain.ActionSucceeded,
			chainStatus(solana.StateFinalized, true),
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := stuckEVM(t, c.steps, c.action, c.chain)
			_, err := r.resolve(app.ResolveEVMWithdrawal{To: domain.WithdrawalFailed})
			if errs.CodeOf(err) != errs.CodeInvalidInput || !strings.Contains(err.Error(), "--override") {
				t.Fatalf("resolve err = %v, want invalid_input naming --override", err)
			}
			if row := r.f.evmRow(t); row.status != "submitted" {
				t.Fatalf("row = %+v, want it left submitted", row)
			}
			r.f.assertInFlight(t, "2000000")
		})
	}
}

func TestResolveEVMWithdrawal_OverrideBooksItAnywayAndRecordsIt(t *testing.T) {
	t.Parallel()
	r := stuckEVM(t, []multichain.ActionStep{solStep(multichain.StepPending)}, multichain.ActionFailed,
		chainStatus(solana.StateNotFound, false))
	if _, err := r.resolve(app.ResolveEVMWithdrawal{To: domain.WithdrawalFailed, Override: true}); err != nil {
		t.Fatal(err)
	}
	r.f.assertInFlight(t, "0")
	if logged := string(r.logs.Bytes()); !strings.Contains(logged, "override=true") &&
		!strings.Contains(logged, `"override":true`) {
		t.Fatalf("log does not record the override:\n%s", logged)
	}
}

func TestResolveEVMWithdrawal_ConfirmedBooksAWithdrawalThatLanded(t *testing.T) {
	t.Parallel()
	r := stuckEVM(t, []multichain.ActionStep{solStep(multichain.StepConfirmed), baseStep(multichain.StepConfirmed)},
		multichain.ActionFailed, chainStatus(solana.StateFinalized, false))
	if _, err := r.resolve(app.ResolveEVMWithdrawal{To: domain.WithdrawalConfirmed}); err != nil {
		t.Fatal(err)
	}
	if row := r.f.evmRow(t); row.status != "confirmed" || row.signature != string(evmSourceSig) {
		t.Fatalf("row = %+v, want confirmed with the Solana signature", row)
	}
	r.f.assertInFlight(t, "0")
	if n := len(r.f.payloads(t, events.TypeWithdrawalConfirmed)); n != 1 {
		t.Fatalf("withdrawal.confirmed = %d, want 1", n)
	}
}

func TestResolveEVMWithdrawal_ConfirmedIsRefusedWhenTheChainShowsItDidNotLand(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		steps []multichain.ActionStep
		chain solana.Status
	}{
		"solana failed": {
			[]multichain.ActionStep{solStep(multichain.StepFailed)},
			chainStatus(solana.StateFinalized, true),
		},
		"nothing landed": {
			[]multichain.ActionStep{solStep(multichain.StepConfirmed)},
			chainStatus(solana.StateFinalized, false),
		},
		"no signature": {nil, chainStatus(solana.StateNotFound, false)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := stuckEVM(t, c.steps, multichain.ActionFailed, c.chain)
			_, err := r.resolve(app.ResolveEVMWithdrawal{To: domain.WithdrawalConfirmed})
			if errs.CodeOf(err) != errs.CodeInvalidInput {
				t.Fatalf("resolve err = %v, want invalid_input", err)
			}
			if row := r.f.evmRow(t); row.status != "submitted" {
				t.Fatalf("row = %+v, want it left submitted", row)
			}
		})
	}
}

func TestResolveEVMWithdrawal_RunningTwiceBooksOnceAndTheOtherWayIsAConflict(t *testing.T) {
	t.Parallel()
	r := stuckEVM(t, []multichain.ActionStep{solStep(multichain.StepFailed)}, multichain.ActionFailed,
		chainStatus(solana.StateFinalized, true))
	for range 2 {
		if _, err := r.resolve(app.ResolveEVMWithdrawal{To: domain.WithdrawalFailed}); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(r.f.payloads(t, events.TypeWithdrawalFailed)); n != 1 {
		t.Fatalf("withdrawal.failed = %d, want 1", n)
	}
	_, err := r.resolve(app.ResolveEVMWithdrawal{To: domain.WithdrawalConfirmed, Override: true})
	if errs.CodeOf(err) != errs.CodeVersionConflict {
		t.Fatalf("opposite resolve err = %v, want version_conflict", err)
	}
}

func TestResolveEVMWithdrawal_RefusesBadInputAndUnknownWithdrawals(t *testing.T) {
	t.Parallel()
	r := stuckEVM(t, []multichain.ActionStep{solStep(multichain.StepFailed)}, multichain.ActionFailed,
		chainStatus(solana.StateFinalized, true))
	id := r.id
	for name, cmd := range map[string]app.ResolveEVMWithdrawal{
		"no operator":   {ID: id, To: domain.WithdrawalFailed, Actor: "ops", Reason: "why"},
		"no reason":     {ID: id, To: domain.WithdrawalFailed, Actor: "operator:ops"},
		"wrong target":  {ID: id, To: domain.WithdrawalSubmitted, Actor: "operator:ops", Reason: "why"},
		"unknown state": {ID: id, To: "paid", Actor: "operator:ops", Reason: "why"},
		"no id":         {To: domain.WithdrawalFailed, Actor: "operator:ops", Reason: "why"},
	} {
		if _, err := r.raw(cmd); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("%s: err = %v, want invalid_input", name, err)
		}
	}
	missing := app.ResolveEVMWithdrawal{
		ID: uuid.UUID{9}, To: domain.WithdrawalFailed, Actor: "operator:ops", Reason: "why",
	}
	if _, err := r.raw(missing); errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("unknown id err = %v, want not_found", err)
	}
	if row := r.f.evmRow(t); row.status != "submitted" {
		t.Fatalf("row = %+v, want it left submitted", row)
	}
}

type resolveHooks struct {
	statuses []solana.Status
	err      error
	before   func()
}

func (h *resolveHooks) SignatureStatuses(context.Context, []chain.Signature) ([]solana.Status, error) {
	if h.before != nil {
		h.before()
	}
	return h.statuses, h.err
}

type brokenActions struct {
	*privyActions
	err error
}

func (a brokenActions) Action(context.Context, string, multichain.ActionID) (multichain.ActionStatus, error) {
	return multichain.ActionStatus{}, a.err
}

type brokenWallets struct{ err error }

func (w brokenWallets) SigningWallet(context.Context, ids.UserID) (chain.Wallet, error) {
	return chain.Wallet{}, w.err
}

func (r *resolveRig) with(t *testing.T, mutate func(*app.EVMWithdrawalResolverDeps)) *app.EVMWithdrawalResolver {
	t.Helper()
	deps := app.EVMWithdrawalResolverDeps{
		UoW: r.f.uow, Reads: r.f.pool, Clock: r.f.clock, Chain: &resolveHooks{statuses: []solana.Status{
			{Signature: evmSourceSig, State: solana.StateFinalized, Failed: true},
		}},
		Wallets: signingWallet{wallet: chain.Wallet{ID: r.f.user.PrivyWalletID, Address: r.f.user.Address}},
		Actions: &privyActions{status: multichain.ActionStatus{
			State: multichain.ActionFailed, Steps: []multichain.ActionStep{solStep(multichain.StepFailed)},
		}},
		Hints: r.f.hints,
	}
	mutate(&deps)
	return app.NewEVMWithdrawalResolver(deps)
}

func (r *resolveRig) handle(resolver *app.EVMWithdrawalResolver, to domain.WithdrawalStatus, override bool) error {
	_, err := resolver.Handle(context.Background(), app.ResolveEVMWithdrawal{
		ID: r.id, To: to, Reason: "why", Actor: "operator:ops", Override: override,
	})
	return err
}

func TestResolveEVMWithdrawal_AFailedChainReadStopsItUnlessOverridden(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeRPCUnavailable, "test")
	r := stuckEVM(t, []multichain.ActionStep{solStep(multichain.StepFailed)}, multichain.ActionFailed,
		chainStatus(solana.StateFinalized, true))
	for name, mutate := range map[string]func(*app.EVMWithdrawalResolverDeps){
		"wallet":  func(d *app.EVMWithdrawalResolverDeps) { d.Wallets = brokenWallets{err: boom} },
		"action":  func(d *app.EVMWithdrawalResolverDeps) { d.Actions = brokenActions{err: boom} },
		"chain":   func(d *app.EVMWithdrawalResolverDeps) { d.Chain = &resolveHooks{err: boom} },
		"no rows": func(d *app.EVMWithdrawalResolverDeps) { d.Chain = &resolveHooks{} },
	} {
		resolver := r.with(t, mutate)
		if err := r.handle(resolver, domain.WithdrawalFailed, false); err == nil {
			t.Fatalf("%s: err = nil, want the failed read", name)
		}
		if row := r.f.evmRow(t); row.status != "submitted" {
			t.Fatalf("%s: row = %+v, want it left submitted", name, row)
		}
	}
	if err := r.handle(r.with(t, func(d *app.EVMWithdrawalResolverDeps) { d.Chain = &resolveHooks{err: boom} }),
		domain.WithdrawalFailed, true); err != nil {
		t.Fatalf("override with an unreadable chain: %v", err)
	}
	r.f.assertInFlight(t, "0")
}

func TestResolveEVMWithdrawal_UsesTheSignatureTheRowAlreadyHolds(t *testing.T) {
	t.Parallel()
	r := stuckEVM(t, nil, multichain.ActionFailed, chainStatus(solana.StateFinalized, true))
	if _, err := r.f.pool.Exec(
		t.Context(),
		`UPDATE withdrawals SET tx_signature = $1`,
		string(evmSourceSig),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := r.resolve(app.ResolveEVMWithdrawal{To: domain.WithdrawalFailed}); err != nil {
		t.Fatal(err)
	}
}

func TestResolveEVMWithdrawal_AReadStoreFailureMovesNothing(t *testing.T) {
	t.Parallel()
	r := stuckEVM(t, []multichain.ActionStep{solStep(multichain.StepFailed)}, multichain.ActionFailed,
		chainStatus(solana.StateFinalized, true))
	if _, err := r.f.pool.Exec(t.Context(), `ALTER TABLE withdrawals RENAME TO withdrawals_down`); err != nil {
		t.Fatal(err)
	}
	resolver := r.with(t, func(*app.EVMWithdrawalResolverDeps) {})
	if err := r.handle(resolver, domain.WithdrawalFailed, false); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err = %v, want internal", err)
	}
}

func TestResolveEVMWithdrawal_ARowThatMovedMeanwhileIsAConflict(t *testing.T) {
	t.Parallel()
	r := stuckEVM(t, []multichain.ActionStep{solStep(multichain.StepFailed)}, multichain.ActionFailed,
		chainStatus(solana.StateFinalized, true))
	hooks := &resolveHooks{
		statuses: []solana.Status{{Signature: evmSourceSig, State: solana.StateFinalized, Failed: true}},
	}
	hooks.before = func() {
		if _, err := r.f.pool.Exec(
			context.Background(),
			`UPDATE withdrawals SET status = 'failed', fail_code = 'x', completed_at = now()`,
		); err != nil {
			panic(err)
		}
	}
	err := r.handle(
		r.with(t, func(d *app.EVMWithdrawalResolverDeps) { d.Chain = hooks }),
		domain.WithdrawalFailed,
		false,
	)
	if errs.CodeOf(err) != errs.CodeVersionConflict {
		t.Fatalf("err = %v, want version_conflict", err)
	}
}

func TestResolveEVMWithdrawal_AWriteFailureRollsBack(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		to     domain.WithdrawalStatus
		column string
		steps  []multichain.ActionStep
		chain  solana.Status
	}{
		"fail": {
			domain.WithdrawalFailed, "completed_at",
			[]multichain.ActionStep{solStep(multichain.StepFailed)},
			chainStatus(solana.StateFinalized, true),
		},
		"confirm": {
			domain.WithdrawalConfirmed, "completed_at",
			[]multichain.ActionStep{solStep(multichain.StepConfirmed), baseStep(multichain.StepConfirmed)},
			chainStatus(solana.StateFinalized, false),
		},
		"signature": {
			domain.WithdrawalConfirmed, "tx_signature",
			[]multichain.ActionStep{solStep(multichain.StepConfirmed), baseStep(multichain.StepConfirmed)},
			chainStatus(solana.StateFinalized, false),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := stuckEVM(t, c.steps, multichain.ActionFailed, c.chain)
			c.chain.Signature = evmSourceSig
			hooks := &resolveHooks{statuses: []solana.Status{c.chain}}
			hooks.before = func() {
				_, err := r.f.pool.Exec(
					context.Background(),
					`ALTER TABLE withdrawals RENAME COLUMN `+c.column+` TO gone`,
				)
				if err != nil {
					panic(err)
				}
			}
			resolver := r.with(t, func(d *app.EVMWithdrawalResolverDeps) {
				d.Chain = hooks
				d.Actions = &privyActions{
					status: multichain.ActionStatus{State: multichain.ActionFailed, Steps: c.steps},
				}
			})
			if err := r.handle(resolver, c.to, true); err == nil {
				t.Fatal("err = nil, want the failed write")
			}
		})
	}
}
