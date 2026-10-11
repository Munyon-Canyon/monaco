package adapters

import (
	"context"
	"log/slog"
	"math/big"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/evmsim"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

const evmVenueChooseOp = "trading.EVMVenue.Choose"

func (v *EVMVenue) candidates(
	ctx context.Context, spec app.EVMSwapSpec, approval approvalWire, attrs []slog.Attr,
) (app.EVMSwapPlan, error) {
	def, err := v.build(ctx, spec, approval, 0, attrs)
	if err != nil {
		return app.EVMSwapPlan{}, err
	}
	hookless := spec
	hookless.Route = mc.RouteOptions{HooksOptions: mc.NoHooks}
	alt, err := v.build(ctx, hookless, approval, 0, attrs)
	if err != nil || alt.Swap.To != def.Swap.To {
		return v.plan(ctx, spec, approval, attrs)
	}
	return app.EVMSwapPlan{Approvals: def.Approvals, Candidates: []app.EVMSwapCandidate{
		{Route: spec.Route, Swap: def.Swap, OutAmount: def.OutAmount},
		{Route: hookless.Route, Swap: alt.Swap, OutAmount: alt.OutAmount},
	}}, nil
}

func (v *EVMVenue) Choose(
	ctx context.Context, spec app.EVMSwapSpec, candidates []app.EVMSwapCandidate,
) (app.EVMSwapPlan, error) {
	attrs := []slog.Attr{slog.String("network", string(spec.Network))}
	client, ok := v.rpc[spec.Network]
	if !ok {
		return app.EVMSwapPlan{}, errs.New(errs.CodeNoRoute, evmVenueChooseOp, attrs...)
	}
	delegated, err := isDelegated(ctx, client, spec.Owner, attrs)
	if err != nil {
		return app.EVMSwapPlan{}, err
	}
	best, bestOut := -1, new(big.Int)
	for i, c := range candidates {
		out, err := simulated(ctx, client, spec, c, !delegated, attrs)
		switch {
		case errs.CodeOf(err) == errs.CodeNoRoute:
		case err != nil:
			return app.EVMSwapPlan{}, err
		case out.Cmp(bestOut) > 0:
			best, bestOut = i, out
		}
	}
	if best < 0 {
		return app.EVMSwapPlan{}, errs.New(errs.CodeNoRoute, evmVenueChooseOp,
			append(attrs, slog.String("reason", "every_route_failed"))...)
	}
	spec.Route = candidates[best].Route
	plan, err := v.plan(ctx, spec, approvalWire{}, attrs)
	if err == nil && plan.Swap.To != candidates[best].Swap.To {
		err = errs.New(
			errs.CodeDecodeFailed,
			evmVenueChooseOp,
			append(attrs, slog.String("to", plan.Swap.To.String()))...)
	}
	if err != nil {
		return app.EVMSwapPlan{}, err
	}
	plan.Approvals = nil
	return plan, nil
}

func isDelegated(
	ctx context.Context, client *httpclient.Client, owner chain.EVMAddress, attrs []slog.Attr,
) (bool, error) {
	code, err := evmsim.Call(ctx, client, "eth_getCode", []any{owner.String(), "latest"}, attrs)
	if err == nil && (code.Error != nil || code.Result == nil) {
		err = errs.New(errs.CodeRPCUnavailable, evmVenueChooseOp, append(attrs, slog.String("reason", "code"))...)
	}
	_, delegated := evmsim.Delegation(code)
	return delegated, err
}

func simulated(
	ctx context.Context, client *httpclient.Client, spec app.EVMSwapSpec, c app.EVMSwapCandidate, direct bool,
	attrs []slog.Attr,
) (*big.Int, error) {
	received, met, err := evmsim.Simulate(ctx, client,
		evmsim.Swap{Owner: spec.Owner, Router: c.Swap.To, Calldata: c.Swap.Data, Direct: direct}, nil, attrs)
	if met {
		return new(big.Int).SetUint64(c.OutAmount), nil
	}
	return received, err
}
