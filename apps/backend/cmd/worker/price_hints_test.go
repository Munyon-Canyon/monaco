package main

import (
	"context"
	"io"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type priceHintFunc struct {
	name string
	fn   func(context.Context, metric.Meter) (func(), error)
}

func (p priceHintFunc) Name() string { return p.name }

func (priceHintFunc) Mount(api.Mount) {}

func (priceHintFunc) Consumers() []bus.Consumer { return nil }

func (priceHintFunc) Pollers() []poller.Poller { return nil }

func (p priceHintFunc) PriceHints(ctx context.Context, meter metric.Meter) (func(), error) {
	return p.fn(ctx, meter)
}

func TestStartPriceHints_ignoresModulesThatDoNotPublishPriceHints(t *testing.T) {
	t.Parallel()
	stop, err := startPriceHints(t.Context(), module.NewSet(consumerModule(nil)), noop.NewMeterProvider())
	if err != nil {
		t.Fatal(err)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
}

func TestStartPriceHints_unsubscribesOnStop(t *testing.T) {
	t.Parallel()
	stopped := make(chan struct{})
	set := module.NewSet(consumerModule(nil), priceHintFunc{
		name: "hints",
		fn: func(context.Context, metric.Meter) (func(), error) {
			return func() { close(stopped) }, nil
		},
	})
	stop, err := startPriceHints(t.Context(), set, noop.NewMeterProvider())
	if err != nil {
		t.Fatal(err)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("stop did not unsubscribe")
	}
}

func TestStartPriceHints_unsubscribesEarlierLoopsWhenALaterOneFails(t *testing.T) {
	t.Parallel()
	stopped := make(chan struct{})
	set := module.NewSet(
		consumerModule(nil),
		priceHintFunc{
			name: "ok",
			fn: func(context.Context, metric.Meter) (func(), error) {
				return func() { close(stopped) }, nil
			},
		},
		priceHintFunc{
			name: "bad",
			fn: func(context.Context, metric.Meter) (func(), error) {
				return nil, errs.New(errs.CodeInternal, "test.later")
			},
		},
	)
	stop, err := startPriceHints(t.Context(), set, noop.NewMeterProvider())
	if errs.CodeOf(err) != errs.CodeInternal || !strings.Contains(err.Error(), "test.later") {
		t.Fatalf("startPriceHints = %v, want internal from the later loop", err)
	}
	if stop != nil {
		t.Fatal("startPriceHints returned a stop func with the error")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("the earlier subscription was left running")
	}
}

func TestRun_stopsPollersAndConsumersWhenPriceHintsFail(t *testing.T) {
	t.Parallel()
	url := testkit.StandaloneNATS(t)
	applyStreams(t, url)
	pool := testkit.DB(t)
	reg := &module.Registry{}
	reg.Add(func(module.Deps) module.Module {
		return priceHintFunc{
			name: "hints",
			fn: func(context.Context, metric.Meter) (func(), error) {
				return nil, errs.New(errs.CodeInternal, "test.price_hints")
			},
		}
	})
	err := run(t.Context(), io.Discard, []string{
		"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=" + url,
		"MONACO_HTTP_ADDR=127.0.0.1:0", "MONACO_WORKER_HEALTH_ADDR=127.0.0.1:0",
	}, noop.NewMeterProvider(), reg)
	if errs.CodeOf(err) != errs.CodeInternal || !strings.Contains(err.Error(), "test.price_hints") {
		t.Fatalf("run = %v, want internal from price hints", err)
	}
}
