package app

import "github.com/monaco/monaco/apps/backend/internal/modules/beta/port"

func Micros(b port.Balances) int64 {
	return b.Balance().Micros
}
