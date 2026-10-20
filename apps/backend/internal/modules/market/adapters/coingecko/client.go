package coingecko

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	upstream       = "coingecko"
	keyHeader      = "x-cg-demo-api-key"
	callsPerMinute = 25
	maxBody        = 8 << 20
)

var (
	_ app.PriceHistory = (*Client)(nil)
	_ app.VolumeSource = (*Client)(nil)
)

type Client struct {
	http  *httpclient.Client
	key   string
	limit *rate.Limiter
}

func Options(cfg config.Config) []httpclient.Option {
	return []httpclient.Option{
		httpclient.WithBaseURL(cfg.CoinGecko.BaseURL),
		httpclient.WithTimeout(cfg.Timeouts.CoinGecko),
		httpclient.WithRetry(3, time.Second, 30*time.Second),
	}
}

func New(c *httpclient.Client, apiKey string) *Client {
	return &Client{http: c, key: apiKey, limit: rate.NewLimiter(rate.Every(time.Minute/callsPerMinute), 1)}
}

func (c *Client) Configured() bool { return c.key != "" }

type chartWire struct {
	Prices       [][2]json.Number `json:"prices"`
	TotalVolumes [][2]json.Number `json:"total_volumes"`
}

func (c *Client) MarketChart(ctx context.Context, mint domain.Mint, days int) (app.History, error) {
	const op = "coingecko.Client.MarketChart"
	if !c.Configured() || (days < 1 && days != app.HistoryMaxDays) {
		return app.History{}, errs.New(errs.CodeInvalidInput, op,
			slog.Bool("keyed", c.Configured()), slog.Int("days", days))
	}
	query := url.Values{"vs_currency": {"usd"}, "days": {daysParam(days)}}
	resp, err := c.get(ctx, op, "/coins/solana/contract/"+url.PathEscape(mint.String())+"/market_chart?"+query.Encode(),
		slog.Int("days", days))
	if err != nil {
		return app.History{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	return decodeHistory(ctx, resp.Body, mint, days)
}

type volumeWire map[string]struct {
	USD24hVol *json.Number `json:"usd_24h_vol"`
}

func (c *Client) Volumes(ctx context.Context, mints []domain.Mint) (map[domain.Mint]money.Micros, error) {
	const op = "coingecko.Client.Volumes"
	if !c.Configured() || len(mints) == 0 || len(mints) > app.VolumeBatch {
		return nil, errs.New(errs.CodeInvalidInput, op,
			slog.Bool("keyed", c.Configured()), slog.Int("mints", len(mints)))
	}
	addresses := make([]string, len(mints))
	for i, m := range mints {
		addresses[i] = m.String()
	}
	query := url.Values{
		"contract_addresses": {strings.Join(addresses, ",")}, "vs_currencies": {"usd"}, "include_24hr_vol": {"true"},
	}
	resp, err := c.get(ctx, op, "/simple/token_price/solana?"+query.Encode(), slog.Int("mints", len(mints)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var w volumeWire
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&w); err != nil {
		return nil, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.Int("mints", len(mints)))
	}
	out := make(map[domain.Mint]money.Micros, len(mints))
	for _, m := range mints {
		entry, ok := w[m.String()]
		if !ok || entry.USD24hVol == nil {
			continue
		}
		if usd, ok := parsePrice(*entry.USD24hVol); ok && !usd.IsZero() {
			out[m] = usd
		}
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, op, target string, attrs ...slog.Attr) (*http.Response, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	req.Header.Set(keyHeader, c.key)
	req.Header.Set("Accept", "application/json")
	if err := c.limit.Wait(ctx); err != nil {
		return nil, errs.Wrap(err, errs.CodeUpstreamUnavailable, op, attrs...)
	}
	resp, err := c.http.Do(ctx, req)
	if err != nil {
		code := errs.CodeOf(err)
		if rateLimited(err) {
			code = errs.CodeCoinGeckoRateLimited
		}
		return nil, errs.Wrap(err, code, op, attrs...)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return resp, nil
	case http.StatusNotFound:
		_ = resp.Body.Close()
		return nil, errs.New(errs.CodeNotFound, op, attrs...)
	default:
		_ = resp.Body.Close()
		return nil, errs.New(errs.CodeUpstreamUnavailable, op, append(attrs, slog.Int("status", resp.StatusCode))...)
	}
}

func decodeHistory(ctx context.Context, body io.Reader, mint domain.Mint, days int) (app.History, error) {
	const op = "coingecko.Client.MarketChart"
	var w chartWire
	if err := json.NewDecoder(io.LimitReader(body, maxBody)).Decode(&w); err != nil {
		return app.History{}, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.Int("days", days))
	}
	out, dropped, err := samples(w, op)
	if dropped > 0 {
		observability.Degraded(ctx, observability.MarketCoinGeckoSubMicroDropped,
			slog.String("mint", mint.String()), slog.Int("days", days), slog.Int("dropped", dropped))
	}
	if err != nil {
		return app.History{}, err
	}
	return app.History{Prices: out, Volume24h: newestVolume(w.TotalVolumes)}, nil
}

func daysParam(days int) string {
	if days == app.HistoryMaxDays {
		return "max"
	}
	return strconv.Itoa(days)
}

func rateLimited(err error) bool {
	for _, a := range errs.Detail(err) {
		if a.Key == "status" && a.Value.Kind() == slog.KindInt64 && a.Value.Int64() == http.StatusTooManyRequests {
			return true
		}
	}
	return false
}

func samples(w chartWire, op string) ([]app.Sample, int, error) {
	out := make([]app.Sample, 0, len(w.Prices))
	dropped := 0
	for _, p := range w.Prices {
		ms, err := p[0].Int64()
		if err != nil {
			return nil, 0, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("ts", string(p[0])))
		}
		price, ok := parsePrice(p[1])
		if !ok {
			return nil, 0, errs.New(errs.CodeDecodeFailed, op, slog.String("price", string(p[1])))
		}
		if price.IsZero() {
			dropped++
			continue
		}
		out = append(out, app.Sample{At: time.UnixMilli(ms).UTC(), Price: price})
	}
	return out, dropped, nil
}

func newestVolume(series [][2]json.Number) *app.Volume {
	var newest *app.Volume
	for _, p := range series {
		ms, err := p[0].Int64()
		if err != nil {
			continue
		}
		at := time.UnixMilli(ms).UTC()
		if newest != nil && !at.After(newest.At) {
			continue
		}
		newest = &app.Volume{At: at}
		if usd, ok := parsePrice(p[1]); ok {
			newest.USD = usd
		}
	}
	if newest == nil || newest.USD.IsZero() {
		return nil
	}
	return newest
}

var decimalNumber = regexp.MustCompile(`^[0-9]{1,40}(\.[0-9]{1,40})?([eE][-+]?[0-9]{1,3})?$`)

func parsePrice(n json.Number) (money.Micros, bool) {
	if !decimalNumber.MatchString(string(n)) {
		return money.Micros{}, false
	}
	r, _ := new(big.Rat).SetString(string(n))
	r.Mul(r, big.NewRat(1_000_000, 1))
	micros := new(big.Int).Quo(r.Num(), r.Denom())
	if !micros.IsInt64() {
		return money.Micros{}, false
	}
	return money.MicrosFromUint64(micros.Uint64()), true
}
