package observability

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func Setup(ctx context.Context, cfg config.Config) (func(context.Context) error, error) {
	if cfg.OTel.Endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}
	base, endpointErr := parseEndpoint(cfg.OTel.Endpoint)
	headers, headersErr := parseHeaders(cfg.OTel.Headers)
	traces, tracesErr := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(base+"/v1/traces"), otlptracehttp.WithHeaders(headers))
	metrics, metricsErr := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpointURL(base+"/v1/metrics"), otlpmetrichttp.WithHeaders(headers))
	logs, logsErr := otlploghttp.New(ctx,
		otlploghttp.WithEndpointURL(base+"/v1/logs"), otlploghttp.WithHeaders(headers))
	if err := errors.Join(endpointErr, headersErr, tracesErr, metricsErr, logsErr); err != nil {
		return nil, errs.Wrap(err, errs.CodeInvalidInput, "observability.Setup")
	}
	res := resource.NewSchemaless(attribute.String("service.name", cfg.OTel.ServiceName))
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(traces), sdktrace.WithResource(res))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metrics)),
		sdkmetric.WithResource(res))
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewBatchProcessor(logs)), sdklog.WithResource(res))
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	global.SetLoggerProvider(lp)
	return func(ctx context.Context) error {
		if err := errors.Join(tp.Shutdown(ctx), mp.Shutdown(ctx), lp.Shutdown(ctx)); err != nil {
			return errs.Wrap(err, errs.CodeUpstreamUnavailable, "observability.Shutdown")
		}
		return nil
	}, nil
}

func parseEndpoint(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errs.New(errs.CodeInvalidInput, "observability.parseEndpoint",
			slog.String("field", "OTEL_EXPORTER_OTLP_ENDPOINT"))
	}
	return strings.TrimSuffix(raw, "/"), nil
}

func parseHeaders(raw string) (map[string]string, error) {
	out := map[string]string{}
	for pair := range strings.SplitSeq(raw, ",") {
		if strings.TrimSpace(pair) == "" {
			continue
		}
		k, v, ok := strings.Cut(pair, "=")
		k = strings.TrimSpace(k)
		value, err := url.PathUnescape(strings.TrimSpace(v))
		if !ok || k == "" || err != nil {
			return nil, errs.New(errs.CodeInvalidInput, "observability.parseHeaders",
				slog.String("field", "OTEL_EXPORTER_OTLP_HEADERS"))
		}
		out[k] = value
	}
	return out, nil
}
