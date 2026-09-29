package relayer

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const FloorLamports = 1_000_000

type RPC interface {
	SOLBalance(ctx context.Context, addr chain.SolanaAddress) (money.BaseUnits, error)
	LatestBlockhash(ctx context.Context) (solana.Blockhash, error)
	MintConfig(ctx context.Context, mint chain.SolanaAddress) (solana.MintConfig, error)
	SendTransaction(ctx context.Context, tx []byte) (chain.Signature, error)
}

type Relayer struct {
	key     ed25519.PrivateKey
	address chain.SolanaAddress
	rpc     RPC
}

func New(cfg config.Config, rpc RPC) (*Relayer, error) {
	raw, ok := chain.DecodeBase58(cfg.Relayer.PrivateKey)
	if !ok || len(raw) != ed25519.PrivateKeySize {
		return nil, errs.New(
			errs.CodeInvalidInput,
			"relayer.New",
			slog.String("reason", "RELAYER_PRIVATE_KEY is not a base58 64-byte key"),
		)
	}
	key := ed25519.NewKeyFromSeed(raw[:ed25519.SeedSize])
	if !bytes.Equal(key, raw) {
		return nil, errs.New(
			errs.CodeInvalidInput,
			"relayer.New",
			slog.String("reason", "RELAYER_PRIVATE_KEY public half does not match"),
		)
	}
	return &Relayer{key: key, address: chain.AddressOf(key.Public().(ed25519.PublicKey)), rpc: rpc}, nil
}

func (r *Relayer) Address() chain.SolanaAddress { return r.address }

func (r *Relayer) CheckFloor(ctx context.Context) error {
	const op = "relayer.CheckFloor"
	balance, err := r.rpc.SOLBalance(ctx, r.address)
	if err != nil {
		return err
	}
	if balance.Uint64() <= FloorLamports {
		return errs.New(errs.CodeRelayerUnderfunded, op, slog.String("relayer", string(r.address)),
			slog.Uint64("lamports", balance.Uint64()), slog.Uint64("floor_lamports", FloorLamports))
	}
	return nil
}

func CheckBoot(ctx context.Context, cfg config.Config, opts ...httpclient.Option) error {
	if cfg.Env != config.EnvStaging && cfg.Env != config.EnvProduction {
		return nil
	}
	r, err := New(cfg, solana.New(cfg, clock.Real{}, opts...))
	if err != nil {
		return err
	}
	return r.CheckFloor(ctx)
}

func (r *Relayer) CoSign(_ context.Context, raw []byte) ([]byte, error) {
	tx, err := chain.DecodeTransaction(raw)
	if err != nil {
		return nil, err
	}
	if tx.Signers[0] != r.address {
		return nil, errs.New(errs.CodeInvalidInput, "relayer.CoSign", slog.String("fee_payer", string(tx.Signers[0])))
	}
	_ = tx.Sign(r.key)
	return tx.Encode(), nil
}
