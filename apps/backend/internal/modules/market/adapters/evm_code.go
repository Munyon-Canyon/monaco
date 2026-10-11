package adapters

import (
	"context"
	"log/slog"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/evmsim"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

const evmCodeOp = "market.EVMCode"

type EVMCode map[chain.Network]*httpclient.Client

var _ app.AccountCode = EVMCode{}

func NewEVMCode(d module.Deps) EVMCode { return EVMCode(wrapperRPC(d)) }

func (c EVMCode) Networks() []chain.Network {
	return slices.DeleteFunc(chain.Networks(), func(n chain.Network) bool { return c[n] == nil })
}

func (c EVMCode) Code(ctx context.Context, n chain.Network, account chain.EVMAddress) (string, error) {
	attrs := []slog.Attr{slog.String("network", string(n)), slog.String("account", account.String())}
	client, ok := c[n]
	if !ok {
		return "", errs.New(errs.CodeNoRoute, evmCodeOp, append(attrs, slog.String("reason", "unsupported"))...)
	}
	reply, err := evmsim.Call(ctx, client, "eth_getCode", []any{account.String(), "latest"}, attrs)
	if err != nil {
		return "", err
	}
	if reply.Result == nil {
		return "", errs.New(errs.CodeRPCUnavailable, evmCodeOp, append(attrs, slog.String("reason", "no_result"))...)
	}
	return *reply.Result, nil
}
