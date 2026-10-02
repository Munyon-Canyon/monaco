package jupiter_test

import (
	"context"
	"encoding/base64"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const (
	executeRoute = "/jupiter/swap/v2/execute"
	orderRoute   = "/jupiter/swap/v2/order"
	signature    = "5VfYmGBjvQKe3fgLtUq6Wr2mDzcYbUo8ZqQoYTmT4Qx1cS7yVJnEM8o2KcTnPDeYh8f9GZyRsAvQ7wHzNHbTzJtE"
)

func TestOrder_buySendsTakerAndReturnsTheUnsignedTransaction(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	got, err := c.Order(t.Context(), jupiter.OrderSpec{
		In: usdc(), Out: aaplx(), Amount: units(25_000_000, usdc()), Taker: treasury, SlippageBps: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := jupiter.Order{
		RequestID: "req-buy-aaplx-1", Transaction: []byte("unsigned-buy-tx"), InMint: usdc(), OutMint: aaplx(),
		InAmount: units(25_000_000, usdc()), OutAmount: units(11_000_000, aaplx()), Router: "iris",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Order = %+v, want %+v", got, want)
	}
	req := u.requests()[0]
	q := req.query
	if req.method != http.MethodGet || req.path != orderRoute || req.apiKey != "test-key" ||
		q.Get("taker") != string(treasury) || q.Get("slippageBps") != "50" || q.Get("inputMint") != usdc().Address ||
		q.Get("outputMint") != aaplx().Address || q.Get("amount") != "25000000" || q.Get("swapMode") != "ExactIn" {
		t.Fatalf("request = %+v", req)
	}
}

func TestOrder_sellParsesAmountsInEachMintsDecimals(t *testing.T) {
	t.Parallel()
	c, _, srv := overFakes(t)
	script(t, srv, fakes.Step{Route: orderRoute, Action: fakes.ActionSucceed, Fixture: orderRoute + "/sell"})
	got, err := c.Order(
		t.Context(),
		jupiter.OrderSpec{In: aaplx(), Out: usdc(), Amount: units(11_000_000, aaplx()), Taker: treasury},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.RequestID != "req-sell-aaplx-1" || string(got.Transaction) != "unsigned-sell-tx" ||
		got.InAmount != units(11_000_000, aaplx()) || got.OutAmount != units(24_870_000, usdc()) {
		t.Fatalf("Order = %+v", got)
	}
}

func TestOrder_refusedOrderIsRejectedWithJupitersCode(t *testing.T) {
	t.Parallel()
	c, _, srv := overFakes(t)
	script(t, srv, fakes.Step{Route: orderRoute, Action: fakes.ActionSucceed, Fixture: orderRoute + "/no-route"})
	_, err := c.Order(
		t.Context(),
		jupiter.OrderSpec{In: usdc(), Out: aaplx(), Amount: units(1, usdc()), Taker: treasury},
	)
	wantCode(t, err, errs.CodeJupiterRejected)
	if attr(err, "jupiter_code") != "1" || attr(err, "jupiter_message") != "No routes found" {
		t.Fatalf("attrs = %v", errs.Detail(err))
	}

	for name, u := range map[string]*upstream{
		"4xx":            replying(http.StatusBadRequest, `{"error":"Insufficient funds","errorCode":2}`),
		"no transaction": replying(http.StatusOK, `{"inAmount":"1","outAmount":"5","routePlan":[{}]}`),
	} {
		_, err := client(u).Order(t.Context(), jupiter.OrderSpec{In: usdc(), Out: aaplx(), Amount: units(1, usdc())})
		if errs.CodeOf(err) != errs.CodeJupiterRejected {
			t.Fatalf("%s: err = %v, want jupiter_rejected", name, err)
		}
	}
}

func TestOrder_malformedResponsesAreDecodeFailures(t *testing.T) {
	t.Parallel()
	routed := `"routePlan":[{}],"transaction":"`
	tx := base64.StdEncoding.EncodeToString([]byte("tx")) + `"`
	for name, body := range map[string]string{
		"not json":       `{"requestId":`,
		"bad base64":     `{"inAmount":"1","outAmount":"5",` + routed + `%%%"}`,
		"bad in amount":  `{"inAmount":"-1","outAmount":"5",` + routed + tx + `}`,
		"bad out amount": `{"inAmount":"1","outAmount":"5.5",` + routed + tx + `}`,
	} {
		_, err := client(replying(http.StatusOK, body)).Order(t.Context(),
			jupiter.OrderSpec{In: usdc(), Out: aaplx(), Amount: units(1, usdc())})
		if errs.CodeOf(err) != errs.CodeDecodeFailed {
			t.Fatalf("%s: err = %v, want decode_failed", name, err)
		}
	}
}

func TestOrder_invalidAmountNeverCallsJupiter(t *testing.T) {
	t.Parallel()
	for name, amount := range map[string]jupiter.OrderSpec{
		"zero":           {In: usdc(), Out: aaplx(), Amount: units(0, usdc())},
		"wrong decimals": {In: usdc(), Out: aaplx(), Amount: units(5, aaplx())},
	} {
		u := replying(http.StatusOK, `{}`)
		_, err := client(u).Order(t.Context(), amount)
		if errs.CodeOf(err) != errs.CodeInvalidInput || len(u.requests()) != 0 {
			t.Fatalf("%s: err = %v after %d calls, want invalid_input and no call", name, err, len(u.requests()))
		}
	}
}

func TestQuote_routedQuoteOmitsTakerAndRoundsImpactUp(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	got, err := c.Quote(t.Context(), jupiter.QuoteSpec{In: usdc(), Out: aaplx(), Amount: units(25_000_000, usdc())})
	if err != nil {
		t.Fatal(err)
	}
	want := jupiter.Quote{
		InAmount: units(25_000_000, usdc()), OutAmount: units(11_000_000, aaplx()), PriceImpactBps: 12, Routable: true,
	}
	if got != want {
		t.Fatalf("Quote = %+v, want %+v", got, want)
	}
	if q := u.requests()[0].query; q.Has("taker") || q.Has("slippageBps") {
		t.Fatalf("quote query = %v, want no taker or slippage", q)
	}
}

func TestQuote_NoRoute(t *testing.T) {
	t.Parallel()
	c, _, srv := overFakes(t)
	script(t, srv, fakes.Step{Route: orderRoute, Action: fakes.ActionSucceed, Fixture: orderRoute + "/no-route"})
	got, err := c.Quote(t.Context(), jupiter.QuoteSpec{In: usdc(), Out: aaplx(), Amount: units(1, usdc())})
	if err != nil || got != (jupiter.Quote{Routable: false}) {
		t.Fatalf("Quote = %+v, %v, want unroutable and no error", got, err)
	}

	got, err = client(replying(http.StatusBadRequest, "Failed to get quotes")).Quote(t.Context(),
		jupiter.QuoteSpec{In: usdc(), Out: aaplx(), Amount: units(1, usdc())})
	if err != nil || got.Routable {
		t.Fatalf("400 Quote = %+v, %v, want unroutable and no error", got, err)
	}
}

func TestQuote_priceImpact(t *testing.T) {
	t.Parallel()
	for pct, want := range map[string]int64{
		``:                           0,
		`,"priceImpactPct":"0.12"`:   12,
		`,"priceImpactPct":-0.121`:   13,
		`,"priceImpactPct":"1.5e-1"`: 15,
		`,"priceImpactPct":"0.32290054666190826"`: 33,
	} {
		body := `{"inAmount":"1","outAmount":"5","routePlan":[{}]` + pct + `}`
		got, err := client(replying(http.StatusOK, body)).Quote(t.Context(),
			jupiter.QuoteSpec{In: usdc(), Out: aaplx(), Amount: units(1, usdc())})
		if err != nil || got.PriceImpactBps != want {
			t.Fatalf("impact %q = %d, %v, want %d", pct, got.PriceImpactBps, err, want)
		}
	}
}

func TestQuote_failures(t *testing.T) {
	t.Parallel()
	synctest.Test(t, quoteFailures)
}

func quoteFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		u    *upstream
		want errs.Code
	}{
		"unauthorized":   {replying(http.StatusUnauthorized, `Unauthorized`), errs.CodeJupiterRejected},
		"server error":   {replying(http.StatusInternalServerError, `oops`), errs.CodeJupiterUnavailable},
		"rate limited":   {replying(http.StatusTooManyRequests, `Too Many Requests`), errs.CodeJupiterUnavailable},
		"transport":      {&upstream{transport: errs.New(errs.CodeUpstreamUnavailable, "test.dial")}, errs.CodeJupiterUnavailable},
		"body cut":       {&upstream{handler: replying(http.StatusOK, `{}`).handler, badBody: true}, errs.CodeJupiterUnavailable},
		"bad amount":     {replying(http.StatusOK, `{"inAmount":"x","outAmount":"5","routePlan":[{}]}`), errs.CodeDecodeFailed},
		"exponent":       {replying(http.StatusOK, `{"inAmount":"1","outAmount":"5","routePlan":[{}],"priceImpactPct":1e1000}`), errs.CodeDecodeFailed},
		"impact too big": {replying(http.StatusOK, `{"inAmount":"1","outAmount":"5","routePlan":[{}],"priceImpactPct":1e17}`), errs.CodeDecodeFailed},
		"invalid":        {replying(http.StatusOK, `{}`), errs.CodeInvalidInput},
	} {
		amount := units(1, usdc())
		if name == "invalid" {
			amount = units(0, usdc())
		}
		_, err := client(tc.u).Quote(t.Context(), jupiter.QuoteSpec{In: usdc(), Out: aaplx(), Amount: amount})
		if errs.CodeOf(err) != tc.want {
			t.Fatalf("%s: err = %v, want %s", name, err, tc.want)
		}
	}
}

func TestQuote_slowJupiterTimesOutAtTheConfiguredDeadline(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c, _, srv := overFakes(t)
		script(t, srv, fakes.Step{Route: orderRoute, Action: fakes.ActionDelay, Delay: "1m"})
		start := now()
		_, err := c.Quote(t.Context(), jupiter.QuoteSpec{In: usdc(), Out: aaplx(), Amount: units(1, usdc())})
		wantCode(t, err, errs.CodeUpstreamTimeout)
		if took := now().Sub(start); took != 5*time.Second {
			t.Fatalf("gave up after %v, want the 5s quote deadline", took)
		}
	})
}

func TestExecute_postsTheSignedBytesAndRequestID(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	got, err := c.Execute(t.Context(), "req-buy-aaplx-1", []byte("signed-buy-tx"))
	if err != nil {
		t.Fatal(err)
	}
	want := jupiter.ExecuteResult{
		Status: jupiter.StatusSuccess, Signature: signature, InAmount: 25_000_000, OutAmount: 10_987_654,
	}
	if got != want {
		t.Fatalf("Execute = %+v, want %+v", got, want)
	}
	req := u.requests()[0]
	wantBody := `{"signedTransaction":"` + base64.StdEncoding.EncodeToString([]byte("signed-buy-tx")) +
		`","requestId":"req-buy-aaplx-1"}`
	if req.method != http.MethodPost || req.path != executeRoute || req.body != wantBody || req.apiKey != "test-key" {
		t.Fatalf("request = %+v, want POST %s %s", req, executeRoute, wantBody)
	}
}

func TestExecute_failedIsAResultNotAnError(t *testing.T) {
	t.Parallel()
	c, _, srv := overFakes(t)
	script(t, srv, fakes.Step{Route: executeRoute, Action: fakes.ActionSucceed, Fixture: executeRoute + "/failed"})
	got, err := c.Execute(t.Context(), "req-1", []byte("signed"))
	if err != nil || got != (jupiter.ExecuteResult{Status: jupiter.StatusFailed, ErrorCode: -1005}) {
		t.Fatalf("Execute = %+v, %v, want Failed with code -1005", got, err)
	}
}

func TestExecute_failures(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		u    *upstream
		want errs.Code
	}{
		"refused":        {replying(http.StatusBadRequest, `{"code":-2,"error":"Invalid signed transaction"}`), errs.CodeJupiterRejected},
		"refused text":   {replying(http.StatusBadRequest, `bad request`), errs.CodeJupiterRejected},
		"not json":       {replying(http.StatusOK, `{"status":`), errs.CodeDecodeFailed},
		"unknown status": {replying(http.StatusOK, `{"status":"Landing"}`), errs.CodeDecodeFailed},
		"bad in":         {replying(http.StatusOK, `{"status":"Success","inputAmountResult":"1.5"}`), errs.CodeDecodeFailed},
		"bad out":        {replying(http.StatusOK, `{"status":"Success","outputAmountResult":"-3"}`), errs.CodeDecodeFailed},
		"server error":   {replying(http.StatusInternalServerError, ``), errs.CodeJupiterUnavailable},
		"bad gateway":    {replying(http.StatusBadGateway, ``), errs.CodeJupiterUnavailable},
		"rate limited":   {replying(http.StatusTooManyRequests, `{"error":"Rate limit exceeded"}`), errs.CodeJupiterUnavailable},
		"unauthorized":   {replying(http.StatusUnauthorized, `Unauthorized`), errs.CodeJupiterRejected},
	} {
		_, err := client(tc.u).Execute(t.Context(), "req-1", []byte("signed"))
		if errs.CodeOf(err) != tc.want {
			t.Fatalf("%s: err = %v, want %s", name, err, tc.want)
		}
	}
	_, err := client(replying(http.StatusBadRequest, `{"code":-2,"error":"Invalid signed transaction"}`)).Execute(
		t.Context(), "req-1", nil)
	if attr(err, "jupiter_code") != "-2" || attr(err, "jupiter_message") != "Invalid signed transaction" {
		t.Fatalf("attrs = %v", errs.Detail(err))
	}
}

func TestExecuteUntilTerminal_PendingThenSuccess(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c, u, srv := overFakes(t)
		script(t, srv, fakes.Step{
			Route: executeRoute, Action: fakes.ActionSucceed, Fixture: executeRoute + "/pending", Times: 2,
		})
		got, err := c.ExecuteUntilTerminal(t.Context(), "req-buy-aaplx-1", []byte("signed-buy-tx"))
		if err != nil || got.Status != jupiter.StatusSuccess || got.Signature != signature {
			t.Fatalf("ExecuteUntilTerminal = %+v, %v, want Success", got, err)
		}
		sent := u.requests()
		if len(sent) != 3 {
			t.Fatalf("sent %d /execute calls, want 3", len(sent))
		}
		for i, s := range sent {
			if s.body != sent[0].body || !strings.Contains(s.body, `"requestId":"req-buy-aaplx-1"`) {
				t.Fatalf("attempt %d body = %s, want the first attempt's %s", i, s.body, sent[0].body)
			}
			if gap := s.at.Sub(sent[0].at); gap != time.Duration(i)*2*time.Second {
				t.Fatalf("attempt %d sent %v after the first, want %v", i, gap, time.Duration(i)*2*time.Second)
			}
		}
	})
}

func TestExecuteUntilTerminal_StopsAtTwoMinutes(t *testing.T) {
	t.Parallel()
	fastest := time.Hour
	for range 3 {
		wall := now()
		synctest.Test(t, stopsAtTwoMinutes)
		fastest = min(fastest, now().Sub(wall))
	}
	if fastest >= 50*time.Millisecond {
		t.Fatalf("2 minutes of fake time took at best %v of wall time over 3 runs, want under 50ms", fastest)
	}
}

func stopsAtTwoMinutes(t *testing.T) {
	c, u, srv := overFakes(t)
	script(t, srv, fakes.Step{
		Route: executeRoute, Action: fakes.ActionSucceed, Fixture: executeRoute + "/pending", Times: 1000,
	})
	start := now()
	got, err := c.ExecuteUntilTerminal(t.Context(), "req-1", []byte("signed"))
	wantCode(t, err, errs.CodeUpstreamTimeout)
	if got.Status != jupiter.StatusPending || attr(err, "request_id") != "req-1" {
		t.Fatalf("ExecuteUntilTerminal = %+v, %v, want Pending naming req-1", got, err)
	}
	if took := now().Sub(start); took != 2*time.Minute || len(u.requests()) != 61 {
		t.Fatalf("gave up after %v and %d calls, want 2m and 61 calls", took, len(u.requests()))
	}
}

func TestExecuteUntilTerminal_returnsAnErrorOrCancellationAsIs(t *testing.T) {
	t.Parallel()
	_, err := client(replying(http.StatusBadRequest, `{}`)).ExecuteUntilTerminal(t.Context(), "req-1", nil)
	wantCode(t, err, errs.CodeJupiterRejected)

	synctest.Test(t, func(t *testing.T) {
		c, _, srv := overFakes(t)
		script(t, srv, fakes.Step{
			Route: executeRoute, Action: fakes.ActionSucceed, Fixture: executeRoute + "/pending", Times: 1000,
		})
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		got, err := c.ExecuteUntilTerminal(ctx, "req-1", nil)
		if errs.CodeOf(err) != errs.CodeUpstreamTimeout || got.Status != jupiter.StatusPending ||
			!errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("ExecuteUntilTerminal = %+v, %v, want Pending wrapping the caller's deadline", got, err)
		}
	})
}

func TestWireTypesAreUnexported(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), e.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range exportedJSONTypes(file) {
			t.Errorf("%s: exported type %s carries Jupiter's JSON shape", e.Name(), name)
		}
	}
}

func exportedJSONTypes(file *ast.File) []string {
	var names []string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok || !spec.Name.IsExported() {
			return true
		}
		st, ok := spec.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, f := range st.Fields.List {
			if f.Tag != nil && strings.Contains(f.Tag.Value, "json:") {
				names = append(names, spec.Name.Name)
				break
			}
		}
		return true
	})
	return names
}

func TestQuote_anEmptyOrMissingRoutePlanIsNotRoutable(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		body     string
		routable bool
	}{
		"empty plan":   {`{"inAmount":"1","outAmount":"5","routePlan":[]}`, false},
		"missing plan": {`{"inAmount":"1","outAmount":"5"}`, false},
		"one hop":      {`{"inAmount":"1","outAmount":"5","routePlan":[{}]}`, true},
	} {
		got, err := client(replying(http.StatusOK, tc.body)).Quote(t.Context(),
			jupiter.QuoteSpec{In: usdc(), Out: aaplx(), Amount: units(1, usdc())})
		if err != nil || got.Routable != tc.routable {
			t.Fatalf("%s: Quote = %+v, %v, want Routable %v", name, got, err, tc.routable)
		}
	}
}

func TestQuote_waitsBetweenRetriesWithinTheBackoffCeilings(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c, u, srv := overFakes(t)
		script(t, srv, fakes.Step{
			Route: orderRoute, Action: fakes.ActionFail, Status: http.StatusServiceUnavailable, Times: 3,
		})
		start := now()
		_, err := c.Quote(t.Context(), jupiter.QuoteSpec{In: usdc(), Out: aaplx(), Amount: units(1, usdc())})
		waited := now().Sub(start)
		wantCode(t, err, errs.CodeJupiterUnavailable)
		if len(u.requests()) != 3 || waited <= 0 || waited > 750*time.Millisecond {
			t.Fatalf("%d attempts waited %v, want 3 attempts and between 0 and 250ms + 500ms of backoff",
				len(u.requests()), waited)
		}
	})
}
