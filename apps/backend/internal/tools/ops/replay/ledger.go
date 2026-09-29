package replay

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"
)

type LedgerCheck struct {
	Name     string
	Tables   []string
	Handlers []string
	Check    func(ctx context.Context, source *pgxpool.Pool) ([]string, error)
}

var ledgerChecks []LedgerCheck

func RegisterLedgerCheck(c LedgerCheck) {
	if slices.ContainsFunc(ledgerChecks, func(have LedgerCheck) bool { return have.Name == c.Name }) {
		panic("replay: ledger check " + c.Name + " registered twice")
	}
	ledgerChecks = append(ledgerChecks, c)
}

func LedgerChecks() []LedgerCheck { return slices.Clone(ledgerChecks) }
