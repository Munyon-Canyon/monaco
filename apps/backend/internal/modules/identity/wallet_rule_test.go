package identity_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	privyadapter "github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/privy"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

type ruleFixture struct {
	rule   *app.WalletRule
	reader *sdkmetric.ManualReader
	logs   *testkit.Logs
}

func newRuleFixture(t *testing.T, wallets app.MemberWallets) (ruleFixture, context.Context) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	rule, err := app.NewWalletRule(wallets, sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	if err != nil {
		t.Fatal(err)
	}
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	return ruleFixture{rule: rule, reader: reader, logs: logs}, ctx
}

func (f ruleFixture) signerMissing(t *testing.T) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := f.reader.Collect(t.Context(), &rm); err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if sum, ok := m.Data.(metricdata.Sum[int64]); ok && m.Name == "identity_wallet_signer_missing_total" {
				for _, p := range sum.DataPoints {
					total += p.Value
				}
			}
		}
	}
	return total
}

func (f ruleFixture) warnings(t *testing.T) []map[string]any {
	t.Helper()
	var warns []map[string]any
	for _, line := range f.lines(t) {
		if line["level"] == "WARN" {
			warns = append(warns, line)
		}
	}
	return warns
}

func (f ruleFixture) expectSignerMissing(t *testing.T, wallet string) {
	t.Helper()
	warns, count := f.warnings(t), f.signerMissing(t)
	if wallet == "" && (len(warns) != 0 || count != 0) {
		t.Fatalf("warned %v and counted %d for a wallet with the app signer", warns, count)
	}
	if wallet != "" && (len(warns) != 1 || warns[0]["msg"] != "identity.wallet.signer_missing" ||
		warns[0]["wallet_id"] != wallet || count != 1) {
		t.Fatalf("warned %v and counted %d, want one signer_missing line for %s and a count of 1", warns, count, wallet)
	}
}

func (f ruleFixture) lines(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(string(f.logs.Bytes())), "\n") {
		if raw == "" {
			continue
		}
		var line map[string]any
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatalf("log line %q: %v", raw, err)
		}
		out = append(out, line)
	}
	return out
}

func TestWalletRule_aStoredWalletMakesNoPrivyCall(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seeded := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	user, err := adapters.Users{}.FindByPrivyUserID(t.Context(), pool, seeded.PrivyUserID)
	if err != nil {
		t.Fatal(err)
	}
	c, u, _ := overPrivyFakes(t)
	f, ctx := newRuleFixture(t, privyadapter.Wallets{Client: c})
	got, err := f.rule.Resolve(ctx, app.PrivyUserID(seeded.PrivyUserID), user.Wallet)
	want := domain.Wallet{PrivyWalletID: seeded.PrivyWalletID, Address: seeded.Address}
	if err != nil || got != want {
		t.Fatalf("Resolve = %+v, %v, want the stored %+v", got, err, want)
	}
	if sent := u.requests(); len(sent) != 0 {
		t.Fatalf("a stored wallet sent Privy %v", sent)
	}
}

func TestWalletRule_findsOrCreatesThroughPrivyAndWarnsOnlyWithoutTheAppSigner(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		user          app.PrivyUserID
		wallet        string
		sent          []string
		signerMissing string
	}{
		"privy holds a wallet with the signer": {
			user: "did:privy:member-with-wallet", wallet: "wallet-member", sent: []string{"GET /privy/v1/wallets"},
		},
		"privy holds a legacy wallet": {
			user: "did:privy:member-legacy", wallet: "wallet-legacy", sent: []string{"GET /privy/v1/wallets"},
			signerMissing: "wallet-legacy",
		},
		"privy holds no wallet": {
			user: "did:privy:member-new", sent: []string{"GET /privy/v1/wallets", "POST /privy/v1/wallets"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c, u, _ := overPrivyFakes(t)
			f, ctx := newRuleFixture(t, privyadapter.Wallets{Client: c})
			got, err := f.rule.Resolve(ctx, tc.user, nil)
			if err != nil || got.PrivyWalletID == "" || got.Address == "" ||
				tc.wallet != "" && got.PrivyWalletID != tc.wallet {
				t.Fatalf("Resolve = %+v, %v, want wallet %q", got, err, tc.wallet)
			}
			sent := u.requests()
			t.Logf("privy requests %v, wallet %s", sent, got.PrivyWalletID)
			if !reflect.DeepEqual(sent, tc.sent) {
				t.Fatalf("requests = %v, want %v", sent, tc.sent)
			}
			f.expectSignerMissing(t, tc.signerMissing)
		})
	}
}

func TestWalletRule_aPrivyFailureStopsSignInWithoutAWarning(t *testing.T) {
	t.Parallel()
	wallets := &privyfake.Wallets{}
	wallets.Fail("FindOrCreate", errs.New(errs.CodePrivyUnavailable, "test"))
	f, ctx := newRuleFixture(t, wallets)
	got, err := f.rule.Resolve(ctx, "did:privy:member-new", nil)
	if errs.CodeOf(err) != errs.CodePrivyUnavailable || got != (domain.Wallet{}) {
		t.Fatalf("Resolve = %+v, %v, want privy_unavailable", got, err)
	}
	if lines := f.lines(t); len(lines) != 0 || f.signerMissing(t) != 0 || wallets.Creates() != 0 {
		t.Fatalf("logged %v, counted %d, created %d", lines, f.signerMissing(t), wallets.Creates())
	}
}

func TestNewWalletRule_refusesWhenTheCounterCannotBeCreated(t *testing.T) {
	t.Parallel()
	rule, err := app.NewWalletRule(&privyfake.Wallets{}, testkit.FailingGauges{Prefix: "identity_"})
	if rule != nil || errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("NewWalletRule = %v, %v, want internal", rule, err)
	}
}
