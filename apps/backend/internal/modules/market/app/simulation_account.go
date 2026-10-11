package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math/big"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/evmsim"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

const (
	simulationWalletKey = "market:simulation-account"
	simulationAccountOp = "market.SimulationAccount"
)

type SimulationAccount struct {
	WalletID string
	Address  chain.EVMAddress
}

type AccountCode interface {
	Code(ctx context.Context, n chain.Network, account chain.EVMAddress) (string, error)
}

func LoadSimulationAccount(ctx context.Context, q sqlc.DBTX) (SimulationAccount, bool, error) {
	row, err := sqlc.New(q).SimulationAccount(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return SimulationAccount{}, false, nil
	}
	if err != nil {
		return SimulationAccount{}, false, errs.Wrap(err, errs.CodeOf(err), simulationAccountOp)
	}
	addr, _ := chain.ParseEVMAddress(row.Address)
	return SimulationAccount{WalletID: row.PrivyWalletID, Address: addr}, true, nil
}

type SimulationAccounts struct {
	q     sqlc.DBTX
	mu    sync.Mutex
	found *SimulationAccount
}

func NewSimulationAccounts(q sqlc.DBTX) *SimulationAccounts { return &SimulationAccounts{q: q} }

func (s *SimulationAccounts) SimulationAccount(ctx context.Context) (chain.EVMAddress, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.found != nil {
		return s.found.Address, true, nil
	}
	a, ok, err := LoadSimulationAccount(ctx, s.q)
	if ok {
		s.found = &a
	}
	return a.Address, ok, err
}

func SetUpSimulationAccount(
	ctx context.Context, q sqlc.DBTX, wallets multichain.EVMWallets, now time.Time,
) (SimulationAccount, bool, error) {
	if a, ok, err := LoadSimulationAccount(ctx, q); ok || err != nil {
		return a, false, err
	}
	w, err := wallets.CreateAppEVMWallet(ctx, simulationWalletKey)
	if err != nil {
		return SimulationAccount{}, false, err
	}
	inserted, err := sqlc.New(q).InsertSimulationAccount(ctx, sqlc.InsertSimulationAccountParams{
		PrivyWalletID: w.ID, Address: w.Address.String(), CreatedAt: now,
	})
	if err != nil {
		return SimulationAccount{}, false, errs.Wrap(err, errs.CodeOf(err), simulationAccountOp)
	}
	a, _, err := LoadSimulationAccount(ctx, q)
	return a, inserted == 1, err
}

type Delegation struct {
	Network  chain.Network
	Code     string
	Delegate chain.EVMAddress
	Sent     string
}

func (d Delegation) Delegated() bool { return d.Delegate != chain.EVMAddress{} }

func ReadDelegation(
	ctx context.Context, code AccountCode, n chain.Network, account chain.EVMAddress,
) (Delegation, error) {
	raw, err := code.Code(ctx, n, account)
	if err != nil {
		return Delegation{Network: n}, err
	}
	delegate, _ := evmsim.Delegation(evmsim.Reply{Result: &raw})
	return Delegation{Network: n, Code: raw, Delegate: delegate}, nil
}

func DelegateSimulationAccount(
	ctx context.Context, wallets multichain.EVMWallets, code AccountCode, a SimulationAccount, n chain.Network,
) (Delegation, error) {
	d, err := ReadDelegation(ctx, code, n, a.Address)
	if err != nil || d.Delegated() {
		return d, err
	}
	d.Sent, err = wallets.SendEVMTransaction(ctx, a.WalletID, multichain.EVMTransaction{
		Network: n, To: a.Address, Value: new(big.Int),
		IdempotencyKey: "market:simulation-account:delegate:" + string(n) + ":" + a.WalletID,
	}, true)
	if err != nil {
		return d, errs.Wrap(err, errs.CodeOf(err), simulationAccountOp, slog.String("network", string(n)))
	}
	return d, nil
}
