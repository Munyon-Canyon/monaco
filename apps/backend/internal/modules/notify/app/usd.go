package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const microsPerCent = 10_000

func usd(m money.Micros) string {
	cents := m.Uint64() / microsPerCent
	return fmt.Sprintf("$%s.%02d", grouped(strconv.FormatUint(cents/100, 10)), cents%100)
}

func grouped(digits string) string {
	var out strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out.WriteByte(',')
		}
		out.WriteRune(d)
	}
	return out.String()
}
