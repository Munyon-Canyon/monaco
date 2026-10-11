package trading_test

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
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
	polls         int
	transferErr   error
	actionErr     error
	afterTransfer func()
	afterAction   func()
}

func newPrivyFake() *privyFake {
	return &privyFake{byKey: map[string]multichain.ActionID{}}
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
	if len(p.states) > 0 {
		state, p.states = p.states[0], p.states[1:]
	}
	hook := p.afterAction
	p.afterAction = nil
	p.mu.Unlock()
	if hook != nil {
		hook()
	}
	return multichain.ActionStatus{ID: id, State: state}, nil
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
