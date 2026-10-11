package privyfake

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

var (
	_ mc.EVMWallets        = (*EVMWallets)(nil)
	_ mc.DepositForwarding = (*Forwarding)(nil)
)

type EVMWallets struct {
	testkit.Faults
	mu      sync.Mutex
	byKey   map[string]mc.EVMWallet
	sent    map[string]string
	creates int
}

func (w *EVMWallets) CreateAppEVMWallet(_ context.Context, key string) (mc.EVMWallet, error) {
	if err := w.Check("CreateAppEVMWallet"); err != nil {
		return mc.EVMWallet{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.creates++
	if got, ok := w.byKey[key]; ok {
		return got, nil
	}
	if w.byKey == nil {
		w.byKey = map[string]mc.EVMWallet{}
	}
	sum := sha256.Sum256([]byte("evm:" + key))
	wallet := mc.EVMWallet{ID: "wallet-evm-" + hex.EncodeToString(sum[:6]), Address: chain.EVMAddress(sum[:20])}
	w.byKey[key] = wallet
	return wallet, nil
}

func (w *EVMWallets) SignEVMTransaction(context.Context, string, mc.EVMTransaction, bool) ([]byte, error) {
	return nil, errs.New(errs.CodeNotImplemented, "privyfake.EVMWallets.SignEVMTransaction")
}

func (w *EVMWallets) SendEVMTransaction(
	_ context.Context,
	walletID string,
	tx mc.EVMTransaction,
	_ bool,
) (string, error) {
	if err := w.Check("SendEVMTransaction"); err != nil {
		return "", err
	}
	if tx.IdempotencyKey == "" {
		return "", errs.New(errs.CodeInvalidInput, "privyfake.EVMWallets.SendEVMTransaction",
			slog.String("reason", "idempotency key"))
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.sent == nil {
		w.sent = map[string]string{}
	}
	if hash, ok := w.sent[tx.IdempotencyKey]; ok {
		return hash, nil
	}
	sum := sha256.Sum256([]byte(walletID + "/" + tx.IdempotencyKey))
	w.sent[tx.IdempotencyKey] = "0x" + hex.EncodeToString(sum[:])
	return w.sent[tx.IdempotencyKey], nil
}

func (w *EVMWallets) Creates() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.creates
}

func (w *EVMWallets) Sends() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.sent)
}

type Forwarding struct {
	testkit.Faults
	OnAttach func()
	mu       sync.Mutex
	attached map[string]chain.SolanaAddress
	attaches int
}

func (f *Forwarding) CreateAutomation(context.Context) (mc.AutomationID, error) {
	if err := f.Check("CreateAutomation"); err != nil {
		return "", err
	}
	return "automation-deposit-forward", nil
}

func (f *Forwarding) Attach(_ context.Context, walletID string, dest chain.SolanaAddress) error {
	if err := f.Check("Attach"); err != nil {
		return err
	}
	if f.OnAttach != nil {
		f.OnAttach()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.attached == nil {
		f.attached = map[string]chain.SolanaAddress{}
	}
	f.attaches++
	f.attached[walletID] = dest
	return nil
}

func (f *Forwarding) Executions(context.Context, string, string) (mc.ExecutionPage, error) {
	if err := f.Check("Executions"); err != nil {
		return mc.ExecutionPage{}, err
	}
	return mc.ExecutionPage{Executions: []mc.Execution{}}, nil
}

func (f *Forwarding) Reindex(context.Context, string, chain.Network) error { return f.Check("Reindex") }

func (f *Forwarding) Attaches() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.attaches
}

func (f *Forwarding) Destination(walletID string) chain.SolanaAddress {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.attached[walletID]
}
