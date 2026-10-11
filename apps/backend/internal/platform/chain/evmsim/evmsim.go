package evmsim

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

const (
	EntryPoint      = "0x0000000071727de22e5e9d8baf0edac6f37da032"
	SimulationGas   = 3_000_000
	op              = "evmsim.Simulate"
	executeSelector = "e9ae5c53"
	v4TooLittle     = "0x8b063d73"
	v3TooLittle     = "0x39d35496"
	wordBytes       = 32
	v4RevertBytes   = 4 + 2*wordBytes
	delegation      = "0xef0100"
	maxBody         = 1 << 20
)

type Swap struct {
	Owner    chain.EVMAddress
	Router   chain.EVMAddress
	Calldata []byte
}

type Override struct {
	StateDiff map[string]string `json:"stateDiff"`
}

type Reply struct {
	Result *string `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    any    `json:"data"`
	} `json:"error"`
}

type rpcCall struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

func Simulate(
	ctx context.Context, client *httpclient.Client, swap Swap, overrides map[string]Override, attrs []slog.Attr,
) (*big.Int, bool, error) {
	call := map[string]string{
		"from": EntryPoint, "to": swap.Owner.String(), "data": execute(swap.Router, swap.Calldata),
		"gas": "0x" + strconv.FormatUint(SimulationGas, 16),
	}
	params := []any{call, "latest"}
	if len(overrides) > 0 {
		params = append(params, overrides)
	}
	reply, err := Call(ctx, client, "eth_call", params, attrs)
	if err != nil {
		return nil, false, err
	}
	return outcome(reply, attrs)
}

func Call(
	ctx context.Context, client *httpclient.Client, method string, params []any, attrs []slog.Attr,
) (Reply, error) {
	raw, _ := json.Marshal(rpcCall{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(ctx, req)
	if err != nil {
		return Reply{}, errs.Wrap(err, errs.CodeRPCUnavailable, op, attrs...)
	}
	defer func() { _ = resp.Body.Close() }()
	var reply Reply
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&reply); err != nil {
		return Reply{}, errs.Wrap(err, errs.CodeDecodeFailed, op, attrs...)
	}
	return reply, nil
}

func outcome(reply Reply, attrs []slog.Attr) (*big.Int, bool, error) {
	if reply.Error == nil && reply.Result != nil {
		return nil, true, nil
	}
	data := ""
	if reply.Error != nil {
		data, _ = reply.Error.Data.(string)
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(data, "0x"))
	switch {
	case err == nil && strings.HasPrefix(data, v4TooLittle) && len(raw) == v4RevertBytes:
		return new(big.Int).SetBytes(raw[4+wordBytes:]), false, nil
	case data == v3TooLittle:
		return nil, false, errs.New(errs.CodeNoRoute, op,
			append(attrs, slog.String("reason", "too_little_received_unmeasured"))...)
	}
	return nil, false, errs.New(errs.CodeNoRoute, op, append(attrs, slog.String("reason", "simulation_reverted"))...)
}

func Delegation(code Reply) (chain.EVMAddress, bool) {
	raw := ""
	if code.Result != nil {
		raw = strings.ToLower(*code.Result)
	}
	if !strings.HasPrefix(raw, delegation) {
		return chain.EVMAddress{}, false
	}
	delegate, err := chain.ParseEVMAddress("0x" + raw[len(delegation):])
	return delegate, err == nil
}

func execute(router chain.EVMAddress, calldata []byte) string {
	inner := slices.Concat(router[:], make([]byte, wordBytes), calldata)
	body := slices.Concat(inner, make([]byte, (wordBytes-len(inner)%wordBytes)%wordBytes))
	out := executeSelector + strings.Repeat("0", 2*wordBytes) + Word(big.NewInt(2 * wordBytes))[2:] +
		Word(big.NewInt(int64(len(inner))))[2:] + hex.EncodeToString(body)
	return "0x" + out
}

func Word(v *big.Int) string { return "0x" + hex.EncodeToString(v.FillBytes(make([]byte, wordBytes))) }
