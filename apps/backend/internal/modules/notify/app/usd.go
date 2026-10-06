package app

import (
	"fmt"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const microsPerCent = 10_000

func usd(m money.Micros) string {
	cents := m.Uint64() / microsPerCent
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}
