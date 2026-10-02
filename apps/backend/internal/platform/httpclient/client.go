package httpclient

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
)

const tracerName = "github.com/monaco/monaco/apps/backend/internal/platform/httpclient"

type Option func(*Client)

type Client struct {
	name       string
	base       *url.URL
	timeout    time.Duration
	attempts   int
	backoff    time.Duration
	maxDelay   time.Duration
	settings   gobreaker.Settings
	breaker    *gobreaker.TwoStepCircuitBreaker[struct{}]
	http       *http.Client
	jitter     func(time.Duration) time.Duration
	callerGone error
}

func WithBaseURL(raw string) Option {
	return func(c *Client) {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			panic("httpclient: " + c.name + " base URL must be absolute: " + raw)
		}
		if u.Path == "" {
			u.Path = "/"
		}
		c.base = u
	}
}

func WithTimeout(d time.Duration) Option { return func(c *Client) { c.timeout = d } }

func WithRetry(maxAttempts int, base, maxDelay time.Duration) Option {
	return func(c *Client) { c.attempts, c.backoff, c.maxDelay = maxAttempts, base, maxDelay }
}

func WithBreaker(st gobreaker.Settings) Option { return func(c *Client) { c.settings = st } }

func WithTransport(rt http.RoundTripper) Option { return func(c *Client) { c.http.Transport = rt } }

func New(name string, opts ...Option) *Client {
	c := &Client{
		name:       name,
		attempts:   1,
		callerGone: errs.New(errs.CodeUpstreamUnavailable, "httpclient.callerGone", slog.String("upstream", name)),
		http:       &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()},
		jitter:     func(d time.Duration) time.Duration { return rand.N(d + 1) },
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.timeout <= 0 {
		panic("httpclient: " + name + " needs WithTimeout from config")
	}
	if tr, ok := c.http.Transport.(*http.Transport); ok {
		tr.ResponseHeaderTimeout = c.timeout
	}
	st := c.settings
	st.Name = name
	excluded := st.IsExcluded
	st.IsExcluded = func(err error) bool {
		return errors.Is(err, c.callerGone) || excluded != nil && excluded(err)
	}
	c.breaker = gobreaker.NewTwoStepCircuitBreaker[struct{}](st)
	return c
}

func (c *Client) CloseIdleConnections() {
	c.http.CloseIdleConnections()
}

func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if req.Body != nil && req.GetBody == nil {
		return nil, errs.New(errs.CodeInvalidInput, "httpclient.Do", slog.String("upstream", c.name))
	}
	deadline := errs.New(errs.CodeUpstreamTimeout, "httpclient.deadline", slog.String("upstream", c.name))
	ctx, cancel := context.WithTimeoutCause(ctx, c.timeout, deadline)
	for attempt := 1; ; attempt++ {
		resp, err := c.send(ctx, req, attempt, deadline)
		if err == nil && !retryable(resp.StatusCode) {
			resp.Body = cancelBody{ReadCloser: resp.Body, cancel: cancel, upstream: c.name}
			return resp, nil
		}
		status := 0
		if resp != nil {
			status = resp.StatusCode
			drain(resp)
		}
		if attempt < c.attempts && ctx.Err() == nil && !breakerOpen(err) && c.wait(ctx, attempt, status, resp) {
			continue
		}
		cancel()
		return nil, c.fail(ctx, err, status, attempt)
	}
}

func (c *Client) send(ctx context.Context, req *http.Request, attempt int, deadline error) (*http.Response, error) {
	r, err := c.outbound(req)
	if err != nil {
		return nil, err
	}
	done, err := c.breaker.Allow()
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeUpstreamUnavailable, "httpclient.send")
	}
	tp := otel.GetTracerProvider()
	if parent := trace.SpanFromContext(ctx); parent.IsRecording() {
		tp = parent.TracerProvider()
	}
	ctx, span := tp.Tracer(tracerName).Start(ctx, c.name+" "+req.Method, trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("upstream", c.name), attribute.Int("attempt", attempt),
			attribute.String("http.request.method", req.Method)))
	defer span.End()
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(r.Header))
	resp, err := c.http.Do(r.WithContext(ctx))
	if err != nil {
		span.SetStatus(codes.Error, "transport")
		if ctx.Err() != nil && !errors.Is(context.Cause(ctx), deadline) {
			done(c.callerGone)
		} else {
			done(err)
		}
		return nil, errs.Wrap(err, errs.CodeUpstreamUnavailable, "httpclient.send")
	}
	span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	if resp.StatusCode >= http.StatusInternalServerError || resp.StatusCode == http.StatusTooManyRequests {
		span.SetStatus(codes.Error, resp.Status)
		done(errs.New(errs.CodeUpstreamUnavailable, "httpclient.send"))
		return resp, nil
	}
	done(nil)
	return resp, nil
}

func (c *Client) outbound(req *http.Request) (*http.Request, error) {
	r := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeUpstreamUnavailable, "httpclient.GetBody")
		}
		r.Body = body
	}
	if c.base != nil {
		r.URL = c.base.JoinPath(req.URL.Path)
		r.URL.RawQuery = req.URL.RawQuery
	}
	return r, nil
}

func (c *Client) wait(ctx context.Context, attempt, status int, resp *http.Response) bool {
	delay, ok := retryAfter(resp)
	if !ok {
		delay = c.backoff
		for i := 1; i < attempt && delay < c.maxDelay; i++ {
			delay *= 2
		}
		delay = c.jitter(min(delay, c.maxDelay))
	}
	boundary.Warn(ctx, observability.HTTPRetry, slog.String("upstream", c.name), slog.Int("attempt", attempt),
		slog.Int("status", status), slog.Duration("delay", delay))
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (c *Client) fail(ctx context.Context, err error, status, attempt int) error {
	attrs := []slog.Attr{
		slog.String("upstream", c.name), slog.Int("status", status), slog.Int("attempt", attempt),
	}
	code := errs.CodeUpstreamUnavailable
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || responseHeaderTimeout(err) {
		code = errs.CodeUpstreamTimeout
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			err = errors.Join(err, context.Cause(ctx))
		}
	}
	return errs.Wrap(err, code, "httpclient.Do", attrs...)
}

func responseHeaderTimeout(err error) bool {
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		return false
	}
	return strings.Contains(netErr.Error(), "timeout awaiting response headers")
}

func breakerOpen(err error) bool {
	return errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests)
}

func retryable(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

func retryAfter(resp *http.Response) (time.Duration, bool) {
	if resp == nil {
		return 0, false
	}
	v := resp.Header.Get("Retry-After")
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second, true
	}
	if at, err := http.ParseTime(v); err == nil {
		return max(at.Sub(clock.Real{}.Now()), 0), true
	}
	return 0, false
}

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
}

type cancelBody struct {
	io.ReadCloser
	cancel   context.CancelFunc
	upstream string
}

func (b cancelBody) Close() error {
	defer b.cancel()
	if err := b.ReadCloser.Close(); err != nil {
		return errs.Wrap(err, errs.CodeUpstreamUnavailable, "httpclient.Body.Close",
			slog.String("upstream", b.upstream))
	}
	return nil
}
