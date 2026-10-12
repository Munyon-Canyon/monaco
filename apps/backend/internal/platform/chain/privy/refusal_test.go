package privy_test

import (
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func detail(err error) map[string]string {
	out := map[string]string{}
	for _, a := range errs.Detail(err) {
		out[a.Key] = a.Value.String()
	}
	return out
}

func TestDo_aRefusalCarriesPrivysErrorAndCode(t *testing.T) {
	t.Parallel()
	body := `{"error":"Transaction simulation failed: insufficient funds","code":"invalid_data"}`
	_, err := client(replying(http.StatusBadRequest, body)).CreateUser(t.Context(), "x@example.com")
	wantCode(t, err, errs.CodeInvalidInput)
	got := detail(err)
	if got["privy_error"] != "Transaction simulation failed: insufficient funds" ||
		got["privy_code"] != "invalid_data" {
		t.Fatalf("detail = %v, want Privy's error and code", got)
	}
}

func TestDo_aLongRefusalIsCutTo200Bytes(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("é", 150)
	_, err := client(replying(http.StatusUnprocessableEntity, `{"error":"`+long+`"}`)).
		CreateUser(t.Context(), "x@example.com")
	got := detail(err)["privy_error"]
	if len(got) != 200 || !strings.HasPrefix(long, got) {
		t.Fatalf("privy_error has %d bytes, want the first 200 of Privy's error", len(got))
	}
}

func TestDo_theUserLimitAndAServerErrorCarryNoRefusalAttrs(t *testing.T) {
	t.Parallel()
	for status, body := range map[int]string{
		http.StatusBadRequest:          `{"error":"User limit reached","code":"max_accounts_reached"}`,
		http.StatusInternalServerError: `{"error":"boom","code":"internal"}`,
	} {
		_, err := client(replying(status, body)).CreateUser(t.Context(), "x@example.com")
		if _, ok := detail(err)["privy_error"]; ok {
			t.Errorf("status %d: detail = %v, want no privy_error", status, detail(err))
		}
	}
}

func TestDo_aRefusalLogsNoWalletKeyOrToken(t *testing.T) {
	t.Parallel()
	var logs testkit.Logs
	ctx := observability.WithLogger(t.Context(),
		observability.NewLogger(config.Config{Env: config.EnvProduction}, &logs))
	key := fakes.PrivyAuthorizationKeyConfig()[len("wallet-auth:"):]
	token := fakes.PrivyAccessToken(appID, "did:privy:member", time.Date(2026, 10, 12, 4, 0, 0, 0, time.UTC), time.Hour)
	for _, secret := range []string{key, token} {
		body := `{"error":"bad authorization ` + secret + `","code":"invalid_data"}`
		_, err := client(replying(http.StatusBadRequest, body)).CreateUser(ctx, "x@example.com")
		boundary.Stopped(ctx, "test", err)
		observability.Degraded(ctx, observability.TradingSwapRefused, slog.String("swap_id", "s"),
			slog.String("step", "swap"), slog.String("code", string(errs.CodeOf(err))),
			slog.GroupAttrs("detail", errs.Detail(err)...))
	}
	out := string(logs.Bytes())
	if !strings.Contains(out, "privy_error") {
		t.Fatalf("logs = %s, want the privy_error attr", out)
	}
	for name, secret := range map[string]string{"key": key, "token": token} {
		if strings.Contains(out, secret) {
			t.Errorf("the %s reached a log line: %s", name, out)
		}
	}
}
