package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/analyticsapi"
)

type HTTP struct {
	Money app.Money
}

var _ api.StrictServerInterface = HTTP{}

func (h HTTP) GetMoneyDashboard(
	ctx context.Context, req api.GetMoneyDashboardRequestObject,
) (api.GetMoneyDashboardResponseObject, error) {
	window, err := app.ParseWindow(req.Params.From, req.Params.To, string(req.Params.Bucket))
	if err != nil {
		return nil, err
	}
	view, err := h.Money.Dashboard(ctx, window)
	if err != nil {
		return nil, err
	}
	buckets := make([]api.MoneyBucket, len(view.Buckets))
	for i, b := range view.Buckets {
		buckets[i] = api.MoneyBucket{
			BucketStart:      b.Start,
			DepositCount:     b.Deposits.Count,
			DepositMicros:    b.Deposits.USDC.String(),
			FundCount:        b.Funds.Count,
			FundMicros:       b.Funds.USDC.String(),
			SwapBuyMicros:    b.SwapBuy.String(),
			SwapSellMicros:   b.SwapSell.String(),
			CashOutCount:     b.CashOuts.Count,
			CashOutMicros:    b.CashOuts.USDC.String(),
			WithdrawalCount:  b.Withdrawals.Count,
			WithdrawalMicros: b.Withdrawals.USDC.String(),
		}
	}
	return api.GetMoneyDashboard200JSONResponse{
		From: window.From, To: window.To, Bucket: api.DashboardBucket(window.Size), AsOf: view.AsOf,
		PotMicros: view.Pots.String(), PlatformBalanceMicros: view.Platform.String(),
		TotalValueHeldMicros: view.Total.String(), Buckets: buckets,
	}, nil
}
