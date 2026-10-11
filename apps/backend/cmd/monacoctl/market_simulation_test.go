package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const (
	defaultDelegate = "0xd6cedde84be40893d153be9d467cd6ad37875b28"
	otherDelegate   = "0x63c0c19a282a1b52b07dd5a65b58948a07dae32b"
)

type simulationRig struct {
	cfg     config.Config
	privy   *fakes.Server
	creates atomic.Int32
}

func codeRPC(t *testing.T, code func(call int32) string) string {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"eth_getCode"`) {
			http.Error(w, "unexpected method", http.StatusBadRequest)
			return
		}
		result := code(calls.Add(1))
		if result == "" {
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"down"}}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":%q}`, result)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func newSimulationRig(t *testing.T, robinhood func(call int32) string) *simulationRig {
	t.Helper()
	rig := &simulationRig{privy: fakes.New()}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/privy/v1/wallets" {
			rig.creates.Add(1)
		}
		rig.privy.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	rig.cfg = config.Config{
		Env: config.EnvLocal,
		DB:  config.DB{URL: testkit.DB(t).Config().ConnString(), MaxConns: 4},
		Privy: config.Privy{
			AppID: "app-fixture", AppSecret: "secret", BaseURL: srv.URL + "/privy",
			VerificationKey:         fakes.PrivyVerificationKey(),
			AuthorizationPrivateKey: fakes.PrivyAuthorizationKeyConfig(),
			AuthorizationKeyID:      fakes.PrivyAuthorizationKeyID,
		},
		Timeouts: config.Timeouts{Privy: 5 * time.Second, RPC: 5 * time.Second},
		EVM: config.EVM{
			RPCURLBase:      codeRPC(t, func(int32) string { return "0xef0100" + defaultDelegate[2:] }),
			RPCURLRobinhood: codeRPC(t, robinhood),
		},
	}
	return rig
}

func (r *simulationRig) run(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := simulationAccount(clock.Real{})(r.cfg, args, &stdout, &stderr)
	if stderr.Len() != 0 {
		t.Fatalf("stderr %q", stderr.String())
	}
	return code, stdout.String()
}

func lineFor(out, network string) string {
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, network+"\t") {
			return line
		}
	}
	return ""
}

func TestMarketSimulationAccount_createsOnceDelegatesAndPrintsEachChainsCode(t *testing.T) {
	t.Parallel()
	rig := newSimulationRig(t, func(call int32) string {
		if call == 1 {
			return "0x"
		}
		return "0xef0100" + otherDelegate[2:]
	})

	code, out := rig.run(t, "--poll", "1ms")
	rig.wantCounts(t, code, out, "created")
	wantBase := "base\tcode=0xef0100" + defaultDelegate[2:] + "\tdelegate=" + defaultDelegate + "\tsent=none"
	wantRH := "robinhood\tcode=0xef0100" + otherDelegate[2:] + "\tdelegate=" + otherDelegate + "\tsent=0x"
	if base, rh := lineFor(out, "base"), lineFor(out, "robinhood"); base != wantBase || !strings.HasPrefix(rh, wantRH) {
		t.Fatalf("base %q, robinhood %q, want base untouched and the delegate Privy set on robinhood", base, rh)
	}
	address := strings.TrimPrefix(strings.Split(out, "\n")[1], "address ")

	code, out = rig.run(t)
	rig.wantCounts(t, code, out, "reused")
	if !strings.Contains(out, "address "+address+"\n") {
		t.Fatalf("second run printed another address than %s:\n%s", address, out)
	}
}

func (r *simulationRig) wantCounts(t *testing.T, code int, out, verb string) {
	t.Helper()
	if code != 0 || !strings.Contains(out, "\n"+verb+"\n") || r.creates.Load() != 1 || r.privy.PrivyEVMSends() != 1 {
		t.Fatalf("exit %d, creates %d, sends %d, want %s with one create and one send:\n%s",
			code, r.creates.Load(), r.privy.PrivyEVMSends(), verb, out)
	}
}

func TestMarketSimulationAccount_failsWhenAChainStaysUndelegated(t *testing.T) {
	t.Parallel()
	rig := newSimulationRig(t, func(int32) string { return "0x" })
	code, out := rig.run(t, "--timeout", "5ms", "--poll", "1ms")
	rh := lineFor(out, "robinhood")
	if code != 1 || !strings.HasPrefix(rh, "robinhood\tcode=0x\tdelegate=none\tsent=0x") {
		t.Fatalf("exit %d, robinhood line %q:\n%s", code, rh, out)
	}
}

func TestMarketSimulationAccount_printsAChainsReadError(t *testing.T) {
	t.Parallel()
	rig := newSimulationRig(t, func(call int32) string {
		if call == 1 {
			return "0x"
		}
		return ""
	})
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	rig.cfg.EVM.RPCURLArbitrum = closed.URL
	code, out := rig.run(t, "--poll", "1ms")
	arb, rh := lineFor(out, "arbitrum"), lineFor(out, "robinhood")
	if code != 1 || !strings.HasPrefix(arb, "arbitrum\terror=") || !strings.HasPrefix(rh, "robinhood\terror=") {
		t.Fatalf("exit %d, arbitrum %q, robinhood %q:\n%s", code, arb, rh, out)
	}
}

func TestMarketSimulationAccount_refusesBadArgumentsAndReportsSetupFailures(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	for _, args := range [][]string{{"extra"}, {"--poll", "0s"}, {"--nope"}} {
		if code := simulationAccount(clock.Real{})(config.Config{}, args, &stdout, &stderr); code != 2 {
			t.Fatalf("%v exit %d, want 2", args, code)
		}
	}
	bad := config.Config{DB: config.DB{URL: "postgres://%zz"}}
	if code := simulationAccount(clock.Real{})(bad, nil, &stdout, &stderr); code != 1 {
		t.Fatalf("bad database exit %d, want 1", code)
	}
	rig := newSimulationRig(t, func(int32) string { return "0x" })
	noKey := rig.cfg
	noKey.Privy.VerificationKey = ""
	if code := simulationAccount(clock.Real{})(noKey, nil, &stdout, &stderr); code != 1 {
		t.Fatalf("no privy verification key exit %d, want 1", code)
	}
	noQuorum := rig.cfg
	noQuorum.Privy.AuthorizationKeyID = ""
	if code := simulationAccount(clock.Real{})(noQuorum, nil, &stdout, &stderr); code != 1 || stdout.Len() != 0 {
		t.Fatalf("no quorum exit %d stdout %q, want 1 before any output", code, stdout.String())
	}
}
