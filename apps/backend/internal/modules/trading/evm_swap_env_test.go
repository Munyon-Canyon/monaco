package trading_test

import (
	"context"
	"strconv"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

type evmFake struct {
	mu           sync.Mutex
	plan         app.EVMSwapPlan
	buildErr     error
	byKey        map[string]string
	sends        []string
	sentTxs      map[string][]multichain.EVMTransaction
	sendErr      map[string]error
	fills        []app.EVMFill
	fillErr      error
	afterSend    map[string]func()
	afterSendAny func(key string)
	builds       int
	specs        []app.EVMSwapSpec
	receipts     []app.EVMFillState
	receiptErr   error
	trail        []string
	chosen       app.EVMSwapPlan
	chooseErr    error
	choices      [][]app.EVMSwapCandidate
	inspects     []app.EVMFill
	inspectErr   error
	inspected    []inspectCall
}

type inspectCall struct {
	network platform.Network
	hash    string
	out     platform.EVMAddress
	owner   platform.EVMAddress
}

func (f *evmFake) Inspect(
	_ context.Context, network platform.Network, hash string, out, owner platform.EVMAddress,
) (app.EVMFill, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inspected = append(f.inspected, inspectCall{network: network, hash: hash, out: out, owner: owner})
	if err := f.inspectErr; err != nil {
		return app.EVMFill{}, err
	}
	if len(f.inspects) == 0 {
		return app.EVMFill{State: app.FillPending}, nil
	}
	fill := f.inspects[0]
	f.inspects = f.inspects[1:]
	return fill, nil
}

func newEVMFake(plan app.EVMSwapPlan) *evmFake {
	return &evmFake{
		plan: plan, byKey: map[string]string{}, sendErr: map[string]error{}, afterSend: map[string]func(){},
		sentTxs: map[string][]multichain.EVMTransaction{},
	}
}

func (f *evmFake) Build(_ context.Context, spec app.EVMSwapSpec) (app.EVMSwapPlan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.builds++
	f.specs = append(f.specs, spec)
	return f.plan, f.buildErr
}

func (f *evmFake) Choose(
	_ context.Context, _ app.EVMSwapSpec, candidates []app.EVMSwapCandidate,
) (app.EVMSwapPlan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.trail = append(f.trail, "choose")
	f.choices = append(f.choices, candidates)
	return f.chosen, f.chooseErr
}

func (f *evmFake) Send(_ context.Context, _ string, tx multichain.EVMTransaction, key string) (string, error) {
	f.mu.Lock()
	if err := f.sendErr[key]; err != nil {
		delete(f.sendErr, key)
		f.mu.Unlock()
		return "", err
	}
	f.sends = append(f.sends, key)
	f.sentTxs[key] = append(f.sentTxs[key], tx)
	f.trail = append(f.trail, "send:"+key)
	hash, ok := f.byKey[key]
	if !ok {
		hash = "0xhash" + strconv.Itoa(len(f.byKey)+1)
		f.byKey[key] = hash
	}
	hook, every := f.afterSend[key], f.afterSendAny
	delete(f.afterSend, key)
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	if every != nil {
		every(key)
	}
	return hash, nil
}

func (f *evmFake) Fill(
	_ context.Context, _ platform.Network, _ string, _, _ platform.EVMAddress,
) (app.EVMFill, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fillErr; err != nil {
		f.fillErr = nil
		return app.EVMFill{}, err
	}
	if len(f.fills) == 0 {
		return app.EVMFill{State: app.FillPending}, nil
	}
	fill := f.fills[0]
	f.fills = f.fills[1:]
	return fill, nil
}

func (f *evmFake) Receipt(_ context.Context, _ platform.Network, hash string) (app.EVMFillState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.trail = append(f.trail, "receipt:"+hash)
	if err := f.receiptErr; err != nil {
		f.receiptErr = nil
		return 0, err
	}
	if len(f.receipts) == 0 {
		return app.FillSucceeded, nil
	}
	state := f.receipts[0]
	f.receipts = f.receipts[1:]
	return state, nil
}

func (f *evmFake) steps() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.trail...)
}

func (f *evmFake) keys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sends...)
}

func (f *evmFake) hashes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.byKey)
}

type sentEVM struct {
	wallet  string
	tx      multichain.EVMTransaction
	sponsor bool
}

type recordingWallets struct {
	mu    sync.Mutex
	sent  []sentEVM
	byKey map[string]string
}

func (w *recordingWallets) CreateAppEVMWallet(context.Context, string) (multichain.EVMWallet, error) {
	return multichain.EVMWallet{}, nil
}

func (w *recordingWallets) SignEVMTransaction(
	context.Context,
	string,
	multichain.EVMTransaction,
	bool,
) ([]byte, error) {
	return nil, nil
}

func (w *recordingWallets) SendEVMTransaction(
	_ context.Context, wallet string, tx multichain.EVMTransaction, sponsor bool,
) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.sent = append(w.sent, sentEVM{wallet: wallet, tx: tx, sponsor: sponsor})
	if w.byKey == nil {
		w.byKey = map[string]string{}
	}
	hash, ok := w.byKey[tx.IdempotencyKey]
	if !ok {
		hash = "0xsent" + strconv.Itoa(len(w.byKey)+1)
		w.byKey[tx.IdempotencyKey] = hash
	}
	return hash, nil
}

func (w *recordingWallets) keys() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, 0, len(w.sent))
	for _, s := range w.sent {
		out = append(out, s.tx.IdempotencyKey)
	}
	return out
}
