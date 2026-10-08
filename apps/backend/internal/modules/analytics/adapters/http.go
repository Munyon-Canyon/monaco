package adapters

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/analyticsapi"
)

type HTTP struct {
	Money      app.Money
	Governance app.Governance
	Social     app.Social
	Safety     app.Safety
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

func (h HTTP) GetGovernanceDashboard(
	ctx context.Context, req api.GetGovernanceDashboardRequestObject,
) (api.GetGovernanceDashboardResponseObject, error) {
	window, err := app.ParseWindow(req.Params.From, req.Params.To, string(req.Params.Bucket))
	if err != nil {
		return nil, err
	}
	view, err := h.Governance.Dashboard(ctx, window)
	if err != nil {
		return nil, err
	}
	buckets := make([]api.GovernanceBucket, len(view.Buckets))
	for i, b := range view.Buckets {
		buckets[i] = api.GovernanceBucket{
			BucketStart: b.Start, Created: b.Created, Passed: b.Passed, Failed: b.Failed, Expired: b.Expired,
			ExecutionBlocked: b.ExecutionBlocked,
		}
	}
	rows := make([]api.GovernanceParticipation, len(view.Participation))
	for i, p := range view.Participation {
		rows[i] = api.GovernanceParticipation{
			CabalId: p.CabalID.UUID(), Proposals: p.Proposals, EligibleVotes: p.Eligible, VotesCast: p.Voted,
			ParticipationBps: p.Bps,
		}
	}
	var median *int64
	if view.Passed.Passed > 0 {
		seconds := int64(view.Passed.Median / time.Second)
		median = &seconds
	}
	return api.GetGovernanceDashboard200JSONResponse{
		From: window.From, To: window.To, Bucket: api.DashboardBucket(window.Size), OpenProposals: view.Open,
		PassedInRange: view.Passed.Passed, MedianSecondsToPass: median, Buckets: buckets, Participation: rows,
	}, nil
}

func dashboardPoints(series []app.SeriesPoint) []api.DashboardPoint {
	out := make([]api.DashboardPoint, len(series))
	for i, p := range series {
		out[i] = api.DashboardPoint{BucketStart: p.Start, Metric: p.Metric, Group: p.Group, Count: p.Count}
	}
	return out
}

func (h HTTP) GetSocialDashboard(
	ctx context.Context, req api.GetSocialDashboardRequestObject,
) (api.GetSocialDashboardResponseObject, error) {
	window, err := app.ParseWindow(req.Params.From, req.Params.To, string(req.Params.Bucket))
	if err != nil {
		return nil, err
	}
	view, err := h.Social.Dashboard(ctx, window)
	if err != nil {
		return nil, err
	}
	states := make([]api.UserStateCount, len(view.States))
	for i, s := range view.States {
		states[i] = api.UserStateCount{AuthState: s.AuthState, AccountStatus: s.AccountStatus, Users: s.Users}
	}
	return api.GetSocialDashboard200JSONResponse{
		From: window.From, To: window.To, Bucket: api.DashboardBucket(window.Size), UsersTotal: view.Users,
		Cabals: api.DashboardCabals{
			Total: view.Cabals.Cabals, Banned: view.Cabals.Banned, MembersP50: view.Cabals.MembersP50,
			MembersP90: view.Cabals.MembersP90, MembersMax: view.Cabals.MembersMax,
		},
		UserStates: states, Series: dashboardPoints(view.Series),
	}, nil
}

func (h HTTP) GetSafetyDashboard(
	ctx context.Context, req api.GetSafetyDashboardRequestObject,
) (api.GetSafetyDashboardResponseObject, error) {
	window, err := app.ParseWindow(req.Params.From, req.Params.To, string(req.Params.Bucket))
	if err != nil {
		return nil, err
	}
	view, err := h.Safety.Dashboard(ctx, window)
	if err != nil {
		return nil, err
	}
	return api.GetSafetyDashboard200JSONResponse{
		From: window.From, To: window.To, Bucket: api.DashboardBucket(window.Size), BannedUsers: view.BannedUsers,
		BannedCabals: view.BannedCabals, Series: dashboardPoints(view.Series),
	}, nil
}
