package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	privyadapter "github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/privy"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	chainprivy "github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const walletFunded = "BGQoQgGkjSQc6c5YjsyRjuj4M5LbYJMHVCdS8BJSrr2R"

func runDevOut(cfg config.Config, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := devCmd(cfg, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

type pruneWorld struct {
	mu          sync.Mutex
	deleted     []string
	deleteHTTP  map[string]int
	listStatus  int
	walletsHTTP int
	without     map[string]bool
	rpcStatus   int
	solFails    bool
	srv         *httptest.Server
}

func (w *pruneWorld) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch {
	case strings.HasPrefix(r.URL.Path, "/rpc"):
		w.rpc(rw, r)
	case strings.HasPrefix(r.URL.Path, "/privy/v1/wallets"):
		w.wallets(rw, r)
	case r.Method == http.MethodDelete:
		id := strings.TrimPrefix(r.URL.Path, "/privy/v1/users/")
		if status := w.deleteHTTP[id]; status != 0 {
			rw.WriteHeader(status)
			return
		}
		w.deleted = append(w.deleted, id)
		rw.WriteHeader(http.StatusNoContent)
	case w.listStatus != 0:
		rw.WriteHeader(w.listStatus)
	default:
		old := clock.Real{}.Now().Add(-48 * time.Hour).Unix()
		fresh := clock.Real{}.Now().Add(-time.Hour).Unix()
		email := func(address string) string { return fmt.Sprintf(`{"type":"email","address":%q}`, address) }
		wallet := func(address string) string {
			return fmt.Sprintf(
				`{"type":"wallet","chain_type":"solana","wallet_client_type":"privy","id":"w","address":%q}`,
				address,
			)
		}
		user := func(id string, created int64, accounts ...string) string {
			return fmt.Sprintf(
				`{"id":%q,"created_at":%d,"linked_accounts":[%s]}`,
				id,
				created,
				strings.Join(accounts, ","),
			)
		}
		users := []string{
			user("did:privy:old", old, email("dev-0a1b2c3d@example.com"), wallet(walletClean)),
			user("did:privy:no-wallet", old, email("dev-1a2b3c4d@example.com")),
			user("did:privy:two-wallets", old, email("dev-6a2b3c4d@example.com")),
			user("did:privy:no-date", 0, email("dev-7a2b3c4d@example.com")),
			user("did:privy:funded", old, email("dev-2a2b3c4d@example.com"), wallet(walletFunded)),
			user("did:privy:fresh", fresh, email("dev-3a2b3c4d@example.com")),
			user("did:privy:pool", old, email("dev-vote-host@example.com")),
			user("did:privy:pool-hex", old, email("dev-ffffffff@example.org")),
			user("did:privy:real", old, email("person@gmail.com")),
			user(
				"did:privy:linked",
				old,
				email("dev-4a2b3c4d@example.com"),
				`{"type":"phone","number":"+14155550100"}`,
			),
			user("did:privy:missing", old, email("dev-5a2b3c4d@example.com")),
			user("did:privy:no-email", old),
		}
		for id := range w.without {
			users = slices.DeleteFunc(users, func(u string) bool { return strings.Contains(u, `"`+id+`"`) })
		}
		_, _ = io.WriteString(rw, `{"data":[`+strings.Join(users, ",")+`]}`)
	}
}

func (w *pruneWorld) wallets(rw http.ResponseWriter, r *http.Request) {
	if w.walletsHTTP != 0 {
		rw.WriteHeader(w.walletsHTTP)
		return
	}
	user := r.URL.Query().Get("user_id")
	if r.URL.Query().Get("limit") != "100" || user == "" {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	var addresses []string
	switch user {
	case "did:privy:old":
		addresses = []string{walletClean}
	case "did:privy:funded":
		addresses = []string{walletFunded}
	case "did:privy:two-wallets":
		addresses = []string{walletClean, walletFunded}
	case "did:privy:missing":
		addresses = []string{walletClean}
	}
	data := make([]string, 0, len(addresses))
	for i, a := range addresses {
		data = append(data, fmt.Sprintf(`{"id":"w%d","address":%q}`, i, a))
	}
	_, _ = io.WriteString(rw, `{"data":[`+strings.Join(data, ",")+`]}`)
}

func (w *pruneWorld) rpc(rw http.ResponseWriter, r *http.Request) {
	if w.rpcStatus != 0 {
		rw.WriteHeader(w.rpcStatus)
		return
	}
	var call struct {
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	_ = json.NewDecoder(r.Body).Decode(&call)
	if w.solFails && call.Method == "getBalance" {
		rw.WriteHeader(http.StatusBadGateway)
		return
	}
	var owner string
	_ = json.Unmarshal(call.Params[0], &owner)
	funded := owner == walletFunded
	var result string
	switch call.Method {
	case "getBalance":
		lamports := 0
		if funded {
			lamports = 10_000_000
		}
		result = fmt.Sprintf(`{"context":{"slot":1},"value":%d}`, lamports)
	default:
		accounts := `[]`
		if funded {
			accounts = `[{"pubkey":"x","account":{"lamports":1,"owner":"x","data":{"parsed":{"info":{"mint":"` + usdcMint + `","tokenAmount":{"amount":"2500000","decimals":6}}}}}}]`
		}
		result = `{"context":{"slot":1},"value":` + accounts + `}`
	}
	_, _ = io.WriteString(rw, `{"jsonrpc":"2.0","id":1,"result":`+result+`}`)
}

func newPruneWorld(t *testing.T) (*pruneWorld, config.Config) {
	t.Helper()
	w := &pruneWorld{deleteHTTP: map[string]int{}}
	w.srv = httptest.NewServer(w)
	t.Cleanup(w.srv.Close)
	cfg := localDevConfig()
	cfg.Privy = config.Privy{
		AppID: "app", AppSecret: "secret", BaseURL: w.srv.URL + "/privy", VerificationKey: fakes.PrivyVerificationKey(),
	}
	cfg.Solana = config.Solana{RPCURL: w.srv.URL + "/rpc", USDCMint: usdcMint}
	cfg.Timeouts = config.Timeouts{Privy: 5 * time.Second, RPC: 5 * time.Second, RPCBreakerOpen: time.Second}
	return w, cfg
}

func TestDevUsersPrune_isADryRunUnlessApplied(t *testing.T) {
	t.Parallel()
	w, cfg := newPruneWorld(t)
	for _, args := range [][]string{{"users", "prune"}, {"users", "prune", "--dry-run"}} {
		code, stdout, stderr := runDevOut(cfg, args...)
		if code != 1 || len(w.deleted) != 0 || !strings.Contains(stderr, "1 deletes failed") {
			t.Fatalf("%v: exit %d stderr %q deleted %v", args, code, stderr, w.deleted)
		}
		for _, want := range []string{
			"did:privy:old dev-0a1b2c3d@example.com", "did:privy:no-wallet", "did:privy:two-wallets", "did:privy:funded", "usdc_micros 2500000",
			"skipped: funded", "did:privy:missing", "5 of 12 Privy users match: 0 deleted, 2 skipped as funded, 1 failed",
		} {
			if !strings.Contains(stdout, want) {
				t.Errorf("%v: stdout lacks %q:\n%s", args, want, stdout)
			}
		}
		for _, never := range []string{"did:privy:no-date", "did:privy:fresh", "did:privy:pool", "did:privy:real", "did:privy:linked", "did:privy:no-email"} {
			if strings.Contains(stdout, never) {
				t.Errorf("%v: stdout lists %q:\n%s", args, never, stdout)
			}
		}
	}
}

func TestDevUsersPrune_applyDeletesOnlyUnfundedThrowaways(t *testing.T) {
	t.Parallel()
	w, cfg := newPruneWorld(t)
	w.deleteHTTP["did:privy:missing"] = http.StatusNotFound
	code, stdout, stderr := runDevOut(cfg, "users", "prune", "--apply")
	if code != 1 || !strings.Contains(stderr, "1 deletes failed") || fmt.Sprint(w.deleted) != "[did:privy:old]" ||
		!strings.Contains(stdout, "2 deleted, 2 skipped as funded, 1 failed") ||
		!strings.Contains(
			stdout,
			"did:privy:no-wallet dev-1a2b3c4d@example.com failed: monacoctl.walletFunds: not_found",
		) {
		t.Fatalf("exit %d stderr %q deleted %v stdout:\n%s", code, stderr, w.deleted, stdout)
	}
}

func TestDevUsersPrune_keepsGoingPastADeleteFailureAndReportsIt(t *testing.T) {
	t.Parallel()
	w, cfg := newPruneWorld(t)
	w.deleteHTTP["did:privy:old"] = http.StatusBadRequest
	code, stdout, stderr := runDevOut(cfg, "users", "prune", "--apply")
	if code != 1 || !strings.Contains(stderr, "2 deletes failed") || !strings.Contains(stdout, "failed:") ||
		fmt.Sprint(w.deleted) != "[did:privy:missing]" {
		t.Fatalf("exit %d stderr %q deleted %v stdout:\n%s", code, stderr, w.deleted, stdout)
	}
}

func TestDevUsersPrune_aWalletItCannotReadIsNeverDeleted(t *testing.T) {
	t.Parallel()
	w, cfg := newPruneWorld(t)
	w.rpcStatus = http.StatusBadGateway
	code, stdout, stderr := runDevOut(cfg, "users", "prune", "--apply")
	if code != 1 || !strings.Contains(stderr, "deletes failed") ||
		fmt.Sprint(w.deleted) != "[]" ||
		!strings.Contains(stdout, "did:privy:old dev-0a1b2c3d@example.com failed:") {
		t.Fatalf("exit %d stderr %q deleted %v stdout:\n%s", code, stderr, w.deleted, stdout)
	}
}

func TestDevUsersPrune_failsOnPrivyAndConfigErrors(t *testing.T) {
	t.Parallel()
	w, cfg := newPruneWorld(t)
	w.listStatus = http.StatusBadGateway
	if code, _, stderr := runDevOut(cfg, "users", "prune"); code != 1 ||
		!strings.HasPrefix(stderr, "monacoctl dev users prune: privy.ListUsers: ") {
		t.Errorf("list failure: exit %d stderr %q", code, stderr)
	}
	noKey := cfg
	noKey.Privy.VerificationKey = ""
	if code, _, stderr := runDevOut(noKey, "users", "prune"); code != 1 || !strings.Contains(stderr, "privy.New") {
		t.Errorf("no key: exit %d stderr %q", code, stderr)
	}
}

func TestDevUsersPrune_refusesAnythingButTheLocalDevDatabase(t *testing.T) {
	t.Parallel()
	w, cfg := newPruneWorld(t)
	for name, mutate := range map[string]func(*config.Config){
		"staging":     func(c *config.Config) { c.Env = config.EnvStaging },
		"production":  func(c *config.Config) { c.Env = config.EnvProduction },
		"test env":    func(c *config.Config) { c.Env = config.EnvTest },
		"remote host": func(c *config.Config) { c.DB.URL = "postgres://u@db.supabase.co:5432/monaco" },
	} {
		c := cfg
		mutate(&c)
		code, stdout, stderr := runDevOut(c, "users", "prune", "--apply")
		if code != 1 || stdout != "" || stderr != "monacoctl dev users prune: refused: only the local dev database\n" ||
			len(w.deleted) != 0 {
			t.Errorf("%s: exit %d stdout %q stderr %q deleted %v", name, code, stdout, stderr, w.deleted)
		}
	}
}

func TestDevUsersPrune_refusesBadArguments(t *testing.T) {
	t.Parallel()
	_, cfg := newPruneWorld(t)
	for name, args := range map[string][]string{
		"no verb": {"users"}, "wrong verb": {"users", "list"}, "both": {"users", "prune", "--dry-run", "--apply"},
		"extra": {"users", "prune", "x"}, "bad flag": {"users", "prune", "--nope"},
	} {
		if code, _, stderr := runDevOut(cfg, args...); code != 2 || stderr != devUsage+"\n" {
			t.Errorf("%s: exit %d stderr %q", name, code, stderr)
		}
	}
}

func TestDevUsersPrune_aSolBalanceItCannotReadIsNeverDeleted(t *testing.T) {
	t.Parallel()
	w, cfg := newPruneWorld(t)
	w.solFails = true
	code, _, stderr := runDevOut(cfg, "users", "prune", "--apply")
	if code != 1 || !strings.Contains(stderr, "deletes failed") ||
		fmt.Sprint(w.deleted) != "[]" {
		t.Fatalf("exit %d stderr %q deleted %v", code, stderr, w.deleted)
	}
}

func TestDevUsersPrune_exitsCleanWhenEveryMatchIsDeletedOrSkippedAsFunded(t *testing.T) {
	t.Parallel()
	w, cfg := newPruneWorld(t)
	w.without = map[string]bool{"did:privy:no-wallet": true}
	code, _, stderr := runDevOut(cfg, "users", "prune", "--apply")
	if code != 0 || stderr != "" || fmt.Sprint(w.deleted) != "[did:privy:old did:privy:missing]" {
		t.Fatalf("exit %d stderr %q deleted %v", code, stderr, w.deleted)
	}
}

func TestDevUsersPrune_aWalletListingThatFailsDeletesNobody(t *testing.T) {
	t.Parallel()
	w, cfg := newPruneWorld(t)
	w.walletsHTTP = http.StatusBadGateway
	code, stdout, stderr := runDevOut(cfg, "users", "prune", "--apply")
	if code != 1 || len(w.deleted) != 0 || !strings.Contains(stderr, "5 deletes failed") ||
		!strings.Contains(stdout, "privy.ListUserWallets") {
		t.Fatalf("exit %d stderr %q deleted %v stdout:\n%s", code, stderr, w.deleted, stdout)
	}
}

func TestPrivyAdapterWallets_listsTheUsersWalletAddresses(t *testing.T) {
	t.Parallel()
	_, cfg := newPruneWorld(t)
	client, err := chainprivy.New(cfg, clock.Real{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := privyadapter.Users{Client: client}.Wallets(t.Context(), app.PrivyUserID("did:privy:two-wallets"))
	if err != nil || len(got) != 2 || got[1] != walletFunded {
		t.Fatalf("Wallets = %v, %v", got, err)
	}
}
