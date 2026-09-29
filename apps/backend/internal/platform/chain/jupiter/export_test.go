package jupiter

import (
	"encoding/json"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func ParsePrice(raw string) (money.Micros, bool) { return parsePrice(json.Number(raw)) }
