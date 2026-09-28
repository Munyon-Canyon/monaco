package main

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func TestConsumerSet_joinsEveryModulesConsumersBuiltFromTheConfig(t *testing.T) {
	t.Parallel()
	var set consumerSet
	named := func(durables ...string) func(config.Config) []bus.Consumer {
		return func(cfg config.Config) []bus.Consumer {
			out := make([]bus.Consumer, len(durables))
			for i, d := range durables {
				out[i] = bus.Consumer{Durable: cfg.Worker.HealthAddr + d}
			}
			return out
		}
	}
	set.add(named("a", "b"))
	set.add(named())
	set.add(named("c"))
	got := set.consumers(config.Config{Worker: config.Worker{HealthAddr: "x-"}})
	if len(got) != 3 || got[0].Durable != "x-a" || got[1].Durable != "x-b" || got[2].Durable != "x-c" {
		t.Fatalf("consumers = %+v, want x-a, x-b, x-c in registration order", got)
	}
}
