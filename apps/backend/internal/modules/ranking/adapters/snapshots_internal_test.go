package adapters

import (
	"math"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestSnapshotFrom(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	good, err := snapshotFrom(at, 9, 3, 4)
	if err != nil || good.Value.Uint64() != 9 || good.NavPerShare.Uint64() != 3 || good.TotalShares.Uint64() != 4 {
		t.Fatalf("snapshotFrom = %+v, %v", good, err)
	}
	for name, row := range map[string][3]int64{
		"value":  {-1, 1, 1},
		"nav":    {1, -1, 1},
		"shares": {1, 1, -1},
	} {
		if _, err := snapshotFrom(at, row[0], row[1], row[2]); err == nil {
			t.Errorf("%s: snapshotFrom accepted a negative amount", name)
		}
	}
}

func TestWirePortfolio_failsOnATotalOrRowThatDoesNotFit(t *testing.T) {
	t.Parallel()
	cabalID := ids.CabalIDFrom(ids.Real{}.NewV7())
	view := app.PortfolioView{Cabals: map[ids.CabalID]app.CabalView{}}
	view.Total = money.MicrosFromUint64(math.MaxUint64)
	if _, err := wirePortfolio(view); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("huge total: err = %v, want internal", err)
	}
	view.Total = money.MicrosFromUint64(1)
	view.Rows = []domain.PortfolioRow{{CabalID: cabalID, Shares: money.SharesUnitsFromUint64(math.MaxUint64)}}
	if _, err := wirePortfolio(view); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("huge shares: err = %v, want internal", err)
	}
}

func TestWirePortfolio_failsOnCashOrCountsThatDoNotFit(t *testing.T) {
	t.Parallel()
	cabalID := ids.CabalIDFrom(ids.Real{}.NewV7())
	row := domain.PortfolioRow{CabalID: cabalID}
	for name, change := range map[string]func(*app.PortfolioView){
		"cash":       func(v *app.PortfolioView) { v.Cash = money.MicrosFromUint64(math.MaxUint64) },
		"net worth":  func(v *app.PortfolioView) { v.NetWorth = money.MicrosFromUint64(math.MaxUint64) },
		"members":    func(v *app.PortfolioView) { v.Cabals[cabalID] = app.CabalView{MemberCount: math.MaxInt32 + 1} },
		"negative":   func(v *app.PortfolioView) { v.Cabals[cabalID] = app.CabalView{MemberCount: -1} },
		"open votes": func(v *app.PortfolioView) { v.OpenVotes = map[ids.CabalID]int{cabalID: math.MaxInt32 + 1} },
	} {
		view := app.PortfolioView{Cabals: map[ids.CabalID]app.CabalView{}}
		view.Rows = []domain.PortfolioRow{row}
		change(&view)
		if _, err := wirePortfolio(view); errs.CodeOf(err) != errs.CodeInternal {
			t.Errorf("%s: err = %v, want internal", name, err)
		}
	}
}
