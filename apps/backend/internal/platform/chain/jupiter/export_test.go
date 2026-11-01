package jupiter

import (
	"encoding/json"

	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func ParsePrice(raw string) (money.Micros, bool) { return parsePrice(json.Number(raw)) }

func (c *Client) ObserveUnpriced(meter metric.Meter) { c.skipped.observe(meter) }
