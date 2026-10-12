package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	evmArrivalsOp    = "trading.EVMArrivals.Inbound"
	arrivalLookback  = 2 * time.Minute
	arrivalSpan      = 50_000
	arrivalMinSpan   = 500
	evmTopicHexChars = 64
)

var _ app.EVMArrivals = (*EVMArrivals)(nil)

type EVMArrivals struct {
	rpc  map[chain.Network]*httpclient.Client
	span int64
}

func NewEVMArrivals(rpc map[chain.Network]*httpclient.Client) *EVMArrivals {
	return &EVMArrivals{rpc: rpc, span: arrivalSpan}
}

func (a *EVMArrivals) WithSpan(span int64) *EVMArrivals {
	a.span = span
	return a
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
	transfer, recipient := transferEventTopic, topicOf(owner)
	s := arrivalScan{
		client: client, attrs: attrs, atMost: atMost, exclude: exclude,
		filter: logFilter{Address: token.String(), Topics: []*string{&transfer, nil, &recipient}},
	}
	var head hexInt
	var from int64
	err := s.call(ctx, "eth_blockNumber", []any{}, &head)
	if err == nil {
		from, err = s.firstBlockAt(ctx, since.Add(-arrivalLookback), int64(head))
	}
	if err != nil {
		return app.EVMArrival{}, false, err
	}
	span := max(a.span, 1)
	for from <= int64(head) {
		to := min(from+span-1, int64(head))
		logs, err := s.logs(ctx, from, to)
		if errs.CodeOf(err) == errs.CodeRPCUnavailable && span > arrivalMinSpan {
			span = max(span/2, arrivalMinSpan)
			continue
		}
		if err != nil {
			return app.EVMArrival{}, false, err
		}
		if got, found, err := s.first(logs); err != nil || found {
			return got, found, err
		}
		from = to + 1
	}
	return app.EVMArrival{}, false, nil
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
	raw, _ := json.Marshal(rpcCall{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(ctx, req)
	if err != nil {
		return errs.Wrap(err, errs.CodeRPCUnavailable, evmArrivalsOp, attrs...)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return errs.New(errs.CodeRPCUnavailable, evmArrivalsOp, append(attrs, slog.Int("status", resp.StatusCode))...)
	}
	var reply struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, evmMaxBody)).Decode(&reply); err != nil {
		return errs.Wrap(err, errs.CodeDecodeFailed, evmArrivalsOp, attrs...)
	}
	if reply.Error != nil {
		return errs.New(errs.CodeRPCUnavailable, evmArrivalsOp, append(attrs, slog.Int("rpc_code", reply.Error.Code),
			slog.String("rpc_message", reply.Error.Message))...)
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
