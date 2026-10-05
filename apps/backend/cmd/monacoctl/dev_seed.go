package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalmod "github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	governance "github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	devSeedUsage   = "usage: monacoctl dev seed-scenario <name> [--actor A=<user-uuid> ...] [--json]"
	devSeedRefused = "monacoctl dev seed-scenario: refused: only the local dev database"
)

type devScenario func(t testkit.SeedT, deps module.Deps, actor func(letter string) ids.UserID) map[string]string

var errBadActor = errors.New("bad --actor")

func devScenarios() map[string]devScenario {
	return map[string]devScenario{
		"cabal-with-confirmed-trade": seedConfirmedTrade,
		"cabal-with-members":         seedCabalWithMembers,
		"cabal-with-open-proposal":   seedOpenProposal,
		"user-with-balance":          seedUserWithBalance,
	}
}

type openDB func(ctx context.Context, cfg config.DB) (*pgxpool.Pool, error)

func devSeedScenario(cfg config.Config, args []string, stdout, stderr io.Writer) int {
	return devSeedScenarioOn(cfg, db.Open, args, stdout, stderr)
}

func devSeedScenarioOn(cfg config.Config, open openDB, args []string, stdout, stderr io.Writer) int {
	if !devSeedAllowed(cfg) {
		_, _ = fmt.Fprintln(stderr, devSeedRefused)
		return 2
	}
	name, actors, asJSON, ok := parseDevSeedArgs(args)
	if !ok {
		_, _ = fmt.Fprintln(stderr, devSeedUsage)
		_, _ = fmt.Fprintf(stderr, "scenarios: %s\n", strings.Join(slices.Sorted(maps.Keys(devScenarios())), ", "))
		return 2
	}
	pool, err := open(context.Background(), cfg.DB)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl dev seed-scenario: %v\n", err)
		return 1
	}
	defer pool.Close()
	out, err := seedDevScenario(
		module.Deps{Config: cfg, Pool: pool, Clock: clock.Real{}, IDs: ids.Real{}},
		name,
		actors,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl dev seed-scenario: %v\n", err)
		return 1
	}
	if asJSON {
		_ = json.NewEncoder(stdout).Encode(out)
		return 0
	}
	for _, k := range slices.Sorted(maps.Keys(out)) {
		_, _ = fmt.Fprintf(stdout, "%s %s\n", k, out[k])
	}
	return 0
}

func devSeedAllowed(cfg config.Config) bool {
	if cfg.Env != config.EnvLocal {
		return false
	}
	u, err := url.Parse(cfg.DB.URL)
	if err != nil || strings.TrimPrefix(u.Path, "/") != "monaco" {
		return false
	}
	return slices.Contains([]string{"localhost", "127.0.0.1", "::1"}, u.Hostname())
}

func parseDevSeedArgs(args []string) (name string, actors map[string]ids.UserID, asJSON, ok bool) {
	if len(args) == 0 || devScenarios()[args[0]] == nil {
		return "", nil, false, false
	}
	actors = map[string]ids.UserID{}
	fs := flag.NewFlagSet("dev seed-scenario", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&asJSON, "json", false, "print the seeded ids as one JSON object")
	fs.Func("actor", "A=<user-uuid>: the existing user that plays actor A", func(v string) error {
		letter, raw, found := strings.Cut(v, "=")
		user, err := ids.ParseUserID(raw)
		if !found || letter == "" || err != nil {
			return fmt.Errorf("%w %q", errBadActor, v)
		}
		actors[letter] = user
		return nil
	})
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		return "", nil, false, false
	}
	return args[0], actors, asJSON, true
}

func seedDevScenario(deps module.Deps, name string, actors map[string]ids.UserID) (out map[string]string, err error) {
	err = testkit.SeedErr(context.Background(), func(t testkit.SeedT) { out = runDevScenario(t, deps, name, actors) })
	return out, err
}

func runDevScenario(t testkit.SeedT, deps module.Deps, name string, actors map[string]ids.UserID) map[string]string {
	actor := func(letter string) ids.UserID {
		if _, ok := actors[letter]; !ok {
			actors[letter] = testkit.SeedUser(t, deps.Pool, testkit.UserOpts{WithWallet: true}).ID
		}
		return actors[letter]
	}
	out := devScenarios()[name](t, deps, actor)
	for letter, user := range actors {
		out["actor_"+letter] = user.String()
	}
	return out
}

const confirmedTradeCabal = "01890a5d-ac96-774b-bcce-b302099a8090"

func seedConfirmedTrade(t testkit.SeedT, deps module.Deps, actor func(string) ids.UserID) map[string]string {
	owner := actor("A")
	cabal := testkit.NewCabal(t, deps.Pool, testkit.WithCreator(owner))
	testkit.NewLedger(t, deps.Pool).WithFundedMember(owner, cabal.ID, money.MicrosFromUint64(100_000_000))
	raw := devScenarioJSONL(t, "cabal-with-confirmed-trade", map[string]string{confirmedTradeCabal: cabal.ID.String()})
	out := map[string]string{"cabal_id": cabal.ID.String(), "invite_code": cabal.InviteCode}
	for _, s := range testkit.SeedJSONL(t, deps.Pool, "cabal-with-confirmed-trade", raw,
		treasury.New(deps).Consumers()...) {
		if trade, ok := s.Event.(events.TradeConfirmed); ok {
			out["swap_id"], out["asset_mint"], out["symbol"] = trade.SwapID.String(), string(
				trade.OutMint,
			), trade.Symbol
		}
	}
	return out
}

func seedCabalWithMembers(t testkit.SeedT, deps module.Deps, actor func(string) ids.UserID) map[string]string {
	cabal := newDevCabal(t, deps, actor)
	raw := devScenarioJSONL(t, "cabal-with-members", map[string]string{
		"01890a5d-ac96-774b-bcce-b302099a8070": cabal.ID.String(),
		"01890a5d-ac96-774b-bcce-b302099a8071": actor("A").String(),
		"01890a5d-ac96-774b-bcce-b302099a8072": actor("B").String(),
		"01890a5d-ac96-774b-bcce-b302099a8073": actor("C").String(),
	})
	testkit.SeedJSONL(t, deps.Pool, "cabal-with-members", raw, cabalmod.New(deps).Consumers()...)
	return map[string]string{"cabal_id": cabal.ID.String(), "invite_code": cabal.InviteCode}
}

func seedOpenProposal(t testkit.SeedT, deps module.Deps, actor func(string) ids.UserID) map[string]string {
	cabal := newDevCabal(t, deps, actor)
	id, now := ids.Real{}.NewV7(), clock.Real{}.Now().UTC()
	params := governance.InsertProposalParams{
		ID: id, CabalID: cabal.ID.UUID(), ProposerID: actor("A").UUID(), Kind: "buy", Symbol: "AAPLx",
		Mint: devProposalMint, UsdcMicros: pgtype.Int8{Int64: 5_000_000, Valid: true},
		QuoteOutAmount: 21_000_000, ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now,
	}
	for _, m := range cabal.Members {
		params.VoterIds = append(params.VoterIds, m.ID.UUID())
	}
	_, err := governance.New(deps.Pool).InsertProposal(t.Context(), params)
	failSeed(t, err, "insert proposal")
	raw := devScenarioJSONL(t, "cabal-with-open-proposal", map[string]string{
		"01890a5d-ac96-774b-bcce-b302099a8091": cabal.ID.String(),
		"01890a5d-ac96-774b-bcce-b302099a8092": actor("A").String(),
		"01890a5d-ac96-774b-bcce-b302099a8096": actor("B").String(),
		"01890a5d-ac96-774b-bcce-b302099a8097": actor("C").String(),
		"01890a5d-ac96-774b-bcce-b302099a8099": id.String(),
	})
	testkit.SeedJSONL(t, deps.Pool, "cabal-with-open-proposal", raw, cabalmod.New(deps).Consumers()...)
	return map[string]string{
		"cabal_id": cabal.ID.String(), "invite_code": cabal.InviteCode, "proposal_id": id.String(),
		"asset_mint": devProposalMint, "symbol": "AAPLx",
	}
}

const devProposalMint = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"

func newDevCabal(t testkit.SeedT, deps module.Deps, actor func(string) ids.UserID) testkit.SeededCabal {
	return testkit.NewCabal(t, deps.Pool, testkit.WithCreator(actor("A")),
		testkit.WithJoiner(actor("B")), testkit.WithJoiner(actor("C")))
}

func seedUserWithBalance(t testkit.SeedT, deps module.Deps, actor func(string) ids.UserID) map[string]string {
	raw := devScenarioJSONL(t, "user-with-balance", map[string]string{
		"01890a5d-ac96-774b-bcce-b302099a80b2": actor("A").String(),
	})
	out := map[string]string{}
	for _, s := range testkit.SeedJSONL(t, deps.Pool, "user-with-balance", raw, treasury.New(deps).Consumers()...) {
		if deposit, ok := s.Event.(events.DepositCredited); ok {
			out["deposit_id"], out["amount_micros"] = deposit.DepositID.String(), deposit.AmountMicros.String()
		}
	}
	return out
}

var (
	uuidPattern      = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	signaturePattern = regexp.MustCompile(`"tx_signature":"([1-9A-HJ-NP-Za-km-z]+)"`)
)

func devScenarioJSONL(t testkit.SeedT, name string, keep map[string]string) []byte {
	t.Helper()
	raw, err := testkit.Scenario(name)
	failSeed(t, err, "scenario "+name)
	fresh := map[string]string{}
	for _, id := range uuidPattern.FindAll(raw, -1) {
		if _, ok := fresh[string(id)]; !ok {
			fresh[string(id)] = ids.Real{}.NewV7().String()
		}
	}
	for _, m := range signaturePattern.FindAllSubmatch(raw, -1) {
		sig := make([]byte, 64)
		_, _ = rand.Read(sig)
		fresh[string(m[1])] = string(chain.SignatureOf(sig))
	}
	for old, id := range keep {
		fresh[old] = id
	}
	for old, id := range fresh {
		raw = bytes.ReplaceAll(raw, []byte(old), []byte(id))
	}
	return raw
}

func failSeed(t testkit.SeedT, err error, what string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}
