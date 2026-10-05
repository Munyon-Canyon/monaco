package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	identity "github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	qaUsage         = "usage: monacoctl qa pot [--address] | qa pot new | qa fund --user <user-uuid> --usdc <amount>"
	qaPotKey        = "QA_POT_PRIVATE_KEY"
	qaMaxFundMicros = 5_000_000
	qaMinPotLamport = 10_000_000
	qaUSDCDecimals  = 6
	qaSOLDecimals   = 9
)

var (
	errNoMemberWallet = errors.New("no member wallet")
	errQAPotKey       = errors.New(qaPotKey + " is not a base58 64-byte ed25519 key: run under " +
		"scripts/with-dotenv-local.sh, or create the pot with monacoctl qa pot new")
	errNotLanded = errors.New("qa fund: the transfer did not land")
)

type qaReads interface {
	SOLBalance(ctx context.Context, addr chain.SolanaAddress) (money.BaseUnits, error)
	TokenBalance(ctx context.Context, owner chain.SolanaAddress, mint chain.Mint) (money.BaseUnits, error)
	MintConfig(ctx context.Context, mint chain.SolanaAddress) (solana.MintConfig, error)
}

type qaSends interface {
	LatestBlockhash(ctx context.Context) (solana.Blockhash, error)
	SendTransaction(ctx context.Context, tx []byte) (chain.Signature, error)
	SignatureStatuses(ctx context.Context, sigs []chain.Signature) ([]solana.Status, error)
}

type qaRPC interface {
	qaReads
	qaSends
}

type qaEnv struct {
	environ []string
	root    string
	rpc     func(config.Config) qaRPC
	wallet  func(ctx context.Context, cfg config.Config, user uuid.UUID) (chain.SolanaAddress, error)
	setKey  func(ctx context.Context, root, value string) error
	poll    time.Duration
	wait    time.Duration
}

type qaPot struct {
	key     ed25519.PrivateKey
	address chain.SolanaAddress
}

func toolQa(env toolEnv) tool {
	return qaTool(qaEnv{
		environ: env.environ,
		root:    repoRoot(env.wd),
		rpc:     func(cfg config.Config) qaRPC { return solana.New(cfg, clock.Real{}) },
		wallet:  memberWallet,
		setKey:  dotenvxSet("dotenvx"),
		poll:    time.Second,
		wait:    90 * time.Second,
	})
}

func qaTool(env qaEnv) tool {
	cmds := map[string]command{
		"pot": func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
			return qaPotCmd(env, cfg, args, stdout, stderr)
		},
		"fund": func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
			return qaFundCmd(env, cfg, args, stdout, stderr)
		},
	}
	return func(args []string, stdout, stderr io.Writer) int {
		if slices.Equal(args, []string{"pot", "new"}) {
			return newPot(env, stdout, stderr)
		}
		if len(args) > 0 && args[0] == "fund" && lookupEnv(env.environ, "MONACO_ENV") == string(config.EnvProduction) {
			_, _ = fmt.Fprintln(stderr, "qa fund: refusing to run with MONACO_ENV=production")
			return 1
		}
		return run(cmds, nil, env.environ, args, stdout, stderr)
	}
}

func qaPotCmd(env qaEnv, cfg config.Config, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("qa pot", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	addressOnly := fs.Bool("address", false, "")
	if fs.Parse(args) != nil || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, qaUsage)
		return 2
	}
	return showPot(env, cfg, *addressOnly, stdout, stderr)
}

func qaFundCmd(env qaEnv, cfg config.Config, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("qa fund", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	user := fs.String("user", "", "")
	usdc := fs.String("usdc", "", "")
	if fs.Parse(args) != nil || fs.NArg() != 0 || *user == "" || *usdc == "" {
		_, _ = fmt.Fprintln(stderr, qaUsage)
		return 2
	}
	userID, err := uuid.Parse(*user)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, qaUsage)
		return 2
	}
	return fundUser(env, cfg, userID, *usdc, stdout, stderr)
}

func newPot(env qaEnv, stdout, stderr io.Writer) int {
	if lookupEnv(env.environ, qaPotKey) != "" || envLocalHas(env.root, qaPotKey+"=") {
		_, _ = fmt.Fprintf(stderr,
			"qa pot new: %s is already set in .env.local; refusing to replace a funded key\n", qaPotKey)
		return 1
	}
	seed := make([]byte, ed25519.SeedSize)
	_, _ = rand.Read(seed)
	key := ed25519.NewKeyFromSeed(seed)
	if err := env.setKey(context.Background(), env.root, chain.EncodeBase58(key)); err != nil {
		return fail(stderr, err)
	}
	if !envLocalHas(env.root, qaPotKey+`="encrypted:`) {
		_, _ = fmt.Fprintf(stderr,
			"qa pot new: %s in .env.local is not encrypted; remove it before you commit\n", qaPotKey)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, chain.AddressOf(key.Public().(ed25519.PublicKey)))
	return 0
}

func showPot(env qaEnv, cfg config.Config, addressOnly bool, stdout, stderr io.Writer) int {
	pot, err := loadPot(env.environ)
	if err != nil {
		return fail(stderr, err)
	}
	if addressOnly {
		_, _ = fmt.Fprintln(stdout, pot.address)
		return 0
	}
	ctx := context.Background()
	rpc := env.rpc(cfg)
	sol, err := rpc.SOLBalance(ctx, pot.address)
	if err != nil {
		return fail(stderr, err)
	}
	usdcMint := chain.Mint{Address: chain.SolanaAddress(cfg.Solana.USDCMint), Decimals: qaUSDCDecimals}
	usdc, err := rpc.TokenBalance(ctx, pot.address, usdcMint)
	if err != nil {
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintf(stdout, "address %s\nsol %s\nusdc %s\n",
		pot.address, units(sol.Uint64(), qaSOLDecimals), units(usdc.Uint64(), qaUSDCDecimals))
	return 0
}

func fundUser(env qaEnv, cfg config.Config, user uuid.UUID, rawUSDC string, stdout, stderr io.Writer) int {
	micros, ok := parseUSDC(rawUSDC)
	if !ok {
		_, _ = fmt.Fprintf(stderr, "qa fund: --usdc %q must be more than 0 USDC, with at most 6 decimals\n", rawUSDC)
		return 1
	}
	if micros > qaMaxFundMicros {
		_, _ = fmt.Fprintln(stderr, "qa fund: at most 5 USDC per transfer")
		return 1
	}
	pot, err := loadPot(env.environ)
	if err != nil {
		return fail(stderr, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), env.wait)
	defer cancel()
	dest, err := env.wallet(ctx, cfg, user)
	if errors.Is(err, errNoMemberWallet) {
		_, _ = fmt.Fprintf(stderr, "qa fund: no member wallet for user %s\n", user)
		return 1
	}
	if err != nil {
		return fail(stderr, err)
	}
	rpc := env.rpc(cfg)
	mint, err := rpc.MintConfig(ctx, chain.SolanaAddress(cfg.Solana.USDCMint))
	if err != nil {
		return fail(stderr, err)
	}
	if mint.Mint.Decimals != qaUSDCDecimals {
		_, _ = fmt.Fprintf(stderr, "qa fund: mint %s has %d decimals, want 6\n", mint.Mint.Address, mint.Mint.Decimals)
		return 1
	}
	if code := checkPotBalances(ctx, rpc, pot, mint.Mint, micros, stderr); code != 0 {
		return code
	}
	sig, err := sendFromPot(ctx, env, rpc, pot, relayer.TransferSpec{
		FromWallet: chain.Wallet{Address: pot.address},
		To:         dest,
		Mint:       mint.Mint,
		Amount:     money.NewBaseUnits(micros, qaUSDCDecimals),
	}, mint.TokenProgram)
	if err != nil {
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintf(stdout, "sent %s USDC to %s\nsignature %s\n", units(micros, qaUSDCDecimals), dest, sig)
	return 0
}

func checkPotBalances(
	ctx context.Context, rpc qaRPC, pot qaPot, mint chain.Mint, micros uint64, stderr io.Writer,
) int {
	sol, err := rpc.SOLBalance(ctx, pot.address)
	if err != nil {
		return fail(stderr, err)
	}
	usdc, err := rpc.TokenBalance(ctx, pot.address, mint)
	if err != nil {
		return fail(stderr, err)
	}
	if sol.Uint64() < qaMinPotLamport || usdc.Uint64() < micros {
		_, _ = fmt.Fprintf(stderr,
			"qa fund: the QA pot %s holds %s SOL and %s USDC, and this transfer needs at least %s SOL and %s USDC. "+
				"The operator must top it up (README.md, Agent QA: the QA pot)\n",
			pot.address, units(sol.Uint64(), qaSOLDecimals), units(usdc.Uint64(), qaUSDCDecimals),
			units(qaMinPotLamport, qaSOLDecimals), units(micros, qaUSDCDecimals))
		return 1
	}
	return 0
}

func sendFromPot(
	ctx context.Context, env qaEnv, rpc qaRPC, pot qaPot, spec relayer.TransferSpec, program chain.SolanaAddress,
) (chain.Signature, error) {
	hash, err := rpc.LatestBlockhash(ctx)
	if err != nil {
		return "", fmt.Errorf("qa fund: %w", err)
	}
	msg := relayer.TransferMessage(pot.address, spec, program, hash.Hash)
	tx := chain.Transaction{Signatures: [][]byte{ed25519.Sign(pot.key, msg)}, Message: msg}
	want := chain.SignatureOf(tx.Signatures[0])
	sig, err := rpc.SendTransaction(ctx, tx.Encode())
	if err != nil {
		return "", fmt.Errorf("qa fund: %w", err)
	}
	if sig != want {
		return "", fmt.Errorf("%w: RPC answered signature %s for transaction %s", errNotLanded, sig, want)
	}
	return sig, awaitLanding(ctx, env, rpc, sig, hash.LastValidBlockHeight)
}

func awaitLanding(
	ctx context.Context, env qaEnv, rpc qaSends, sig chain.Signature, lastValidBlockHeight uint64,
) error {
	for {
		statuses, err := rpc.SignatureStatuses(ctx, []chain.Signature{sig})
		if err != nil {
			return fmt.Errorf("qa fund: %w", err)
		}
		switch s := statuses[0]; {
		case s.Failed:
			return fmt.Errorf("%w: transaction %s failed on chain", errNotLanded, sig)
		case s.State != solana.StateNotFound:
			return nil
		case s.BlockhashExpired(lastValidBlockHeight):
			return fmt.Errorf("%w: transaction %s expired before it landed; nothing moved", errNotLanded, sig)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: transaction %s not confirmed in %s; check it on chain before you retry",
				errNotLanded, sig, env.wait)
		case <-time.After(env.poll):
		}
	}
}

func loadPot(environ []string) (qaPot, error) {
	raw, ok := chain.DecodeBase58(lookupEnv(environ, qaPotKey))
	if !ok || len(raw) != ed25519.PrivateKeySize {
		return qaPot{}, errQAPotKey
	}
	key := ed25519.NewKeyFromSeed(raw[:ed25519.SeedSize])
	if !bytes.Equal(key, raw) {
		return qaPot{}, errQAPotKey
	}
	return qaPot{key: key, address: chain.AddressOf(key.Public().(ed25519.PublicKey))}, nil
}

func memberWallet(ctx context.Context, cfg config.Config, user uuid.UUID) (chain.SolanaAddress, error) {
	pool, err := db.Open(ctx, cfg.DB)
	if err != nil {
		return "", fmt.Errorf("qa fund: %w", err)
	}
	defer pool.Close()
	return destination(identity.New(pool).QAPotDestination(ctx, user))
}

func destination(address string, err error) (chain.SolanaAddress, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errNoMemberWallet
	}
	if err != nil {
		return "", fmt.Errorf("qa fund: %w", err)
	}
	return chain.SolanaAddress(address), nil
}

func dotenvxSet(bin string) func(ctx context.Context, root, value string) error {
	return func(ctx context.Context, root, value string) error {
		cmd := exec.CommandContext(ctx, bin)
		cmd.Args = append(cmd.Args, "set", qaPotKey, value, "-f", ".env.local")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("dotenvx set %s: %w: %s", qaPotKey, err, strings.ReplaceAll(string(out), value, "<key>"))
		}
		return nil
	}
}

func parseUSDC(raw string) (uint64, bool) {
	whole, frac, _ := strings.Cut(raw, ".")
	if whole == "" || len(frac) > qaUSDCDecimals || strings.HasPrefix(whole, "+") {
		return 0, false
	}
	micros, err := strconv.ParseUint(whole+frac+strings.Repeat("0", qaUSDCDecimals-len(frac)), 10, 64)
	return micros, err == nil && micros > 0
}

func units(v uint64, decimals int) string {
	s := fmt.Sprintf("%0*d", decimals+1, v)
	return s[:len(s)-decimals] + "." + s[len(s)-decimals:]
}

func lookupEnv(environ []string, key string) string {
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v
		}
	}
	return ""
}

func envLocalHas(root, prefix string) bool {
	data, err := fs.ReadFile(os.DirFS(root), ".env.local")
	if err != nil {
		return false
	}
	for line := range strings.Lines(string(data)) {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func repoRoot(wd string) string {
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		if dir == filepath.Dir(dir) {
			return wd
		}
	}
}
