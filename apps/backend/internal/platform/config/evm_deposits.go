package config

import (
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const noEVMDepositNetworks = "none"

func evmDepositNetworks(key string, at func(*Config) *[]chain.Network) field {
	return field{
		key:      key,
		fallback: string(chain.NetworkBase) + "," + string(chain.NetworkArbitrum) + "," + string(chain.NetworkEthereum),
		want:     "comma-separated distinct EVM networks taking USDC: base, arbitrum or ethereum, or none",
		set: func(c *Config, v string) bool {
			*at(c) = nil
			if v == noEVMDepositNetworks {
				return true
			}
			for _, raw := range strings.Split(v, ",") {
				n, err := chain.ParseNetwork(raw)
				if err != nil || raw == "" || !n.IsEVM() || n.Cash().Symbol != "USDC" || slices.Contains(*at(c), n) {
					return false
				}
				*at(c) = append(*at(c), n)
			}
			return true
		},
		get: func(c *Config) string {
			if len(*at(c)) == 0 {
				return noEVMDepositNetworks
			}
			names := make([]string, len(*at(c)))
			for i, n := range *at(c) {
				names[i] = string(n)
			}
			return strings.Join(names, ",")
		},
	}
}
