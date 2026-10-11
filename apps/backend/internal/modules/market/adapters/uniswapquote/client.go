package uniswapquote

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"regexp"
	"time"

	"golang.org/x/time/rate"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	DefaultBaseURL = "https://trade-api.gateway.uniswap.org"
	upstream       = "uniswap-quote"
	keyHeader      = "x-api-key"
	quoteTTL       = 30 * time.Second
	callsPerSecond = 5
	maxBody        = 1 << 20
	feeDenominator = 1_000_000
	v2PoolFee      = 3000
	noRoute        = "NoRouteFoundError"
	classicRouting = "CLASSIC"
)

var _ mc.Quoter = (*Client)(nil)

type Client struct {
	http    *httpclient.Client
	key     string
	swapper chain.EVMAddress
	clock   clock.Clock
	limit   *rate.Limiter
}

func Options(baseURL string, timeout time.Duration) []httpclient.Option {
	return []httpclient.Option{
		httpclient.WithBaseURL(cmp.Or(baseURL, DefaultBaseURL)),
		httpclient.WithTimeout(timeout),
	}
}

func New(c *httpclient.Client, apiKey string, swapper chain.EVMAddress, clk clock.Clock) *Client {
	return &Client{
		http: c, key: apiKey, swapper: swapper, clock: clk,
		limit: rate.NewLimiter(rate.Limit(callsPerSecond), callsPerSecond),
	}
}

type quoteRequest struct {
	Type              string   `json:"type"`
	Amount            string   `json:"amount"`
	TokenInChainID    int64    `json:"tokenInChainId"`
	TokenOutChainID   int64    `json:"tokenOutChainId"`
	TokenIn           string   `json:"tokenIn"`
	TokenOut          string   `json:"tokenOut"`
	Swapper           string   `json:"swapper"`
	RoutingPreference string   `json:"routingPreference"`
	Protocols         []string `json:"protocols"`
}

type amountWire struct {
	Amount string `json:"amount"`
}

type poolWire struct {
	Fee      *string `json:"fee"`
	AmountIn *string `json:"amountIn"`
}

type quoteWire struct {
	ErrorCode string `json:"errorCode"`
	Routing   string `json:"routing"`
	Quote     struct {
		QuoteID   string       `json:"quoteId"`
		Input     amountWire   `json:"input"`
		Output    amountWire   `json:"output"`
		GasFeeUSD string       `json:"gasFeeUSD"`
		Route     [][]poolWire `json:"route"`
	} `json:"quote"`
}

func (c *Client) Quote(ctx context.Context, l mc.Listing, side mc.Side, amount money.Micros) (mc.Quote, error) {
	const op = "uniswapquote.Client.Quote"
	attrs := []slog.Attr{slog.String("network", string(l.Network)), slog.String("side", string(side))}
	req, ok := c.request(l, side, amount)
	if !ok {
		return mc.Quote{}, errs.New(errs.CodeInvalidInput, op, append(attrs, slog.String("amount", amount.String()))...)
	}
	w, err := c.post(ctx, op, req, attrs)
	if err != nil {
		return mc.Quote{}, err
	}
	q, err := mapQuote(w, side)
	if err != nil {
		return mc.Quote{}, errs.Wrap(err, errs.CodeDecodeFailed, op, attrs...)
	}
	q.ListingID, q.Network, q.Venue, q.Side = l.ID, l.Network, mc.VenueUniswap, side
	q.QuotedAt = c.clock.Now()
	q.ExpiresAt = q.QuotedAt.Add(quoteTTL)
	return q, nil
}

func (c *Client) request(l mc.Listing, side mc.Side, amount money.Micros) (quoteRequest, bool) {
	cash := l.Network.Cash().Address
	r := quoteRequest{
		Amount: amount.String(), TokenInChainID: l.Network.EVMChainID(), TokenOutChainID: l.Network.EVMChainID(),
		Swapper: c.swapper.String(), RoutingPreference: "BEST_PRICE", Protocols: []string{"V2", "V3", "V4"},
	}
	switch side {
	case mc.SideBuy:
		r.Type, r.TokenIn, r.TokenOut = "EXACT_INPUT", cash, l.TokenAddress
	case mc.SideSell:
		r.Type, r.TokenIn, r.TokenOut = "EXACT_OUTPUT", l.TokenAddress, cash
	default:
		return quoteRequest{}, false
	}
	return r, l.Network.IsEVM() && !amount.IsZero() && l.TokenAddress != ""
}

func (c *Client) post(ctx context.Context, op string, body quoteRequest, attrs []slog.Attr) (quoteWire, error) {
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/v1/quote", bytes.NewReader(raw))
	req.Header.Set(keyHeader, c.key)
	req.Header.Set("Content-Type", "application/json")
	if err := c.limit.Wait(ctx); err != nil {
		return quoteWire{}, errs.Wrap(err, errs.CodeUpstreamUnavailable, op, attrs...)
	}
	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return quoteWire{}, errs.Wrap(err, errs.CodeOf(err), op, attrs...)
	}
	defer func() { _ = resp.Body.Close() }()
	attrs = append(attrs, slog.Int("status", resp.StatusCode))
	if resp.StatusCode >= http.StatusInternalServerError {
		return quoteWire{}, errs.New(errs.CodeUpstreamUnavailable, op, attrs...)
	}
	var w quoteWire
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&w); err != nil {
		return quoteWire{}, errs.Wrap(err, errs.CodeDecodeFailed, op, attrs...)
	}
	switch {
	case w.ErrorCode == noRoute:
		return quoteWire{}, errs.New(errs.CodeNoRoute, op, attrs...)
	case resp.StatusCode != http.StatusOK:
		return quoteWire{}, errs.New(
			errs.CodeInvalidInput,
			op,
			append(attrs, slog.String("error_code", w.ErrorCode))...)
	case w.Routing != classicRouting:
		return quoteWire{}, errs.New(errs.CodeDecodeFailed, op, append(attrs, slog.String("routing", w.Routing))...)
	}
	return w, nil
}

func mapQuote(w quoteWire, side mc.Side) (mc.Quote, error) {
	cashLeg, tokenLeg := w.Quote.Input.Amount, w.Quote.Output.Amount
	if side == mc.SideSell {
		cashLeg, tokenLeg = tokenLeg, cashLeg
	}
	cash, err := money.ParseMicros(cashLeg)
	if err != nil {
		return mc.Quote{}, err
	}
	gross, ok := new(big.Int).SetString(tokenLeg, 10)
	if !ok || gross.Sign() <= 0 {
		return mc.Quote{}, errs.New(errs.CodeDecodeFailed, "uniswapquote.gross", slog.String("amount", tokenLeg))
	}
	fee, err := poolFee(cash, w.Quote.Input.Amount, w.Quote.Route)
	if err != nil {
		return mc.Quote{}, err
	}
	gas, err := gasMicros(w.Quote.GasFeeUSD)
	if err != nil {
		return mc.Quote{}, err
	}
	return mc.Quote{
		InputMicros: cash, GrossOutput: gross, RouteID: w.Quote.QuoteID,
		Costs: []mc.Cost{{Kind: mc.CostPoolFee, Micros: fee}, {Kind: mc.CostGas, Micros: gas}},
	}, nil
}

func poolFee(cash money.Micros, input string, route [][]poolWire) (money.Micros, error) {
	const op = "uniswapquote.poolFee"
	total, ok := new(big.Int).SetString(input, 10)
	if !ok || total.Sign() <= 0 || len(route) == 0 {
		return money.Micros{}, errs.New(errs.CodeDecodeFailed, op, slog.String("input", input))
	}
	weighted := new(big.Rat)
	for _, split := range route {
		fee, ok := compoundFee(split)
		share, sok := splitShare(split, total, len(route) == 1)
		if !ok || !sok {
			return money.Micros{}, errs.New(errs.CodeDecodeFailed, op, slog.String("input", input))
		}
		weighted.Add(weighted, share.Mul(share, fee))
	}
	scale := new(big.Rat).SetUint64(cash.Uint64())
	return ceilMicros(weighted.Mul(weighted, scale))
}

func compoundFee(split []poolWire) (*big.Rat, bool) {
	kept := big.NewRat(1, 1)
	for _, hop := range split {
		fee := int64(v2PoolFee)
		if hop.Fee != nil {
			if !wholeNumber.MatchString(*hop.Fee) {
				return nil, false
			}
			parsed, _ := new(big.Int).SetString(*hop.Fee, 10)
			if parsed.Cmp(big.NewInt(feeDenominator)) > 0 {
				return nil, false
			}
			fee = parsed.Int64()
		}
		kept.Mul(kept, big.NewRat(feeDenominator-fee, feeDenominator))
	}
	return kept.Sub(big.NewRat(1, 1), kept), len(split) > 0
}

func splitShare(split []poolWire, total *big.Int, only bool) (*big.Rat, bool) {
	if only {
		return big.NewRat(1, 1), true
	}
	if len(split) == 0 || split[0].AmountIn == nil || !wholeNumber.MatchString(*split[0].AmountIn) {
		return nil, false
	}
	in, _ := new(big.Int).SetString(*split[0].AmountIn, 10)
	return new(big.Rat).SetFrac(in, total), true
}

var (
	wholeNumber = regexp.MustCompile(`^[0-9]{1,40}$`)
	usdDecimal  = regexp.MustCompile(`^[0-9]{1,20}(\.[0-9]{1,40})?$`)
)

func gasMicros(raw string) (money.Micros, error) {
	usd, ok := new(big.Rat).SetString(raw)
	if !usdDecimal.MatchString(raw) || !ok {
		return money.Micros{}, errs.New(
			errs.CodeDecodeFailed,
			"uniswapquote.gasMicros",
			slog.String("gas_fee_usd", raw),
		)
	}
	return ceilMicros(usd.Mul(usd, big.NewRat(1_000_000, 1)))
}

func ceilMicros(r *big.Rat) (money.Micros, error) {
	q, m := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int))
	if m.Sign() > 0 {
		q.Add(q, big.NewInt(1))
	}
	if !q.IsUint64() {
		return money.Micros{}, errs.New(
			errs.CodeDecodeFailed,
			"uniswapquote.ceilMicros",
			slog.String("value", r.String()),
		)
	}
	return money.MicrosFromUint64(q.Uint64()), nil
}
