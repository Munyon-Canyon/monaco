package apns

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/payload"
	"github.com/sideshow/apns2/token"
	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

const (
	tracerName     = "github.com/monaco/monaco/apps/backend/internal/platform/apns"
	opNew          = "apns.New"
	opSend         = "apns.Send"
	maxRetryAfter  = time.Hour
	maxCollapseID  = 64
	breakerOpenFor = 30 * time.Second
	tripAfter      = 5
)

type Option func(*Client)

func WithBreaker(st gobreaker.Settings) Option { return func(c *Client) { c.settings = st } }

func WithTimeout(d time.Duration) Option { return func(c *Client) { c.timeout = d } }

type endpoint struct {
	client  *apns2.Client
	breaker *gobreaker.TwoStepCircuitBreaker[struct{}]
}

type Client struct {
	topic      string
	timeout    time.Duration
	settings   gobreaker.Settings
	transport  http.RoundTripper
	callerGone error
	aborted    error
	endpoints  map[Environment]*endpoint
}

var _ Sender = (*Client)(nil)

func New(cfg config.Config, opts ...Option) (*Client, error) {
	c := &Client{
		topic:      cfg.APNs.Topic,
		timeout:    cfg.Timeouts.APNs,
		callerGone: errs.New(errs.CodeAPNSUnavailable, "apns.callerGone"),
		aborted:    errs.New(errs.CodePanic, opSend),
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.timeout <= 0 {
		return nil, invalidConfig("MONACO_TIMEOUT_APNS must be positive")
	}
	auth, err := authToken(cfg.APNs)
	if err != nil {
		return nil, err
	}
	hosts, err := resolveHosts(cfg.APNs.BaseURL)
	if err != nil {
		return nil, err
	}
	c.endpoints = make(map[Environment]*endpoint, len(hosts))
	for env, host := range hosts {
		c.endpoints[env] = c.newEndpoint(env, auth, host, cfg.APNs.BaseURL != "")
	}
	return c, nil
}

func invalidConfig(reason string) error {
	return errs.New(errs.CodeInvalidConfig, opNew, slog.String("reason", reason))
}

func authToken(cfg config.APNs) (*token.Token, error) {
	key, err := token.AuthKeyFromBytes([]byte(strings.ReplaceAll(cfg.KeyP8, `\n`, "\n")))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInvalidConfig, opNew,
			slog.String("reason", "APNS_KEY_P8 is not a PKCS8 EC private key in PEM form"))
	}
	auth := &token.Token{AuthKey: key, KeyID: cfg.KeyID, TeamID: cfg.TeamID}
	if ok, err := auth.Generate(); !ok {
		return nil, errs.Wrap(err, errs.CodeInvalidConfig, opNew,
			slog.String("reason", "APNS_KEY_P8 cannot sign an ES256 token, and Apple keys are P-256"))
	}
	return auth, nil
}

func resolveHosts(base string) (map[Environment]string, error) {
	if base == "" {
		return map[Environment]string{Sandbox: apns2.HostDevelopment, Production: apns2.HostProduction}, nil
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || !reachableSafely(u) {
		return nil, invalidConfig("APNS_BASE_URL must be an absolute https URL, or http on a loopback host")
	}
	host := strings.TrimSuffix(base, "/")
	return map[Environment]string{Sandbox: host, Production: host}, nil
}

func reachableSafely(u *url.URL) bool {
	if u.Scheme == "https" {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return u.Scheme == "http" && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback())
}

func (c *Client) newEndpoint(env Environment, auth *token.Token, host string, plain bool) *endpoint {
	client := apns2.NewTokenClient(auth)
	client.Host = host
	rt := client.HTTPClient.Transport
	if plain {
		client.HTTPClient = &http.Client{}
		rt = http.DefaultTransport
	}
	if c.transport != nil {
		rt = c.transport
	}
	client.HTTPClient.Timeout = 0
	client.HTTPClient.Transport = tap{base: rt}
	settings := c.settings
	if settings.Timeout == 0 {
		settings.Timeout = breakerOpenFor
	}
	if settings.ReadyToTrip == nil {
		settings.ReadyToTrip = tripAtFive
	}
	settings.Name = "apns-" + string(env)
	excluded := settings.IsExcluded
	settings.IsExcluded = func(err error) bool {
		return errors.Is(err, c.callerGone) || excluded != nil && excluded(err)
	}
	return &endpoint{client: client, breaker: gobreaker.NewTwoStepCircuitBreaker[struct{}](settings)}
}

func tripAtFive(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= tripAfter }

func (c *Client) Send(ctx context.Context, p Push) (Result, error) {
	ep, ok := c.endpoints[p.Environment]
	if !ok {
		return Result{}, errs.New(errs.CodeInvalidInput, opSend,
			slog.String("reason", "environment is not sandbox or production"))
	}
	if !isDeviceToken(p.Token) {
		return Result{Status: http.StatusBadRequest, Reason: apns2.ReasonBadDeviceToken}, nil
	}
	if !isCollapseID(p.CollapseID) {
		return Result{Status: http.StatusBadRequest, Reason: apns2.ReasonBadCollapseID}, nil
	}
	n, err := c.notification(p)
	if err != nil {
		return Result{}, err
	}
	tp := otel.GetTracerProvider()
	if parent := trace.SpanFromContext(ctx); parent.IsRecording() {
		tp = parent.TracerProvider()
	}
	ctx, span := tp.Tracer(tracerName).Start(ctx, "apns Send", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("upstream", "apns"), attribute.String("apns.environment", string(p.Environment))),
	)
	defer span.End()
	res, err := c.deliver(ctx, ep, n, p.Environment)
	if err != nil {
		span.SetStatus(codes.Error, "send failed")
		return Result{}, err
	}
	span.SetAttributes(attribute.Int("http.response.status_code", res.Status))
	if serverFault(res) {
		span.SetStatus(codes.Error, http.StatusText(res.Status))
	}
	return res, nil
}

func (c *Client) notification(p Push) (*apns2.Notification, error) {
	if _, clash := p.Data["aps"]; clash {
		return nil, errs.New(errs.CodeInvalidInput, opSend, slog.String("reason", "data may not set the aps key"))
	}
	body := payload.NewPayload().AlertTitle(p.Title).AlertBody(p.Body)
	for k, v := range p.Data {
		body.Custom(k, v)
	}
	return &apns2.Notification{DeviceToken: p.Token, Topic: c.topic, CollapseID: p.CollapseID, Payload: body}, nil
}

func (c *Client) deliver(
	ctx context.Context, ep *endpoint, n *apns2.Notification, env Environment,
) (Result, error) {
	environment := slog.String("environment", string(env))
	done, err := ep.breaker.Allow()
	if err != nil {
		return Result{}, errs.Wrap(err, errs.CodeAPNSUnavailable, opSend, environment)
	}
	verdict := c.aborted
	defer func() { done(verdict) }()
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	h := &hint{}
	resp, err := ep.client.PushWithContext(context.WithValue(callCtx, hintKey{}, h), n)
	if err != nil {
		err = errs.Wrap(withoutURL(err), errs.CodeAPNSUnavailable, opSend, environment)
		verdict = c.failure(ctx, err)
		return Result{}, err
	}
	res := Result{Status: resp.StatusCode, Reason: resp.Reason, APNsID: resp.ApnsID, RetryAfter: h.retryAfter}
	verdict = statusFault(res)
	return res, nil
}

func (c *Client) failure(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return c.callerGone
	}
	return err
}

func serverFault(r Result) bool { return r.Status/100 == 5 }

func statusFault(r Result) error {
	if serverFault(r) {
		return errs.New(errs.CodeAPNSUnavailable, opSend, slog.Int("status", r.Status))
	}
	return nil
}

func isDeviceToken(s string) bool {
	return s != "" && strings.IndexFunc(s, func(r rune) bool { return !unicode.Is(unicode.ASCII_Hex_Digit, r) }) < 0
}

func isCollapseID(s string) bool {
	return len(s) <= maxCollapseID && strings.IndexFunc(s, unicode.IsControl) < 0
}

func withoutURL(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return uerr.Err
	}
	return err
}

type hintKey struct{}

type hint struct{ retryAfter time.Duration }

type tap struct{ base http.RoundTripper }

func (t tap) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(r)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeAPNSUnavailable, "apns.RoundTrip")
	}
	if h, ok := r.Context().Value(hintKey{}).(*hint); ok {
		h.retryAfter = retryAfter(resp.Header.Get("Retry-After"))
	}
	return resp, nil
}

func retryAfter(v string) time.Duration {
	secs, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0
	}
	return time.Duration(min(secs, uint64(maxRetryAfter/time.Second))) * time.Second
}
