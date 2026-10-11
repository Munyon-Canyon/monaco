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

type rpcResponse struct {
	Result *receiptWire `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

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
	raw, _ := json.Marshal(
		rpcRequest{JSONRPC: "2.0", ID: 1, Method: "eth_getTransactionReceipt", Params: []string{txHash}},
	)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(ctx, req)
	if err != nil {
		return 0, nil, attrs, errs.Wrap(err, errs.CodeRPCUnavailable, evmVenueFillOp, attrs...)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0, nil, attrs, errs.New(errs.CodeRPCUnavailable, evmVenueFillOp,
			append(attrs, slog.Int("status", resp.StatusCode))...)
	}
	var w rpcResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, evmMaxBody)).Decode(&w); err != nil {
		return 0, nil, attrs, errs.Wrap(err, errs.CodeDecodeFailed, evmVenueFillOp, attrs...)
	}
	switch {
	case w.Error != nil:
		return 0, nil, attrs, errs.New(errs.CodeRPCUnavailable, evmVenueFillOp,
			append(attrs, slog.Int("rpc_code", w.Error.Code))...)
	case w.Result == nil:
		return app.FillPending, nil, attrs, nil
	case w.Result.Status == "0x0":
		return app.FillReverted, nil, attrs, nil
	case w.Result.Status != "0x1":
		return 0, nil, attrs, errs.New(errs.CodeDecodeFailed, evmVenueFillOp,
			append(attrs, slog.String("status", w.Result.Status))...)
	}
	return app.FillSucceeded, w.Result, attrs, nil
}

func transferredTo(logs []logWire, token, owner chain.EVMAddress) (uint64, error) {
	recipient := "0x" + strings.Repeat("0", 24) + strings.TrimPrefix(owner.String(), "0x")
	total := new(big.Int)
	for _, l := range logs {
		if !strings.EqualFold(l.Address, token.String()) || len(l.Topics) != 3 ||
			!strings.EqualFold(l.Topics[0], transferEventTopic) || !strings.EqualFold(l.Topics[2], recipient) {
			continue
		}
		amount, err := hexBig(l.Data)
		if err != nil || len(l.Data) != 66 {
			return 0, errs.New(errs.CodeDecodeFailed, "trading.EVMVenue.transfer", slog.String("data", l.Data))
		}
		total.Add(total, amount)
	}
	if total.Sign() == 0 || !total.IsUint64() {
		return 0, errs.New(errs.CodeDecodeFailed, "trading.EVMVenue.transfer", slog.String("total", total.String()))
	}
	return total.Uint64(), nil
}
