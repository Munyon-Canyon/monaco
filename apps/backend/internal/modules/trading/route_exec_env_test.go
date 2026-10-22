package trading_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type steppingClock struct{ *testkit.Clock }

var _ clock.Clock = steppingClock{}

func (c steppingClock) After(d time.Duration) <-chan time.Time {
	c.Advance(d)
	ch := make(chan time.Time, 1)
	ch <- c.Now()
	return ch
}

type transferCall struct {
	wallet string
	spec   multichain.TransferSpec
	key    string
}

type privyFake struct {
	mu            sync.Mutex
	byKey         map[string]multichain.ActionID
	transfers     []transferCall
	states        []multichain.ActionState
	landed        map[multichain.ActionID]bool
	noArrival     bool
	delivered     uint64
	polls         int
	transferErr   error
	actionErr     error
	afterTransfer func()
	afterAction   func()
}

func newPrivyFake() *privyFake {
	return &privyFake{byKey: map[string]multichain.ActionID{}, landed: map[multichain.ActionID]bool{}}
}

func (p *privyFake) Transfer(
	_ context.Context, wallet string, spec multichain.TransferSpec, key string,
) (multichain.ActionID, error) {
	p.mu.Lock()
	if err := p.transferErr; err != nil {
		p.transferErr = nil
		p.mu.Unlock()
		return "", err
	}
	p.transfers = append(p.transfers, transferCall{wallet: wallet, spec: spec, key: key})
	id, ok := p.byKey[key]
	if !ok {
		id = multichain.ActionID("privy-action-" + strconv.Itoa(len(p.byKey)+1))
		p.byKey[key] = id
	}
	hook := p.afterTransfer
	p.afterTransfer = nil
	p.mu.Unlock()
	if hook != nil {
		hook()
	}
	return id, nil
}

func (p *privyFake) Action(
	_ context.Context, _ string, id multichain.ActionID,
) (multichain.ActionStatus, error) {
	p.mu.Lock()
	p.polls++
	if err := p.actionErr; err != nil {
		p.actionErr = nil
		p.mu.Unlock()
		return multichain.ActionStatus{}, err
	}
	state := multichain.ActionPending
	switch {
	case p.landed[id]:
		state = multichain.ActionSucceeded
	case len(p.states) > 0:
		state, p.states = p.states[0], p.states[1:]
	}
	p.landed[id] = state == multichain.ActionSucceeded
	noArrival, delivered := p.noArrival, p.delivered
	hook := p.afterAction
	p.afterAction = nil
	p.mu.Unlock()
	if hook != nil {
		hook()
	}
	status := multichain.ActionStatus{ID: id, State: state, Delivered: money.MicrosFromUint64(delivered)}
	if state == multichain.ActionSucceeded && delivered > 0 {
		status.Steps = []multichain.ActionStep{
			{Network: chain.NetworkBase, TxHash: "0xsource", State: multichain.StepConfirmed},
			{State: multichain.StepConfirmed},
		}
		return status, nil
	}
	if state == multichain.ActionSucceeded && !noArrival {
		status.Steps = []multichain.ActionStep{
			{Network: chain.NetworkBase, TxHash: "0xsource", State: multichain.StepConfirmed},
			{Network: chain.NetworkSolana, TxHash: arrivalPrefix + string(id), State: multichain.StepConfirmed},
		}
	}
	return status, nil
}

func (p *privyFake) sent(id multichain.ActionID) uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.transfers {
		if p.byKey[c.key] == id {
			return c.spec.AmountMicros.Uint64()
		}
	}
	return 0
}

const arrivalPrefix = "arrival:"

type evmArrivalsFake struct {
	mu      sync.Mutex
	short   uint64
	missing int
	err     error
	seen    []time.Time
}

func (f *evmArrivalsFake) Inbound(
	_ context.Context, _ chain.Network, _, _ chain.EVMAddress, since time.Time, atMost money.Micros, exclude []string,
) (app.EVMArrival, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen = append(f.seen, since)
	if err := f.err; err != nil {
		f.err = nil
		return app.EVMArrival{}, false, err
	}
	if f.missing > 0 {
		f.missing--
		return app.EVMArrival{}, false, nil
	}
	got := atMost.Uint64() - min(f.short, atMost.Uint64()-1)
	return app.EVMArrival{
		Amount: money.MicrosFromUint64(got),
		TxHash: fmt.Sprintf("0x%064x", len(exclude)+1),
	}, true, nil
}

type arrivalsFake struct {
	privy *privyFake
	short uint64
	over  uint64
	err   error
}

func (a *arrivalsFake) InboundAmount(
	_ context.Context, sig chain.Signature, _ chain.SolanaAddress, mint chain.Mint,
) (money.BaseUnits, error) {
	if err := a.err; err != nil {
		a.err = nil
		return money.BaseUnits{}, err
	}
	sent := a.privy.sent(multichain.ActionID(strings.TrimPrefix(string(sig), arrivalPrefix)))
	return money.NewBaseUnits(sent-min(a.short, sent)+a.over, mint.Decimals), nil
}

func (p *privyFake) script(states ...multichain.ActionState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.states = append(p.states, states...)
}

func (p *privyFake) calls() []transferCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]transferCall(nil), p.transfers...)
}

func (p *privyFake) actions() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.byKey)
}
