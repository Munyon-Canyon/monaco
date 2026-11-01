package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	evmArrivalsOp        = "trading.EVMArrivals.Inbound"
	arrivalLookback      = 2 * time.Minute
	arrivalChunksPerTick = 30
	arrivalDefaultDepth  = 32
	arrivalPace          = 250 * time.Millisecond
	arrivalBackoffFirst  = 15 * time.Second
	arrivalBackoffCap    = 60 * time.Second
	evmTopicHexChars     = 64
)

var _ app.EVMArrivals = (*EVMArrivals)(nil)

func safeDepth(network chain.Network) int64 {
	depths := map[chain.Network]int64{
		chain.NetworkBase:      30,
		chain.NetworkRobinhood: 240,
		chain.NetworkArbitrum:  240,
	}
	if depth, ok := depths[network]; ok {
		return depth
	}
	return arrivalDefaultDepth
}

type EVMArrivals struct {
	rpc       map[chain.Network]*httpclient.Client
	chunk     int64
	clock     clock.Clock
	mu        sync.Mutex
	cursors   map[arrivalKey]arrivalCursor
	throttled map[chain.Network]arrivalBackoff
}

type arrivalBackoff struct {
	until time.Time
	wait  time.Duration
}

type arrivalKey struct {
	network chain.Network
	owner   chain.EVMAddress
	since   int64
}

type arrivalCursor struct {
	next  int64
	chunk int64
}

func NewEVMArrivals(rpc map[chain.Network]*httpclient.Client, maxBlocks int64, clk clock.Clock) *EVMArrivals {
	return &EVMArrivals{
		rpc: rpc, chunk: max(maxBlocks, 1), clock: clk,
		cursors: map[arrivalKey]arrivalCursor{}, throttled: map[chain.Network]arrivalBackoff{},
	}
}

type pacer struct {
	clock   clock.Clock
	started bool
}

func (p *pacer) wait() {
	if p.started {
		<-p.clock.After(arrivalPace)
	}
	p.started = true
}

type arrivalLog struct {
	Address string   `json:"address"`
	Topics  []string `json:"topics"`
	Data    string   `json:"data"`
	TxHash  string   `json:"transactionHash"`
	Removed bool     `json:"removed"`
}

type logFilter struct {
	FromBlock string    `json:"fromBlock"`
	ToBlock   string    `json:"toBlock"`
	Address   string    `json:"address"`
	Topics    []*string `json:"topics"`
}

type arrivalScan struct {
	client  *httpclient.Client
	pace    *pacer
	network chain.Network
	owner   chain.EVMAddress
	attrs   []slog.Attr
	filter  logFilter
	atMost  money.Micros
	exclude []string
}

func (a *EVMArrivals) Inbound(
	ctx context.Context,
	network chain.Network,
	token, owner chain.EVMAddress,
	since time.Time,
	atMost money.Micros,
	exclude []string,
) (app.EVMArrival, bool, error) {
	attrs := []slog.Attr{slog.String("network", string(network)), slog.String("owner", owner.String())}
	client, ok := a.rpc[network]
	if !ok {
		return app.EVMArrival{}, false, errs.New(errs.CodeInvalidInput, evmArrivalsOp, attrs...)
	}
	if until, waiting := a.backingOff(network); waiting {
		return app.EVMArrival{}, false, errs.New(errs.CodeRPCUnavailable, evmArrivalsOp,
			append(attrs, slog.String("reason", "rate_limited"), slog.Time("retry_at", until))...)
	}
	transfer, recipient := transferEventTopic, topicOf(owner)
	s := arrivalScan{
		client: client, pace: &pacer{clock: a.clock}, network: network, owner: owner, attrs: attrs, atMost: atMost,
		exclude: exclude, filter: logFilter{Address: token.String(), Topics: []*string{&transfer, nil, &recipient}},
	}
	key := arrivalKey{network: network, owner: owner, since: since.UnixNano()}
	got, found, err := a.inbound(ctx, s, key, since)
	a.mu.Lock()
	defer a.mu.Unlock()
	switch {
	case rateLimited(err):
		b := a.throttled[network]
		b.wait = min(max(2*b.wait, arrivalBackoffFirst), arrivalBackoffCap)
		b.until = a.clock.Now().Add(b.wait)
		a.throttled[network] = b
	case err == nil:
		delete(a.throttled, network)
	}
	return got, found, err
}

func (a *EVMArrivals) inbound(
	ctx context.Context, s arrivalScan, key arrivalKey, since time.Time,
) (app.EVMArrival, bool, error) {
	var head hexInt
	if err := s.call(ctx, "eth_blockNumber", []any{}, &head); err != nil {
		return app.EVMArrival{}, false, err
	}
	cur, err := a.cursor(ctx, s, key, since, int64(head))
	if err != nil {
		return app.EVMArrival{}, false, err
	}
	got, found, cur, err := s.scan(ctx, cur, int64(head)-safeDepth(s.network))
	a.mu.Lock()
	defer a.mu.Unlock()
	if found {
		delete(a.cursors, key)
	} else {
		a.cursors[key] = cur
	}
	return got, found, err
}

func (a *EVMArrivals) backingOff(network chain.Network) (time.Time, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	until := a.throttled[network].until
	return until, a.clock.Now().Before(until)
}

func rateLimited(err error) bool {
	return slices.ContainsFunc(errs.Detail(err), func(a slog.Attr) bool {
		return a.Key == "status" && a.Value.String() == strconv.Itoa(http.StatusTooManyRequests)
	})
}

func (a *EVMArrivals) cursor(
	ctx context.Context, s arrivalScan, key arrivalKey, since time.Time, head int64,
) (arrivalCursor, error) {
	a.mu.Lock()
	cur, ok := a.cursors[key]
	a.mu.Unlock()
	if ok {
		return cur, nil
	}
	from, err := s.firstBlockAt(ctx, since.Add(-arrivalLookback), head)
	return arrivalCursor{next: from, chunk: a.chunk}, err
}

func (s arrivalScan) scan(
	ctx context.Context, cur arrivalCursor, settled int64,
) (app.EVMArrival, bool, arrivalCursor, error) {
	for range arrivalChunksPerTick {
		if cur.next > settled {
			break
		}
		to := min(cur.next+cur.chunk-1, settled)
		logs, err := s.logs(ctx, cur.next, to)
		if fault, capped := rangeCapped(err); capped && cur.chunk > 1 {
			observability.Info(ctx, observability.TradingEVMArrivalsChunk,
				slog.String("network", string(s.network)), slog.String("owner", s.owner.String()),
				slog.Int64("from", cur.next), slog.Int64("to", to), slog.Int64("chunk", cur.chunk/2),
				slog.String("rpc_message", fault.Message))
			cur.chunk /= 2
			to = min(cur.next+cur.chunk-1, settled)
			logs, err = s.logs(ctx, cur.next, to)
		}
		if err != nil {
			return app.EVMArrival{}, false, cur, err
		}
		if got, found, err := s.first(logs); err != nil || found {
			return got, found, cur, err
		}
		cur.next = to + 1
	}
	return app.EVMArrival{}, false, cur, nil
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (f *rpcError) Error() string { return f.Message }

func rangeCapped(err error) (*rpcError, bool) {
	words := []string{"range", "too large", "more than"}
	var fault *rpcError
	if !errors.As(err, &fault) {
		return nil, false
	}
	msg := strings.ToLower(fault.Message)
	return fault, slices.ContainsFunc(words, func(w string) bool { return strings.Contains(msg, w) })
}

func topicOf(a chain.EVMAddress) string {
	raw := strings.TrimPrefix(a.String(), "0x")
	return "0x" + strings.Repeat("0", evmTopicHexChars-len(raw)) + raw
}

func (s arrivalScan) first(logs []arrivalLog) (app.EVMArrival, bool, error) {
	for _, l := range logs {
		if l.Removed || slices.Contains(s.exclude, strings.ToLower(l.TxHash)) {
			continue
		}
		amount, err := hexBig(l.Data)
		if err != nil || !amount.IsUint64() || len(l.Data) != 2+evmTopicHexChars {
			return app.EVMArrival{}, false, errs.New(errs.CodeDecodeFailed, evmArrivalsOp,
				append(s.attrs, slog.String("tx_hash", l.TxHash), slog.String("data", l.Data))...)
		}
		got := money.MicrosFromUint64(amount.Uint64())
		if got.IsZero() || got.Cmp(s.atMost) > 0 {
			continue
		}
		return app.EVMArrival{Amount: got, TxHash: strings.ToLower(l.TxHash)}, true, nil
	}
	return app.EVMArrival{}, false, nil
}

func (s arrivalScan) firstBlockAt(ctx context.Context, at time.Time, head int64) (int64, error) {
	lo, hi := int64(0), head+1
	for lo < hi {
		mid := lo + (hi-lo)/2
		var block struct {
			Timestamp hexInt `json:"timestamp"`
		}
		if err := s.call(ctx, "eth_getBlockByNumber", []any{hexQuantity(mid), false}, &block); err != nil {
			return 0, err
		}
		if time.Unix(int64(block.Timestamp), 0).Before(at) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, nil
}

func (s arrivalScan) logs(ctx context.Context, from, to int64) ([]arrivalLog, error) {
	f := s.filter
	f.FromBlock, f.ToBlock = hexQuantity(from), hexQuantity(to)
	var logs []arrivalLog
	err := s.call(ctx, "eth_getLogs", []any{f}, &logs)
	return logs, err
}

func hexQuantity(n int64) string { return "0x" + strconv.FormatInt(n, 16) }

type hexInt int64

func (h *hexInt) UnmarshalJSON(raw []byte) error {
	var s string
	jsonErr := json.Unmarshal(raw, &s)
	digits, ok := strings.CutPrefix(s, "0x")
	n, err := strconv.ParseInt(digits, 16, 64)
	if jsonErr != nil || err != nil || !ok || n < 0 {
		return errs.New(errs.CodeDecodeFailed, evmArrivalsOp, slog.String("quantity", s))
	}
	*h = hexInt(n)
	return nil
}

func (s arrivalScan) call(ctx context.Context, method string, params []any, into any) error {
	attrs := append(slices.Clone(s.attrs), slog.String("method", method))
	s.pace.wait()
	raw, _ := json.Marshal(rpcCall{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(ctx, req)
	if err != nil {
		return errs.Wrap(err, errs.CodeRPCUnavailable, evmArrivalsOp, attrs...)
	}
	defer func() { _ = resp.Body.Close() }()
	var reply struct {
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, evmMaxBody)).Decode(&reply)
	if reply.Error != nil {
		return errs.Wrap(reply.Error, errs.CodeRPCUnavailable, evmArrivalsOp, append(attrs,
			slog.Int("status", resp.StatusCode), slog.Int("rpc_code", reply.Error.Code),
			slog.String("rpc_message", reply.Error.Message))...)
	}
	if resp.StatusCode != http.StatusOK {
		return errs.New(errs.CodeRPCUnavailable, evmArrivalsOp, append(attrs, slog.Int("status", resp.StatusCode))...)
	}
	if decodeErr != nil {
		return errs.Wrap(decodeErr, errs.CodeDecodeFailed, evmArrivalsOp, attrs...)
	}
	if err := json.Unmarshal(reply.Result, into); err != nil {
		return errs.Wrap(err, errs.CodeDecodeFailed, evmArrivalsOp, attrs...)
	}
	return nil
}

type rpcCall struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}
