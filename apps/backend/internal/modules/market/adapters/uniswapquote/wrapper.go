package uniswapquote

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/sha3"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	wrapperOp          = "uniswapquote.Wrapper"
	wrapperCacheTTL    = 15 * time.Second
	entryPoint         = "0x0000000071727de22e5e9d8baf0edac6f37da032"
	permit2            = "0x000000000022d473030f116ddee9f6b43ac78ba3"
	executeSelector    = "e9ae5c53"
	tooLittleSelector  = "0x8b063d73"
	delegationPrefix   = "0xef0100"
	permit2AllowSlot   = 1
	permitLifetime     = 365 * 24 * time.Hour
	wordBytes          = 32
	revertWords        = 2
	DefaultDelegate    = "0xd6cedde84be40893d153be9d467cd6ad37875b28"
	SimulationGas      = 3_000_000
	fiatBalanceSlot    = 9
	fiatAllowanceSlot  = 10
	expirationShift    = 160
	maxRevertDataBytes = 4 + revertWords*wordBytes
)

type cashSlots struct{ balance, allowance byte }

func cashLayout(n chain.Network) (cashSlots, bool) {
	switch n {
	case chain.NetworkBase, chain.NetworkArbitrum, chain.NetworkEthereum:
		return cashSlots{balance: fiatBalanceSlot, allowance: fiatAllowanceSlot}, true
	case chain.NetworkSolana, chain.NetworkRobinhood:
	}
	return cashSlots{}, false
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
	key := wrapperKey{l.Network, strings.ToLower(l.TokenAddress), amount.String()}
	c.wrapper.cache[key] = wrapperEntry{quote: q, until: c.clock.Now().Add(wrapperCacheTTL)}
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

type rpcCall struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type rpcReply struct {
	Result *string `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    any    `json:"data"`
	} `json:"error"`
}

type overrideAccount struct {
	StateDiff map[string]string `json:"stateDiff"`
}

func (c *Client) simulate(
	ctx context.Context, l mc.Listing, amount money.Micros, router chain.EVMAddress, calldata []byte,
	attrs []slog.Attr,
) (*big.Int, bool, error) {
	slots, ok := cashLayout(l.Network)
	client, rpcOK := c.wrapper.rpc[l.Network]
	if !ok || !rpcOK {
		return nil, false, errs.New(errs.CodeNoRoute, wrapperOp, append(attrs, slog.String("reason", "unsupported"))...)
	}
	owner := c.swapper
	code, err := rpc(ctx, client, "eth_getCode", []any{owner.String(), "latest"}, attrs)
	want := delegationPrefix + strings.TrimPrefix(c.wrapper.delegate.String(), "0x")
	if err != nil || code.Result == nil || !strings.EqualFold(*code.Result, want) {
		return nil, false, errors.Join(err, errs.New(errs.CodeNoRoute, wrapperOp,
			append(attrs, slog.String("reason", "account_not_delegated"))...))
	}
	cash, _ := chain.ParseEVMAddress(strings.ToLower(l.Network.Cash().Address))
	value := word(new(big.Int).SetUint64(amount.Uint64()))
	expiry := new(big.Int).SetInt64(c.clock.Now().Add(permitLifetime).Unix())
	packed := new(big.Int).Or(new(big.Int).SetUint64(amount.Uint64()), expiry.Lsh(expiry, expirationShift))
	overrides := map[string]overrideAccount{
		cash.String(): {StateDiff: map[string]string{
			slotOf(owner[:], []byte{slots.balance}):                                value,
			slotOf(permit2Bytes(), mappingSlot(owner[:], []byte{slots.allowance})): value,
		}},
		permit2: {StateDiff: map[string]string{
			slotOf(router[:], mappingSlot(cash[:], mappingSlot(owner[:], []byte{permit2AllowSlot}))): word(packed),
		}},
	}
	call := map[string]string{
		"from": entryPoint, "to": owner.String(), "data": execute(router, calldata),
		"gas": "0x" + strconv.FormatUint(SimulationGas, 16),
	}
	reply, err := rpc(ctx, client, "eth_call", []any{call, "latest", overrides}, attrs)
	if err != nil {
		return nil, false, err
	}
	return outcome(reply, attrs)
}

func rpc(
	ctx context.Context,
	client *httpclient.Client,
	method string,
	params []any,
	attrs []slog.Attr,
) (rpcReply, error) {
	raw, _ := json.Marshal(rpcCall{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(ctx, req)
	if err != nil {
		return rpcReply{}, errs.Wrap(err, errs.CodeRPCUnavailable, wrapperOp, attrs...)
	}
	defer func() { _ = resp.Body.Close() }()
	var reply rpcReply
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&reply); err != nil {
		return rpcReply{}, errs.Wrap(err, errs.CodeDecodeFailed, wrapperOp, attrs...)
	}
	return reply, nil
}

func outcome(reply rpcReply, attrs []slog.Attr) (*big.Int, bool, error) {
	if reply.Error == nil && reply.Result != nil {
		return nil, true, nil
	}
	data := ""
	if reply.Error != nil {
		data, _ = reply.Error.Data.(string)
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(data, "0x"))
	if !strings.HasPrefix(data, tooLittleSelector) || err != nil || len(raw) != maxRevertDataBytes {
		return nil, false, errs.New(errs.CodeNoRoute, wrapperOp,
			append(attrs, slog.String("reason", "simulation_reverted"))...)
	}
	return new(big.Int).SetBytes(raw[4+wordBytes:]), false, nil
}

func execute(router chain.EVMAddress, calldata []byte) string {
	inner := slices.Concat(router[:], make([]byte, wordBytes), calldata)
	body := slices.Concat(inner, make([]byte, (wordBytes-len(inner)%wordBytes)%wordBytes))
	out := executeSelector + strings.Repeat("0", 2*wordBytes) + word(big.NewInt(2 * wordBytes))[2:] +
		word(big.NewInt(int64(len(inner))))[2:] + hex.EncodeToString(body)
	return "0x" + out
}

func word(v *big.Int) string { return "0x" + hex.EncodeToString(v.FillBytes(make([]byte, wordBytes))) }

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
