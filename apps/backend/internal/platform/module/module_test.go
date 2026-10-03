package module_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type fake struct {
	name      string
	durables  []string
	pollers   []string
	routed    *[]string
	built     module.Deps
	routesSaw httpx.Routes
}

func (f *fake) Name() string { return f.name }

func (f *fake) Routes(r *httpx.Routes) {
	f.routesSaw = *r
	*f.routed = append(*f.routed, f.name)
}

func (f *fake) Consumers() []bus.Consumer {
	out := make([]bus.Consumer, len(f.durables))
	for i, d := range f.durables {
		out[i] = bus.Consumer{Durable: d}
	}
	return out
}

func (f *fake) Pollers() []poller.Poller {
	out := make([]poller.Poller, len(f.pollers))
	for i, p := range f.pollers {
		out[i] = namedPoller(p)
	}
	return out
}

type namedPoller string

func (p namedPoller) Name() string { return string(p) }

func (namedPoller) Interval() time.Duration { return time.Minute }

func (namedPoller) Tick(context.Context) (poller.Report, error) { return poller.Report{}, nil }

func TestRegistry_buildsThePlatformThenEachModuleInRegistrationOrder(t *testing.T) {
	t.Parallel()
	hub, err := sse.NewHub(sse.NoMemberships{}, noop.NewMeterProvider())
	if err != nil {
		t.Fatal(err)
	}
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	deps := module.Deps{Clock: clk, Hub: hub}
	var routed []string
	alpha := &fake{
		name: "alpha", durables: []string{"alpha.a", "alpha.b"}, pollers: []string{"alpha.p"}, routed: &routed,
	}
	beta := &fake{name: "beta", durables: []string{"beta.a"}, routed: &routed}
	var reg module.Registry
	for _, m := range []*fake{alpha, beta} {
		reg.Add(func(d module.Deps) module.Module {
			m.built = d
			return m
		})
	}
	set := reg.Build(deps)

	if alpha.built.Hub != hub || beta.built.Clock != clk {
		t.Fatal("a module was built without the deps passed to Build")
	}
	if got := names(set, module.Module.Name); !slices.Equal(got, []string{"platform", "alpha", "beta"}) {
		t.Fatalf("modules = %v, want platform, alpha, beta", got)
	}
	routes := set.Routes()
	if want := sse.NewStream(hub, clk); routes.Stream != want || alpha.routesSaw.Stream != want {
		t.Fatal("the platform module did not mount the stream route before the other modules")
	}
	if !slices.Equal(routed, []string{"alpha", "beta"}) {
		t.Fatalf("Routes called on %v, want alpha then beta", routed)
	}
	durables := names(set.Consumers(), func(c bus.Consumer) string { return c.Durable })
	if !slices.Equal(durables, []string{"alpha.a", "alpha.b", "beta.a"}) {
		t.Fatalf("consumers = %v, want alpha.a, alpha.b, beta.a", durables)
	}
	if got := names(set.Pollers(), poller.Poller.Name); !slices.Equal(got, []string{"platform.retention", "alpha.p"}) {
		t.Fatalf("pollers = %v, want platform.retention, alpha.p", got)
	}
}

func names[T any](all []T, name func(T) string) []string {
	out := make([]string, len(all))
	for i, v := range all {
		out[i] = name(v)
	}
	return out
}

func TestNewSet_panicsOnADuplicateModuleName(t *testing.T) {
	t.Parallel()
	var routed []string
	defer func() {
		if got := recover(); got != "module: platform registered twice" {
			t.Fatalf("recover = %v, want the duplicate named", got)
		}
	}()
	var reg module.Registry
	reg.Add(func(module.Deps) module.Module { return &fake{name: "platform", routed: &routed} })
	reg.Build(module.Deps{})
}

type wiring struct {
	fake
	saw []string
}

func (w *wiring) Wire(set module.Set) {
	w.saw = append(w.saw, names(set, module.Module.Name)...)
}

func TestNewSet_wiresEachWirerWithTheWholeSetOnce(t *testing.T) {
	t.Parallel()
	var routed []string
	wirer := &wiring{fake: fake{name: "wirer", routed: &routed}}
	var reg module.Registry
	reg.Add(func(module.Deps) module.Module { return wirer })
	reg.Add(func(module.Deps) module.Module { return &fake{name: "later", routed: &routed} })
	reg.Build(module.Deps{})
	if want := []string{"platform", "wirer", "later"}; !slices.Equal(wirer.saw, want) {
		t.Fatalf("Wire saw %v, want %v: once, with every module including ones built after it", wirer.saw, want)
	}
}
