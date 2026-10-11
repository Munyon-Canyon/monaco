package uniswapquote

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/sha3"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/evmsim"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	wrapperOp         = "uniswapquote.Wrapper"
	wrapperCacheTTL   = 15 * time.Second
	permit2           = "0x000000000022d473030f116ddee9f6b43ac78ba3"
	permit2AllowSlot  = 1
	permitLifetime    = 365 * 24 * time.Hour
	wordBytes         = 32
	DefaultDelegate   = "0xd6cedde84be40893d153be9d467cd6ad37875b28"
	fiatBalanceSlot   = 9
	fiatAllowanceSlot = 10
	usdgBalanceSlot   = 1
	usdgAllowanceSlot = 3
	expirationShift   = 160
)

type cashSlots struct{ balance, allowance byte }

func cashLayouts() map[chain.Network]cashSlots {
	fiat := cashSlots{balance: fiatBalanceSlot, allowance: fiatAllowanceSlot}
	return map[chain.Network]cashSlots{
		chain.NetworkBase:      fiat,
		chain.NetworkArbitrum:  fiat,
		chain.NetworkEthereum:  fiat,
		chain.NetworkRobinhood: {balance: usdgBalanceSlot, allowance: usdgAllowanceSlot},
	}
}

type Wrapper struct {
	rpc      map[chain.Network]*httpclient.Client
	delegate chain.EVMAddress
	mu       sync.Mutex
	cache    map[wrapperKey]wrapperEntry
}

type wrapperKey struct {
	network chain.Network
	token   string
	amount  string
}

type wrapperEntry struct {
	quote mc.Quote
	until time.Time
}

func NewWrapper(rpc map[chain.Network]*httpclient.Client, delegate chain.EVMAddress) *Wrapper {
	return &Wrapper{rpc: rpc, delegate: delegate, cache: map[wrapperKey]wrapperEntry{}}
}

func (c *Client) WithWrapper(w *Wrapper) *Client {
	c.wrapper = w
	return c
}

func (c *Client) cached(l mc.Listing, amount money.Micros) (mc.Quote, bool) {
	c.wrapper.mu.Lock()
	defer c.wrapper.mu.Unlock()
	e, ok := c.wrapper.cache[wrapperKey{l.Network, strings.ToLower(l.TokenAddress), amount.String()}]
	return e.quote, ok && c.clock.Now().Before(e.until)
}

func (c *Client) remember(l mc.Listing, amount money.Micros, q mc.Quote) {
	c.wrapper.mu.Lock()
	defer c.wrapper.mu.Unlock()
	now := c.clock.Now()
	for key, e := range c.wrapper.cache {
		if !now.Before(e.until) {
			delete(c.wrapper.cache, key)
		}
	}
	key := wrapperKey{l.Network, strings.ToLower(l.TokenAddress), amount.String()}
	c.wrapper.cache[key] = wrapperEntry{quote: q, until: now.Add(wrapperCacheTTL)}
}

type routeResult struct {
	quote mc.Quote
	err   error
}

func (c *Client) sponsored(
	ctx context.Context, l mc.Listing, amount money.Micros, req quoteRequest, attrs []slog.Attr,
) (mc.Quote, error) {
	if q, hit := c.cached(l, amount); hit {
		return q, nil
	}
	zero := json.Number("0")
	req.SlippageTolerance, req.PermitAsTx = &zero, true
	hookless := req
	hookless.HooksOptions = mc.NoHooks
	routes := []quoteRequest{req, hookless}
	found := make([]routeResult, len(routes))
	var g errgroup.Group
	for i, r := range routes {
		g.Go(func() error {
			q, raw, err := c.quote(ctx, l, mc.SideBuy, r, attrs)
			q.Options = mc.RouteOptions{HooksOptions: r.HooksOptions}
			if err == nil {
				q, err = c.penalize(ctx, l, amount, raw, q, attrs)
			}
			found[i] = routeResult{quote: q, err: err}
			return nil
		})
	}
	_ = g.Wait()
	best := found[0]
	for _, r := range found[1:] {
		if best.err != nil || r.err == nil && r.quote.GrossOutput.Cmp(best.quote.GrossOutput) > 0 {
			best = r
		}
	}
	if best.err != nil {
		return mc.Quote{}, best.err
	}
	c.remember(l, amount, best.quote)
	return best.quote, nil
}

type swapRequest struct {
	Quote json.RawMessage `json:"quote"`
}

type swapWire struct {
	ErrorCode string `json:"errorCode"`
	Swap      *struct {
		To    string `json:"to"`
		Data  string `json:"data"`
		Value string `json:"value"`
	} `json:"swap"`
}

func (c *Client) penalize(
	ctx context.Context, l mc.Listing, amount money.Micros, raw json.RawMessage, q mc.Quote, attrs []slog.Attr,
) (mc.Quote, error) {
	var swap swapWire
	if err := c.postJSON(ctx, "/v1/swap", swapRequest{Quote: raw}, &swap, attrs); err != nil {
		return mc.Quote{}, err
	}
	if swap.Swap == nil || strings.Trim(swap.Swap.Value, "0x") != "" {
		return mc.Quote{}, errs.New(errs.CodeDecodeFailed, wrapperOp, append(attrs, slog.String("reason", "swap"))...)
	}
	router, routerErr := chain.ParseEVMAddress(strings.ToLower(swap.Swap.To))
	calldata, dataErr := hex.DecodeString(strings.TrimPrefix(swap.Swap.Data, "0x"))
	if routerErr != nil || dataErr != nil {
		return mc.Quote{}, errs.New(errs.CodeDecodeFailed, wrapperOp, append(attrs, slog.String("reason", "swap"))...)
	}
	received, met, err := c.simulate(ctx, l, amount, router, calldata, attrs)
	if err != nil || met || received.Cmp(q.GrossOutput) >= 0 {
		return q, err
	}
	short := new(big.Int).Sub(q.GrossOutput, received)
	short.Mul(short, new(big.Int).SetUint64(amount.Uint64()))
	loss := short.Quo(short, q.GrossOutput).Uint64()
	q.GrossOutput = received
	q.Costs = append(q.Costs, mc.Cost{Kind: mc.CostPriceImpact, Micros: money.MicrosFromUint64(loss)})
	return q, nil
}

func (c *Client) simulate(
	ctx context.Context, l mc.Listing, amount money.Micros, router chain.EVMAddress, calldata []byte,
	attrs []slog.Attr,
) (*big.Int, bool, error) {
	slots, ok := cashLayouts()[l.Network]
	client, rpcOK := c.wrapper.rpc[l.Network]
	if !ok || !rpcOK {
		return nil, false, errs.New(errs.CodeNoRoute, wrapperOp, append(attrs, slog.String("reason", "unsupported"))...)
	}
	owner := c.swapper
	code, err := evmsim.Call(ctx, client, "eth_getCode", []any{owner.String(), "latest"}, attrs)
	if delegate, ok := evmsim.Delegation(code); err != nil || !ok || delegate != c.wrapper.delegate {
		return nil, false, errors.Join(err, errs.New(errs.CodeNoRoute, wrapperOp,
			append(attrs, slog.String("reason", "account_not_delegated"))...))
	}
	cash, _ := chain.ParseEVMAddress(strings.ToLower(l.Network.Cash().Address))
	value := evmsim.Word(new(big.Int).SetUint64(amount.Uint64()))
	expiry := new(big.Int).SetInt64(c.clock.Now().Add(permitLifetime).Unix())
	packed := new(big.Int).Or(new(big.Int).SetUint64(amount.Uint64()), expiry.Lsh(expiry, expirationShift))
	overrides := map[string]evmsim.Override{
		cash.String(): {StateDiff: map[string]string{
			slotOf(owner[:], []byte{slots.balance}):                                value,
			slotOf(permit2Bytes(), mappingSlot(owner[:], []byte{slots.allowance})): value,
		}},
		permit2: {StateDiff: map[string]string{
			slotOf(router[:], mappingSlot(cash[:], mappingSlot(owner[:], []byte{permit2AllowSlot}))): evmsim.Word(
				packed,
			),
		}},
	}
	return evmsim.Simulate(ctx, client, evmsim.Swap{Owner: owner, Router: router, Calldata: calldata}, overrides, attrs)
}

func padded(b []byte) []byte { return append(make([]byte, wordBytes-len(b)), b...) }

func mappingSlot(key, slot []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(padded(key))
	h.Write(padded(slot))
	return h.Sum(nil)
}

func slotOf(key, slot []byte) string { return "0x" + hex.EncodeToString(mappingSlot(key, slot)) }

func permit2Bytes() []byte {
	a, _ := chain.ParseEVMAddress(permit2)
	return a[:]
}
