package app

import (
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestPayResidual_refusesAnUnrepresentableAmount(t *testing.T) {
	t.Parallel()
	s := CashOutSales{ids: ids.Real{}}
	err := s.payResidual(
		t.Context(),
		db.Tx{},
		ids.CabalID{},
		ids.UserID{},
		money.MicrosFromUint64(math.MaxUint64),
		clock.Real{}.Now(),
	)
	if err == nil {
		t.Fatal("payResidual error = nil")
	}
	if err := s.payResidual(
		t.Context(),
		db.Tx{},
		ids.CabalID{},
		ids.UserID{},
		money.Micros{},
		clock.Real{}.Now(),
	); err != nil {
		t.Fatalf("payResidual of nothing = %v", err)
	}
}
