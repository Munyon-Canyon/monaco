package module_test

import (
	"testing"

	"golang.org/x/time/rate"

	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestRegistry_givesEveryModuleOneUniswapLimiter(t *testing.T) {
	t.Parallel()
	var reg module.Registry
	var built []module.Deps
	var routed []string
	for _, name := range []string{"alpha", "beta"} {
		reg.Add(func(d module.Deps) module.Module {
			built = append(built, d)
			return &fake{name: name, routed: &routed}
		})
	}

	reg.Build(module.Deps{})

	first, second := built[0].UniswapLimiter(), built[1].UniswapLimiter()
	if first == nil || first != second || first.Limit() != rate.Limit(module.UniswapCallsPerSecond) ||
		first.Burst() != module.UniswapCallsPerSecond {
		t.Fatalf("limiters %p and %p, want one shared 5/s limiter", first, second)
	}
	if lone := (module.Deps{}).UniswapLimiter(); lone == first {
		t.Fatal("deps built outside a registry share the registry's limiter, want their own")
	}
	given := rate.NewLimiter(1, 1)
	reg.Build(module.Deps{Uniswap: given})
	if built[2].UniswapLimiter() != given {
		t.Fatal("Build replaced the limiter it was given")
	}
}
