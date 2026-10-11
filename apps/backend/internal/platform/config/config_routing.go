package config

import (
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

type Routing struct {
	UniswapAPIKey       string
	UniswapBaseURL      string
	UniswapTimeout      time.Duration
	UniswapQuoteTimeout time.Duration
	MinEdgeBps          int32
	MaxImpactBps        int32
}

func routingFields() []field {
	return []field{
		text("UNISWAP_API_KEY", "", func(c *Config) *string { return &c.Routing.UniswapAPIKey }).secret(),
		text("UNISWAP_BASE_URL", "https://trade-api.gateway.uniswap.org",
			func(c *Config) *string { return &c.Routing.UniswapBaseURL }),
		duration("MONACO_TIMEOUT_UNISWAP", 5*time.Second,
			func(c *Config) *time.Duration { return &c.Routing.UniswapTimeout }),
		duration("MONACO_TIMEOUT_UNISWAP_QUOTE", 4*time.Second,
			func(c *Config) *time.Duration { return &c.Routing.UniswapQuoteTimeout }),
		count("ROUTER_MIN_EDGE_BPS", 10, func(c *Config) *int32 { return &c.Routing.MinEdgeBps }),
		count(
			"ROUTER_MAX_IMPACT_BPS",
			multichain.MaxImpactBps,
			func(c *Config) *int32 { return &c.Routing.MaxImpactBps },
		),
	}
}
