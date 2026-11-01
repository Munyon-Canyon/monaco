package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

type rpcRequest struct {
	JSONRPC string   `json:"jsonrpc"`
	ID      int      `json:"id"`
	Method  string   `json:"method"`
	Params  []string `json:"params"`
}

type logWire struct {
	Address string   `json:"address"`
	Topics  []string `json:"topics"`
	Data    string   `json:"data"`
}

type receiptWire struct {
	Status string    `json:"status"`
	Logs   []logWire `json:"logs"`
}

type rpcResponse[T any] struct {
	Result *T `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type userOperationWire struct {
	Success bool         `json:"success"`
	Receipt *receiptWire `json:"receipt"`
}

const userOperationReceiptMethod = "eth_getUserOperationReceipt"

func (v *EVMVenue) Fill(
	ctx context.Context, network chain.Network, txHash string, out, owner chain.EVMAddress,
) (app.EVMFill, error) {
	state, receipt, attrs, err := v.receipt(ctx, network, txHash)
	if err != nil || state != app.FillSucceeded {
		return app.EVMFill{State: state}, err
	}
	received, err := transferredTo(receipt.Logs, out, owner)
	if err != nil {
		return app.EVMFill{}, errs.Wrap(err, errs.CodeDecodeFailed, evmVenueFillOp, attrs...)
	}
	return app.EVMFill{State: app.FillSucceeded, Out: received}, nil
}

func (v *EVMVenue) Inspect(
	ctx context.Context, network chain.Network, txHash string, out, owner chain.EVMAddress,
) (app.EVMFill, error) {
	state, receipt, attrs, err := v.receipt(ctx, network, txHash)
	if err != nil || state != app.FillSucceeded {
		return app.EVMFill{State: state}, err
	}
	total, err := transferTotal(receipt.Logs, out, owner)
	if err == nil && !total.IsUint64() {
		err = errs.New(errs.CodeDecodeFailed, evmVenueFillOp, slog.String("total", total.String()))
	}
	if err != nil {
		return app.EVMFill{}, errs.Wrap(err, errs.CodeDecodeFailed, evmVenueFillOp, attrs...)
	}
	return app.EVMFill{State: app.FillSucceeded, Out: total.Uint64()}, nil
}

func (v *EVMVenue) Receipt(ctx context.Context, network chain.Network, txHash string) (app.EVMFillState, error) {
	state, _, _, err := v.receipt(ctx, network, txHash)
	return state, err
}

func (v *EVMVenue) receipt(
	ctx context.Context, network chain.Network, txHash string,
) (app.EVMFillState, *receiptWire, []slog.Attr, error) {
	attrs := []slog.Attr{slog.String("network", string(network)), slog.String("tx_hash", txHash)}
	client, ok := v.rpc[network]
	if !ok || !evmHex.MatchString(txHash) || len(txHash) != 66 {
		return 0, nil, attrs, errs.New(errs.CodeInvalidInput, evmVenueFillOp, attrs...)
	}
	w, err := receiptCall[receiptWire](ctx, client, "eth_getTransactionReceipt", txHash, attrs)
	switch {
	case err != nil:
		return 0, nil, attrs, err
	case w.Result == nil:
		return v.userOperationReceipt(ctx, client, txHash, attrs)
	}
	state, err := receiptState(w.Result, attrs)
	if err != nil || state != app.FillSucceeded {
		return state, nil, attrs, err
	}
	return state, w.Result, attrs, nil
}

func (v *EVMVenue) userOperationReceipt(
	ctx context.Context, client *httpclient.Client, hash string, attrs []slog.Attr,
) (app.EVMFillState, *receiptWire, []slog.Attr, error) {
	w, err := receiptCall[userOperationWire](ctx, client, userOperationReceiptMethod, hash, attrs)
	if found := err == nil && w.Result != nil && w.Result.Receipt != nil; !found {
		return app.FillPending, nil, attrs, nil
	}
	if !w.Result.Success {
		return app.FillReverted, nil, attrs, nil
	}
	state, err := receiptState(w.Result.Receipt, attrs)
	if err != nil || state != app.FillSucceeded {
		return state, nil, attrs, err
	}
	return state, w.Result.Receipt, attrs, nil
}

func receiptState(r *receiptWire, attrs []slog.Attr) (app.EVMFillState, error) {
	switch r.Status {
	case "0x0":
		return app.FillReverted, nil
	case "0x1":
		return app.FillSucceeded, nil
	}
	return 0, errs.New(errs.CodeDecodeFailed, evmVenueFillOp, append(attrs, slog.String("status", r.Status))...)
}

func receiptCall[T any](
	ctx context.Context, client *httpclient.Client, method, hash string, attrs []slog.Attr,
) (rpcResponse[T], error) {
	var w rpcResponse[T]
	raw, _ := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: []string{hash}})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(ctx, req)
	if err != nil {
		return w, errs.Wrap(err, errs.CodeRPCUnavailable, evmVenueFillOp, attrs...)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return w, errs.New(errs.CodeRPCUnavailable, evmVenueFillOp,
			append(attrs, slog.Int("status", resp.StatusCode))...)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, evmMaxBody)).Decode(&w); err != nil {
		return w, errs.Wrap(err, errs.CodeDecodeFailed, evmVenueFillOp, attrs...)
	}
	if w.Error != nil {
		return w, errs.New(
			errs.CodeRPCUnavailable,
			evmVenueFillOp,
			append(attrs, slog.Int("rpc_code", w.Error.Code))...)
	}
	return w, nil
}

func transferredTo(logs []logWire, token, owner chain.EVMAddress) (uint64, error) {
	total, err := transferTotal(logs, token, owner)
	if err != nil {
		return 0, err
	}
	if total.Sign() == 0 || !total.IsUint64() {
		return 0, errs.New(errs.CodeDecodeFailed, "trading.EVMVenue.transfer", slog.String("total", total.String()))
	}
	return total.Uint64(), nil
}

func transferTotal(logs []logWire, token, owner chain.EVMAddress) (*big.Int, error) {
	recipient := "0x" + strings.Repeat("0", 24) + strings.TrimPrefix(owner.String(), "0x")
	total := new(big.Int)
	for _, l := range logs {
		if !strings.EqualFold(l.Address, token.String()) || len(l.Topics) != 3 ||
			!strings.EqualFold(l.Topics[0], transferEventTopic) || !strings.EqualFold(l.Topics[2], recipient) {
			continue
		}
		amount, err := hexBig(l.Data)
		if err != nil || len(l.Data) != 66 {
			return nil, errs.New(errs.CodeDecodeFailed, "trading.EVMVenue.transfer", slog.String("data", l.Data))
		}
		total.Add(total, amount)
	}
	return total, nil
}
