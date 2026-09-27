package bus

import (
	"maps"
	"slices"
	"strings"

	"github.com/nats-io/nats.go"
)

type natsCarrier nats.Header

func (c natsCarrier) Get(key string) string {
	if v := c[key]; len(v) > 0 {
		return v[0]
	}
	for k, v := range c {
		if strings.EqualFold(k, key) && len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

func (c natsCarrier) Set(key, value string) { c[key] = []string{value} }

func (c natsCarrier) Keys() []string { return slices.Sorted(maps.Keys(c)) }
