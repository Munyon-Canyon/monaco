package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const localMonaco = "postgres://postgres@127.0.0.1:54322/monaco?sslmode=disable"

func TestDevSeed_refusesEveryDatabaseButTheLocalMonacoOne(t *testing.T) {
	t.Parallel()
	for name, cfg := range map[string]config.Config{
		"production":     {Env: config.EnvProduction, DB: config.DB{URL: localMonaco}},
		"staging":        {Env: config.EnvStaging, DB: config.DB{URL: localMonaco}},
		"test env":       {Env: config.EnvTest, DB: config.DB{URL: localMonaco}},
		"remote host":    {Env: config.EnvLocal, DB: config.DB{URL: "postgres://u@db.supabase.co:5432/monaco"}},
		"lookalike host": {Env: config.EnvLocal, DB: config.DB{URL: "postgres://u@localhost.evil.io:5432/monaco"}},
		"test database":  {Env: config.EnvLocal, DB: config.DB{URL: "postgres://u@localhost:54323/monaco_test"}},
		"no database":    {Env: config.EnvLocal, DB: config.DB{URL: "postgres://u@localhost:54322"}},
		"keyword dsn":    {Env: config.EnvLocal, DB: config.DB{URL: "host=localhost dbname=monaco"}},
		"empty url":      {Env: config.EnvLocal},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			code := devCmd(cfg, []string{"seed-scenario", "cabal-with-confirmed-trade"}, &stdout, &stderr)
			if code != 2 || stderr.String() != devSeedRefused+"\n" || stdout.Len() != 0 {
				t.Fatalf("exit %d, stdout %q, stderr %q, want exit 2 and the refusal", code, stdout.String(),
					stderr.String())
			}
		})
	}
}

func TestDevSeed_allowsTheLocalMonacoDatabaseOnEveryLoopbackHost(t *testing.T) {
	t.Parallel()
	for _, url := range []string{
		localMonaco,
		"postgres://postgres@localhost:54322/monaco",
		"postgres://postgres@[::1]:54322/monaco",
	} {
		if !devSeedAllowed(config.Config{Env: config.EnvLocal, DB: config.DB{URL: url}}) {
			t.Errorf("refused %s", url)
		}
	}
}

func TestDevSeed_rejectsUnknownScenariosAndBadActors(t *testing.T) {
	t.Parallel()
	cfg := config.Config{Env: config.EnvLocal, DB: config.DB{URL: localMonaco}}
	for _, args := range [][]string{
		{"seed-scenario"},
		{"seed-scenario", "no-such-scenario"},
		{"seed-scenario", "cabal-with-confirmed-trade", "--actor", "A"},
		{"seed-scenario", "cabal-with-confirmed-trade", "--actor", "A=" + devUserV4},
	} {
		var stdout, stderr bytes.Buffer
		if code := devCmd(
			cfg,
			args,
			&stdout,
			&stderr,
		); code != 2 ||
			!bytes.Contains(stderr.Bytes(), []byte(devSeedUsage)) {
			t.Errorf("%v: exit %d, stderr %q, want the usage", args, code, stderr.String())
		}
	}
}

func TestDevSeed_confirmedTradeLeavesActorAOwningAFundedCabalThatHoldsTheTrade(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	owner := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true}).ID
	deps := module.Deps{Config: testkit.Config(), Pool: pool, Clock: clock.Real{}, IDs: ids.Real{}}
	seen := map[string]bool{}
	for range 2 {
		out, err := seedDevScenario(deps, "cabal-with-confirmed-trade", map[string]ids.UserID{"A": owner})
		if err != nil {
			t.Fatal(err)
		}
		if seen[out["cabal_id"]] || out["actor_A"] != owner.String() ||
			out["asset_mint"] != "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp" || out["symbol"] != "AAPLx" {
			t.Fatalf("seeded %v, want a new cabal owned by %s holding AAPLx", out, owner)
		}
		seen[out["cabal_id"]] = true
		var creator, role, usdc, aapl string
		if err := pool.QueryRow(
			t.Context(),
			`SELECT c.creator_id::text, m.role,
			(SELECT units::text FROM cabal_positions WHERE cabal_id = c.id AND asset = $2),
			(SELECT units::text FROM cabal_positions WHERE cabal_id = c.id AND asset = $3)
			FROM cabals c JOIN cabal_members m ON m.cabal_id = c.id WHERE c.id = $1`,
			out["cabal_id"],
			string(testkit.USDCMint),
			out["asset_mint"],
		).Scan(&creator, &role, &usdc, &aapl); err != nil {
			t.Fatal(err)
		}
		if creator != owner.String() || role != "creator" || usdc != "75000000" || aapl != "105000000" {
			t.Fatalf("cabal %s: creator %s %s, usdc %s, aapl %s; want %s as creator, 75 USDC and 105 AAPLx units",
				out["cabal_id"], creator, role, usdc, aapl, owner)
		}
	}
}

func TestDevSeed_unmappedActorsGetFreshUsers(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	deps := module.Deps{Config: testkit.Config(), Pool: pool, Clock: clock.Real{}, IDs: ids.Real{}}
	out, err := seedDevScenario(deps, "cabal-with-confirmed-trade", map[string]ids.UserID{})
	if err != nil {
		t.Fatal(err)
	}
	var creator string
	if err := pool.QueryRow(t.Context(), `SELECT u.id::text FROM cabals c JOIN users u ON u.id = c.creator_id
		WHERE c.id = $1`, out["cabal_id"]).Scan(&creator); err != nil || creator != out["actor_A"] {
		t.Fatalf("creator %q, %v; want the fresh actor A %q", creator, err, out["actor_A"])
	}
	if _, err := json.Marshal(out); err != nil {
		t.Fatal(err)
	}
}

func TestDevSeed_reportsALocalDatabaseItCannotReach(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	cfg := config.Config{Env: config.EnvLocal, DB: config.DB{URL: "postgres://postgres@127.0.0.1:1/monaco"}}
	code := devCmd(cfg, []string{"seed-scenario", "cabal-with-confirmed-trade"}, &stdout, &stderr)
	if code != 1 || !strings.HasPrefix(stderr.String(), "monacoctl dev seed-scenario: ") || stdout.Len() != 0 {
		t.Fatalf("exit %d, stdout %q, stderr %q, want exit 1 and the connect error", code, stdout.String(),
			stderr.String())
	}
}

func runDevSeedOnTestDB(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	pool := testkit.DB(t)
	open := func(context.Context, config.DB) (*pgxpool.Pool, error) { return pool, nil }
	if len(args) > 1 && args[1] == "A=new-user" {
		args[1] = "A=" + testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true}).ID.String()
	}
	var out, errOut bytes.Buffer
	cfg := testkit.Config()
	cfg.Env, cfg.DB.URL = config.EnvLocal, localMonaco
	code = devSeedScenarioOn(cfg, open, append([]string{"cabal-with-confirmed-trade"}, args...), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestDevSeed_printsOneLinePerSeededID(t *testing.T) {
	t.Parallel()
	code, stdout, stderr := runDevSeedOnTestDB(t, "--actor", "A=new-user")
	out := map[string]string{}
	for line := range strings.Lines(stdout) {
		k, v, _ := strings.Cut(strings.TrimSpace(line), " ")
		out[k] = v
	}
	if code != 0 || out["actor_A"] == "" || out["cabal_id"] == "" || out["symbol"] != "AAPLx" {
		t.Fatalf("exit %d, stdout %q, stderr %q, want actor A, a cabal and AAPLx", code, stdout, stderr)
	}
}

func TestDevSeed_printsJSONWithTheJSONFlag(t *testing.T) {
	t.Parallel()
	code, stdout, stderr := runDevSeedOnTestDB(t, "--actor", "A=new-user", "--json")
	out := map[string]string{}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil || code != 0 || out["asset_mint"] == "" {
		t.Fatalf("exit %d, stdout %q, stderr %q, %v; want one JSON object with the asset mint", code, stdout, stderr,
			err)
	}
}

func TestDevSeed_failsWhenTheMappedActorIsNotAUser(t *testing.T) {
	t.Parallel()
	code, stdout, stderr := runDevSeedOnTestDB(t, "--actor", "A="+ids.NewUserID(ids.Real{}).String())
	if code != 1 || !strings.Contains(stderr, testkit.ErrSeed.Error()) || stdout != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q, want exit 1 and the seed error", code, stdout, stderr)
	}
}

func TestDevSeed_aMissingScenarioFileIsASeedError(t *testing.T) {
	t.Parallel()
	err := testkit.SeedErr(t.Context(), func(st testkit.SeedT) { devScenarioJSONL(st, "no-such-scenario", nil) })
	if !errors.Is(err, testkit.ErrSeed) {
		t.Fatalf("err %v, want testkit.ErrSeed", err)
	}
}

func seedDevOnTestDB(t *testing.T, name string, letters ...string) (*pgxpool.Pool, map[string]string) {
	t.Helper()
	pool := testkit.DB(t)
	actors := map[string]ids.UserID{}
	for _, letter := range letters {
		actors[letter] = testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true}).ID
	}
	deps := module.Deps{Config: testkit.Config(), Pool: pool, Clock: clock.Real{}, IDs: ids.Real{}}
	out, err := seedDevScenario(deps, name, actors)
	if err != nil {
		t.Fatal(err)
	}
	for letter, user := range actors {
		if out["actor_"+letter] != user.String() {
			t.Fatalf("actor %s = %q, want the mapped user %s", letter, out["actor_"+letter], user)
		}
	}
	return pool, out
}

func TestDevSeed_cabalWithMembersSeatsACreatorAndTwoMembers(t *testing.T) {
	t.Parallel()
	pool, out := seedDevOnTestDB(t, "cabal-with-members", "A", "B", "C")
	var roles string
	var joins int
	if err := pool.QueryRow(t.Context(), `SELECT
		(SELECT string_agg(user_id::text || '=' || role, ',' ORDER BY joined_at) FROM cabal_members WHERE cabal_id = $1),
		(SELECT count(*) FROM events WHERE type = 'cabal.member_joined' AND payload->>'cabal_id' = $1::text)`,
		out["cabal_id"]).Scan(&roles, &joins); err != nil {
		t.Fatal(err)
	}
	want := out["actor_A"] + "=creator," + out["actor_B"] + "=member," + out["actor_C"] + "=member"
	if roles != want || joins != 3 {
		t.Fatalf("members %q and %d join events, want %q and 3", roles, joins, want)
	}
}

func TestDevSeed_cabalWithOpenProposalOpensABuyEveryMemberCanVoteOn(t *testing.T) {
	t.Parallel()
	pool, out := seedDevOnTestDB(t, "cabal-with-open-proposal", "A")
	var cabal, proposer, status, voters string
	var live bool
	if err := pool.QueryRow(t.Context(), `SELECT p.cabal_id::text, p.proposer_id::text, p.status,
		p.expires_at > now(),
		(SELECT string_agg(v.voter_id::text, ',' ORDER BY v.voter_id) FROM proposal_voters v WHERE v.proposal_id = p.id)
		FROM proposals p WHERE p.id = $1`, out["proposal_id"]).Scan(&cabal, &proposer, &status, &live, &voters); err != nil {
		t.Fatal(err)
	}
	var members string
	if err := pool.QueryRow(t.Context(), `SELECT string_agg(user_id::text, ',' ORDER BY user_id)
		FROM cabal_members WHERE cabal_id = $1`, cabal).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if cabal != out["cabal_id"] || proposer != out["actor_A"] || status != "open" || !live ||
		voters != members || strings.Count(members, ",") != 2 {
		t.Fatalf("proposal in %s by %s is %s (live %t), voters %s of members %s; want an open, live buy by %s "+
			"in %s that its three members vote on", cabal, proposer, status, live, voters, members, out["actor_A"],
			out["cabal_id"])
	}
}

func TestDevSeed_userWithBalancePostsABalancedSettledDeposit(t *testing.T) {
	t.Parallel()
	pool, out := seedDevOnTestDB(t, "user-with-balance", "A")
	var kind, status, wallet string
	var sum int64
	if err := pool.QueryRow(t.Context(), `SELECT t.kind, t.status,
		(SELECT amount::text FROM user_txn_entries WHERE txn_id = t.id AND account = 'wallet' AND asset = $3),
		(SELECT sum(amount) FROM user_txn_entries WHERE txn_id = t.id)
		FROM user_txns t WHERE t.id = $1 AND t.user_id = $2`,
		out["deposit_id"], out["actor_A"], string(testkit.USDCMint)).Scan(&kind, &status, &wallet, &sum); err != nil {
		t.Fatal(err)
	}
	if kind != "deposit" || status != "settled" || wallet != "25000000" || out["amount_micros"] != wallet || sum != 0 {
		t.Fatalf("%s %s, wallet +%s, entries sum %d, out %v; want a settled 25 USDC deposit that balances",
			status, kind, wallet, sum, out)
	}
}
