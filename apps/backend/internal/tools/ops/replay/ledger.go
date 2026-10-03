package replay

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

type LedgerCheck struct {
	Name     string
	Tables   []string
	Handlers []string
	Check    func(ctx context.Context, source *pgxpool.Pool) ([]string, error)
}

var ledgerChecks []func(config.Config) LedgerCheck

func RegisterLedgerCheck(build func(config.Config) LedgerCheck) {
	ledgerChecks = append(ledgerChecks, build)
}

func LedgerChecks(cfg config.Config) []LedgerCheck {
	out := make([]LedgerCheck, 0, len(ledgerChecks))
	seen := map[string]bool{}
	for _, build := range ledgerChecks {
		c := build(cfg)
		if seen[c.Name] {
			panic("replay: ledger check " + c.Name + " registered twice")
		}
		seen[c.Name] = true
		out = append(out, c)
	}
	return out
}
