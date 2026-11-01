package relayer

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"log/slog"
	"strings"

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
	raw, ok := decodeKey(cfg.Relayer.PrivateKey)
	if !ok {
		return nil, errs.New(
			errs.CodeInvalidInput,
			"relayer.New",
			slog.String("reason", "RELAYER_PRIVATE_KEY is neither a base58 64-byte key nor a JSON array of 64 bytes"),
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

func decodeKey(value string) ([]byte, bool) {
	value = strings.TrimSpace(value)
	var raw []byte
	if strings.HasPrefix(value, "[") {
		var nums []uint8
		if err := json.Unmarshal([]byte(value), &nums); err != nil {
			return nil, false
		}
		raw = nums
	} else {
		var ok bool
		if raw, ok = chain.DecodeBase58(value); !ok {
			return nil, false
		}
	}
	return raw, len(raw) == ed25519.PrivateKeySize
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
	if !cfg.Env.Deployed() {
		return nil
	}
	r, err := New(cfg, solana.New(cfg, clock.Real{}, opts...))
	if err != nil {
		return err
	}
	return r.CheckFloor(ctx)
}

func (r *Relayer) CoSign(_ context.Context, raw []byte) ([]byte, error) {
	const op = "relayer.CoSign"
	tx, err := chain.DecodeTransaction(raw)
	if err != nil {
		return nil, err
	}
	if tx.Signers[0] != r.address {
		return nil, errs.New(errs.CodeInvalidInput, op, slog.String("fee_payer", string(tx.Signers[0])))
	}
	if err := tx.Sign(r.key); err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	return tx.Encode(), nil
}
