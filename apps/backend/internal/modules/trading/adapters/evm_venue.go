package adapters

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/time/rate"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

const (
	uniswapKeyHeader   = "x-api-key"
	uniswapPerSecond   = 5
	evmMaxBody         = 1 << 20
	uniswapNoRoute     = "NoRouteFoundError"
	uniswapClassic     = "CLASSIC"
	uniswapExactInput  = "EXACT_INPUT"
	uniswapPreference  = "BEST_PRICE"
	bpsPerPercent      = 100
	evmVenueBuildOp    = "trading.EVMVenue.Build"
	evmVenueFillOp     = "trading.EVMVenue.Fill"
	transferEventTopic = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
)

var (
	_ app.EVMVenue = (*EVMVenue)(nil)

	evmWhole = regexp.MustCompile(`^[0-9]{1,78}$`)
	evmHex   = regexp.MustCompile(`^0x[0-9a-fA-F]*$`)
)

type EVMVenue struct {
	uniswap *httpclient.Client
	key     string
	rpc     map[chain.Network]*httpclient.Client
	limit   *rate.Limiter
}

func NewEVMVenue(uniswap *httpclient.Client, apiKey string, rpc map[chain.Network]*httpclient.Client) *EVMVenue {
	return &EVMVenue{
		uniswap: uniswap, key: apiKey, rpc: rpc,
		limit: rate.NewLimiter(rate.Limit(uniswapPerSecond), uniswapPerSecond),
	}
}

type approvalRequest struct {
	WalletAddress string `json:"walletAddress"`
	Token         string `json:"token"`
	Amount        string `json:"amount"`
	ChainID       int64  `json:"chainId"`
}

type txWire struct {
	To    string `json:"to"`
	Data  string `json:"data"`
	Value string `json:"value"`
}

type approvalWire struct {
	ErrorCode string  `json:"errorCode"`
	Approval  *txWire `json:"approval"`
}

type swapQuoteRequest struct {
	Type              string      `json:"type"`
	Amount            string      `json:"amount"`
	TokenInChainID    int64       `json:"tokenInChainId"`
	TokenOutChainID   int64       `json:"tokenOutChainId"`
	TokenIn           string      `json:"tokenIn"`
	TokenOut          string      `json:"tokenOut"`
	Swapper           string      `json:"swapper"`
	SlippageTolerance json.Number `json:"slippageTolerance"`
	RoutingPreference string      `json:"routingPreference"`
	Protocols         []string    `json:"protocols"`
}

type outputWire struct {
	Recipient string `json:"recipient"`
	MinAmount string `json:"minAmount"`
}

type swapQuoteWire struct {
	ErrorCode string          `json:"errorCode"`
	Routing   string          `json:"routing"`
	Raw       json.RawMessage `json:"quote"`
	Quote     struct {
		Output struct {
			Amount string `json:"amount"`
		} `json:"output"`
		AggregatedOutputs []outputWire `json:"aggregatedOutputs"`
	} `json:"-"`
	PermitTransaction *txWire `json:"permitTransaction"`
}

type swapRequest struct {
	Quote json.RawMessage `json:"quote"`
}

type swapWire struct {
	ErrorCode string  `json:"errorCode"`
	Swap      *txWire `json:"swap"`
}

func (v *EVMVenue) Build(ctx context.Context, spec app.EVMSwapSpec) (app.EVMSwapPlan, error) {
	attrs := []slog.Attr{slog.String("network", string(spec.Network))}
	if !spec.Network.IsEVM() || spec.AmountIn == 0 || spec.MinOut == 0 {
		return app.EVMSwapPlan{}, errs.New(errs.CodeInvalidInput, evmVenueBuildOp, attrs...)
	}
	var approval approvalWire
	err := v.post(ctx, "/v1/check_approval", approvalRequest{
		WalletAddress: spec.Owner.String(), Token: spec.TokenIn.String(),
		Amount: strconv.FormatUint(spec.AmountIn, 10), ChainID: spec.Network.EVMChainID(),
	}, &approval, attrs)
	if err != nil {
		return app.EVMSwapPlan{}, err
	}
	probe, err := v.quote(ctx, spec, 0, attrs)
	if err != nil {
		return app.EVMSwapPlan{}, err
	}
	out, err := wholeUint(probe.Quote.Output.Amount)
	if err != nil {
		return app.EVMSwapPlan{}, errs.Wrap(err, errs.CodeDecodeFailed, evmVenueBuildOp, attrs...)
	}
	if out < spec.MinOut {
		return app.EVMSwapPlan{}, errs.New(errs.CodePriceMoved, evmVenueBuildOp,
			append(attrs, slog.Uint64("quoted", out), slog.Uint64("floor", spec.MinOut))...)
	}
	quote, err := v.quote(ctx, spec, toleranceBps(out, spec.MinOut), attrs)
	if err != nil {
		return app.EVMSwapPlan{}, err
	}
	var swap swapWire
	if err := v.post(ctx, "/v1/swap", swapRequest{Quote: quote.Raw}, &swap, attrs); err != nil {
		return app.EVMSwapPlan{}, err
	}
	plan, err := buildPlan(spec, approval, quote, swap)
	if err != nil {
		return app.EVMSwapPlan{}, errs.Wrap(err, errs.CodeDecodeFailed, evmVenueBuildOp, attrs...)
	}
	return plan, nil
}

func (v *EVMVenue) quote(
	ctx context.Context,
	spec app.EVMSwapSpec,
	bps uint64,
	attrs []slog.Attr,
) (swapQuoteWire, error) {
	var quote swapQuoteWire
	err := v.post(ctx, "/v1/quote", swapQuoteRequest{
		Type: uniswapExactInput, Amount: strconv.FormatUint(spec.AmountIn, 10),
		TokenInChainID: spec.Network.EVMChainID(), TokenOutChainID: spec.Network.EVMChainID(),
		TokenIn: spec.TokenIn.String(), TokenOut: spec.TokenOut.String(), Swapper: spec.Owner.String(),
		SlippageTolerance: percent(bps), RoutingPreference: uniswapPreference, Protocols: []string{"V2", "V3", "V4"},
	}, &quote, attrs)
	if err != nil {
		return swapQuoteWire{}, err
	}
	if quote.Routing != uniswapClassic || json.Unmarshal(quote.Raw, &quote.Quote) != nil {
		return swapQuoteWire{}, errs.New(errs.CodeDecodeFailed, evmVenueBuildOp,
			append(attrs, slog.String("routing", quote.Routing))...)
	}
	return quote, nil
}

func toleranceBps(out, floor uint64) uint64 {
	gap := new(big.Int).Mul(new(big.Int).SetUint64(out-floor), big.NewInt(10_000))
	bps := gap.Div(gap, new(big.Int).SetUint64(out)).Uint64()
	return max(bps, 1) - 1
}

func percent(bps uint64) json.Number {
	return json.Number(strconv.FormatUint(bps/bpsPerPercent, 10) + "." + strconv.FormatUint(bps%bpsPerPercent/10, 10) +
		strconv.FormatUint(bps%10, 10))
}

func buildPlan(
	spec app.EVMSwapSpec,
	approval approvalWire,
	quote swapQuoteWire,
	swap swapWire,
) (app.EVMSwapPlan, error) {
	if swap.Swap == nil {
		return app.EVMSwapPlan{}, errs.New(errs.CodeDecodeFailed, "trading.EVMVenue.plan")
	}
	minOut, minErr := minOutFor(quote.Quote.AggregatedOutputs, spec.Owner)
	tx, txErr := evmTx(spec.Network, *swap.Swap)
	if txErr == nil {
		txErr = checkSwapTarget(spec, tx)
	}
	plan := app.EVMSwapPlan{Swap: tx, MinOut: minOut}
	errList := []error{minErr, txErr}
	for _, raw := range []*txWire{approval.Approval, quote.PermitTransaction} {
		if raw == nil {
			continue
		}
		approve, err := evmTx(spec.Network, *raw)
		if err == nil {
			err = checkApproval(spec, tx.To, approve)
		}
		plan.Approvals = append(plan.Approvals, approve)
		errList = append(errList, err)
	}
	for _, err := range errList {
		if err != nil {
			return app.EVMSwapPlan{}, err
		}
	}
	return plan, nil
}

func minOutFor(outputs []outputWire, owner chain.EVMAddress) (uint64, error) {
	for _, o := range outputs {
		recipient, err := chain.ParseEVMAddress(strings.ToLower(o.Recipient))
		if err == nil && recipient == owner {
			return wholeUint(o.MinAmount)
		}
	}
	return 0, errs.New(errs.CodeDecodeFailed, "trading.EVMVenue.minOut", slog.String("owner", owner.String()))
}

func wholeUint(raw string) (uint64, error) {
	n, err := strconv.ParseUint(raw, 10, 64)
	if !evmWhole.MatchString(raw) || err != nil {
		return 0, errs.New(errs.CodeDecodeFailed, "trading.EVMVenue.amount", slog.String("amount", raw))
	}
	return n, nil
}

func evmTx(network chain.Network, w txWire) (mc.EVMTransaction, error) {
	to, err := chain.ParseEVMAddress(strings.ToLower(w.To))
	data, dataErr := hexBytes(w.Data)
	value, valueErr := hexBig(w.Value)
	if err != nil || dataErr != nil || valueErr != nil {
		return mc.EVMTransaction{}, errs.New(errs.CodeDecodeFailed, "trading.EVMVenue.tx", slog.String("to", w.To))
	}
	return mc.EVMTransaction{Network: network, To: to, Data: data, Value: value}, nil
}

func hexBytes(raw string) ([]byte, error) {
	b, err := hex.DecodeString(strings.TrimPrefix(raw, "0x"))
	if !evmHex.MatchString(raw) || err != nil {
		return nil, errs.New(errs.CodeDecodeFailed, "trading.EVMVenue.hex")
	}
	return b, nil
}

func hexBig(raw string) (*big.Int, error) {
	if raw == "" || raw == "0x" {
		return new(big.Int), nil
	}
	n, ok := new(big.Int).SetString(strings.TrimPrefix(raw, "0x"), 16)
	if !evmHex.MatchString(raw) || !ok || n.Sign() < 0 {
		return nil, errs.New(errs.CodeDecodeFailed, "trading.EVMVenue.value", slog.String("value", raw))
	}
	return n, nil
}

type errorCoded interface{ code() string }

func (w approvalWire) code() string  { return w.ErrorCode }
func (w swapQuoteWire) code() string { return w.ErrorCode }
func (w swapWire) code() string      { return w.ErrorCode }

func (v *EVMVenue) post(ctx context.Context, path string, body, into any, attrs []slog.Attr) error {
	attrs = append(attrs, slog.String("path", path))
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set(uniswapKeyHeader, v.key)
	req.Header.Set("Content-Type", "application/json")
	if err := v.limit.Wait(ctx); err != nil {
		return errs.Wrap(err, errs.CodeUpstreamUnavailable, evmVenueBuildOp, attrs...)
	}
	resp, err := v.uniswap.Do(ctx, req)
	if err != nil {
		return errs.Wrap(err, errs.CodeOf(err), evmVenueBuildOp, attrs...)
	}
	defer func() { _ = resp.Body.Close() }()
	attrs = append(attrs, slog.Int("status", resp.StatusCode))
	if resp.StatusCode >= http.StatusInternalServerError {
		return errs.New(errs.CodeUpstreamUnavailable, evmVenueBuildOp, attrs...)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, evmMaxBody)).Decode(into); err != nil {
		return errs.Wrap(err, errs.CodeDecodeFailed, evmVenueBuildOp, attrs...)
	}
	code := into.(errorCoded).code()
	switch {
	case code == uniswapNoRoute:
		return errs.New(errs.CodeNoRoute, evmVenueBuildOp, attrs...)
	case resp.StatusCode != http.StatusOK:
		return errs.New(errs.CodeInvalidInput, evmVenueBuildOp, append(attrs, slog.String("error_code", code))...)
	}
	return nil
}
