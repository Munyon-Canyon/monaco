package solana_test

import (
	"crypto/ed25519"
	"encoding/base64"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestLatestBlockhash_decodesTheHashAndExpiryHeight(t *testing.T) {
	t.Parallel()
	c, _, _ := overFakes(t)
	got, err := c.LatestBlockhash(t.Context())
	want, _ := chain.DecodeBase58("7be9CjQttHDBAkNkocfWMgzW5yDB853CYkhN7ZmBJsgF")
	if err != nil || string(got.Hash[:]) != string(want) || got.LastValidBlockHeight != 380_000_150 {
		t.Fatalf("LatestBlockhash = %+v, %v", got, err)
	}
	_, err = client(result(`{"value":{"blockhash":"short"}}`)).LatestBlockhash(t.Context())
	wantCode(t, err, errs.CodeDecodeFailed)
	_, err = client(replying(503, "")).LatestBlockhash(t.Context())
	wantCode(t, err, errs.CodeRPCUnavailable)
}

func TestSendTransaction_returnsTheFirstSignature(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	sig := ed25519.Sign(ed25519.NewKeyFromSeed(make([]byte, 32)), []byte("message"))
	tx := append(append([]byte{1}, sig...), []byte("message")...)
	got, err := c.SendTransaction(t.Context(), tx)
	if err != nil || got != chain.SignatureOf(sig) {
		t.Fatalf("SendTransaction = %q, %v", got, err)
	}
	if p := string(u.requests()[0].params[0]); p != `"`+base64.StdEncoding.EncodeToString(tx)+`"` {
		t.Fatalf("params[0] = %s, want the base64 transaction", p)
	}
	_, err = c.SendTransaction(t.Context(), []byte{0})
	wantCode(t, err, errs.CodeRPCUnavailable)
	_, err = client(result(`"not-a-signature"`)).SendTransaction(t.Context(), tx)
	wantCode(t, err, errs.CodeDecodeFailed)
}

func TestCall_mapsTransportAndWireFailures(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		for name, tc := range map[string]struct {
			u    *upstream
			want errs.Code
		}{
			"503 after retries": {replying(http.StatusServiceUnavailable, ""), errs.CodeRPCUnavailable},
			"401":               {replying(http.StatusUnauthorized, "no key"), errs.CodeRPCUnavailable},
			"transport":         {&upstream{transport: errs.New(errs.CodeUpstreamUnavailable, "test.dial")}, errs.CodeRPCUnavailable},
			"body cut":          {&upstream{handler: result(`1`).handler, badBody: true}, errs.CodeRPCUnavailable},
			"not json":          {replying(http.StatusOK, "<html>"), errs.CodeDecodeFailed},
			"rpc error":         {replying(http.StatusOK, `{"error":{"code":-32005,"message":"node is behind"}}`), errs.CodeRPCUnavailable},
			"wrong shape":       {result(`"text"`), errs.CodeDecodeFailed},
		} {
			_, err := client(tc.u).SOLBalance(t.Context(), member)
			if errs.CodeOf(err) != tc.want {
				t.Fatalf("%s: err = %v, want %s", name, err, tc.want)
			}
		}
	})
}

func TestCall_scripted503IsRetryableRPCUnavailable(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c, u, srv := overFakes(t)
		script(t, srv, fakes.Step{Route: "/rpc/getBalance", Action: fakes.ActionFail, Status: 503, Times: 3})
		_, err := c.SOLBalance(t.Context(), member)
		wantCode(t, err, errs.CodeRPCUnavailable)
		if !errs.Retryable(errs.CodeOf(err)) || len(u.requests()) != 3 {
			t.Fatalf("err = %v after %d attempts, want retryable after 3", err, len(u.requests()))
		}
	})
}

func TestCall_hangHitsTheConfiguredRPCDeadline(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c, _, srv := overFakes(t)
		script(t, srv, fakes.Step{Route: "/rpc/getBalance", Action: fakes.ActionHang})
		start := clock.Real{}.Now()
		_, err := c.SOLBalance(t.Context(), member)
		wantCode(t, err, errs.CodeUpstreamTimeout)
		if took := (clock.Real{}).Now().Sub(start); took != 5*time.Second {
			t.Fatalf("gave up after %v, want the 5s RPC deadline", took)
		}
	})
}

func TestCall_errorsNeverCarryTheRPCURLSecret(t *testing.T) {
	t.Parallel()
	var logs testkit.Logs
	ctx := observability.WithLogger(
		t.Context(),
		observability.NewLogger(config.Config{Env: config.EnvProduction}, &logs),
	)
	transport := &upstream{
		transport: errs.New(errs.CodeUpstreamUnavailable, "dial http://rpc.test/rpc?api-key="+hiddenPart),
	}
	_, err := client(transport).SOLBalance(ctx, member)
	boundary.Stopped(ctx, "test", err)
	observability.Info(ctx, observability.BootConfig, slog.String("service", "test"), slog.Any("config", err))
	if strings.Contains(string(logs.Bytes()), hiddenPart) || strings.Contains(err.Error(), hiddenPart) {
		t.Fatalf("the RPC key reached a log line:\n%s", logs.Bytes())
	}
}

func TestCall_waitsBetweenRetriesWithinTheBackoffCeilings(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c, u, srv := overFakes(t)
		script(t, srv, fakes.Step{Route: "/rpc/getBalance", Action: fakes.ActionFail, Status: 503, Times: 3})
		start := clock.Real{}.Now()
		_, err := c.SOLBalance(t.Context(), member)
		waited := clock.Real{}.Now().Sub(start)
		wantCode(t, err, errs.CodeRPCUnavailable)
		if len(u.requests()) != 3 || waited <= 0 || waited > 750*time.Millisecond {
			t.Fatalf("%d attempts waited %v, want 3 attempts and between 0 and 250ms + 500ms of backoff",
				len(u.requests()), waited)
		}
	})
}

func TestSendTransaction_aSimulationShortOfFundsSaysSoAndKeepsTheRPCMessage(t *testing.T) {
	t.Parallel()
	tx := append(append([]byte{1}, make([]byte, 64)...), []byte("message")...)
	simulation := func(txErr, log string) *upstream {
		return replying(http.StatusOK, `{"jsonrpc":"2.0","id":1,"error":{"code":-32002,`+
			`"message":"Transaction simulation failed: Error processing Instruction 0: custom program error: 0x1",`+
			`"data":{"err":`+txErr+`,"logs":["Program Tokenkeg invoke [1]","`+log+`"]}}}`)
	}
	for name, tc := range map[string]struct {
		upstream *upstream
		code     errs.Code
	}{
		"a token transfer short of funds": {
			upstream: simulation(`{"InstructionError":[0,{"Custom":1}]}`, "Program log: Error: insufficient funds"),
			code:     errs.CodeInsufficientFunds,
		},
		"a fee payer short of SOL": {
			upstream: simulation(`"InsufficientFundsForFee"`, "Program log: done"),
			code:     errs.CodeInsufficientFunds,
		},
		"a transfer short of lamports": {
			upstream: simulation(`{"InstructionError":[0,{"Custom":1}]}`, "Transfer: insufficient lamports 5, need 9"),
			code:     errs.CodeInsufficientFunds,
		},
		"another simulation failure": {
			upstream: simulation(`{"InstructionError":[0,{"Custom":6001}]}`, "Program log: slippage"),
			code:     errs.CodeRPCUnavailable,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := client(tc.upstream).SendTransaction(t.Context(), tx)
			wantCode(t, err, tc.code)
			if !slices.ContainsFunc(errs.Detail(err), func(a slog.Attr) bool {
				return a.Key == "rpc_message" && strings.HasPrefix(a.Value.String(), "Transaction simulation failed")
			}) {
				t.Fatalf("detail = %v, want the RPC message", errs.Detail(err))
			}
		})
	}
}
