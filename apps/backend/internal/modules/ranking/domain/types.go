package domain

import (
	"cmp"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Bps int64

type Range string

const (
	Range1H  Range = "1H"
	Range1D  Range = "1D"
	Range1W  Range = "1W"
	Range1M  Range = "1M"
	RangeAll Range = "ALL"
)

func ParseRange(raw string) (Range, error) {
	r := Range(raw)
	if _, ok := r.span(); ok || r == RangeAll {
		return r, nil
	}
	return "", errs.New(errs.CodeInvalidInput, "ranking.ParseRange", slog.String("raw", raw))
}

func (r Range) Start(now time.Time) (time.Time, bool) {
	span, ok := r.span()
	if !ok {
		return time.Time{}, false
	}
	return now.Add(-span), true
}

func (r Range) span() (time.Duration, bool) {
	switch r {
	case Range1H:
		return time.Hour, true
	case Range1D:
		return 24 * time.Hour, true
	case Range1W:
		return 7 * 24 * time.Hour, true
	case Range1M:
		return 30 * 24 * time.Hour, true
	case RangeAll:
	}
	return 0, false
}

type Board string

const (
	BoardCabals       Board = "cabals"
	BoardPeople       Board = "people"
	BoardCabalMembers Board = "cabal_members"
)

type Flag string

const (
	FlagStalePrices    Flag = "stale_prices"
	FlagUnpricedAssets Flag = "unpriced_assets"
)

type Filter string

const (
	FilterAll     Filter = "all"
	FilterFriends Filter = "friends"
)

func ParseFilter(raw string) (Filter, error) {
	switch f := Filter(raw); f {
	case FilterAll, FilterFriends:
		return f, nil
	}
	return "", errs.New(errs.CodeInvalidInput, "ranking.ParseFilter", slog.String("raw", raw))
}

type Sort string

const (
	SortReturn Sort = "return_pct"
	SortPnL    Sort = "pnl"
	SortName   Sort = "name"
)

func ParseSort(raw string) (Sort, error) {
	switch s := Sort(raw); s {
	case SortReturn, SortPnL, SortName:
		return s, nil
	}
	return "", errs.New(errs.CodeInvalidInput, "ranking.ParseSort", slog.String("raw", raw))
}

type Membership string

const (
	MembershipAll               Membership = "all"
	MembershipAcceptingRequests Membership = "accepting_requests"
	MembershipOpen              Membership = "open"
	MembershipApproval          Membership = "approval"
)

func ParseMembership(raw string) (Membership, error) {
	switch m := Membership(raw); m {
	case MembershipAll, MembershipAcceptingRequests, MembershipOpen, MembershipApproval:
		return m, nil
	}
	return "", errs.New(errs.CodeInvalidInput, "ranking.ParseMembership", slog.String("raw", raw))
}

func (m Membership) JoinModes() []string {
	switch m {
	case MembershipAcceptingRequests:
		return []string{"open", "request"}
	case MembershipOpen:
		return []string{"open"}
	case MembershipApproval:
		return []string{"request"}
	case MembershipAll:
	}
	return []string{}
}

const MaxMinMembers = 10000

func ParseMinMembers(raw *int) (int, error) {
	if raw == nil {
		return 0, nil
	}
	if *raw < 0 || *raw > MaxMinMembers {
		return 0, errs.New(errs.CodeInvalidInput, "ranking.ParseMinMembers", slog.Int("min_members", *raw))
	}
	return *raw, nil
}

type View struct {
	Sort       Sort
	Membership Membership
	MinMembers int
}

func (v View) IsDefault() bool {
	return (v.Sort == "" || v.Sort == SortReturn) &&
		(v.Membership == "" || v.Membership == MembershipAll) && v.MinMembers == 0
}

func SortEntries(rows []Entry, sort Sort) {
	if sort == "" || sort == SortReturn {
		return
	}
	slices.SortStableFunc(rows, func(a, b Entry) int {
		if sort == SortName {
			if c := cmp.Compare(strings.ToLower(a.Subject.Name), strings.ToLower(b.Subject.Name)); c != 0 {
				return c
			}
			return cmp.Compare(a.Rank, b.Rank)
		}
		if c := cmp.Compare(b.PnL.Int64(), a.PnL.Int64()); c != 0 {
			return c
		}
		return cmp.Compare(a.Rank, b.Rank)
	})
}
