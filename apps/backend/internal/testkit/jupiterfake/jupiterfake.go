package jupiterfake

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type pair struct{ in, out string }

type Venue struct {
	testkit.Faults

	mu       sync.Mutex
	orders   map[pair]jupiter.Order
	quotes   map[pair]jupiter.Quote
	executes map[string][]jupiter.ExecuteResult
	sent     map[string][][]byte
	sol      map[jupiter.SolanaAddress]uint64
}

func (v *Venue) SetSOL(taker jupiter.SolanaAddress, lamports uint64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.sol == nil {
		v.sol = map[jupiter.SolanaAddress]uint64{}
	}
	v.sol[taker] = lamports
}

func (v *Venue) SetOrder(in, out jupiter.Mint, o jupiter.Order) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.orders == nil {
		v.orders = map[pair]jupiter.Order{}
	}
	v.orders[pair{in.Address, out.Address}] = o
}

func (v *Venue) SetQuote(in, out jupiter.Mint, q jupiter.Quote) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.quotes == nil {
		v.quotes = map[pair]jupiter.Quote{}
	}
	v.quotes[pair{in.Address, out.Address}] = q
}

func (v *Venue) SetExecute(requestID string, results ...jupiter.ExecuteResult) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.executes == nil {
		v.executes = map[string][]jupiter.ExecuteResult{}
	}
	v.executes[requestID] = results
}

func (v *Venue) Sent(requestID string) [][]byte {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([][]byte(nil), v.sent[requestID]...)
}

func (v *Venue) Order(_ context.Context, spec jupiter.OrderSpec) (jupiter.Order, error) {
	if err := v.Check("Order"); err != nil {
		return jupiter.Order{}, err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if spec.Payer == "" && v.sol[spec.Taker] == 0 {
		return jupiter.Order{}, errs.New(errs.CodeJupiterRejected, "jupiterfake.Order",
			slog.String("taker", string(spec.Taker)), slog.String("reason", "Failed to get quotes"))
	}
	o, ok := v.orders[pair{spec.In.Address, spec.Out.Address}]
	if !ok {
		return jupiter.Order{}, errs.New(errs.CodeJupiterRejected, "jupiterfake.Order",
			slog.String("in", spec.In.Address), slog.String("out", spec.Out.Address))
	}
	return o, nil
}

func (v *Venue) Quote(_ context.Context, spec jupiter.QuoteSpec) (jupiter.Quote, error) {
	if err := v.Check("Quote"); err != nil {
		return jupiter.Quote{}, err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.quotes[pair{spec.In.Address, spec.Out.Address}], nil
}

func (v *Venue) Execute(_ context.Context, requestID string, signed []byte) (jupiter.ExecuteResult, error) {
	res, _, err := v.execute(requestID, signed)
	return res, err
}

func (v *Venue) ExecuteUntilTerminal(
	_ context.Context, requestID string, signed []byte,
) (jupiter.ExecuteResult, error) {
	for {
		res, last, err := v.execute(requestID, signed)
		if err != nil || res.Status != jupiter.StatusPending {
			return res, err
		}
		if last {
			return res, errs.New(errs.CodeUpstreamTimeout, "jupiterfake.ExecuteUntilTerminal",
				slog.String("request_id", requestID))
		}
	}
}

func (v *Venue) execute(requestID string, signed []byte) (jupiter.ExecuteResult, bool, error) {
	if err := v.Check("Execute"); err != nil {
		return jupiter.ExecuteResult{}, true, err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.sent == nil {
		v.sent = map[string][][]byte{}
	}
	v.sent[requestID] = append(v.sent[requestID], append([]byte(nil), signed...))
	queue := v.executes[requestID]
	switch len(queue) {
	case 0:
		return jupiter.ExecuteResult{Status: jupiter.StatusSuccess, Signature: "sig-" + requestID}, true, nil
	case 1:
		return queue[0], true, nil
	}
	v.executes[requestID] = queue[1:]
	return queue[0], false, nil
}

type PriceSource struct {
	testkit.Faults

	mu     sync.Mutex
	prices map[jupiter.Mint]jupiter.Price
}

func (p *PriceSource) SetPrice(price jupiter.Price) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.prices == nil {
		p.prices = map[jupiter.Mint]jupiter.Price{}
	}
	p.prices[price.Mint] = price
}

func (p *PriceSource) Prices(_ context.Context, mints []jupiter.Mint) (map[jupiter.Mint]jupiter.Price, error) {
	if err := p.Check("Prices"); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[jupiter.Mint]jupiter.Price, len(mints))
	for _, m := range mints {
		if price, ok := p.prices[m]; ok {
			out[m] = price
		}
	}
	return out, nil
}

type PriceAPI struct {
	Clock  clock.Clock
	Limit  int
	Window time.Duration

	mu      sync.Mutex
	resets  time.Time
	used    int
	limited int
}

func (a *PriceAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !a.admit() {
		w.Header().Set("x-ratelimit-reset", strconv.FormatInt(a.resetUnix(), 10))
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	body := map[string]map[string]string{}
	for i, mint := range strings.Split(r.URL.Query().Get("ids"), ",") {
		body[mint] = map[string]string{"usdPrice": strconv.Itoa(100+i) + ".25"}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func (a *PriceAPI) admit() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if now := a.Clock.Now(); !now.Before(a.resets) {
		a.resets, a.used = now.Add(a.Window), 0
	}
	a.used++
	if a.used > a.Limit {
		a.limited++
		return false
	}
	return true
}

func (a *PriceAPI) resetUnix() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.resets.Add(time.Second - 1).Unix()
}

func (a *PriceAPI) Limited() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.limited
}
