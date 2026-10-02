package errs

import (
	"maps"
	"slices"
)

type Code string

type Row struct {
	Name      string
	Kind      Kind
	Retryable bool
	Alert     bool
	Message   string
}

func rowGroups() []func() map[Code]Row {
	return []func() map[Code]Row{
		platformRows, identityRows, treasuryRows, marketRows,
		tradingRows, governanceRows, rankingRows, apnsRows,
		analyticsRows, cabalRows, socialRows, referralsRows,
	}
}

func table() map[Code]Row {
	rows := map[Code]Row{}
	for _, group := range rowGroups() {
		maps.Copy(rows, group())
	}
	return rows
}

func row(code Code) Row {
	for _, group := range rowGroups() {
		if r, ok := group()[code]; ok {
			return r
		}
	}
	return platformRows()[CodeInternal]
}

func Name(code Code) string { return row(code).Name }

func KindOf(code Code) Kind { return row(code).Kind }

func Retryable(code Code) bool { return row(code).Retryable }

func Alert(code Code) bool { return row(code).Alert }

func Message(code Code) string { return row(code).Message }

func All() []Code { return slices.Sorted(maps.Keys(table())) }
