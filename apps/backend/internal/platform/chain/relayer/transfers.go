package relayer

import (
	"bytes"
	"context"
	"encoding/binary"
	"log/slog"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	createIdempotent = 1
	transferChecked  = 12
)

type Signer interface {
	SignTransaction(ctx context.Context, walletID string, unsigned []byte) ([]byte, error)
}

type TransferSpec struct {
	FromWallet chain.Wallet
	To         chain.SolanaAddress
	Mint       chain.Mint
	Amount     money.BaseUnits
}

type SignedTx struct {
	Bytes                []byte
	Signature            chain.Signature
	LastValidBlockHeight uint64
}

type Transfers struct {
	relayer *Relayer
	signer  Signer
}

func NewTransfers(r *Relayer, signer Signer) *Transfers {
	return &Transfers{relayer: r, signer: signer}
}

func (t *Transfers) Build(ctx context.Context, spec TransferSpec) (SignedTx, error) {
	const op = "relayer.Transfers.Build"
	if err := validate(op, t.relayer.address, spec); err != nil {
		return SignedTx{}, err
	}
	mint, err := t.relayer.rpc.MintConfig(ctx, spec.Mint.Address)
	if err != nil {
		return SignedTx{}, err
	}
	if mint.Mint.Decimals != spec.Mint.Decimals {
		return SignedTx{}, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "mint decimals"))
	}
	hash, err := t.relayer.rpc.LatestBlockhash(ctx)
	if err != nil {
		return SignedTx{}, err
	}
	msg := transferMessage(t.relayer.address, spec, mint.TokenProgram, hash.Hash)
	unsigned := chain.Transaction{Signatures: [][]byte{make([]byte, 64), make([]byte, 64)}, Message: msg}
	raw, err := t.signer.SignTransaction(ctx, spec.FromWallet.ID, unsigned.Encode())
	if err != nil {
		return SignedTx{}, err
	}
	tx, err := chain.DecodeTransaction(raw)
	if err != nil || !bytes.Equal(tx.Message, msg) || len(tx.Signatures) != 2 || !tx.Signed(1) {
		return SignedTx{}, errs.New(
			errs.CodeInternal,
			op,
			slog.String("reason", "wallet signature does not cover the built message"),
			slog.String("wallet_id", spec.FromWallet.ID),
		)
	}
	if err := tx.Sign(t.relayer.key); err != nil {
		return SignedTx{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return SignedTx{
		Bytes:                tx.Encode(),
		Signature:            chain.SignatureOf(tx.Signatures[0]),
		LastValidBlockHeight: hash.LastValidBlockHeight,
	}, nil
}

func (t *Transfers) Broadcast(ctx context.Context, tx SignedTx) error {
	sig, err := t.relayer.rpc.SendTransaction(ctx, tx.Bytes)
	if err != nil {
		return err
	}
	if sig != tx.Signature {
		return errs.New(
			errs.CodeInternal,
			"relayer.Transfers.Broadcast",
			slog.String("reason", "RPC answered another signature"),
		)
	}
	return nil
}

func validate(op string, payer chain.SolanaAddress, spec TransferSpec) error {
	for _, a := range []chain.SolanaAddress{spec.FromWallet.Address, spec.To, spec.Mint.Address} {
		if _, err := chain.ParseAddress(string(a)); err != nil {
			return errs.Wrap(err, errs.CodeInvalidAddress, op)
		}
	}
	parties := []chain.SolanaAddress{payer, spec.FromWallet.Address, spec.To}
	if spec.Amount.IsZero() || spec.Amount.Decimals() != spec.Mint.Decimals ||
		len(slices.Compact(slices.Sorted(slices.Values(parties)))) != 3 {
		return errs.New(errs.CodeInvalidInput, op, slog.String("reason", "amount, decimals or recipient"))
	}
	return nil
}

func transferMessage(
	payer chain.SolanaAddress,
	spec TransferSpec,
	program chain.SolanaAddress,
	blockhash [32]byte,
) []byte {
	from, to, mint := spec.FromWallet.Address, spec.To, spec.Mint.Address
	source, _ := chain.AssociatedTokenAccount(from, mint, program)
	dest, _ := chain.AssociatedTokenAccount(to, mint, program)
	keys := []chain.SolanaAddress{payer, from, source, dest, to, mint, chain.SystemProgram, program, chain.ATAProgram}
	msg := []byte{2, 1, 5}
	msg = append(msg, chain.CompactU16(len(keys))...)
	for _, k := range keys {
		b, _ := k.Bytes()
		msg = append(msg, b...)
	}
	msg = append(msg, blockhash[:]...)
	amount := binary.LittleEndian.AppendUint64([]byte{transferChecked}, spec.Amount.Uint64())
	msg = append(msg, 2)
	msg = append(msg, instruction(8, []byte{0, 3, 4, 5, 6, 7}, []byte{createIdempotent})...)
	return append(msg, instruction(7, []byte{2, 5, 3, 1}, append(amount, spec.Mint.Decimals))...)
}

func instruction(program byte, accounts, data []byte) []byte {
	out := append([]byte{program}, chain.CompactU16(len(accounts))...)
	out = append(out, accounts...)
	out = append(out, chain.CompactU16(len(data))...)
	return append(out, data...)
}
