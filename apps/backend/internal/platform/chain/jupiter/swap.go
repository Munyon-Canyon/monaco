package jupiter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"strconv"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const marketMakerRouter = "jupiterz"

type orderWire struct {
	RequestID            string            `json:"requestId"`
	Transaction          string            `json:"transaction"`
	InAmount             string            `json:"inAmount"`
	OutAmount            string            `json:"outAmount"`
	OtherAmountThreshold string            `json:"otherAmountThreshold"`
	Router               string            `json:"router"`
	PriceImpactPct       json.Number       `json:"priceImpactPct"`
	RoutePlan            []json.RawMessage `json:"routePlan"`
	ErrorCode            int               `json:"errorCode"`
	ErrorMessage         string            `json:"errorMessage"`
	Error                string            `json:"error"`
}

func (w orderWire) routed() bool {
	return w.ErrorCode == 0 && w.Error == "" && w.ErrorMessage == "" && len(w.RoutePlan) > 0 &&
		w.OutAmount != "" && w.OutAmount != "0"
}

func (w orderWire) message() string {
	if w.ErrorMessage != "" {
		return w.ErrorMessage
	}
	return w.Error
}

type executeRequest struct {
	SignedTransaction string `json:"signedTransaction"`
	RequestID         string `json:"requestId"`
}

type executeWire struct {
	Status             string `json:"status"`
	Signature          string `json:"signature"`
	Code               int    `json:"code"`
	InputAmountResult  string `json:"inputAmountResult"`
	OutputAmountResult string `json:"outputAmountResult"`
	Error              string `json:"error"`
}

func (c *Client) Order(ctx context.Context, spec OrderSpec) (Order, error) {
	q := url.Values{"taker": {string(spec.Taker)}, "slippageBps": {strconv.FormatInt(spec.SlippageBps, 10)}}
	if spec.Payer != "" {
		q.Set("payer", string(spec.Payer))
	}
	o, router, err := c.signedOrder(ctx, spec, q)
	if err != nil && errs.CodeOf(err) == errs.CodeJupiterRejected && router == marketMakerRouter &&
		q.Get("excludeRouters") == "" {
		q.Set("excludeRouters", marketMakerRouter)
		o, _, err = c.signedOrder(ctx, spec, q)
	}
	return o, err
}

func (c *Client) signedOrder(ctx context.Context, spec OrderSpec, q url.Values) (Order, string, error) {
	const op = "jupiter.Order"
	w, status, err := c.order(ctx, op, spec.In, spec.Out, spec.Amount, q)
	if err != nil {
		return Order{}, "", err
	}
	if status != http.StatusOK || !w.routed() || w.Transaction == "" {
		return Order{}, w.Router, rejected(op, status, w.ErrorCode, w.message())
	}
	tx, err := base64.StdEncoding.DecodeString(w.Transaction)
	if err != nil {
		return Order{}, w.Router, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("request_id", w.RequestID))
	}
	in, _, err := amounts(w, spec.In, spec.Out, op)
	if err != nil {
		return Order{}, w.Router, err
	}
	if _, err := baseUnits(w.OtherAmountThreshold, spec.Out, op); err != nil {
		return Order{}, w.Router, err
	}
	route, err := signedRoute(tx, w.RequestID, op)
	if err != nil {
		return Order{}, w.Router, err
	}
	if route.In != in.Uint64() || route.In != spec.Amount.Uint64() {
		return Order{}, w.Router, errs.New(errs.CodeJupiterRejected, op, slog.String("request_id", w.RequestID),
			slog.String("reason", "transaction input differs from the order"),
			slog.Uint64("transaction_in", route.In), slog.Uint64("order_in", in.Uint64()))
	}
	out := money.NewBaseUnits(route.QuotedOut, spec.Out.Decimals)
	minOut := money.NewBaseUnits(route.MinOut(), spec.Out.Decimals)
	return Order{
		RequestID: w.RequestID, Transaction: tx, InMint: spec.In, OutMint: spec.Out,
		InAmount: in, OutAmount: out, MinOut: minOut, Router: w.Router,
	}, w.Router, nil
}

func signedRoute(raw []byte, requestID, op string) (chain.SwapRoute, error) {
	tx, err := chain.DecodeTransaction(raw)
	if err != nil {
		return chain.SwapRoute{}, errs.Wrap(err, errs.CodeJupiterRejected, op, slog.String("request_id", requestID))
	}
	msg, err := chain.ParseMessage(tx.Message)
	if err != nil {
		return chain.SwapRoute{}, errs.Wrap(err, errs.CodeJupiterRejected, op, slog.String("request_id", requestID))
	}
	var routes []chain.SwapRoute
	for _, ix := range msg.Instructions {
		if ix.Program >= len(msg.Keys) || msg.Keys[ix.Program] != chain.JupiterV6Program {
			continue
		}
		route, ok := chain.JupiterRoute(chain.Instruction{Program: chain.JupiterV6Program, Data: ix.Data})
		if !ok {
			return chain.SwapRoute{}, errs.New(errs.CodeJupiterRejected, op, slog.String("request_id", requestID),
				slog.String("reason", "Jupiter instruction is not a route"))
		}
		routes = append(routes, route)
	}
	if len(routes) != 1 {
		return chain.SwapRoute{}, errs.New(errs.CodeJupiterRejected, op, slog.String("request_id", requestID),
			slog.String("reason", "transaction has no single Jupiter route"), slog.Int("routes", len(routes)))
	}
	return routes[0], nil
}

func (c *Client) Quote(ctx context.Context, spec QuoteSpec) (Quote, error) {
	const op = "jupiter.Quote"
	w, status, err := c.order(ctx, op, spec.In, spec.Out, spec.Amount, url.Values{})
	switch {
	case err != nil:
		return Quote{}, err
	case status != http.StatusOK && status != http.StatusBadRequest:
		return Quote{}, rejected(op, status, w.ErrorCode, w.message())
	case status == http.StatusBadRequest || !w.routed():
		return Quote{Routable: false}, nil
	}
	in, out, err := amounts(w, spec.In, spec.Out, op)
	if err != nil {
		return Quote{}, err
	}
	impact, ok := impactBps(w.PriceImpactPct)
	if !ok {
		return Quote{}, errs.New(errs.CodeDecodeFailed, op, slog.String("price_impact_pct", string(w.PriceImpactPct)))
	}
	return Quote{InAmount: in, OutAmount: out, PriceImpactBps: impact, Routable: true}, nil
}

func (c *Client) order(
	ctx context.Context, op string, in, out Mint, amount money.BaseUnits, q url.Values,
) (orderWire, int, error) {
	if amount.IsZero() || amount.Decimals() != in.Decimals {
		return orderWire{}, 0, errs.New(errs.CodeInvalidInput, op,
			slog.String("amount", amount.String()), slog.Int("decimals", int(amount.Decimals())))
	}
	q.Set("inputMint", in.Address)
	q.Set("outputMint", out.Address)
	q.Set("amount", amount.String())
	q.Set("swapMode", "ExactIn")
	w, status, err := c.orderOnce(ctx, op, q)
	if err != nil || status != http.StatusBadRequest {
		return w, status, err
	}
	q.Set("excludeRouters", marketMakerRouter)
	return c.orderOnce(ctx, op, q)
}

func (c *Client) orderOnce(ctx context.Context, op string, q url.Values) (orderWire, int, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/order?"+q.Encode(), nil)
	r, err := c.call(ctx, c.swap, swapLane, req, op)
	if err != nil {
		return orderWire{}, 0, err
	}
	var w orderWire
	if err := decode(r, op, &w); err != nil && r.status == http.StatusOK {
		return orderWire{}, 0, err
	}
	return w, r.status, nil
}

func amounts(w orderWire, inMint, outMint Mint, op string) (money.BaseUnits, money.BaseUnits, error) {
	in, err := baseUnits(w.InAmount, inMint, op)
	if err != nil {
		return money.BaseUnits{}, money.BaseUnits{}, err
	}
	out, err := baseUnits(w.OutAmount, outMint, op)
	return in, out, err
}

func impactBps(pct json.Number) (int64, bool) {
	if pct == "" {
		return 0, true
	}
	r, ok := decimal(pct)
	if !ok {
		return 0, false
	}
	r.Abs(r).Mul(r, big.NewRat(100, 1))
	bps, rem := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int))
	if rem.Sign() != 0 {
		bps.Add(bps, big.NewInt(1))
	}
	return bps.Int64(), bps.IsInt64()
}

func (c *Client) Execute(ctx context.Context, requestID string, signed []byte) (ExecuteResult, error) {
	const op = "jupiter.Execute"
	body, _ := json.Marshal(executeRequest{
		SignedTransaction: base64.StdEncoding.EncodeToString(signed), RequestID: requestID,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/execute", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r, err := c.call(ctx, c.execute, swapLane, req, op)
	if err != nil {
		return ExecuteResult{}, err
	}
	var w executeWire
	err = decode(r, op, &w)
	if r.status != http.StatusOK {
		return ExecuteResult{}, rejected(op, r.status, w.Code, w.Error)
	}
	if err != nil {
		return ExecuteResult{}, err
	}
	status, ok := map[string]Status{"Success": StatusSuccess, "Failed": StatusFailed, "Pending": StatusPending}[w.Status]
	if !ok {
		return ExecuteResult{}, errs.New(errs.CodeDecodeFailed, op, slog.String("status", w.Status))
	}
	in, err := optionalCount(w.InputAmountResult, op)
	if err != nil {
		return ExecuteResult{}, err
	}
	out, err := optionalCount(w.OutputAmountResult, op)
	if err != nil {
		return ExecuteResult{}, err
	}
	return ExecuteResult{Status: status, Signature: w.Signature, InAmount: in, OutAmount: out, ErrorCode: w.Code}, nil
}

func optionalCount(raw, op string) (uint64, error) {
	if raw == "" {
		return 0, nil
	}
	return count(raw, op)
}

func (c *Client) ExecuteUntilTerminal(ctx context.Context, requestID string, signed []byte) (ExecuteResult, error) {
	const op = "jupiter.ExecuteUntilTerminal"
	start := c.clock.Now()
	for {
		res, err := c.Execute(ctx, requestID, signed)
		if err != nil || res.Status != StatusPending {
			return res, err
		}
		if c.clock.Now().Sub(start) >= c.window {
			return res, errs.New(errs.CodeUpstreamTimeout, op, slog.String("request_id", requestID))
		}
		select {
		case <-ctx.Done():
			return res, errs.Wrap(
				context.Cause(ctx),
				errs.CodeUpstreamTimeout,
				op,
				slog.String("request_id", requestID),
			)
		case <-c.clock.After(pendingEvery):
		}
	}
}
