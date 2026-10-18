package adapters

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	evmCashOp         = "trading.EVMCash.Balance"
	balanceOfSelector = "0x70a08231"
)

var _ app.EVMCash = (*EVMCash)(nil)

type EVMCash struct {
	rpc map[chain.Network]*httpclient.Client
}

func NewEVMCash(rpc map[chain.Network]*httpclient.Client) *EVMCash { return &EVMCash{rpc: rpc} }

func (c *EVMCash) Balance(ctx context.Context, network chain.Network, owner chain.EVMAddress) (money.Micros, error) {
	attrs := []slog.Attr{slog.String("network", string(network)), slog.String("owner", owner.String())}
	client, ok := c.rpc[network]
	if !ok {
		return money.Micros{}, errs.New(errs.CodeInvalidInput, evmCashOp, attrs...)
	}
	call := map[string]string{"to": network.Cash().Address, "data": balanceOfSelector + topicOf(owner)[2:]}
	var result string
	scan := arrivalScan{client: client, pace: &pacer{}, network: network, owner: owner, attrs: attrs}
	if err := scan.call(ctx, "eth_call", []any{call, "latest"}, &result); err != nil {
		return money.Micros{}, err
	}
	balance, err := hexBig(result)
	if err != nil || len(result) != 2+evmTopicHexChars || !balance.IsUint64() {
		return money.Micros{}, errs.New(errs.CodeDecodeFailed, evmCashOp,
			append(attrs, slog.String("result", result))...)
	}
	return money.MicrosFromUint64(balance.Uint64()), nil
}
