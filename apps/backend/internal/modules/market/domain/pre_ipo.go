package domain

import (
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type SellWindow string

const (
	SellWindowAnyTime         SellWindow = "any_time"
	SellWindowTenderQuarterly SellWindow = "tender_quarterly"
)

func ParseSellWindow(raw string) (SellWindow, error) {
	switch w := SellWindow(raw); w {
	case SellWindowAnyTime, SellWindowTenderQuarterly:
		return w, nil
	}
	return "", errs.New(errs.CodeDecodeFailed, "market.ParseSellWindow", slog.String("sell_window", raw))
}

func DateOn(t time.Time) Date { return dateOf(t) }

func (d Date) Time() time.Time { return d.midnight() }

type Estimate struct {
	Price money.Micros
	AsOf  Date
}

type Round struct {
	On    Date
	Price money.Micros
}

type Profile struct {
	CompanyKey   string
	Estimate     Estimate
	Valuation    string
	MinOrder     money.Micros
	SellWindow   SellWindow
	NextTenderOn *Date
	ImageURL     string
	Rounds       []Round
}
