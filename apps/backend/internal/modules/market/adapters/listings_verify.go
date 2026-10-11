package adapters

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/coinbaseb20"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

const (
	symbolSelector       = "0x95d89b41"
	decimalsSelector     = "0x313ce567"
	policyIDSelector     = "0xdb3de624"
	isAuthorizedSelector = "0x55a1179e"
	b20PolicyRegistry    = "0x8453000000000000000000000000000000000002"
	b20ProbeHolder       = "1111111111111111111111111111111111111111"
	maxRPCBody           = 1 << 20
	abiWordSize          = 32
	maxListingDecimals   = 36
)

const (
	b20TransferSenderPolicy   = "b81736c875ab819dd97f59f2a6542cfb731ad52b4ae15a6f24df2fb02b0327f5"
	b20TransferReceiverPolicy = "8a4b3fa2d8b921852bc0089c6ef0958aa6961897be36fd731330fe2cd23f8363"
	b20TransferExecutorPolicy = "10be5173aff2a44e748bd9acd8b19fe34689581398a9db7ba2fb671e786ff7d8"
)

type SolanaMints interface {
	MintConfig(ctx context.Context, mint chain.SolanaAddress) (solana.MintConfig, error)
}

var _ app.ListingVerifier = (*ListingVerifier)(nil)

type ListingVerifier struct {
	http   *httpclient.Client
	rpcURL func(chain.Network) string
	mints  SolanaMints
}

func NewListingVerifier(c *httpclient.Client, rpcURL func(chain.Network) string, mints SolanaMints) *ListingVerifier {
	return &ListingVerifier{http: c, rpcURL: rpcURL, mints: mints}
}

func (v *ListingVerifier) Verify(ctx context.Context, l app.ListingToVerify) (uint8, error) {
	if l.Network == chain.NetworkSolana {
		cfg, err := v.mints.MintConfig(ctx, chain.SolanaAddress(l.TokenAddress))
		return cfg.Mint.Decimals, err
	}
	rpc := evmRPC{http: v.http, url: v.rpcURL(l.Network)}
	if err := rpc.expectSymbol(ctx, l.TokenAddress, l.Symbol); err != nil {
		return 0, err
	}
	if l.WrapperAddress != "" {
		if err := rpc.expectSymbol(ctx, l.WrapperAddress, "w"+l.Symbol); err != nil {
			return 0, err
		}
	}
	decimals, err := rpc.word(ctx, l.TokenAddress, decimalsSelector)
	if err != nil {
		return 0, err
	}
	if decimals > maxListingDecimals {
		return 0, errs.New(errs.CodeInvalidAddress, "market.ListingVerifier.decimals",
			slog.String("token", l.TokenAddress), slog.Uint64("decimals", decimals))
	}
	if l.Issuer == coinbaseb20.Issuer {
		if err := rpc.expectOpenTransfers(ctx, l.TokenAddress); err != nil {
			return 0, err
		}
	}
	return uint8(decimals), nil
}

type evmRPC struct {
	http *httpclient.Client
	url  string
}

func (r evmRPC) expectSymbol(ctx context.Context, token, want string) error {
	out, err := r.call(ctx, token, symbolSelector)
	if err != nil {
		return err
	}
	got, ok := abiString(out)
	if !ok || got != want {
		return errs.New(errs.CodeInvalidAddress, "market.ListingVerifier.symbol",
			slog.String("token", token), slog.String("symbol", got), slog.String("want", want))
	}
	return nil
}

func (r evmRPC) expectOpenTransfers(ctx context.Context, token string) error {
	for _, scope := range [...]string{b20TransferSenderPolicy, b20TransferReceiverPolicy, b20TransferExecutorPolicy} {
		policy, err := r.word(ctx, token, policyIDSelector+scope)
		if err != nil {
			return err
		}
		authorized, err := r.word(ctx, b20PolicyRegistry,
			fmt.Sprintf("%s%064x%024d%s", isAuthorizedSelector, policy, 0, b20ProbeHolder))
		if err != nil {
			return err
		}
		if authorized != 1 {
			return errs.New(errs.CodeInvalidAddress, "market.ListingVerifier.policy", slog.String("token", token),
				slog.Uint64("policy", policy), slog.String("reason", "transfer policy is an allowlist"))
		}
	}
	return nil
}

func (r evmRPC) word(ctx context.Context, to, data string) (uint64, error) {
	out, err := r.call(ctx, to, data)
	if err != nil {
		return 0, err
	}
	n, ok := abiWord(out, 0)
	if !ok {
		return 0, errs.New(errs.CodeDecodeFailed, "market.evmRPC.word", slog.String("to", to))
	}
	return n, nil
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type callArgs struct {
	To   string `json:"to"`
	Data string `json:"data"`
}

type rpcResponse struct {
	Result string `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (r evmRPC) call(ctx context.Context, to, data string) ([]byte, error) {
	const op = "market.evmRPC.call"
	body, _ := json.Marshal(rpcRequest{
		JSONRPC: "2.0", ID: 1, Method: "eth_call", Params: []any{callArgs{To: to, Data: data}, "latest"},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(body))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.http.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, errs.New(errs.CodeUpstreamUnavailable, op, slog.Int("status", resp.StatusCode))
	}
	var w rpcResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxRPCBody)).Decode(&w); err != nil {
		return nil, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	if w.Error != nil {
		return nil, errs.New(
			errs.CodeInvalidAddress,
			op,
			slog.String("to", to),
			slog.String("reverted", w.Error.Message),
		)
	}
	out, err := hex.DecodeString(strings.TrimPrefix(w.Result, "0x"))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	return out, nil
}

func abiWord(b []byte, at uint64) (uint64, bool) {
	if at > uint64(len(b)) || uint64(len(b))-at < abiWordSize {
		return 0, false
	}
	n := new(big.Int).SetBytes(b[at : at+abiWordSize])
	return n.Uint64(), n.IsUint64()
}

func abiString(b []byte) (string, bool) {
	offset, ok := abiWord(b, 0)
	if !ok {
		return "", false
	}
	n, ok := abiWord(b, offset)
	start := offset + abiWordSize
	if !ok || n > uint64(len(b))-start {
		return "", false
	}
	return string(b[start : start+n]), true
}
